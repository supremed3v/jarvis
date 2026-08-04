import { app, type BrowserWindow, type IpcMain } from "electron";
import { randomUUID } from "crypto";
import {
  IpcChannels,
  assertIpcChannel,
  fail,
  ok,
  validateToolApprovalResponse,
  validateUserCommand,
  type CommandCancelResult,
  type RuntimeStatus,
  type ToolApprovalResult,
  type UserCommandResult,
} from "../shared/ipc";

export function registerIpcHandlers(ipc: IpcMain): void {
  ipc.handle(assertIpcChannel(IpcChannels.runtime.ping), () => ok("pong"));

  ipc.handle(assertIpcChannel(IpcChannels.runtime.getStatus), () => ok<RuntimeStatus>({
    state: "ready",
    version: app.getVersion(),
  }));

  ipc.handle(assertIpcChannel(IpcChannels.command.submit), (_event, payload: unknown) => {
    const result = validateUserCommand(payload);
    if (!result.ok) {
      return result;
    }
    const accepted: UserCommandResult = { id: randomUUID(), accepted: true };
    return ok(accepted);
  });

  ipc.handle(assertIpcChannel(IpcChannels.command.cancel), (_event, payload: unknown) => {
    if (typeof payload !== "string" || payload.trim().length === 0) {
      return fail("INVALID_COMMAND_ID", "Command id must be a non-empty string");
    }
    const result: CommandCancelResult = { id: payload, cancelled: true };
    return ok(result);
  });

  ipc.handle(assertIpcChannel(IpcChannels.tool.approvalResponse), (_event, payload: unknown) => {
    const result = validateToolApprovalResponse(payload);
    if (!result.ok) {
      return result;
    }
    const received: ToolApprovalResult = { id: result.data.id, received: true };
    return ok(received);
  });
}

export function broadcast(win: BrowserWindow, channel: string, payload: unknown): void {
  win.webContents.send(assertIpcChannel(channel), payload);
}
