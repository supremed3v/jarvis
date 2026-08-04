// agentsRenderer.ts drives the SPEC-0070 agent management dashboard
// (agents.html). Like settingsRenderer.ts, the sandboxed renderer runs this as
// a plain script with no module system, so the AgentSnapshot shape and the
// jarvis bridge surface are mirrored here and only ever used at compile time;
// the authoritative models live in ../shared/agents.ts and ../shared/ipc.ts.
// The page renders the main-process snapshot from jarvis.agents.list() and
// live-updates from jarvis.agents.onUpdated(), and submits enable/disable
// toggles to jarvis.agents.setEnabled() (SPEC-0070's enable/disable support;
// the main process persists the intent locally and drives the runtime's
// lifecycle best-effort).
//
// Script-scope declarations are prefixed with Agents to avoid colliding with
// the other renderer scripts (renderer.ts, settingsRenderer.ts), which share
// this compilation unit's global scope.

interface AgentIpcError {
  code: string;
  message: string;
}

type AgentIpcResult<T> = { ok: true; data: T } | { ok: false; error: AgentIpcError };

interface AgentUiView {
  id: string;
  name: string;
  description?: string;
  capabilities?: string[];
  permissions?: string[];
  memoryAccess?: string[];
  status: string;
  enabled: boolean;
}

interface AgentSnapshot {
  agents: AgentUiView[];
  loading: boolean;
  error?: string;
}

interface AgentEnabledPatch {
  id: string;
  enabled: boolean;
}

// AgentsBridge is the compile-time mirror of the preload surface this page
// uses; only the agents domain is needed here.
interface AgentsBridge {
  agents: {
    list: () => Promise<AgentIpcResult<AgentSnapshot>>;
    setEnabled: (patch: AgentEnabledPatch) => Promise<AgentIpcResult<AgentEnabledPatch>>;
    onUpdated: (cb: (snapshot: AgentSnapshot) => void) => () => void;
  };
}

function getAgentsBridge(): AgentsBridge | undefined {
  return (window as unknown as { jarvis?: AgentsBridge }).jarvis;
}

function element(tag: string, className?: string, text?: string): HTMLElement {
  const el = document.createElement(tag);
  if (className) {
    el.className = className;
  }
  if (text !== undefined) {
    el.textContent = text;
  }
  return el;
}

function chips(values: string[] | undefined): HTMLElement {
  const wrap = element("div", "chip-list");
  if (!values || values.length === 0) {
    wrap.appendChild(element("span", "none", "None"));
    return wrap;
  }
  for (const value of values) {
    wrap.appendChild(element("span", "chip", value));
  }
  return wrap;
}

function configSection(title: string, values: string[] | undefined): HTMLElement {
  const section = element("div");
  section.appendChild(element("h3", undefined, title));
  section.appendChild(chips(values));
  return section;
}

function renderAgentCard(agent: AgentUiView, onToggle: (agent: AgentUiView, enabled: boolean) => void): HTMLElement {
  const card = element("div", `agent${agent.enabled ? "" : " disabled"}`);

  const head = element("div", "agent-head");
  const identity = element("div");
  identity.appendChild(element("div", "agent-id", agent.name));
  if (agent.description) {
    identity.appendChild(element("div", "agent-desc", agent.description));
  }
  identity.appendChild(element("div", "agent-desc", agent.id));
  head.appendChild(identity);
  head.appendChild(element("span", `agent-status ${agent.status}`, agent.status));
  card.appendChild(head);

  const toggleRow = element("div", "toggle-row");
  const checkbox = document.createElement("input");
  checkbox.type = "checkbox";
  checkbox.id = `toggle-${agent.id}`;
  checkbox.checked = agent.enabled;
  checkbox.addEventListener("change", () => {
    checkbox.disabled = true;
    onToggle(agent, checkbox.checked);
  });
  const label = element("label", undefined, "Enabled");
  label.setAttribute("for", checkbox.id);
  toggleRow.appendChild(checkbox);
  toggleRow.appendChild(label);
  card.appendChild(toggleRow);

  const details = element("details");
  details.appendChild(element("summary", undefined, "View configuration"));
  const config = element("div", "config");
  config.appendChild(configSection("Capabilities", agent.capabilities));
  config.appendChild(configSection("Permissions", agent.permissions));
  config.appendChild(configSection("Memory access", agent.memoryAccess));
  details.appendChild(config);
  card.appendChild(details);

  return card;
}

function renderAgentList(
  root: HTMLElement,
  snapshot: AgentSnapshot,
  onToggle: (agent: AgentUiView, enabled: boolean) => void,
): void {
  root.replaceChildren();
  if (snapshot.agents.length === 0) {
    root.appendChild(
      element("div", "empty", snapshot.loading ? "Loading agents…" : "No agents registered."),
    );
    return;
  }
  for (const agent of snapshot.agents) {
    root.appendChild(renderAgentCard(agent, onToggle));
  }
}

document.addEventListener("DOMContentLoaded", () => {
  const jarvis = getAgentsBridge();
  const listEl = document.getElementById("list");
  const errorEl = document.getElementById("error");
  const subtitleEl = document.getElementById("subtitle");

  if (!jarvis || !listEl || !errorEl || !subtitleEl) {
    if (errorEl) {
      errorEl.textContent = "Agents UI unavailable";
      errorEl.classList.add("visible");
    }
    return;
  }

  const showError = (message: string): void => {
    errorEl.textContent = message;
    errorEl.classList.add("visible");
  };

  const clearError = (): void => {
    errorEl.textContent = "";
    errorEl.classList.remove("visible");
  };

  const render = (snapshot: AgentSnapshot): void => {
    clearError();
    subtitleEl.textContent =
      snapshot.loading ? "Loading…" : `${snapshot.agents.length} agent${snapshot.agents.length === 1 ? "" : "s"} registered`;
    renderAgentList(listEl, snapshot, (agent, enabled) => {
      clearError();
      jarvis.agents
        .setEnabled({ id: agent.id, enabled })
        .then((result) => {
          if (result.ok) {
            return;
          }
          // The main process rejected the toggle (e.g. the local store could
          // not persist); restore the previous snapshot so the checkbox
          // reflects reality.
          showError(`Toggle failed: ${result.error.code} — ${result.error.message}`);
          jarvis.agents.list().then((reload) => {
            if (reload.ok) {
              render(reload.data);
            }
          });
        })
        .catch((error: unknown) => {
          showError(`Toggle failed: ${error instanceof Error ? error.message : String(error)}`);
        });
    });
    if (snapshot.error) {
      showError(snapshot.error);
    }
  };

  jarvis.agents.onUpdated(render);

  jarvis.agents.list().then((result) => {
    if (result.ok) {
      render(result.data);
    } else {
      showError(`Failed to load agents: ${result.error.code} — ${result.error.message}`);
    }
  });
});
