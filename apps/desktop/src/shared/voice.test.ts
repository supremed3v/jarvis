import { test } from "node:test";
import assert from "node:assert";
import {
  VoiceEventType,
  VoiceState,
  VoiceUiStore,
  createVoiceUi,
  isVoiceEventType,
  reduceVoiceUi,
  type VoiceUiEvent,
  type VoiceUiSnapshot,
} from "./voice";

function event(eventType: string, timestamp: number, payload?: unknown): VoiceUiEvent {
  return { eventType, timestamp, payload };
}

function sessionIds(snapshot: VoiceUiSnapshot): string[] {
  return snapshot.sessions.map((session) => session.id);
}

test("voice event type allowlist accepts the six session events", () => {
  const types = Object.values(VoiceEventType);
  assert.strictEqual(types.length, 6);
  for (const type of types) {
    assert.strictEqual(isVoiceEventType(type), true, `expected ${type} to be a voice event`);
  }
  for (const type of ["TASK_STARTED", "VOICE_SESSION_FINISHED", ""]) {
    assert.strictEqual(isVoiceEventType(type), false, `expected ${type} to be rejected`);
  }
});

test("initial snapshot is IDLE with no sessions", () => {
  const snapshot = createVoiceUi();
  assert.strictEqual(snapshot.state, VoiceState.idle);
  assert.deepStrictEqual(snapshot.sessions, []);
  assert.strictEqual(snapshot.error, undefined);
});

test("session start sets LISTENING and makes the session visible", () => {
  const snapshot = reduceVoiceUi(createVoiceUi(), event(VoiceEventType.started, 1000, { sessionId: "s1" }));
  assert.strictEqual(snapshot.state, VoiceState.listening);
  assert.deepStrictEqual(sessionIds(snapshot), ["s1"]);
  const session = snapshot.sessions[0];
  assert.strictEqual(session.status, "active");
  assert.strictEqual(session.state, VoiceState.listening);
  assert.strictEqual(session.startedAt, 1000);
});

test("processing event sets THINKING and records the transcript", () => {
  let snapshot = createVoiceUi();
  snapshot = reduceVoiceUi(snapshot, event(VoiceEventType.started, 1000, { sessionId: "s1" }));
  snapshot = reduceVoiceUi(snapshot, event(VoiceEventType.processing, 1001, { sessionId: "s1", transcript: "turn on the lights" }));
  assert.strictEqual(snapshot.state, VoiceState.thinking);
  assert.strictEqual(snapshot.sessions[0].state, VoiceState.thinking);
  assert.strictEqual(snapshot.sessions[0].transcript, "turn on the lights");
  assert.strictEqual(snapshot.sessions[0].status, "active");
});

test("speaking event sets SPEAKING", () => {
  let snapshot = createVoiceUi();
  snapshot = reduceVoiceUi(snapshot, event(VoiceEventType.started, 1000, { sessionId: "s1" }));
  snapshot = reduceVoiceUi(snapshot, event(VoiceEventType.processing, 1001, { sessionId: "s1" }));
  snapshot = reduceVoiceUi(snapshot, event(VoiceEventType.speaking, 1002, { sessionId: "s1" }));
  assert.strictEqual(snapshot.state, VoiceState.speaking);
  assert.strictEqual(snapshot.sessions[0].state, VoiceState.speaking);
});

test("completion returns to IDLE and ends the session", () => {
  let snapshot = createVoiceUi();
  snapshot = reduceVoiceUi(snapshot, event(VoiceEventType.started, 1000, { sessionId: "s1" }));
  snapshot = reduceVoiceUi(snapshot, event(VoiceEventType.completed, 1003, { sessionId: "s1", transcript: "turn on the lights" }));
  assert.strictEqual(snapshot.state, VoiceState.idle);
  assert.strictEqual(snapshot.sessions[0].status, "ended");
  assert.strictEqual(snapshot.sessions[0].state, VoiceState.idle);
  assert.strictEqual(snapshot.error, undefined);
});

test("interruption returns to IDLE and ends the session", () => {
  let snapshot = createVoiceUi();
  snapshot = reduceVoiceUi(snapshot, event(VoiceEventType.started, 1000, { sessionId: "s1" }));
  snapshot = reduceVoiceUi(snapshot, event(VoiceEventType.interrupted, 1004, { sessionId: "s1" }));
  assert.strictEqual(snapshot.state, VoiceState.idle);
  assert.strictEqual(snapshot.sessions[0].status, "ended");
});

