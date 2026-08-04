import { contextBridge, ipcRenderer, type IpcRendererEvent } from "electron";
import type {
  AgentEnabledPatch,
  CommandCancelResult,
  IpcResult,
  JarvisBridge,
  MemoryDeleteRequest,
  MemoryListRequest,
  MemorySearchRequest,
  MemoryUpdateRequest,
  RuntimeStatus,
  ToolApprovalRequest,
  ToolApprovalResponse,
  ToolApprovalResult,
  UserCommandMessage,
  UserCommandResult,
} from "../shared/ipc";
import type { CommandResult, RuntimeEvent, StreamChunk } from "../shared/runtime";
import type { Settings, SettingsPatch } from "../shared/settings";
import type { VoiceUiSnapshot } from "../shared/voice";
import type { AgentDashboardState } from "../shared/agents";
import type { MemoryViewerState } from "../shared/memory";

// Sandboxed preload scripts run as a single bundled file and cannot require
// local modules (Electron docs, Process Sandboxing), so channel names are
// repeated as literals here rather than imported from ../shared/ipc. Type-only
// imports are erased at compile time and keep this file sandbox-compatible.
const CHANNEL = {
  runtimePing: "jarvis:runtime:ping",
  runtimeGetStatus: "jarvis:runtime:get-status",
  runtimeStatusChanged: "jarvis:runtime:status-changed",
  runtimeEvent: "jarvis:runtime:event",
  commandSubmit: "jarvis:command:submit",
  commandCancel: "jarvis:command:cancel",
  commandStream: "jarvis:command:stream",
  commandResult: "jarvis:command:result",
  toolApprovalRequested: "jarvis:tool:approval-requested",
  toolApprovalResponse: "jarvis:tool:approval-response",
  voiceEvent: "jarvis:voice:event",
  settingsGet: "jarvis:settings:get",
  settingsSave: "jarvis:settings:save",
  settingsChanged: "jarvis:settings:changed",
  agentsList: "jarvis:agents:list",
  agentsSetEnabled: "jarvis:agents:set-enabled",
  agentsUpdated: "jarvis:agents:updated",
  memoryList: "jarvis:memory:list",
  memorySearch: "jarvis:memory:search",
  memoryUpdate: "jarvis:memory:update",
  memoryDelete: "jarvis:memory:delete",
  memoryUpdated: "jarvis:memory:updated",
} as const;

function invoke<T>(channel: string, payload?: unknown): Promise<IpcResult<T>> {
  return ipcRenderer.invoke(channel, payload).then(
    (result) => result as IpcResult<T>,
    (error: unknown): IpcResult<T> => ({
      ok: false,
      error: {
        code: "IPC_ERROR",
        message: error instanceof Error ? error.message : String(error),
      },
    }),
  );
}

function subscribe<T>(channel: string, callback: (payload: T) => void): () => void {
  const listener = (_event: IpcRendererEvent, payload: T): void => {
    callback(payload);
  };
  ipcRenderer.on(channel, listener);
  return () => {
    ipcRenderer.removeListener(channel, listener);
  };
}

const bridge: JarvisBridge = {
  getVersion: (): string => process.versions.electron,
  platform: process.platform,

  runtime: {
    ping: (): Promise<IpcResult<string>> => invoke(CHANNEL.runtimePing),
    getStatus: (): Promise<IpcResult<RuntimeStatus>> => invoke(CHANNEL.runtimeGetStatus),
    onStatusChanged: (cb: (status: RuntimeStatus) => void): (() => void) =>
      subscribe<RuntimeStatus>(CHANNEL.runtimeStatusChanged, cb),
    onEvent: (cb: (event: RuntimeEvent) => void): (() => void) =>
      subscribe<RuntimeEvent>(CHANNEL.runtimeEvent, cb),
  },

  commands: {
    submit: (text: string): Promise<IpcResult<UserCommandResult>> =>
      invoke<UserCommandResult>(CHANNEL.commandSubmit, { text } satisfies UserCommandMessage),
    cancel: (id: string): Promise<IpcResult<CommandCancelResult>> =>
      invoke<CommandCancelResult>(CHANNEL.commandCancel, id),
    onStream: (cb: (chunk: StreamChunk) => void): (() => void) =>
      subscribe<StreamChunk>(CHANNEL.commandStream, cb),
    onResult: (cb: (result: CommandResult) => void): (() => void) =>
      subscribe<CommandResult>(CHANNEL.commandResult, cb),
  },

  tools: {
    respond: (response: ToolApprovalResponse): Promise<IpcResult<ToolApprovalResult>> =>
      invoke<ToolApprovalResult>(CHANNEL.toolApprovalResponse, response),
    onApprovalRequested: (cb: (request: ToolApprovalRequest) => void): (() => void) =>
      subscribe<ToolApprovalRequest>(CHANNEL.toolApprovalRequested, cb),
  },

  voice: {
    onEvent: (cb: (snapshot: VoiceUiSnapshot) => void): (() => void) =>
      subscribe<VoiceUiSnapshot>(CHANNEL.voiceEvent, cb),
  },

  settings: {
    get: (): Promise<IpcResult<Settings>> => invoke<Settings>(CHANNEL.settingsGet),
    save: (patch: SettingsPatch): Promise<IpcResult<Settings>> =>
      invoke<Settings>(CHANNEL.settingsSave, patch),
    onChanged: (cb: (settings: Settings) => void): (() => void) =>
      subscribe<Settings>(CHANNEL.settingsChanged, cb),
  },

  agents: {
    list: (): Promise<IpcResult<AgentDashboardState>> => invoke<AgentDashboardState>(CHANNEL.agentsList),
    setEnabled: (patch: AgentEnabledPatch): Promise<IpcResult<AgentEnabledPatch>> =>
      invoke<AgentEnabledPatch>(CHANNEL.agentsSetEnabled, patch),
    onUpdated: (cb: (snapshot: AgentDashboardState) => void): (() => void) =>
      subscribe<AgentDashboardState>(CHANNEL.agentsUpdated, cb),
  },

  memory: {
    list: (request?: MemoryListRequest): Promise<IpcResult<MemoryViewerState>> =>
      invoke<MemoryViewerState>(CHANNEL.memoryList, request),
    search: (request: MemorySearchRequest): Promise<IpcResult<MemoryViewerState>> =>
      invoke<MemoryViewerState>(CHANNEL.memorySearch, request),
    update: (request: MemoryUpdateRequest): Promise<IpcResult<MemoryUpdateRequest>> =>
      invoke<MemoryUpdateRequest>(CHANNEL.memoryUpdate, request),
    delete: (request: MemoryDeleteRequest): Promise<IpcResult<MemoryDeleteRequest>> =>
      invoke<MemoryDeleteRequest>(CHANNEL.memoryDelete, request),
    onUpdated: (cb: (snapshot: MemoryViewerState) => void): (() => void) =>
      subscribe<MemoryViewerState>(CHANNEL.memoryUpdated, cb),
  },
};

contextBridge.exposeInMainWorld("jarvis", bridge);
