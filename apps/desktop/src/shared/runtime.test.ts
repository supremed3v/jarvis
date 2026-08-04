import { test } from "node:test";
import assert from "node:assert";
import {
  RUNTIME_FRAME_TYPES,
  RUNTIME_URL,
  RuntimeFrameType,
  asAgentControlResult,
  asAgentList,
  asCommandResult,
  asErrorPayload,
  asEvent,
  asStatus,
  asStreamChunk,
  asToolApprovalRequested,
  asVoiceResult,
  frameAgentStart,
  frameAgentStop,
  frameAgentsList,
  frameApprovalResponse,
  frameCancelCommand,
  frameGetStatus,
  framePing,
  frameSubmitCommand,
  frameVoiceStart,
  frameVoiceStop,
  isRuntimeFrameType,
  parseFrame,
} from "./runtime";

test("runtime url defaults to the bridge's loopback endpoint", () => {
  assert.strictEqual(RUNTIME_URL, "ws://127.0.0.1:42321/ws");
});

test("frame type allowlist covers the full protocol", () => {
  for (const type of RUNTIME_FRAME_TYPES) {
    assert.strictEqual(isRuntimeFrameType(type), true, `expected ${type} to be a frame type`);
  }
  for (const type of ["command.run", "shell.exec", "event", "pong", "", "constructor"]) {
    assert.strictEqual(isRuntimeFrameType(type), type === "event" || type === "pong", `unexpected ${type}`);
  }
});

test("frame builders produce the protocol envelopes", () => {
  assert.deepStrictEqual(framePing("p-1"), { type: "ping", id: "p-1" });
  assert.deepStrictEqual(frameGetStatus("s-1"), { type: "get_status", id: "s-1" });
  assert.deepStrictEqual(frameSubmitCommand("c-1", "hello core"), {
    type: "command.submit",
    id: "c-1",
    payload: { text: "hello core" },
  });
  assert.deepStrictEqual(frameCancelCommand("c-1"), {
    type: "command.cancel",
    payload: { id: "c-1" },
  });
  assert.deepStrictEqual(frameApprovalResponse("a-1", true), {
    type: "tool.approval_response",
    payload: { id: "a-1", approved: true },
  });
});

test("voice frame builders produce the protocol envelopes", () => {
  assert.deepStrictEqual(frameVoiceStart("v-1"), { type: "voice.start", id: "v-1" });
  assert.deepStrictEqual(frameVoiceStop("v-2"), { type: "voice.stop", id: "v-2" });
});

test("agent frame builders produce the protocol envelopes", () => {
  assert.deepStrictEqual(frameAgentsList("l-1"), { type: "agents.list", id: "l-1" });
  assert.deepStrictEqual(frameAgentStart("c-1", "core-agent"), {
    type: "agent.start",
    id: "c-1",
    payload: { id: "core-agent" },
  });
  assert.deepStrictEqual(frameAgentStop("c-2", "core-agent"), {
    type: "agent.stop",
    id: "c-2",
    payload: { id: "core-agent" },
  });
});

test("parseFrame decodes a server frame envelope", () => {
  const result = parseFrame(JSON.stringify({ type: "status.changed", payload: { state: "ready", version: "0.1.0" } }));
  assert.strictEqual(result.ok, true);
  if (result.ok) {
    assert.strictEqual(result.frame.type, RuntimeFrameType.statusChanged);
    assert.strictEqual(result.frame.payload?.state, "ready");
  }
});

test("parseFrame rejects malformed JSON and non-object frames", () => {
  for (const raw of ["not json", "42", "[]", '"ping"', "", "null"]) {
    const result = parseFrame(raw);
    assert.strictEqual(result.ok, false, `expected ${JSON.stringify(raw)} to be rejected`);
    if (!result.ok) {
      assert.strictEqual(result.error.code, "INVALID_FRAME");
    }
  }
});

