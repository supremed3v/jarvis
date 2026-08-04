import { test } from "node:test";
import assert from "node:assert";
import {
  IPC_CHANNELS,
  IpcChannels,
  assertIpcChannel,
  isIpcChannel,
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