test("failure sets ERROR with the reason and ends the session", () => {
  let snapshot = createVoiceUi();
  snapshot = reduceVoiceUi(snapshot, event(VoiceEventType.started, 1000, { sessionId: "s1" }));
  snapshot = reduceVoiceUi(snapshot, event(VoiceEventType.failed, 1005, { sessionId: "s1", reason: "agent exploded" }));
  assert.strictEqual(snapshot.state, VoiceState.error);
  assert.strictEqual(snapshot.error, "agent exploded");
  assert.strictEqual(snapshot.sessions[0].status, "ended");
  assert.strictEqual(snapshot.sessions[0].state, VoiceState.error);
  assert.strictEqual(snapshot.sessions[0].error, "agent exploded");
});

test("failure without a reason falls back to a default message", () => {
  let snapshot = createVoiceUi();
  snapshot = reduceVoiceUi(snapshot, event(VoiceEventType.started, 1000, { sessionId: "s1" }));
  snapshot = reduceVoiceUi(snapshot, event(VoiceEventType.failed, 1005, { sessionId: "s1" }));
  assert.strictEqual(snapshot.state, VoiceState.error);
  assert.strictEqual(snapshot.error, "voice session failed");
});

test("sessions accumulate most recent first and stay visible after ending", () => {
  let snapshot = createVoiceUi();
  snapshot = reduceVoiceUi(snapshot, event(VoiceEventType.started, 1000, { sessionId: "s1" }));
  snapshot = reduceVoiceUi(snapshot, event(VoiceEventType.started, 2000, { sessionId: "s2" }));
  snapshot = reduceVoiceUi(snapshot, event(VoiceEventType.started, 3000, { sessionId: "s3" }));
  assert.deepStrictEqual(sessionIds(snapshot), ["s3", "s2", "s1"]);
  snapshot = reduceVoiceUi(snapshot, event(VoiceEventType.completed, 3001, { sessionId: "s3" }));
  assert.strictEqual(snapshot.sessions[0].status, "ended");
  assert.strictEqual(snapshot.sessions.length, 3);
});

test("a new session clears a prior error", () => {
  let snapshot = createVoiceUi();
  snapshot = reduceVoiceUi(snapshot, event(VoiceEventType.started, 1000, { sessionId: "s1" }));
  snapshot = reduceVoiceUi(snapshot, event(VoiceEventType.failed, 1005, { sessionId: "s1", reason: "boom" }));
  snapshot = reduceVoiceUi(snapshot, event(VoiceEventType.started, 2000, { sessionId: "s2" }));
  assert.strictEqual(snapshot.state, VoiceState.listening);
  assert.strictEqual(snapshot.error, undefined);
});

test("events for an unknown session still drive the UI state without fabricating a session", () => {
  const snapshot = reduceVoiceUi(createVoiceUi(), event(VoiceEventType.failed, 1005, { sessionId: "ghost", reason: "x" }));
  assert.strictEqual(snapshot.state, VoiceState.error);
  assert.strictEqual(snapshot.error, "x");
  assert.deepStrictEqual(snapshot.sessions, []);
});

test("a session id is required to create a session", () => {
  const snapshot = reduceVoiceUi(createVoiceUi(), event(VoiceEventType.started, 1000, {}));
  assert.deepStrictEqual(snapshot, createVoiceUi());
});

test("the session list is capped at MAX_VISIBLE_SESSIONS", () => {
  let snapshot = createVoiceUi();
  for (let i = 0; i < 25; i += 1) {
    snapshot = reduceVoiceUi(snapshot, event(VoiceEventType.started, 1000 + i, { sessionId: `s${i}` }));
  }
  assert.strictEqual(snapshot.sessions.length, 20);
  assert.deepStrictEqual(sessionIds(snapshot).slice(0, 3), ["s24", "s23", "s22"]);
});

test("unknown event types are ignored", () => {
  const snapshot = createVoiceUi();
  assert.deepStrictEqual(reduceVoiceUi(snapshot, event("TASK_STARTED", 1000, {})), snapshot);
});

test("the store reduces events sequentially and keeps its snapshot current", () => {
  const store = new VoiceUiStore();
  store.reduce(event(VoiceEventType.started, 1000, { sessionId: "s1" }));
  assert.strictEqual(store.current.state, VoiceState.listening);
  store.reduce(event(VoiceEventType.speaking, 1002, { sessionId: "s1" }));
  assert.strictEqual(store.current.state, VoiceState.speaking);
});
