interface IpcError {
  code: string;
  message: string;
}

type IpcResult<T> =
  | { ok: true; data: T }
  | { ok: false; error: IpcError };

interface RuntimeStatus {
  state: string;
  version: string;
}

interface VoiceEvent {
  type: string;
  timestamp: number;
}

// Sandboxed renderer scripts run as plain scripts (no CommonJS module system),
// so this page cannot import ../shared/ipc at runtime; the bridge type is
// mirrored here and only ever used at compile time.
interface JarvisBridge {
  getVersion: () => string;
  platform: string;
  runtime: {
    getStatus: () => Promise<IpcResult<RuntimeStatus>>;
    onStatusChanged: (cb: (status: RuntimeStatus) => void) => () => void;
  };
  voice: {
    onEvent: (cb: (event: VoiceEvent) => void) => () => void;
  };
}

function getBridge(): JarvisBridge | undefined {
  return (window as unknown as { jarvis?: JarvisBridge }).jarvis;
}

document.addEventListener("DOMContentLoaded", () => {
  const statusEl = document.getElementById("status");
  const versionEl = document.getElementById("version");
  const eventEl = document.getElementById("voice-event");
  const jarvis = getBridge();

  if (!jarvis) {
    if (statusEl) statusEl.textContent = "Preload bridge unavailable";
    return;
  }

  if (versionEl) {
    versionEl.textContent = `Electron ${jarvis.getVersion()} — ${jarvis.platform}`;
  }

  jarvis.runtime.getStatus().then((result) => {
    if (statusEl) {
      if (result.ok) {
        statusEl.textContent = `Runtime: ${result.data.state} (v${result.data.version})`;
      } else {
        statusEl.textContent = `Runtime error: ${result.error.code} — ${result.error.message}`;
      }
    }
  });

  jarvis.voice.onEvent((event) => {
    if (eventEl) {
      const time = new Date(event.timestamp).toLocaleTimeString();
      eventEl.textContent = `Voice event: ${event.type} @ ${time}`;
    }
  });
});
