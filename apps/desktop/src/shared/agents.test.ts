import { test } from "node:test";
import assert from "node:assert";
import {
  AgentStatus,
  AgentUiStore,
  createAgentDashboard,
  isAgentStatus,
  reduceAgentList,
  setAgentEnabled,
  setAgentError,
  setAgentLoading,
} from "./agents";
import type { AgentListResult, AgentView as WireAgentView } from "./runtime";

function remoteAgents(views: WireAgentView[]): AgentListResult {
  return { agents: views };
}

test("initial dashboard has no agents and is not loading", () => {
  const state = createAgentDashboard();
  assert.deepStrictEqual(state.agents, []);
  assert.strictEqual(state.loading, false);
  assert.strictEqual(state.error, undefined);
});

test("agent status allowlist covers the seven lifecycle states", () => {
  assert.strictEqual(Object.values(AgentStatus).length, 7);
  for (const status of Object.values(AgentStatus)) {
    assert.strictEqual(isAgentStatus(status), true, `expected ${status} to be a known status`);
  }
  for (const status of ["active", "idle", ""]) {
    assert.strictEqual(isAgentStatus(status), false, `expected ${status} to be rejected`);
  }
});

test("reduceAgentList surfaces the remote agents, enabled by default", () => {
  const list = remoteAgents([
    {
      id: "core-agent",
      name: "Core Agent",
      description: "main assistant",
      capabilities: ["shell", "read_file"],
      permissions: ["terminal"],
      memoryAccess: ["conversation"],
      status: "running",
    },
    { id: "research-agent", name: "Research Agent", status: "ready" },
  ]);
  const state = reduceAgentList(createAgentDashboard(), list);
  assert.strictEqual(state.loading, false);
  assert.strictEqual(state.agents.length, 2);
  assert.strictEqual(state.agents[0].id, "core-agent");
  assert.strictEqual(state.agents[0].name, "Core Agent");
  assert.strictEqual(state.agents[0].description, "main assistant");
  assert.deepStrictEqual(state.agents[0].capabilities, ["shell", "read_file"]);
  assert.deepStrictEqual(state.agents[0].permissions, ["terminal"]);
  assert.deepStrictEqual(state.agents[0].memoryAccess, ["conversation"]);
  assert.strictEqual(state.agents[0].status, "running");
  assert.strictEqual(state.agents[0].enabled, true);
  assert.strictEqual(state.agents[1].enabled, true);
});

test("reduceAgentList with no agents clears the dashboard", () => {
  const withAgents = reduceAgentList(createAgentDashboard(), remoteAgents([{ id: "a", name: "A", status: "ready" }]));
  const state = reduceAgentList(withAgents, remoteAgents([]));
  assert.deepStrictEqual(state.agents, []);
  assert.strictEqual(state.loading, false);
});

test("reduceAgentList preserves local enabled flags across refreshes", () => {
  const first = reduceAgentList(createAgentDashboard(), remoteAgents([{ id: "a", name: "A", status: "ready" }]));
  const toggled = setAgentEnabled(first, "a", false);
  assert.strictEqual(toggled.agents[0].enabled, false);

  const refreshed = reduceAgentList(toggled, remoteAgents([{ id: "a", name: "A", status: "running" }]));
  assert.strictEqual(refreshed.agents[0].enabled, false, "a refresh must not reset the user's disable");
  assert.strictEqual(refreshed.agents[0].status, "running", "the refreshed status is applied");
});

test("reduceAgentList defaults unknown statuses to registered", () => {
  const state = reduceAgentList(createAgentDashboard(), remoteAgents([{ id: "a", name: "A", status: "teleporting" }]));
  assert.strictEqual(state.agents[0].status, "registered");
});

test("reduceAgentList drops agents no longer reported by the runtime", () => {
  const first = reduceAgentList(
    createAgentDashboard(),
    remoteAgents([
      { id: "a", name: "A", status: "ready" },
      { id: "b", name: "B", status: "ready" },
    ]),
  );
  const state = reduceAgentList(first, remoteAgents([{ id: "a", name: "A", status: "ready" }]));
  assert.deepStrictEqual(state.agents.map((agent) => agent.id), ["a"]);
});

test("setAgentEnabled flips exactly one agent and leaves the rest alone", () => {
  const state = reduceAgentList(
    createAgentDashboard(),
    remoteAgents([
      { id: "a", name: "A", status: "ready" },
      { id: "b", name: "B", status: "running" },
    ]),
  );
  const toggled = setAgentEnabled(state, "b", false);
  assert.strictEqual(toggled.agents[0].enabled, true);
  assert.strictEqual(toggled.agents[1].enabled, false);
  assert.strictEqual(toggled.agents[1].status, "running");
});

test("setAgentEnabled for an unknown id leaves the state unchanged", () => {
  const state = reduceAgentList(createAgentDashboard(), remoteAgents([{ id: "a", name: "A", status: "ready" }]));
  assert.deepStrictEqual(setAgentEnabled(state, "ghost", false), state);
});

test("setAgentLoading and setAgentError update their fields without touching agents", () => {
  const state = reduceAgentList(createAgentDashboard(), remoteAgents([{ id: "a", name: "A", status: "ready" }]));
  const loading = setAgentLoading(state, true);
  assert.strictEqual(loading.loading, true);
  assert.deepStrictEqual(loading.agents, state.agents);
  const errored = setAgentError(loading, "boom");
  assert.strictEqual(errored.error, "boom");
  assert.deepStrictEqual(errored.agents, state.agents);
});

test("the store reduces lists and toggles sequentially", () => {
  const store = new AgentUiStore();
  store.reduceList(remoteAgents([{ id: "a", name: "A", status: "ready" }]));
  assert.strictEqual(store.current.agents.length, 1);
  store.setEnabled("a", false);
  assert.strictEqual(store.current.agents[0].enabled, false);
  store.reduceList(remoteAgents([{ id: "a", name: "A", status: "running" }]));
  assert.strictEqual(store.current.agents[0].enabled, false);
  assert.strictEqual(store.current.agents[0].status, "running");
});