test("parseFrame rejects unknown frame types", () => {
  for (const type of ["shell.exec", "command.run", "jarvis:runtime:ping", ""]) {
    const result = parseFrame(JSON.stringify({ type }));
    assert.strictEqual(result.ok, false, `expected ${type} to be rejected`);
    if (!result.ok) {
      assert.strictEqual(result.error.code, "UNKNOWN_FRAME_TYPE");
    }
  }
});

test("asStatus decodes status payloads and rejects bad shapes", () => {
  assert.deepStrictEqual(asStatus({ state: "ready", version: "0.1.0" }), { state: "ready", version: "0.1.0" });
  assert.deepStrictEqual(asStatus({ state: "error", version: "0.1.0", lastError: "boom" }), {
    state: "error",
    version: "0.1.0",
    lastError: "boom",
  });
  for (const payload of [null, {}, { state: "ready" }, { state: "broken", version: "0.1.0" }, { state: "ready", version: 7 }]) {
    assert.strictEqual(asStatus(payload), null, `expected ${JSON.stringify(payload)} to be rejected`);
  }
});

test("asStreamChunk decodes command.stream payloads", () => {
  assert.deepStrictEqual(asStreamChunk({ id: "c-1", text: "hel", partial: "hel", done: false }), {
    id: "c-1",
    text: "hel",
    partial: "hel",
    done: false,
  });
  assert.deepStrictEqual(asStreamChunk({ id: "c-1", text: "", partial: "hello", done: true }), {
    id: "c-1",
    text: "",
    partial: "hello",
    done: true,
  });
  for (const payload of [null, {}, { id: "c-1", text: "hi" }, { text: "hi", partial: "hi" }]) {
    assert.strictEqual(asStreamChunk(payload), null, `expected ${JSON.stringify(payload)} to be rejected`);
  }
});

test("asCommandResult decodes success, error, and cancelled payloads", () => {
  assert.deepStrictEqual(asCommandResult({ id: "c-1", ok: true, result: { text: "hi" } }), {
    id: "c-1",
    ok: true,
    result: { text: "hi" },
  });
  assert.deepStrictEqual(asCommandResult({ id: "c-1", ok: false, error: { code: "COMMAND_FAILED", message: "nope" } }), {
    id: "c-1",
    ok: false,
    error: { code: "COMMAND_FAILED", message: "nope" },
  });
  assert.deepStrictEqual(asCommandResult({ id: "c-1", ok: false, cancelled: true }), {
    id: "c-1",
    ok: false,
    cancelled: true,
  });
  for (const payload of [null, {}, { id: "c-1" }, { ok: true }]) {
    assert.strictEqual(asCommandResult(payload), null, `expected ${JSON.stringify(payload)} to be rejected`);
  }
});

test("asEvent decodes event payloads", () => {
  assert.deepStrictEqual(asEvent({ eventType: "EVENT_TASK_COMPLETED", timestamp: 1234 }), {
    eventType: "EVENT_TASK_COMPLETED",
    timestamp: 1234,
  });
  assert.deepStrictEqual(asEvent({ eventType: "X", timestamp: 1, source: "core.wsbridge", payload: { taskId: "t" } }), {
    eventType: "X",
    timestamp: 1,
    source: "core.wsbridge",
    payload: { taskId: "t" },
  });
  for (const payload of [null, {}, { eventType: "X" }, { timestamp: 1 }]) {
    assert.strictEqual(asEvent(payload), null, `expected ${JSON.stringify(payload)} to be rejected`);
  }
});

test("asToolApprovalRequested decodes approval payloads", () => {
  assert.deepStrictEqual(asToolApprovalRequested({ id: "a-1", agentId: "core-agent", category: "terminal" }), {
    id: "a-1",
    agentId: "core-agent",
    category: "terminal",
  });
  for (const payload of [null, {}, { id: "a-1" }, { id: "a-1", agentId: "core-agent" }]) {
    assert.strictEqual(asToolApprovalRequested(payload), null, `expected ${JSON.stringify(payload)} to be rejected`);
  }
});

