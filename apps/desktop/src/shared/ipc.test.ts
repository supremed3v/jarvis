import { test } from "node:test";
import assert from "node:assert";
import {
  IPC_CHANNELS,
  IpcChannels,
  assertIpcChannel,
  isIpcChannel,
  validateAgentEnabledPatch,
  validateMemoryDelete,
  validateMemoryList,
  validateMemorySearch,
  validateMemoryUpdate,
  validateToolApprovalResponse,
  validateUserCommand,
} from "./ipc";

test("allowlist accepts every registered channel", () => {
  for (const channel of IPC_CHANNELS) {
    assert.strictEqual(isIpcChannel(channel), true, `expected ${channel} to be allowed`);
    assert.strictEqual(assertIpcChannel(channel), channel);
  }
});

test("allowlist contains every domain group", () => {
  const groups = Object.values(IpcChannels).map((group) => Object.values(group));
  const flat = groups.flat();
  assert.strictEqual(flat.length, IPC_CHANNELS.length);
  for (const channel of flat) {
    assert.ok(IPC_CHANNELS.includes(channel));
  }
});

test("allowlist rejects unauthorized channels", () => {
  const unauthorized = [
    "runtime:ping",
    "jarvis:shell:exec",
    "jarvis:command:run",
    "jarvis:tool:exec",
    "constructor",
    "__proto__",
    "jarvis:",
    "",
  ];
  for (const channel of unauthorized) {
    assert.strictEqual(isIpcChannel(channel), false, `expected ${channel} to be blocked`);
    assert.throws(() => assertIpcChannel(channel), { name: "Error" }, `expected ${channel} to throw`);
  }
});

test("validateUserCommand accepts non-empty text and trims it", () => {
  const result = validateUserCommand({ text: "  hello world  " });
  assert.strictEqual(result.ok, true);
  if (result.ok) {
    assert.strictEqual(result.data.text, "hello world");
  }
});

test("validateUserCommand rejects empty, missing, or non-string text", () => {
  for (const payload of [null, 42, "hi", {}, { text: "" }, { text: "   " }, { text: 7 }]) {
    const result = validateUserCommand(payload);
    assert.strictEqual(result.ok, false, `expected ${JSON.stringify(payload)} to be rejected`);
    if (!result.ok) {
      assert.strictEqual(typeof result.error.code, "string");
      assert.ok(result.error.message.length > 0);
    }
  }
});

test("validateToolApprovalResponse accepts valid responses", () => {
  for (const response of [
    { id: "a", approved: true },
    { id: "b", approved: false, reason: "unsafe" },
  ]) {
    const result = validateToolApprovalResponse(response);
    assert.strictEqual(result.ok, true);
  }
});

test("validateToolApprovalResponse rejects missing id or non-boolean approved", () => {
  for (const payload of [
    null,
    42,
    {},
    { id: "", approved: true },
    { approved: true },
    { id: "a", approved: "yes" },
    { id: "a" },
  ]) {
    const result = validateToolApprovalResponse(payload);
    assert.strictEqual(result.ok, false, `expected ${JSON.stringify(payload)} to be rejected`);
  }
});

test("validateAgentEnabledPatch accepts valid toggles", () => {
  for (const payload of [
    { id: "core-agent", enabled: true },
    { id: "core-agent", enabled: false },
  ]) {
    const result = validateAgentEnabledPatch(payload);
    assert.strictEqual(result.ok, true, `expected ${JSON.stringify(payload)} to be accepted`);
    if (result.ok) {
      assert.deepStrictEqual(result.data, payload);
    }
  }
});

test("validateAgentEnabledPatch rejects missing id or non-boolean enabled", () => {
  for (const payload of [
    null,
    42,
    {},
    { id: "", enabled: true },
    { id: "a" },
    { enabled: true },
    { id: "a", enabled: "yes" },
    { id: 7, enabled: true },
  ]) {
    const result = validateAgentEnabledPatch(payload);
    assert.strictEqual(result.ok, false, `expected ${JSON.stringify(payload)} to be rejected`);
  }
});

test("validateMemoryList accepts no payload, an empty object, or a type scope", () => {
  assert.strictEqual(validateMemoryList(undefined).ok, false, "missing payload is not an object");
  for (const payload of [{}, { type: "knowledge" }]) {
    const result = validateMemoryList(payload);
    assert.strictEqual(result.ok, true, `expected ${JSON.stringify(payload)} to be accepted`);
    if (result.ok) {
      assert.deepStrictEqual(result.data, payload);
    }
  }
});

test("validateMemoryList rejects a non-string type", () => {
  for (const payload of [null, 42, { type: 7 }, { type: ["knowledge"] }]) {
    const result = validateMemoryList(payload);
    assert.strictEqual(result.ok, false, `expected ${JSON.stringify(payload)} to be rejected`);
  }
});

test("validateMemorySearch accepts a trimmed query with optional type and limit", () => {
  const result = validateMemorySearch({ query: "  architecture  ", type: "knowledge", limit: 5 });
  assert.strictEqual(result.ok, true);
  if (result.ok) {
    assert.deepStrictEqual(result.data, { query: "architecture", type: "knowledge", limit: 5 });
  }
  const minimal = validateMemorySearch({ query: "go" });
  assert.strictEqual(minimal.ok, true);
  if (minimal.ok) {
    assert.deepStrictEqual(minimal.data, { query: "go" });
  }
});

test("validateMemorySearch rejects empty query, bad type, or bad limit", () => {
  for (const payload of [
    null,
    42,
    {},
    { query: "" },
    { query: "   " },
    { query: 7 },
    { query: "go", type: 7 },
    { query: "go", limit: 0 },
    { query: "go", limit: -1 },
    { query: "go", limit: 2.5 },
    { query: "go", limit: "5" },
  ]) {
    const result = validateMemorySearch(payload);
    assert.strictEqual(result.ok, false, `expected ${JSON.stringify(payload)} to be rejected`);
  }
});

test("validateMemoryUpdate accepts id and trimmed content", () => {
  const result = validateMemoryUpdate({ id: "local::1", content: "  new fact  " });
  assert.strictEqual(result.ok, true);
  if (result.ok) {
    assert.deepStrictEqual(result.data, { id: "local::1", content: "new fact" });
  }
});

test("validateMemoryUpdate rejects missing id or empty content", () => {
  for (const payload of [
    null,
    42,
    {},
    { id: "", content: "x" },
    { content: "x" },
    { id: "local::1" },
    { id: "local::1", content: "" },
    { id: "local::1", content: "   " },
    { id: 7, content: "x" },
  ]) {
    const result = validateMemoryUpdate(payload);
    assert.strictEqual(result.ok, false, `expected ${JSON.stringify(payload)} to be rejected`);
  }
});

test("validateMemoryDelete accepts a non-empty id", () => {
  const result = validateMemoryDelete({ id: "local::1" });
  assert.strictEqual(result.ok, true);
});

test("validateMemoryDelete rejects missing or non-string id", () => {
  for (const payload of [null, 42, {}, { id: "" }, { id: 7 }]) {
    const result = validateMemoryDelete(payload);
    assert.strictEqual(result.ok, false, `expected ${JSON.stringify(payload)} to be rejected`);
  }
});
