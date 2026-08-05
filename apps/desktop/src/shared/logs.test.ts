import { test } from "node:test";
import assert from "node:assert";
import {
  LogCategory,
  LogCategoryLabel,
  LogUiStore,
  categorizeEvent,
  eventToLogEntry,
  isLogCategory,
  addLogEntry,
  clearLogs,
  createLogsViewer,
} from "./logs";
import type { RuntimeEvent } from "./runtime";

test("isLogCategory accepts valid categories", () => {
  for (const cat of Object.values(LogCategory)) {
    assert.strictEqual(isLogCategory(cat), true, `expected ${cat} to be valid`);
  }
});

test("isLogCategory rejects invalid values", () => {
  assert.strictEqual(isLogCategory("unknown"), false);
  assert.strictEqual(isLogCategory(""), false);
});

test("every category has a label", () => {
  for (const cat of Object.values(LogCategory)) {
    assert.ok(LogCategoryLabel[cat], `missing label for ${cat}`);
  }
});

test("categorizeEvent classifies error events", () => {
  assert.strictEqual(categorizeEvent("TASK_ERROR"), "error");
  assert.strictEqual(categorizeEvent("agent_failure"), "error");
  assert.strictEqual(categorizeEvent("PANIC_RECOVERED"), "error");
});

test("categorizeEvent classifies agent events", () => {
  assert.strictEqual(categorizeEvent("AGENT_STARTED"), "agent");
  assert.strictEqual(categorizeEvent("agent_stopped"), "agent");
});

test("categorizeEvent classifies tool events", () => {
  assert.strictEqual(categorizeEvent("TOOL_EXECUTED"), "tool");
  assert.strictEqual(categorizeEvent("APPROVAL_REQUESTED"), "tool");
});

test("categorizeEvent defaults to system", () => {
  assert.strictEqual(categorizeEvent("RUNTIME_STARTED"), "system");
  assert.strictEqual(categorizeEvent("CONFIG_LOADED"), "system");
});

test("categorizeEvent prefers error over agent", () => {
  assert.strictEqual(categorizeEvent("AGENT_ERROR"), "error");
});

test("eventToLogEntry converts a runtime event", () => {
  const event: RuntimeEvent = {
    eventType: "RUNTIME_STARTED",
    timestamp: 1000,
    source: "core",
    payload: { message: "runtime is up" },
  };
  const entry = eventToLogEntry(event);
  assert.strictEqual(entry.category, "system");
  assert.strictEqual(entry.eventType, "RUNTIME_STARTED");
  assert.strictEqual(entry.source, "core");
  assert.strictEqual(entry.message, "runtime is up");
  assert.strictEqual(entry.timestamp, 1000);
  assert.ok(entry.id.startsWith("log-"));
});

test("eventToLogEntry uses eventType as message when payload has no message", () => {
  const event: RuntimeEvent = {
    eventType: "HEARTBEAT",
    timestamp: 2000,
  };
  const entry = eventToLogEntry(event);
  assert.strictEqual(entry.message, "HEARTBEAT");
  assert.strictEqual(entry.source, "");
});

test("eventToLogEntry uses string payload as message", () => {
  const event: RuntimeEvent = {
    eventType: "LOG",
    timestamp: 3000,
    payload: "plain text log",
  };
  const entry = eventToLogEntry(event);
  assert.strictEqual(entry.message, "plain text log");
});

test("eventToLogEntry uses error field from payload", () => {
  const event: RuntimeEvent = {
    eventType: "TASK_ERROR",
    timestamp: 4000,
    payload: { error: "something broke" },
  };
  const entry = eventToLogEntry(event);
  assert.strictEqual(entry.message, "something broke");
});

test("addLogEntry prepends and caps at 500", () => {
  let state = createLogsViewer();
  for (let i = 0; i < 510; i++) {
    const entry = eventToLogEntry({
      eventType: `EVENT_${i}`,
      timestamp: i,
    });
    state = addLogEntry(state, entry);
  }
  assert.strictEqual(state.entries.length, 500);
  assert.strictEqual(state.entries[0].eventType, "EVENT_509");
});

test("clearLogs empties the entries", () => {
  let state = createLogsViewer();
  state = addLogEntry(state, eventToLogEntry({ eventType: "A", timestamp: 1 }));
  assert.strictEqual(state.entries.length, 1);
  state = clearLogs(state);
  assert.strictEqual(state.entries.length, 0);
});

test("LogUiStore accumulates events and clears", () => {
  const store = new LogUiStore();
  assert.strictEqual(store.current.entries.length, 0);

  const entry = store.ingestEvent({ eventType: "AGENT_STARTED", timestamp: 100, source: "agent-1" });
  assert.strictEqual(entry.category, "agent");
  assert.strictEqual(store.current.entries.length, 1);
  assert.strictEqual(store.current.entries[0].id, entry.id);

  store.ingestEvent({ eventType: "TOOL_EXECUTED", timestamp: 200 });
  assert.strictEqual(store.current.entries.length, 2);
  assert.strictEqual(store.current.entries[0].eventType, "TOOL_EXECUTED");

  store.clear();
  assert.strictEqual(store.current.entries.length, 0);
});

test("LogUiStore tracks loading and error state", () => {
  const store = new LogUiStore();
  assert.strictEqual(store.current.loading, false);
  assert.strictEqual(store.current.error, undefined);

  store.setLoading(true);
  assert.strictEqual(store.current.loading, true);

  store.setError("connection lost");
  assert.strictEqual(store.current.error, "connection lost");

  store.setLoading(false);
  assert.strictEqual(store.current.loading, false);
});
