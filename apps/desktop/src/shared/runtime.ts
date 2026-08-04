// runtime.ts implements the desktop half of the SPEC-0065 wire protocol: the
// WebSocket frames exchanged with the core runtime's Bridge
// (services/core/ws_bridge.go). Frame type names, envelope shape, and payload
// fields mirror the Go side exactly so either end can be reimplemented
// independently. The renderer never sees these frames directly - the main
// process maps them onto the jarvis:* IPC channels defined in ./ipc.

export const RUNTIME_DEFAULT_ADDR = "127.0.0.1:42321";
export const RUNTIME_PATH = "/ws";
export const RUNTIME_URL = `ws://${RUNTIME_DEFAULT_ADDR}${RUNTIME_PATH}`;

export const RuntimeFrameType = {
  ping: "ping",
  getStatus: "get_status",
  commandSubmit: "command.submit",
  commandCancel: "command.cancel",
  toolApprovalResponse: "tool.approval_response",
  voiceStart: "voice.start",
  voiceStop: "voice.stop",
  pong: "pong",
  status: "status",
  statusChanged: "status.changed",
  event: "event",
  commandStream: "command.stream",
  commandResult: "command.result",
  toolApprovalRequested: "tool.approval_requested",
  voiceResult: "voice.result",
  error: "error",
} as const;

export type RuntimeFrameType = (typeof RuntimeFrameType)[keyof typeof RuntimeFrameType];

export const RUNTIME_FRAME_TYPES: readonly string[] = Object.values(RuntimeFrameType);

const RUNTIME_FRAME_TYPE_SET: ReadonlySet<string> = new Set<string>(RUNTIME_FRAME_TYPES);

export function isRuntimeFrameType(type: string): type is RuntimeFrameType {
  return RUNTIME_FRAME_TYPE_SET.has(type);
}

export interface RuntimeError {
  code: string;
  message: string;
}

// RuntimeFrame is the JSON envelope on the wire (bridgeFrame on the Go side).
export interface RuntimeFrame {
  type: RuntimeFrameType;
  id?: string;
  payload?: Record<string, unknown>;
}

export type RuntimeState = "starting" | "ready" | "stopping" | "stopped" | "error";

export interface RuntimeStatus {
  state: RuntimeState;
  version: string;
  lastError?: string;
}

export interface RuntimeEvent {
  eventType: string;
  timestamp: number;
  source?: string;
  payload?: unknown;
}

export interface StreamChunk {
  id: string;
  text: string;
  partial: string;
  done: boolean;
}

export interface CommandResult {
  id: string;
  ok: boolean;
  result?: Record<string, unknown>;
  error?: RuntimeError;
  cancelled?: boolean;
}

export interface ToolApprovalRequested {
  id: string;
  agentId: string;
  category: string;
}

export interface ErrorFramePayload {
  error: RuntimeError;
}

export function framePing(id: string): RuntimeFrame {
  return { type: RuntimeFrameType.ping, id };
}

export function frameGetStatus(id: string): RuntimeFrame {
  return { type: RuntimeFrameType.getStatus, id };
}

export function frameSubmitCommand(id: string, text: string): RuntimeFrame {
  return { type: RuntimeFrameType.commandSubmit, id, payload: { text } };
}

export function frameCancelCommand(id: string): RuntimeFrame {
  return { type: RuntimeFrameType.commandCancel, payload: { id } };
}

export function frameApprovalResponse(id: string, approved: boolean): RuntimeFrame {
  return { type: RuntimeFrameType.toolApprovalResponse, payload: { id, approved } };
}

export function frameVoiceStart(id: string): RuntimeFrame {
  return { type: RuntimeFrameType.voiceStart, id };
}

export function frameVoiceStop(id: string): RuntimeFrame {
  return { type: RuntimeFrameType.voiceStop, id };
}

export type ParseFrameResult =
  | { ok: true; frame: RuntimeFrame }
  | { ok: false; error: RuntimeError };

