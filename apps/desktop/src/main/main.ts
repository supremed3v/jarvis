import { app, BrowserWindow, ipcMain } from "electron";
import * as path from "path";
import { broadcast, registerIpcHandlers } from "./ipc";
import { IpcChannels } from "../shared/ipc";

let mainWindow: BrowserWindow | null = null;

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
    if (!mainWindow) {
      return;
    }
    broadcast(mainWindow, IpcChannels.runtime.statusChanged, {
      state: "ready",
      version: app.getVersion(),
    });
    broadcast(mainWindow, IpcChannels.voice.event, {
      type: "wake-word",
      timestamp: Date.now(),
    });
  });

  mainWindow.once("ready-to-show", () => {
    mainWindow?.show();
  });

  mainWindow.on("closed", () => {
    mainWindow = null;
  });
}

app.whenReady().then(() => {
  registerIpcHandlers(ipcMain);

  createWindow();

  app.on("activate", () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createWindow();
    }
  });
});

app.on("window-all-closed", () => {
  if (process.platform !== "darwin") {
    app.quit();
  }
});
