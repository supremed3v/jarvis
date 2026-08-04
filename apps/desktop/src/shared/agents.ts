// agents.ts implements the renderer-facing agent-management view model for
// SPEC-0070's Agent Management Dashboard.
//
// The main process fetches the runtime's registered agents over the SPEC-0065
// bridge (agents.list -> agents.result), merges them with the locally persisted
// enabled/disabled intent (main/agentStore.ts), and pushes the resulting
// snapshot to the renderer over the jarvis:agents:updated IPC channel
// (SPEC-0064); the renderer renders the snapshot as-is. Keeping the reducer
// here rather than in the sandboxed renderer script makes it unit-testable in
// Node.
//
// The status values map onto the SPEC-0021 lifecycle states reported by the
// core runtime (services/core/agent_lifecycle.go).

import type { AgentListResult } from "./runtime";

export const AgentStatus = {
  registered: "registered",
  initializing: "initializing",
  ready: "ready",
  running: "running",
  stopping: "stopping",
  stopped: "stopped",
  failed: "failed",
} as const;

export type AgentStatus = (typeof AgentStatus)[keyof typeof AgentStatus];

const AGENT_STATUS_SET: ReadonlySet<string> = new Set<string>(Object.values(AgentStatus));

export function isAgentStatus(value: string): value is AgentStatus {
  return AGENT_STATUS_SET.has(value);
}

// AgentView is one agent as the dashboard renders it: the runtime-reported
// identity/capabilities/permissions/status (shared/runtime.ts AgentView, with
// status narrowed to the AgentStatus union) plus the local enabled flag that
// persists in the desktop's agent store.
export interface AgentView {
  id: string;
  name: string;
  description?: string;
  capabilities?: string[];
  permissions?: string[];
  memoryAccess?: string[];
  status: AgentStatus;
  enabled: boolean;
}

export interface AgentDashboardState {
  agents: AgentView[];
  loading: boolean;
  error?: string;
}

export function createAgentDashboard(): AgentDashboardState {
  return { agents: [], loading: false };
}

// reduceAgentList merges a runtime agents.result into the current state. The
// runtime is authoritative for which agents are installed and their
// status/capabilities/permissions; the local enabled flag is preserved per
// agent id (defaulting to enabled for agents the store has not seen), so a
// refresh never resets the user's enable/disable choice.
export function reduceAgentList(state: AgentDashboardState, list: AgentListResult): AgentDashboardState {
  const enabledById = new Map(state.agents.map((agent) => [agent.id, agent.enabled]));
  const agents: AgentView[] = list.agents.map((raw) => ({
    id: raw.id,
    name: raw.name,
    description: raw.description,
    capabilities: raw.capabilities,
    permissions: raw.permissions,
    memoryAccess: raw.memoryAccess,
    status: isAgentStatus(raw.status) ? raw.status : AgentStatus.registered,
    enabled: enabledById.get(raw.id) ?? true,
  }));
  return { agents, loading: false, error: undefined };
}

// setAgentEnabled flips one agent's enabled flag, keeping the rest of the
// state unchanged. An id that is not currently listed (list not loaded yet)
// leaves the state unchanged.
export function setAgentEnabled(state: AgentDashboardState, id: string, enabled: boolean): AgentDashboardState {
  return {
    ...state,
    agents: state.agents.map((agent) => (agent.id === id ? { ...agent, enabled } : agent)),
  };
}

export function setAgentLoading(state: AgentDashboardState, loading: boolean): AgentDashboardState {
  return { ...state, loading };
}

export function setAgentError(state: AgentDashboardState, error?: string): AgentDashboardState {
  return { ...state, error };
}

// AgentUiStore owns a single dashboard snapshot, reducing remote agent lists
// and local toggles onto it as they arrive (one instance per main process).
export class AgentUiStore {
  private snapshot: AgentDashboardState;

  constructor() {
    this.snapshot = createAgentDashboard();
  }

  get current(): AgentDashboardState {
    return this.snapshot;
  }

  reduceList(list: AgentListResult): AgentDashboardState {
    this.snapshot = reduceAgentList(this.snapshot, list);
    return this.snapshot;
  }

  setEnabled(id: string, enabled: boolean): AgentDashboardState {
    this.snapshot = setAgentEnabled(this.snapshot, id, enabled);
    return this.snapshot;
  }

  setLoading(loading: boolean): AgentDashboardState {
    this.snapshot = setAgentLoading(this.snapshot, loading);
    return this.snapshot;
  }

  setError(error?: string): AgentDashboardState {
    this.snapshot = setAgentError(this.snapshot, error);
    return this.snapshot;
  }
}
