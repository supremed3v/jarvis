// runtimeClient.ts is the main-process WebSocket client for the core runtime's
// Bridge (SPEC-0065). It speaks the wire protocol defined in ../shared/runtime
// and maps inbound frames onto the handler callbacks the main process forwards
// to the renderer over the jarvis:* IPC channels. The socket is created through
// an injectable factory so the client can be unit-tested without a live
// runtime.

import { randomUUID } from "crypto";
import {
  RUNTIME_URL,
  RuntimeFrameType,
  asCommandResult,
  asErrorPayload,
  asEvent,
  asStatus,
  asStreamChunk,
  asToolApprovalRequested,
  asVoiceResult,
  asAgentList,
  asAgentControlResult,
  asMemoryList,
  asMemoryControlResult,
  frameApprovalResponse,
  frameCancelCommand,
  frameGetStatus,
  framePing,
  frameSubmitCommand,
  frameVoiceStart,
  frameVoiceStop,
  frameAgentsList,
  frameAgentStart,
  frameAgentStop,
  frameMemoryList,
  frameMemorySearch,
  frameMemoryUpdate,
  frameMemoryDelete,
  parseFrame,
  type CommandResult,
  type RuntimeError,
  type RuntimeEvent,
  type RuntimeFrame,
  type RuntimeStatus,
  type StreamChunk,
  type ToolApprovalRequested,
  type VoiceResult,
  type AgentListResult,
  type AgentControlResult,
  type MemoryListResult,
  type MemoryControlResult,
} from "../shared/runtime";

export type RuntimeConnectionState = "idle" | "connecting" | "connected" | "disconnected";

// RuntimeSocket is the minimal socket surface RuntimeClient needs, so tests can
// inject a fake and production uses the platform WebSocket.
export interface RuntimeSocket {
  readyState: number;
  send(data: string): void;
  close(code?: number, reason?: string): void;
  addEventListener(type: string, listener: (event: unknown) => void): void;
  removeEventListener(type: string, listener: (event: unknown) => void): void;
}

export interface RuntimeClientHandlers {
  onStatusChanged: (status: RuntimeStatus) => void;
  onEvent: (event: RuntimeEvent) => void;
  onStreamChunk: (chunk: StreamChunk) => void;
  onCommandResult: (result: CommandResult) => void;
  onApprovalRequested: (request: ToolApprovalRequested) => void;
  onError: (error: RuntimeError) => void;
  onStateChanged: (state: RuntimeConnectionState) => void;
}

export interface RuntimeClientOptions {
  url?: string;
  handlers: RuntimeClientHandlers;
  reconnectDelayMs?: number;
  maxReconnectDelayMs?: number;
  requestTimeoutMs?: number;
  createSocket?: (url: string) => RuntimeSocket;
}

interface PendingRequest {
  resolve: (payload: Record<string, unknown>) => void;
  reject: (error: RuntimeError) => void;
  timer: NodeJS.Timeout;
}

export const RECONNECT_BASE_DELAY_MS = 1000;
export const RECONNECT_MAX_DELAY_MS = 30000;
export const REQUEST_TIMEOUT_MS = 10000;

export class RuntimeClient {
  private readonly url: string;
  private readonly handlers: RuntimeClientHandlers;
  private readonly reconnectDelayMs: number;
  private readonly maxReconnectDelayMs: number;
  private readonly requestTimeoutMs: number;
  private readonly createSocket: (url: string) => RuntimeSocket;

  private socket: RuntimeSocket | null = null;
  private state: RuntimeConnectionState = "idle";
  private reconnectAttempt = 0;
  private reconnectTimer: NodeJS.Timeout | null = null;
  private stopped = false;
  private readonly pending = new Map<string, PendingRequest>();

  constructor(options: RuntimeClientOptions) {
    this.url = options.url ?? RUNTIME_URL;
    this.handlers = options.handlers;
    this.reconnectDelayMs = options.reconnectDelayMs ?? RECONNECT_BASE_DELAY_MS;
    this.maxReconnectDelayMs = options.maxReconnectDelayMs ?? RECONNECT_MAX_DELAY_MS;
    this.requestTimeoutMs = options.requestTimeoutMs ?? REQUEST_TIMEOUT_MS;
    this.createSocket = options.createSocket ?? ((url: string) => new WebSocket(url));
  }

  get connectionState(): RuntimeConnectionState {
    return this.state;
  }

  connect(): void {
    if (this.state === "connecting" || this.state === "connected") {
      return;
    }
    this.stopped = false;
    this.setState("connecting");

    const socket = this.createSocket(this.url);
    this.socket = socket;
    socket.addEventListener("open", () => this.handleOpen(socket));
    socket.addEventListener("message", (event) => this.handleMessage(event));
    socket.addEventListener("close", () => this.handleClose(socket));
    socket.addEventListener("error", () => this.handleSocketError(socket));
  }

  disconnect(): void {
    this.stopped = true;
    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    const socket = this.socket;
    this.socket = null;
    socket?.close(1000, "client disconnect");
    this.rejectAllPending({
      code: "CONNECTION_CLOSED",
      message: "runtime connection closed",
    });
    this.setState("disconnected");
  }

