import { app, type BrowserWindow, type IpcMain } from "electron";
import {
  IpcChannels,
  assertIpcChannel,
  fail,
  ok,
  validateAgentEnabledPatch,
  validateMemoryDelete,
  validateMemoryList,
  validateMemorySearch,
  validateMemoryUpdate,
  validateToolApprovalResponse,
  validateUserCommand,
  type AgentEnabledPatch,
  type CommandCancelResult,
  type IpcResult,
  type MemoryDeleteRequest,
  type MemoryUpdateRequest,
  type RuntimeStatus,
  type ToolApprovalResult,
  type UserCommandResult,
} from "../shared/ipc";
import type { AgentDashboardState, AgentUiStore } from "../shared/agents";
import type { MemoryUiStore, MemoryViewerState } from "../shared/memory";
import type { LogUiStore, LogsViewerState } from "../shared/logs";
import type { Settings, SettingsPatch } from "../shared/settings";
import type { RuntimeClient } from "./runtimeClient";
import type { SettingsStore } from "./settingsStore";

export function registerIpcHandlers(
  ipc: IpcMain,
  runtime: RuntimeClient,
  settings: SettingsStore,
  agents: AgentUiStore,
  toggleAgent: (patch: AgentEnabledPatch) => Promise<IpcResult<AgentEnabledPatch>>,
  memory: MemoryUiStore,
  applyMemoryUpdate: (request: MemoryUpdateRequest) => Promise<IpcResult<MemoryUpdateRequest>>,
  applyMemoryDelete: (request: MemoryDeleteRequest) => Promise<IpcResult<MemoryDeleteRequest>>,
  logs: LogUiStore,
): void {
  ipc.handle(assertIpcChannel(IpcChannels.runtime.ping), () => ok("pong"));

  ipc.handle(assertIpcChannel(IpcChannels.runtime.getStatus), async (): Promise<IpcResult<RuntimeStatus>> => {
    try {
      return ok<RuntimeStatus>(await runtime.getStatus());
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      return fail<RuntimeStatus>("RUNTIME_UNREACHABLE", message);
    }
  });

  ipc.handle(assertIpcChannel(IpcChannels.command.submit), (_event, payload: unknown) => {
    const result = validateUserCommand(payload);
    if (!result.ok) {
      return result;
    }
    const accepted: UserCommandResult = runtime.submitCommand(result.data.text);
    return ok(accepted);
  });

  ipc.handle(assertIpcChannel(IpcChannels.command.cancel), (_event, payload: unknown) => {
    if (typeof payload !== "string" || payload.trim().length === 0) {
      return fail("INVALID_COMMAND_ID", "Command id must be a non-empty string");
    }
    const result: CommandCancelResult = runtime.cancelCommand(payload);
    return ok(result);
  });

  ipc.handle(assertIpcChannel(IpcChannels.tool.approvalResponse), (_event, payload: unknown) => {
    const result = validateToolApprovalResponse(payload);
    if (!result.ok) {
      return result;
    }
    runtime.respondApproval(result.data.id, result.data.approved);
    const received: ToolApprovalResult = { id: result.data.id, received: true };
    return ok(received);
  });

  ipc.handle(assertIpcChannel(IpcChannels.settings.get), (): IpcResult<Settings> => ok(settings.get()));

  ipc.handle(assertIpcChannel(IpcChannels.settings.save), (_event, payload: unknown): IpcResult<Settings> => {
    if (typeof payload !== "object" || payload === null || Array.isArray(payload)) {
      return fail("INVALID_SETTINGS", "Settings payload must be an object");
    }
    return settings.update(payload as SettingsPatch);
  });

  // jarvis:agents:list returns the dashboard snapshot the main process keeps
  // current (SPEC-0070). jarvis:agents:set-enabled validates the toggle and
  // hands it to the main-process toggleAgent orchestration (persist locally,
  // drive the runtime's lifecycle best-effort), which reports back the result.
  ipc.handle(assertIpcChannel(IpcChannels.agents.list), (): IpcResult<AgentDashboardState> => ok(agents.current));

  ipc.handle(
    assertIpcChannel(IpcChannels.agents.setEnabled),
    async (_event, payload: unknown): Promise<IpcResult<AgentEnabledPatch>> => {
      const result = validateAgentEnabledPatch(payload);
      if (!result.ok) {
        return result;
      }
      return toggleAgent(result.data);
    },
  );

  // jarvis:memory:list returns the viewer snapshot scoped to an optional
  // MemoryType, fetching it fresh from the runtime (SPEC-0071's data source).
  // jarvis:memory:search fetches a query-scoped snapshot. Both reduce their
  // result into the main-process store, so a later refresh or edit broadcast
  // carries a consistent state.
  ipc.handle(
    assertIpcChannel(IpcChannels.memory.list),
    async (_event, payload: unknown): Promise<IpcResult<MemoryViewerState>> => {
      const validated = validateMemoryList(payload ?? {});
      if (!validated.ok) {
        return validated;
      }
      memory.setLoading(true);
      try {
        const list = await runtime.listMemories(validated.data.type);
        return ok<MemoryViewerState>(memory.reduceList(list));
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        memory.setError(message);
        return fail<MemoryViewerState>("MEMORY_UNREACHABLE", message);
      } finally {
        memory.setLoading(false);
      }
    },
  );

  ipc.handle(
    assertIpcChannel(IpcChannels.memory.search),
    async (_event, payload: unknown): Promise<IpcResult<MemoryViewerState>> => {
      const validated = validateMemorySearch(payload);
      if (!validated.ok) {
        return validated;
      }
      memory.setLoading(true);
      try {
        const list = await runtime.searchMemories(validated.data.query, validated.data.type, validated.data.limit);
        return ok<MemoryViewerState>(memory.reduceList(list));
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        memory.setError(message);
        return fail<MemoryViewerState>("MEMORY_UNREACHABLE", message);
      } finally {
        memory.setLoading(false);
      }
    },
  );

  // jarvis:memory:update / jarvis:memory:delete validate the payload and hand
  // it to the main-process apply* orchestration (drive the runtime's
  // memory.update / memory.delete best-effort, update the store, broadcast the
  // fresh snapshot), which reports back the result.
  ipc.handle(
    assertIpcChannel(IpcChannels.memory.update),
    async (_event, payload: unknown): Promise<IpcResult<MemoryUpdateRequest>> => {
      const validated = validateMemoryUpdate(payload);
      if (!validated.ok) {
        return validated;
      }
      return applyMemoryUpdate(validated.data);
    },
  );

  ipc.handle(
    assertIpcChannel(IpcChannels.memory.delete),
    async (_event, payload: unknown): Promise<IpcResult<MemoryDeleteRequest>> => {
      const validated = validateMemoryDelete(payload);
      if (!validated.ok) {
        return validated;
      }
      return applyMemoryDelete(validated.data);
    },
  );

  ipc.handle(
    assertIpcChannel(IpcChannels.logs.list),
    (): IpcResult<LogsViewerState> => ok(logs.current),
  );

  ipc.handle(
    assertIpcChannel(IpcChannels.logs.clear),
    (): IpcResult<LogsViewerState> => ok(logs.clear()),
  );
}

export function broadcast(win: BrowserWindow, channel: string, payload: unknown): void {
  win.webContents.send(assertIpcChannel(channel), payload);
}
