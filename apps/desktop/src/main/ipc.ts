import { app, type BrowserWindow, type IpcMain } from "electron";
import {
  IpcChannels,
  assertIpcChannel,
  fail,
  ok,
  validateToolApprovalResponse,
  validateUserCommand,
  type CommandCancelResult,
  type IpcResult,
  type RuntimeStatus,
  type ToolApprovalResult,
  type UserCommandResult,
} from "../shared/ipc";
import type { Settings, SettingsPatch } from "../shared/settings";
import type { RuntimeClient } from "./runtimeClient";
import type { SettingsStore } from "./settingsStore";

export function registerIpcHandlers(ipc: IpcMain, runtime: RuntimeClient, settings: SettingsStore): void {
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
}

export function broadcast(win: BrowserWindow, channel: string, payload: unknown): void {
  win.webContents.send(assertIpcChannel(channel), payload);
}
