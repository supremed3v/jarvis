import { app, BrowserWindow, ipcMain } from "electron";
import * as path from "path";
import { broadcast, registerIpcHandlers } from "./ipc";
import { IpcChannels, type RuntimeStatus } from "../shared/ipc";
import { RuntimeClient } from "./runtimeClient";
import { VoiceUiStore, isVoiceEventType } from "../shared/voice";

let mainWindow: BrowserWindow | null = null;
let runtimeClient: RuntimeClient | null = null;
let voiceStore: VoiceUiStore = new VoiceUiStore();

function createWindow(): void {
  mainWindow = new BrowserWindow({
    width: 1024,
    height: 768,
    minWidth: 480,
    minHeight: 360,
    title: "JARVIS",
    show: false,
    webPreferences: {
      preload: path.join(__dirname, "preload.js"),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  });

  mainWindow.loadFile(path.join(__dirname, "..", "renderer", "index.html"));

  mainWindow.webContents.on("did-finish-load", () => {
    if (mainWindow) {
      broadcast(mainWindow, IpcChannels.voice.event, voiceStore.current);
    }
  });

  mainWindow.once("ready-to-show", () => {
    mainWindow?.show();
  });

  mainWindow.on("closed", () => {
    mainWindow = null;
  });
}

function forwardToRenderer(channel: string, payload: unknown): void {
  if (mainWindow !== null) {
    broadcast(mainWindow, channel, payload);
  }
}

function runtimeOfflineStatus(): RuntimeStatus {
  return {
    state: "error",
    version: app.getVersion(),
    lastError: "core runtime not connected",
  };
}

function createRuntimeClient(): RuntimeClient {
  return new RuntimeClient({
    handlers: {
      onStatusChanged: (status) => forwardToRenderer(IpcChannels.runtime.statusChanged, status),
      onEvent: (event) => {
        // Voice session events (VOICE_SESSION_*) drive the SPEC-0067 UI state;
        // every other runtime event goes out on the generic event channel.
        if (isVoiceEventType(event.eventType)) {
          const snapshot = voiceStore.reduce({
            eventType: event.eventType,
            timestamp: event.timestamp,
            payload: event.payload,
          });
          forwardToRenderer(IpcChannels.voice.event, snapshot);
          return;
        }
        forwardToRenderer(IpcChannels.runtime.event, event);
      },
      onStreamChunk: (chunk) => forwardToRenderer(IpcChannels.command.stream, chunk),
      onCommandResult: (result) => forwardToRenderer(IpcChannels.command.result, result),
      // The bridge reports the approval category on the wire; the renderer's
      // ToolApprovalRequest carries tool + args, so category maps onto tool.
      onApprovalRequested: (request) =>
        forwardToRenderer(IpcChannels.tool.approvalRequested, {
          id: request.id,
          tool: request.category,
          args: {},
        }),
      onError: (error) => {
        console.error(`[runtime] error: ${error.code}: ${error.message}`);
      },
      onStateChanged: (state) => {
        if (state === "disconnected") {
          forwardToRenderer(IpcChannels.runtime.statusChanged, runtimeOfflineStatus());
        }
      },
    },
  });
}

app.whenReady().then(() => {
  runtimeClient = createRuntimeClient();
  registerIpcHandlers(ipcMain, runtimeClient);
  runtimeClient.connect();

  createWindow();

  app.on("activate", () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createWindow();
    }
  });
});

app.on("will-quit", () => {
  runtimeClient?.disconnect();
});

app.on("window-all-closed", () => {
  if (process.platform !== "darwin") {
    app.quit();
  }
});
