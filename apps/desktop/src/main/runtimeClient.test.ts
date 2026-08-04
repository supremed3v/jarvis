import { test } from "node:test";
import assert from "node:assert";
import { RuntimeClient, type RuntimeConnectionState, type RuntimeSocket } from "./runtimeClient";
import type { RuntimeFrame, RuntimeStatus } from "../shared/runtime";

class FakeSocket implements RuntimeSocket {
  readyState = 0;
  readonly sent: string[] = [];
  private readonly listeners = new Map<string, Set<(event: unknown) => void>>();

  addEventListener(type: string, listener: (event: unknown) => void): void {
    let set = this.listeners.get(type);
    if (set === undefined) {
      set = new Set();
      this.listeners.set(type, set);
    }
    set.add(listener);
  }

  removeEventListener(type: string, listener: (event: unknown) => void): void {
    this.listeners.get(type)?.delete(listener);
  }

  send(data: string): void {
    this.sent.push(data);
  }

  close(): void {
    this.readyState = 3;
    this.emit("close", {});
  }

  open(): void {
    this.readyState = 1;
    this.emit("open", {});
  }

  serverPush(frame: RuntimeFrame): void {
    this.emit("message", { data: JSON.stringify(frame) });
  }

  emit(type: string, event: unknown): void {
    for (const listener of this.listeners.get(type) ?? []) {
      listener(event);
    }
  }
}

function makeClient(
  overrides: { reconnectDelayMs?: number; maxReconnectDelayMs?: number } = {},
): {
  client: RuntimeClient;
  socket: () => FakeSocket;
  states: RuntimeConnectionState[];
  statuses: RuntimeStatus[];
  chunks: Array<{ text: string; partial: string; done: boolean }>;
  results: unknown[];
  events: unknown[];
  approvals: unknown[];
  errors: unknown[];
  socketCount: () => number;
} {
  const sockets: FakeSocket[] = [];
  const states: RuntimeConnectionState[] = [];
  const statuses: RuntimeStatus[] = [];
  const chunks: Array<{ text: string; partial: string; done: boolean }> = [];
  const results: unknown[] = [];
  const events: unknown[] = [];
  const approvals: unknown[] = [];
  const errors: unknown[] = [];

  const client = new RuntimeClient({
    url: "ws://test:1/ws",
    reconnectDelayMs: overrides.reconnectDelayMs ?? 5,
    maxReconnectDelayMs: overrides.maxReconnectDelayMs ?? 100,
    createSocket: (url: string) => {
      assert.strictEqual(url, "ws://test:1/ws");
      const socket = new FakeSocket();
      sockets.push(socket);
      return socket;
    },
    handlers: {
      onStatusChanged: (status) => statuses.push(status),
      onEvent: (event) => events.push(event),
      onStreamChunk: (chunk) => chunks.push(chunk),
      onCommandResult: (result) => results.push(result),
      onApprovalRequested: (request) => approvals.push(request),
      onError: (error) => errors.push(error),
      onStateChanged: (state) => states.push(state),
    },
  });

  return {
    client,
    socket: () => sockets[sockets.length - 1],
    socketCount: () => sockets.length,
    states,
    statuses,
    chunks,
    results,
    events,
    approvals,
    errors,
  };
}

function waitFor(predicate: () => boolean, timeoutMs = 500): Promise<void> {
  return new Promise((resolve, reject) => {
    const start = Date.now();
    const check = (): void => {
      if (predicate()) {
        resolve();
        return;
      }
      if (Date.now() - start > timeoutMs) {
        reject(new Error("condition not met within timeout"));
        return;
      }
      setTimeout(check, 2);
    };
    check();
  });
}

test("connect opens a socket and reports connecting then connected", () => {
  const { client, socket, states } = makeClient();
  client.connect();
  assert.strictEqual(client.connectionState, "connecting");
  socket().open();
  assert.strictEqual(client.connectionState, "connected");
  assert.deepStrictEqual(states, ["connecting", "connected"]);
});

test("connected socket forwards status.changed pushes", () => {
  const { client, socket, statuses } = makeClient();
  client.connect();
  socket().open();
  socket().serverPush({ type: "status.changed", payload: { state: "ready", version: "0.1.0" } });
  assert.deepStrictEqual(statuses, [{ state: "ready", version: "0.1.0" }]);
  client.disconnect();
});

test("getStatus sends a get_status frame and resolves on the matching status", async () => {
  const { client, socket } = makeClient();
  client.connect();
  socket().open();

  const promise = client.getStatus();
  assert.strictEqual(socket().sent.length, 1);
  const sent = JSON.parse(socket().sent[0]) as RuntimeFrame;
  assert.strictEqual(sent.type, "get_status");
  assert.strictEqual(typeof sent.id, "string");

  socket().serverPush({ type: "status", id: sent.id, payload: { state: "ready", version: "0.1.0" } });
  const status = await promise;
  assert.deepStrictEqual(status, { state: "ready", version: "0.1.0" });
  client.disconnect();
});

test("getStatus rejects when the client is not connected", async () => {
  const { client, socket } = makeClient();
  client.connect();
  await assert.rejects(client.getStatus(), (error: { code?: string }) => error.code === "NOT_CONNECTED");
  assert.strictEqual(socket().sent.length, 0);
});

test("ping resolves true on the matching pong", async () => {
  const { client, socket } = makeClient();
  client.connect();
  socket().open();

  const promise = client.ping();
  const sent = JSON.parse(socket().sent[0]) as RuntimeFrame;
  socket().serverPush({ type: "pong", id: sent.id, payload: { pong: true } });
  assert.strictEqual(await promise, true);
  client.disconnect();
});