// parseFrame decodes one raw text message into a RuntimeFrame, rejecting
// malformed JSON and frame types the protocol does not define.
export function parseFrame(raw: string): ParseFrameResult {
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return {
      ok: false,
      error: { code: "INVALID_FRAME", message: "frame is not valid JSON" },
    };
  }
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    return {
      ok: false,
      error: { code: "INVALID_FRAME", message: "frame must be a JSON object" },
    };
  }
  const type = (parsed as { type?: unknown }).type;
  if (typeof type !== "string" || !isRuntimeFrameType(type)) {
    return {
      ok: false,
      error: { code: "UNKNOWN_FRAME_TYPE", message: `unknown frame type: ${String(type)}` },
    };
  }
  const frame = parsed as RuntimeFrame;
  return { ok: true, frame };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

// asStatus decodes a status / status.changed payload.
export function asStatus(payload: unknown): RuntimeStatus | null {
  if (!isRecord(payload)) {
    return null;
  }
  const state = payload.state;
  if (typeof state !== "string" || !["starting", "ready", "stopping", "stopped", "error"].includes(state)) {
    return null;
  }
  if (typeof payload.version !== "string") {
    return null;
  }
  const status: RuntimeStatus = { state: state as RuntimeState, version: payload.version };
  if (typeof payload.lastError === "string") {
    status.lastError = payload.lastError;
  }
  return status;
}

// asStreamChunk decodes a command.stream payload.
export function asStreamChunk(payload: unknown): StreamChunk | null {
  if (!isRecord(payload)) {
    return null;
  }
  if (typeof payload.id !== "string" || typeof payload.text !== "string" || typeof payload.partial !== "string") {
    return null;
  }
  return {
    id: payload.id,
    text: payload.text,
    partial: payload.partial,
    done: payload.done === true,
  };
}

// asCommandResult decodes a command.result payload.
export function asCommandResult(payload: unknown): CommandResult | null {
  if (!isRecord(payload)) {
    return null;
  }
  if (typeof payload.id !== "string" || typeof payload.ok !== "boolean") {
    return null;
  }
  const result: CommandResult = { id: payload.id, ok: payload.ok };
  if (isRecord(payload.result)) {
    result.result = payload.result;
  }
  if (isRecord(payload.error) && typeof payload.error.code === "string" && typeof payload.error.message === "string") {
    result.error = { code: payload.error.code, message: payload.error.message };
  }
  if (payload.cancelled === true) {
    result.cancelled = true;
  }
  return result;
}

// asEvent decodes an event payload.
export function asEvent(payload: unknown): RuntimeEvent | null {
  if (!isRecord(payload)) {
    return null;
  }
  if (typeof payload.eventType !== "string" || typeof payload.timestamp !== "number") {
    return null;
  }
  const event: RuntimeEvent = { eventType: payload.eventType, timestamp: payload.timestamp };
  if (typeof payload.source === "string") {
    event.source = payload.source;
  }
  if ("payload" in payload) {
    event.payload = payload.payload;
  }
  return event;
}

// asToolApprovalRequested decodes a tool.approval_requested payload.
export function asToolApprovalRequested(payload: unknown): ToolApprovalRequested | null {
  if (!isRecord(payload)) {
    return null;
  }
  if (
    typeof payload.id !== "string" ||
    typeof payload.agentId !== "string" ||
    typeof payload.category !== "string"
  ) {
    return null;
  }
  return { id: payload.id, agentId: payload.agentId, category: payload.category };
}

// asErrorPayload decodes an error frame payload.
export function asErrorPayload(payload: unknown): RuntimeError | null {
  if (!isRecord(payload) || !isRecord(payload.error)) {
    return null;
  }
  if (typeof payload.error.code !== "string" || typeof payload.error.message !== "string") {
    return null;
  }
  return { code: payload.error.code, message: payload.error.message };
}

// VoiceResult is the synchronous acknowledgement for a voice.start / voice.stop
// frame (SPEC-0068's tray "Start voice mode" control): ok is true when the
// transition took effect, and a structured error explains a failure.
export interface VoiceResult {
  ok: boolean;
  error?: RuntimeError;
}

// asVoiceResult decodes a voice.result payload.
export function asVoiceResult(payload: unknown): VoiceResult | null {
  if (!isRecord(payload) || typeof payload.ok !== "boolean") {
    return null;
  }
  const result: VoiceResult = { ok: payload.ok };
  if (isRecord(payload.error) && typeof payload.error.code === "string" && typeof payload.error.message === "string") {
    result.error = { code: payload.error.code, message: payload.error.message };
  }
  return result;
}
