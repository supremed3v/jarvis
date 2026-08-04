import type { CommandResult, RuntimeEvent, StreamChunk } from "./runtime";
import type { Settings, SettingsPatch } from "./settings";
import type { VoiceUiSnapshot } from "./voice";
import type { AgentDashboardState } from "./agents";

export const IpcChannels = {
  runtime: {
    ping: "jarvis:runtime:ping",
    getStatus: "jarvis:runtime:get-status",
    statusChanged: "jarvis:runtime:status-changed",
    event: "jarvis:runtime:event",
  },
  command: {
    submit: "jarvis:command:submit",
    cancel: "jarvis:command:cancel",
    stream: "jarvis:command:stream",
    result: "jarvis:command:result",
  },
  tool: {
    approvalRequested: "jarvis:tool:approval-requested",
    approvalResponse: "jarvis:tool:approval-response",
  },
  voice: {
    event: "jarvis:voice:event",
  },
  settings: {
    get: "jarvis:settings:get",
    save: "jarvis:settings:save",
    changed: "jarvis:settings:changed",
  },
  agents: {
    list: "jarvis:agents:list",
    setEnabled: "jarvis:agents:set-enabled",
    updated: "jarvis:agents:updated",
  },
} as const;

type IpcChannelGroup = (typeof IpcChannels)[keyof typeof IpcChannels];
export type IpcChannel = IpcChannelGroup[keyof IpcChannelGroup];

export const IPC_CHANNELS: readonly string[] = Object.values(IpcChannels).flatMap(
  (group: IpcChannelGroup) => Object.values(group),
);

const IPC_CHANNEL_SET: ReadonlySet<string> = new Set<string>(IPC_CHANNELS);

export function isIpcChannel(channel: string): channel is IpcChannel {
  return IPC_CHANNEL_SET.has(channel);
}

export function assertIpcChannel(channel: string): IpcChannel {
  if (!isIpcChannel(channel)) {
    throw new Error(`IPC channel not in allowlist: ${channel}`);
  }
  return channel;
}

export interface IpcError {
  code: string;
  message: string;
}

export type IpcResult<T> =
  | { ok: true; data: T }
  | { ok: false; error: IpcError };

export function ok<T>(data: T): IpcResult<T> {
  return { ok: true, data };
}

export function fail<T = never>(code: string, message: string): IpcResult<T> {
  return { ok: false, error: { code, message } };
}

export interface UserCommandMessage {
  text: string;
}

export interface UserCommandResult {
  id: string;
  accepted: boolean;
}

export function validateUserCommand(input: unknown): IpcResult<UserCommandMessage> {
  if (typeof input !== "object" || input === null) {
    return fail("INVALID_PAYLOAD", "User command payload must be an object");
  }
  const text = (input as { text?: unknown }).text;
  if (typeof text !== "string" || text.trim().length === 0) {
    return fail("INVALID_COMMAND", "User command text must be a non-empty string");
  }
  return ok({ text: text.trim() });
}

export interface CommandCancelResult {
  id: string;
  cancelled: boolean;
}

export interface RuntimeStatus {
  state: "starting" | "ready" | "stopping" | "stopped" | "error";
  version: string;
  lastError?: string;
}

export interface ToolApprovalRequest {
  id: string;
  tool: string;
  args: Record<string, unknown>;
}

export interface ToolApprovalResponse {
  id: string;
  approved: boolean;
  reason?: string;
}

export interface ToolApprovalResult {
  id: string;
  received: boolean;
}

// AgentEnabledPatch is the payload of jarvis:agents:set-enabled: the agent
// whose enabled flag the dashboard is changing, and the new value. The main
// process persists the flag locally (main/agentStore.ts) and, as a best-effort,
// drives the runtime's agent.start / agent.stop lifecycle frames.
export interface AgentEnabledPatch {
  id: string;
  enabled: boolean;
}

export function validateAgentEnabledPatch(input: unknown): IpcResult<AgentEnabledPatch> {
  if (typeof input !== "object" || input === null) {
    return fail("INVALID_PAYLOAD", "Agent enabled patch must be an object");
  }
  const record = input as { id?: unknown; enabled?: unknown };
  if (typeof record.id !== "string" || record.id.length === 0) {
    return fail("INVALID_AGENT_ID", "Agent enabled patch requires a non-empty id");
  }
  if (typeof record.enabled !== "boolean") {
    return fail("INVALID_AGENT_ENABLED", "Agent enabled patch requires a boolean enabled flag");
  }
  return ok({ id: record.id, enabled: record.enabled });
}

export function validateToolApprovalResponse(input: unknown): IpcResult<ToolApprovalResponse> {
  if (typeof input !== "object" || input === null) {
    return fail("INVALID_PAYLOAD", "Tool approval response must be an object");
  }
  const record = input as { id?: unknown; approved?: unknown };
  if (typeof record.id !== "string" || record.id.length === 0) {
    return fail("INVALID_APPROVAL_ID", "Tool approval response requires a non-empty id");
  }
  if (typeof record.approved !== "boolean") {
    return fail("INVALID_APPROVAL", "Tool approval response requires a boolean approved flag");
  }
  return ok({ id: record.id, approved: record.approved });
}

export type Subscribe<T> = (callback: (payload: T) => void) => () => void;

export interface JarvisBridge {
  getVersion: () => string;
  platform: string;
  runtime: {
    ping: () => Promise<IpcResult<string>>;
    getStatus: () => Promise<IpcResult<RuntimeStatus>>;
    onStatusChanged: Subscribe<RuntimeStatus>;
    onEvent: Subscribe<RuntimeEvent>;
  };
  commands: {
    submit: (text: string) => Promise<IpcResult<UserCommandResult>>;
    cancel: (id: string) => Promise<IpcResult<CommandCancelResult>>;
    onStream: Subscribe<StreamChunk>;
    onResult: Subscribe<CommandResult>;
  };
  tools: {
    respond: (response: ToolApprovalResponse) => Promise<IpcResult<ToolApprovalResult>>;
    onApprovalRequested: Subscribe<ToolApprovalRequest>;
  };
  voice: {
    onEvent: Subscribe<VoiceUiSnapshot>;
  };
  settings: {
    get: () => Promise<IpcResult<Settings>>;
    save: (patch: SettingsPatch) => Promise<IpcResult<Settings>>;
    onChanged: Subscribe<Settings>;
  };
  agents: {
    list: () => Promise<IpcResult<AgentDashboardState>>;
    setEnabled: (patch: AgentEnabledPatch) => Promise<IpcResult<AgentEnabledPatch>>;
    onUpdated: Subscribe<AgentDashboardState>;
  };
}