test("asErrorPayload decodes error frames", () => {
  assert.deepStrictEqual(asErrorPayload({ error: { code: "INVALID_COMMAND", message: "bad" } }), {
    code: "INVALID_COMMAND",
    message: "bad",
  });
  for (const payload of [null, {}, { error: {} }, { error: { code: "X" } }]) {
    assert.strictEqual(asErrorPayload(payload), null, `expected ${JSON.stringify(payload)} to be rejected`);
  }
});

test("asVoiceResult decodes voice.result acknowledgements", () => {
  assert.deepStrictEqual(asVoiceResult({ ok: true }), { ok: true });
  assert.deepStrictEqual(
    asVoiceResult({ ok: false, error: { code: "VOICE_DISABLED", message: "no session manager" } }),
    { ok: false, error: { code: "VOICE_DISABLED", message: "no session manager" } },
  );
  // A malformed optional error is dropped, matching asStatus/asCommandResult.
  assert.deepStrictEqual(asVoiceResult({ ok: true, error: { code: "X" } }), { ok: true });
  for (const payload of [null, {}, { ok: "yes" }]) {
    assert.strictEqual(asVoiceResult(payload), null, `expected ${JSON.stringify(payload)} to be rejected`);
  }
});

test("asAgentList decodes agents.result payloads", () => {
  assert.deepStrictEqual(asAgentList({ agents: [] }), { agents: [] });
  assert.deepStrictEqual(
    asAgentList({
      agents: [
        {
          id: "core-agent",
          name: "Core Agent",
          description: "assistant",
          capabilities: ["shell", "read"],
          permissions: ["terminal"],
          memoryAccess: ["conversation"],
          status: "running",
        },
      ],
    }),
    {
      agents: [
        {
          id: "core-agent",
          name: "Core Agent",
          description: "assistant",
          capabilities: ["shell", "read"],
          permissions: ["terminal"],
          memoryAccess: ["conversation"],
          status: "running",
        },
      ],
    },
  );
  // Optional fields may be absent; id/name/status are required.
  assert.deepStrictEqual(asAgentList({ agents: [{ id: "a", name: "A", status: "registered" }] }), {
    agents: [{ id: "a", name: "A", status: "registered" }],
  });
  // Malformed optional arrays are dropped rather than rejected, matching the
  // lenient handling of other optional fields.
  assert.deepStrictEqual(
    asAgentList({ agents: [{ id: "a", name: "A", status: "running", capabilities: ["ok", 3] }] }),
    { agents: [{ id: "a", name: "A", status: "running" }] },
  );
  for (const payload of [
    null,
    {},
    { agents: "nope" },
    { agents: [null] },
    { agents: [{}] },
    { agents: [{ id: "a", name: "A" }] },
    { agents: [{ id: "a", name: "A", status: 7 }] },
  ]) {
    assert.strictEqual(asAgentList(payload), null, `expected ${JSON.stringify(payload)} to be rejected`);
  }
});

test("asAgentControlResult decodes agent.result acknowledgements", () => {
  assert.deepStrictEqual(asAgentControlResult({ id: "core-agent", ok: true }), { id: "core-agent", ok: true });
  assert.deepStrictEqual(asAgentControlResult({ ok: true }), { ok: true });
  assert.deepStrictEqual(
    asAgentControlResult({ id: "core-agent", ok: false, error: { code: "AGENT_LIFECYCLE_DISABLED", message: "no" } }),
    { id: "core-agent", ok: false, error: { code: "AGENT_LIFECYCLE_DISABLED", message: "no" } },
  );
  // A malformed optional error is dropped, matching asVoiceResult.
  assert.deepStrictEqual(asAgentControlResult({ id: "a", ok: true, error: { code: "X" } }), { id: "a", ok: true });
  for (const payload of [null, {}, { id: "a" }, { ok: "yes" }]) {
    assert.strictEqual(asAgentControlResult(payload), null, `expected ${JSON.stringify(payload)} to be rejected`);
  }
});