test("submitCommand sends command.submit and accepts while connected", () => {
  const { client, socket } = makeClient();
  client.connect();
  socket().open();

  const accepted = client.submitCommand("  hello core  ");
  assert.strictEqual(accepted.accepted, true);
  const sent = JSON.parse(socket().sent[0]) as RuntimeFrame;
  assert.strictEqual(sent.type, "command.submit");
  assert.strictEqual(sent.id, accepted.id);
  assert.deepStrictEqual(sent.payload, { text: "  hello core  " });
  client.disconnect();
});

test("submitCommand declines while disconnected", () => {
  const { client, socket } = makeClient();
  client.connect();
  const accepted = client.submitCommand("hello");
  assert.strictEqual(accepted.accepted, false);
  assert.strictEqual(socket().sent.length, 0);
});

test("cancelCommand sends command.cancel", () => {
  const { client, socket } = makeClient();
  client.connect();
  socket().open();

  const result = client.cancelCommand("c-1");
  assert.deepStrictEqual(result, { id: "c-1", cancelled: true });
  const sent = JSON.parse(socket().sent[0]) as RuntimeFrame;
  assert.deepStrictEqual(sent, { type: "command.cancel", payload: { id: "c-1" } });
  client.disconnect();
});

test("respondApproval sends tool.approval_response", () => {
  const { client, socket } = makeClient();
  client.connect();
  socket().open();

  client.respondApproval("a-1", false);
  const sent = JSON.parse(socket().sent[0]) as RuntimeFrame;
  assert.deepStrictEqual(sent, { type: "tool.approval_response", payload: { id: "a-1", approved: false } });
  client.disconnect();
});

test("stream frames drive stream and result handlers", () => {
  const { client, socket, chunks, results } = makeClient();
  client.connect();
  socket().open();

  socket().serverPush({ type: "command.stream", id: "c-1", payload: { id: "c-1", text: "hel", partial: "hel", done: false } });
  socket().serverPush({ type: "command.stream", id: "c-1", payload: { id: "c-1", text: "lo", partial: "hello", done: true } });
  socket().serverPush({ type: "command.result", id: "c-1", payload: { id: "c-1", ok: true, result: { text: "hello" } } });

  assert.deepStrictEqual(chunks, [
    { id: "c-1", text: "hel", partial: "hel", done: false },
    { id: "c-1", text: "lo", partial: "hello", done: true },
  ]);
  assert.deepStrictEqual(results, [{ id: "c-1", ok: true, result: { text: "hello" } }]);
  client.disconnect();
});

test("event frames forward to the event handler", () => {
  const { client, socket, events } = makeClient();
  client.connect();
  socket().open();

  socket().serverPush({
    type: "event",
    payload: { eventType: "EVENT_TASK_COMPLETED", timestamp: 1234, source: "core.wsbridge", payload: { taskId: "t" } },
  });
  assert.deepStrictEqual(events, [
    { eventType: "EVENT_TASK_COMPLETED", timestamp: 1234, source: "core.wsbridge", payload: { taskId: "t" } },
  ]);
  client.disconnect();
});

test("approval_requested frames forward to the approval handler", () => {
  const { client, socket, approvals } = makeClient();
  client.connect();
  socket().open();

  socket().serverPush({
    type: "tool.approval_requested",
    payload: { id: "a-1", agentId: "core-agent", category: "terminal" },
  });
  assert.deepStrictEqual(approvals, [{ id: "a-1", agentId: "core-agent", category: "terminal" }]);
  client.disconnect();
});

test("error and malformed frames surface through onError", () => {
  const { client, socket, errors } = makeClient();
  client.connect();
  socket().open();

  socket().serverPush({ type: "error", payload: { error: { code: "INVALID_COMMAND", message: "bad" } } });
  socket().emit("message", { data: "not json" });

  assert.deepStrictEqual(errors, [
    { code: "INVALID_COMMAND", message: "bad" },
    { code: "INVALID_FRAME", message: "frame is not valid JSON" },
  ]);
  client.disconnect();
});

test("closing the socket schedules a reconnect with backoff", async () => {
  const { client, socket, states, socketCount } = makeClient();
  client.connect();
  socket().open();
  assert.strictEqual(socketCount(), 1);

  socket().close();
  assert.strictEqual(client.connectionState, "disconnected");
  assert.deepStrictEqual(states, ["connecting", "connected", "disconnected"]);

  await waitFor(() => socketCount() === 2);
  assert.strictEqual(client.connectionState, "connecting");
  socket().open();
  assert.strictEqual(client.connectionState, "connected");
  assert.deepStrictEqual(states, ["connecting", "connected", "disconnected", "connecting", "connected"]);
  client.disconnect();
});

test("reconnect delay grows with each failed attempt", async () => {
  const { client, socket, socketCount } = makeClient({ reconnectDelayMs: 1, maxReconnectDelayMs: 16 });
  client.connect();
  socket().open();
  socket().close();
  await waitFor(() => socketCount() === 2);
  socket().open();
  socket().close();
  await waitFor(() => socketCount() === 3);
  assert.strictEqual(socketCount(), 3);
  client.disconnect();
});

test("disconnect stops reconnecting and closes the socket", () => {
  const { client, socket } = makeClient();
  client.connect();
  socket().open();

  client.disconnect();
  assert.strictEqual(client.connectionState, "disconnected");
  assert.strictEqual(socket().readyState, 3);
});

test("pending requests reject when the connection closes", async () => {
  const { client, socket } = makeClient();
  client.connect();
  socket().open();

  const promise = client.getStatus();
  socket().close();
  await assert.rejects(promise, (error: { code?: string }) => error.code === "CONNECTION_CLOSED");
  client.disconnect();
});
