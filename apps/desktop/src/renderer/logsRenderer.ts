interface LogsIpcError {
  code: string;
  message: string;
}

type LogsIpcResult<T> = { ok: true; data: T } | { ok: false; error: LogsIpcError };

interface LogUiEntry {
  id: string;
  category: string;
  eventType: string;
  source: string;
  message: string;
  timestamp: number;
  payload?: unknown;
}

interface LogsViewerSnapshot {
  entries: LogUiEntry[];
  loading: boolean;
  error?: string;
}

interface LogsBridge {
  logs: {
    list: () => Promise<LogsIpcResult<LogsViewerSnapshot>>;
    clear: () => Promise<LogsIpcResult<LogsViewerSnapshot>>;
    onUpdated: (cb: (snapshot: LogsViewerSnapshot) => void) => () => void;
  };
}

function getLogsBridge(): LogsBridge | undefined {
  return (window as unknown as { jarvis?: LogsBridge }).jarvis;
}

function logsElement(tag: string, className?: string, text?: string): HTMLElement {
  const el = document.createElement(tag);
  if (className) {
    el.className = className;
  }
  if (text !== undefined) {
    el.textContent = text;
  }
  return el;
}

const LOGS_CATEGORY_LABELS: Record<string, string> = {
  system: "System",
  agent: "Agent",
  tool: "Tool",
  error: "Error",
};

function logsTimestamp(ts: number): string {
  if (!ts || ts <= 0) {
    return "";
  }
  const date = new Date(ts);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  return date.toLocaleTimeString(undefined, { hour12: false, hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function renderLogEntry(entry: LogUiEntry): HTMLElement {
  const card = logsElement("div", "log-entry");

  const head = logsElement("div", "log-head");
  head.appendChild(logsElement("span", `log-category ${entry.category}`, LOGS_CATEGORY_LABELS[entry.category] ?? entry.category));
  head.appendChild(logsElement("span", "log-event-type", entry.eventType));
  if (entry.source) {
    head.appendChild(logsElement("span", "log-source", entry.source));
  }
  const time = logsTimestamp(entry.timestamp);
  if (time) {
    head.appendChild(logsElement("span", "log-time", time));
  }
  card.appendChild(head);

  if (entry.message && entry.message !== entry.eventType) {
    card.appendChild(logsElement("div", "log-message", entry.message));
  }

  if (entry.payload !== undefined && entry.payload !== null) {
    const payloadText = typeof entry.payload === "string"
      ? entry.payload
      : JSON.stringify(entry.payload, null, 2);
    const payloadEl = logsElement("div", "log-payload", payloadText);
    const toggle = document.createElement("button");
    toggle.className = "log-toggle";
    toggle.textContent = "Show payload";
    toggle.addEventListener("click", () => {
      const visible = payloadEl.classList.toggle("visible");
      toggle.textContent = visible ? "Hide payload" : "Show payload";
    });
    card.appendChild(toggle);
    card.appendChild(payloadEl);
  }

  return card;
}

function filterEntries(
  entries: LogUiEntry[],
  category: string,
  search: string,
): LogUiEntry[] {
  let filtered = entries;
  if (category) {
    filtered = filtered.filter((e) => e.category === category);
  }
  if (search) {
    const lower = search.toLowerCase();
    filtered = filtered.filter(
      (e) =>
        e.eventType.toLowerCase().includes(lower) ||
        e.message.toLowerCase().includes(lower) ||
        e.source.toLowerCase().includes(lower),
    );
  }
  return filtered;
}

function renderLogsList(
  root: HTMLElement,
  snapshot: LogsViewerSnapshot,
  category: string,
  search: string,
): void {
  root.replaceChildren();
  const visible = filterEntries(snapshot.entries, category, search);
  if (visible.length === 0) {
    root.appendChild(
      logsElement("div", "empty", snapshot.loading ? "Loading logs…" : "No log entries."),
    );
    return;
  }
  for (const entry of visible) {
    root.appendChild(renderLogEntry(entry));
  }
}

document.addEventListener("DOMContentLoaded", () => {
  const jarvis = getLogsBridge();
  const listEl = document.getElementById("list");
  const errorEl = document.getElementById("error");
  const subtitleEl = document.getElementById("subtitle");
  const searchEl = document.getElementById("search") as HTMLInputElement | null;
  const filterEl = document.getElementById("filter") as HTMLSelectElement | null;
  const clearButtonEl = document.getElementById("clearButton") as HTMLButtonElement | null;

  if (!jarvis || !listEl || !errorEl || !subtitleEl || !searchEl || !filterEl || !clearButtonEl) {
    if (errorEl) {
      errorEl.textContent = "Logs UI unavailable";
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

  let currentSnapshot: LogsViewerSnapshot = { entries: [], loading: false };

  const render = (snapshot: LogsViewerSnapshot): void => {
    currentSnapshot = snapshot;
    clearError();
    const category = filterEl.value;
    const search = searchEl.value.trim();
    const visible = filterEntries(snapshot.entries, category, search);
    subtitleEl.textContent = snapshot.loading
      ? "Loading…"
      : `${visible.length} of ${snapshot.entries.length} entries`;
    renderLogsList(listEl, snapshot, category, search);
    if (snapshot.error) {
      showError(snapshot.error);
    }
  };

  searchEl.addEventListener("input", () => render(currentSnapshot));
  filterEl.addEventListener("change", () => render(currentSnapshot));

  clearButtonEl.addEventListener("click", () => {
    clearError();
    jarvis.logs
      .clear()
      .then((result) => {
        if (result.ok) {
          render(result.data);
        } else {
          showError(`Clear failed: ${result.error.code} — ${result.error.message}`);
        }
      })
      .catch((error: unknown) => {
        showError(`Clear failed: ${error instanceof Error ? error.message : String(error)}`);
      });
  });

  jarvis.logs.onUpdated(render);

  jarvis.logs
    .list()
    .then((result) => {
      if (result.ok) {
        render(result.data);
      } else {
        showError(`Failed to load logs: ${result.error.code} — ${result.error.message}`);
      }
    })
    .catch((error: unknown) => {
      showError(`Failed to load logs: ${error instanceof Error ? error.message : String(error)}`);
    });
});