  getStatus(): Promise<RuntimeStatus> {
    const id = randomUUID();
    return this.request(id, frameGetStatus(id)).then((payload) => {
      const status = asStatus(payload);
      if (!status) {
        throw { code: "INVALID_STATUS", message: "malformed status frame from runtime" } satisfies RuntimeError;
      }
      return status;
    });
  }

  ping(): Promise<boolean> {
    const id = randomUUID();
    return this.request(id, framePing(id)).then((payload) => payload.pong === true);
  }

  submitCommand(text: string): { id: string; accepted: boolean } {
    const id = randomUUID();
    if (this.state !== "connected" || this.socket === null) {
      return { id, accepted: false };
    }
    this.socket.send(JSON.stringify(frameSubmitCommand(id, text)));
    return { id, accepted: true };
  }

  cancelCommand(id: string): { id: string; cancelled: boolean } {
    this.sendFrame(frameCancelCommand(id));
    return { id, cancelled: true };
  }

  respondApproval(id: string, approved: boolean): void {
    this.sendFrame(frameApprovalResponse(id, approved));
  }

  // startVoice sends a voice.start frame (SPEC-0068's tray "Start voice mode"
  // control) and resolves with the runtime's voice.result acknowledgement.
  startVoice(): Promise<VoiceResult> {
    return this.voiceRequest(frameVoiceStart);
  }

  // stopVoice sends a voice.stop frame and resolves with the runtime's
  // voice.result acknowledgement.
  stopVoice(): Promise<VoiceResult> {
    return this.voiceRequest(frameVoiceStop);
  }

  // listAgents requests the runtime's registered agents (agents.list ->
  // agents.result, SPEC-0070's dashboard data source).
  listAgents(): Promise<AgentListResult> {
    const id = randomUUID();
    return this.request(id, frameAgentsList(id)).then((payload) => {
      const result = asAgentList(payload);
      if (!result) {
        throw { code: "INVALID_AGENTS_RESULT", message: "malformed agents.result frame from runtime" } satisfies RuntimeError;
      }
      return result;
    });
  }

  // setAgentEnabled drives an agent.start / agent.stop lifecycle frame and
  // resolves with the runtime's agent.result acknowledgement. An ok:false
  // result (e.g. the runtime is not wired with a LifecycleManager yet) is
  // returned to the caller rather than thrown, so the desktop can still record
  // the local intent.
  setAgentEnabled(id: string, enabled: boolean): Promise<AgentControlResult> {
    const requestId = randomUUID();
    const frame = enabled ? frameAgentStart(requestId, id) : frameAgentStop(requestId, id);
    return this.request(requestId, frame).then((payload) => {
      const result = asAgentControlResult(payload);
      if (!result) {
        throw {
          code: "INVALID_AGENT_RESULT",
          message: "malformed agent.result frame from runtime",
        } satisfies RuntimeError;
      }
      return result;
    });
  }

  // listMemories requests the runtime's memory records (memory.list ->
  // memory.result, SPEC-0071's viewer data source), optionally scoped to one
  // MemoryType.
  listMemories(type?: string): Promise<MemoryListResult> {
    const id = randomUUID();
    return this.request(id, frameMemoryList(id, type)).then((payload) => {
      const result = asMemoryList(payload);
      if (!result) {
        throw {
          code: "INVALID_MEMORY_RESULT",
          message: "malformed memory.result frame from runtime",
        } satisfies RuntimeError;
      }
      return result;
    });
  }

  // searchMemories requests the runtime's memory search (memory.search ->
  // memory.result, SPEC-0071's search support), scoped to one MemoryType when
  // given and capped at limit results when given.
  searchMemories(query: string, type?: string, limit?: number): Promise<MemoryListResult> {
    const id = randomUUID();
    return this.request(id, frameMemorySearch(id, query, type, limit)).then((payload) => {
      const result = asMemoryList(payload);
      if (!result) {
        throw {
          code: "INVALID_MEMORY_RESULT",
          message: "malformed memory.result frame from runtime",
        } satisfies RuntimeError;
      }
      return result;
    });
  }

  // updateMemory sends a memory.update frame (SPEC-0071's "editing where
  // allowed") and resolves with the runtime's memory.result acknowledgement.
  updateMemory(id: string, content: string): Promise<MemoryControlResult> {
    const requestId = randomUUID();
    return this.request(requestId, frameMemoryUpdate(requestId, id, content)).then((payload) => {
      const result = asMemoryControlResult(payload);
      if (!result) {
        throw {
          code: "INVALID_MEMORY_RESULT",
          message: "malformed memory.result frame from runtime",
        } satisfies RuntimeError;
      }
      return result;
    });
  }

  // deleteMemory sends a memory.delete frame (SPEC-0071's deletion support)
  // and resolves with the runtime's memory.result acknowledgement.
  deleteMemory(id: string): Promise<MemoryControlResult> {
    const requestId = randomUUID();
    return this.request(requestId, frameMemoryDelete(requestId, id)).then((payload) => {
      const result = asMemoryControlResult(payload);
      if (!result) {
        throw {
          code: "INVALID_MEMORY_RESULT",
          message: "malformed memory.result frame from runtime",
        } satisfies RuntimeError;
      }
      return result;
    });
  }

