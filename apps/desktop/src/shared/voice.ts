// voice.ts implements the renderer-facing voice state model for SPEC-0067.
//
// The main process reduces inbound VOICE_SESSION_* runtime events (delivered by
// the core runtime's Bridge, SPEC-0065) into a VoiceUiSnapshot and pushes that
// snapshot to the renderer over the jarvis:voice:event IPC channel (SPEC-0064);
// the renderer renders the snapshot as-is. Keeping the reducer here rather than
// in the sandboxed renderer script makes it unit-testable in Node.
//
// The UI states (IDLE / LISTENING / THINKING / SPEAKING / ERROR) map onto the
// core session events (services/core/voice/session_manager.go):
//
//	VOICE_SESSION_STARTED      -> LISTENING
//	VOICE_SESSION_PROCESSING   -> THINKING
//	VOICE_SESSION_SPEAKING     -> SPEAKING
//	VOICE_SESSION_COMPLETED    -> IDLE
//	VOICE_SESSION_INTERRUPTED  -> IDLE
//	VOICE_SESSION_FAILED       -> ERROR (with the failure reason)

export const VoiceState = {
  idle: "IDLE",
  listening: "LISTENING",
  thinking: "THINKING",
  speaking: "SPEAKING",
  error: "ERROR",
} as const;

export type VoiceUiState = (typeof VoiceState)[keyof typeof VoiceState];

export const VoiceEventType = {
  started: "VOICE_SESSION_STARTED",
  processing: "VOICE_SESSION_PROCESSING",
  speaking: "VOICE_SESSION_SPEAKING",
  completed: "VOICE_SESSION_COMPLETED",
  failed: "VOICE_SESSION_FAILED",
  interrupted: "VOICE_SESSION_INTERRUPTED",
} as const;

export type VoiceEventType = (typeof VoiceEventType)[keyof typeof VoiceEventType];

const VOICE_EVENT_TYPE_SET: ReadonlySet<string> = new Set<string>(Object.values(VoiceEventType));

export function isVoiceEventType(type: string): type is VoiceEventType {
  return VOICE_EVENT_TYPE_SET.has(type);
}

// VoiceUiEvent is the subset of a runtime event (shared/runtime.ts RuntimeEvent)
// the reducer consumes; kept local so the reducer's contract is explicit and
// independent of the wire protocol types.
export interface VoiceUiEvent {
  eventType: string;
  timestamp: number;
  payload?: unknown;
}

// VoiceSessionView is one voice session the UI can show. Active sessions are in
// progress; ended sessions are retained (most recent first) so the UI renders a
// visible session history.
export interface VoiceSessionView {
  id: string;
  status: "active" | "ended";
  state: VoiceUiState;
  startedAt: number;
  transcript?: string;
  error?: string;
}

export interface VoiceUiSnapshot {
  state: VoiceUiState;
  sessions: VoiceSessionView[];
  error?: string;
}

export const MAX_VISIBLE_SESSIONS = 20;

export function createVoiceUi(): VoiceUiSnapshot {
  return { state: VoiceState.idle, sessions: [] };
}

function asString(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function asPayload(payload: unknown): Record<string, unknown> {
  return typeof payload === "object" && payload !== null && !Array.isArray(payload)
    ? (payload as Record<string, unknown>)
    : {};
}

function updateSession(
  sessions: VoiceSessionView[],
  id: string | undefined,
  update: (session: VoiceSessionView) => VoiceSessionView,
): VoiceSessionView[] {
  if (id === undefined) {
    return sessions;
  }
  return sessions.map((session) => (session.id === id ? update(session) : session));
}

function trimSessions(sessions: VoiceSessionView[]): VoiceSessionView[] {
  if (sessions.length <= MAX_VISIBLE_SESSIONS) {
    return sessions;
  }
  return sessions.slice(0, MAX_VISIBLE_SESSIONS);
}

// reduceVoiceUi applies one voice event to a snapshot, returning a new
// snapshot. Events are applied in arrival order; events for a session the
// store has not seen (e.g. state published before the app started listening)
// still update the current UI state without fabricating a session.
export function reduceVoiceUi(snapshot: VoiceUiSnapshot, event: VoiceUiEvent): VoiceUiSnapshot {
  const payload = asPayload(event.payload);
  const sessionId = asString(payload.sessionId);

  switch (event.eventType) {
    case VoiceEventType.started: {
      if (sessionId === undefined) {
        return snapshot;
      }
      const session: VoiceSessionView = {
        id: sessionId,
        status: "active",
        state: VoiceState.listening,
        startedAt: event.timestamp,
      };
      return {
        ...snapshot,
        state: VoiceState.listening,
        error: undefined,
        sessions: trimSessions([session, ...snapshot.sessions]),
      };
    }
    case VoiceEventType.processing: {
      const sessions = updateSession(snapshot.sessions, sessionId, (session) => ({
        ...session,
        state: VoiceState.thinking,
        transcript: asString(payload.transcript) ?? session.transcript,
      }));
      return { ...snapshot, state: VoiceState.thinking, error: undefined, sessions };
    }
    case VoiceEventType.speaking: {
      const sessions = updateSession(snapshot.sessions, sessionId, (session) => ({
        ...session,
        state: VoiceState.speaking,
      }));
      return { ...snapshot, state: VoiceState.speaking, error: undefined, sessions };
    }
    case VoiceEventType.completed:
    case VoiceEventType.interrupted: {
      const sessions = updateSession(snapshot.sessions, sessionId, (session) => ({
        ...session,
        status: "ended",
        state: VoiceState.idle,
      }));
      return { ...snapshot, state: VoiceState.idle, error: undefined, sessions };
    }
    case VoiceEventType.failed: {
      const reason = asString(payload.reason) ?? "voice session failed";
      const sessions = updateSession(snapshot.sessions, sessionId, (session) => ({
        ...session,
        status: "ended",
        state: VoiceState.error,
        error: reason,
      }));
      return { ...snapshot, state: VoiceState.error, error: reason, sessions };
    }
    default:
      return snapshot;
  }
}

// VoiceUiStore owns a single snapshot, reducing events onto it as they arrive
// (one instance per main process).
export class VoiceUiStore {
  private snapshot: VoiceUiSnapshot;

  constructor() {
    this.snapshot = createVoiceUi();
  }

  get current(): VoiceUiSnapshot {
    return this.snapshot;
  }

  reduce(event: VoiceUiEvent): VoiceUiSnapshot {
    this.snapshot = reduceVoiceUi(this.snapshot, event);
    return this.snapshot;
  }
}
