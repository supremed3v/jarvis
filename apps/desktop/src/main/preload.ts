import { contextBridge, ipcRenderer } from "electron";

contextBridge.exposeInMainWorld("jarvis", {
  getVersion: (): string => process.versions.electron,
  platform: process.platform,

  // Runtime connection stub — concrete IPC channels are defined by SPEC-0064.
  runtime: {
    ping: (): Promise<string> =>
      ipcRenderer.invoke("runtime:ping"),
  },
});