  private voiceRequest(buildFrame: (id: string) => RuntimeFrame): Promise<VoiceResult> {
    const id = randomUUID();
    return this.request(id, buildFrame(id)).then((payload) => {
      const result = asVoiceResult(payload);
      if (!result) {
        throw { code: "INVALID_VOICE_RESULT", message: "malformed voice.result frame from runtime" } satisfies RuntimeError;
      }
      return result;
    });
  }

  private request(id: string, frame: RuntimeFrame): Promise<Record<string, unknown>> {
    return new Promise<Record<string, unknown>>((resolve, reject) => {
      if (this.state !== "connected" || this.socket === null) {
        reject({ code: "NOT_CONNECTED", message: "runtime connection is not open" } satisfies RuntimeError);
        return;
      }
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject({ code: "REQUEST_TIMEOUT", message: `timed out waiting for reply to ${id}` } satisfies RuntimeError);
      }, this.requestTimeoutMs);
      this.pending.set(id, { resolve, reject, timer });
      this.socket?.send(JSON.stringify(frame));
    });
  }

  private sendFrame(frame: RuntimeFrame): void {
    this.socket?.send(JSON.stringify(frame));
  }

  private handleOpen(socket: RuntimeSocket): void {
    if (this.socket !== socket) {
      return;
    }
    this.reconnectAttempt = 0;
    this.setState("connected");
  }

  private handleMessage(event: unknown): void {
    const data = (event as { data?: unknown }).data;
    if (typeof data !== "string") {
      return;
    }
    const parsed = parseFrame(data);
    if (!parsed.ok) {
      this.handlers.onError(parsed.error);
      return;
    }
    this.dispatch(parsed.frame);
  }

  private handleClose(socket: RuntimeSocket): void {
    if (this.socket !== socket) {
      return;
    }
    this.socket = null;
    this.rejectAllPending({
      code: "CONNECTION_CLOSED",
      message: "runtime connection closed",
    });
    this.setState("disconnected");
    if (this.stopped) {
      return;
    }
    this.scheduleReconnect();
  }

  private handleSocketError(socket: RuntimeSocket): void {
    if (this.socket === socket) {
      this.handlers.onError({
        code: "CONNECTION_ERROR",
        message: `websocket error on ${this.url}`,
      });
    }
  }

  private dispatch(frame: RuntimeFrame): void {
    switch (frame.type) {
      case RuntimeFrameType.pong:
      case RuntimeFrameType.status:
      case RuntimeFrameType.voiceResult:
      case RuntimeFrameType.agentsResult:
      case RuntimeFrameType.agentResult:
      case RuntimeFrameType.memoryResult:
        this.settleRequest(frame);
        return;
      case RuntimeFrameType.statusChanged: {
        const status = asStatus(frame.payload);
        if (status !== null) {
          this.handlers.onStatusChanged(status);
        }
        return;
      }
      case RuntimeFrameType.event: {
        const event = asEvent(frame.payload);
        if (event !== null) {
          this.handlers.onEvent(event);
        }
        return;
      }
      case RuntimeFrameType.commandStream: {
        const chunk = asStreamChunk(frame.payload);
        if (chunk !== null) {
          this.handlers.onStreamChunk(chunk);
        }
        return;
      }
      case RuntimeFrameType.commandResult: {
        const result = asCommandResult(frame.payload);
        if (result !== null) {
          this.handlers.onCommandResult(result);
        }
        return;
      }
      case RuntimeFrameType.toolApprovalRequested: {
        const request = asToolApprovalRequested(frame.payload);
        if (request !== null) {
          this.handlers.onApprovalRequested(request);
        }
        return;
      }
      case RuntimeFrameType.error: {
        const error = asErrorPayload(frame.payload);
        if (error !== null) {
          this.handlers.onError(error);
        }
        return;
      }
      default:
        return;
    }
  }

  private settleRequest(frame: RuntimeFrame): void {
    if (frame.id === undefined) {
      return;
    }
    const pending = this.pending.get(frame.id);
    if (pending === undefined) {
      return;
    }
    this.pending.delete(frame.id);
    clearTimeout(pending.timer);
    pending.resolve(frame.payload ?? {});
  }

  private scheduleReconnect(): void {
    const attempt = this.reconnectAttempt;
    const delay = Math.min(
      this.reconnectDelayMs * 2 ** attempt,
      this.maxReconnectDelayMs,
    );
    this.reconnectAttempt = attempt + 1;
    this.reconnectTimer = setTimeout(() => this.connect(), delay);
  }

  private rejectAllPending(error: RuntimeError): void {
    for (const [id, pending] of this.pending) {
      clearTimeout(pending.timer);
      pending.reject(error);
      this.pending.delete(id);
    }
  }

  private setState(state: RuntimeConnectionState): void {
    this.state = state;
    this.handlers.onStateChanged(state);
  }
}
