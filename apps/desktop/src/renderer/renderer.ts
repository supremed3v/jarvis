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

type VoiceUiState = "IDLE" | "LISTENING" | "THINKING" | "SPEAKING" | "ERROR";

interface VoiceSessionView {
  id: string;
  status: "active" | "ended";
  state: VoiceUiState;
  startedAt: number;
  transcript?: string;
  error?: string;
}

interface VoiceUiSnapshot {
  state: VoiceUiState;
  sessions: VoiceSessionView[];
  error?: string;
}

// Sandboxed renderer scripts run as plain scripts (no CommonJS module system),
// so this page cannot import ../shared/ipc or ../shared/voice at runtime; the
// bridge type is mirrored here and only ever used at compile time.
interface JarvisBridge {
  getVersion: () => string;
  platform: string;
  runtime: {
    getStatus: () => Promise<IpcResult<RuntimeStatus>>;
    onStatusChanged: (cb: (status: RuntimeStatus) => void) => () => void;
  };
  voice: {
    onEvent: (cb: (snapshot: VoiceUiSnapshot) => void) => () => void;
  };
}

function getBridge(): JarvisBridge | undefined {
  return (window as unknown as { jarvis?: JarvisBridge }).jarvis;
}

function stateClass(state: VoiceUiState): string {
  return state.toLowerCase();
}

function renderSnapshot(snapshot: VoiceUiSnapshot): void {
  const orb = document.getElementById("orb");
  const stateEl = document.getElementById("state");
  const errorEl = document.getElementById("error");
  const sessionsEl = document.getElementById("sessions");

  const stateClassValue = stateClass(snapshot.state);
  if (orb) {
    orb.className = `orb ${stateClassValue}`;
  }
  if (stateEl) {
    stateEl.textContent = snapshot.state;
    stateEl.classList.toggle("error-label", snapshot.state === "ERROR");
  }

  if (errorEl) {
    if (snapshot.error) {
      errorEl.textContent = `Voice error: ${snapshot.error}`;
      errorEl.classList.add("visible");
    } else {
      errorEl.textContent = "";
      errorEl.classList.remove("visible");
    }
  }

  if (sessionsEl) {
    sessionsEl.classList.toggle("empty", snapshot.sessions.length === 0);
    for (const child of Array.from(sessionsEl.querySelectorAll(".session-row"))) {
      child.remove();
    }
    for (const session of snapshot.sessions) {
      const row = document.createElement("div");
      row.className = "session-row";

      const dot = document.createElement("span");
      dot.className = `session-dot ${session.status}`;

      const id = document.createElement("span");
      id.className = "session-id";
      id.textContent = session.id;

      const meta = document.createElement("span");
      meta.className = "session-meta";
      const bits: string[] = [];
      if (session.transcript) {
        bits.push(`"${session.transcript}"`);
      }
      if (session.error) {
        bits.push(session.error);
      }
      meta.textContent = bits.join(" — ");

      const state = document.createElement("span");
      state.className = "session-state";
      state.textContent = session.status === "ended" ? "ended" : session.state;

      row.appendChild(dot);
      row.appendChild(id);
      row.appendChild(meta);
      row.appendChild(state);
      sessionsEl.appendChild(row);
    }
  }
}

document.addEventListener("DOMContentLoaded", () => {
  const statusEl = document.getElementById("status");
  const versionEl = document.getElementById("version");
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

  jarvis.voice.onEvent(renderSnapshot);
});
