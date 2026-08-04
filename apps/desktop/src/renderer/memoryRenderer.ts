// memoryRenderer.ts drives the SPEC-0071 Memory Viewer (memory.html). Like
// agentsRenderer.ts, the sandboxed renderer runs this as a plain script with
// no module system, so the snapshot shape and the jarvis bridge surface are
// mirrored here and only ever used at compile time; the authoritative models
// live in ../shared/memory.ts and ../shared/ipc.ts. The page lists, searches,
// edits, and deletes memory records through the jarvis:memory:* channels and
// live-updates from jarvis.memory.onUpdated() (SPEC-0071's display, search,
// filtering, and editing/deletion support; the main process drives the
// runtime's memory.* bridge frames, which are authoritative for what memory
// contains).
//
// Script-scope declarations are prefixed with Memory to avoid colliding with
// the other renderer scripts (renderer.ts, settingsRenderer.ts,
// agentsRenderer.ts), which share this compilation unit's global scope.

interface MemoryIpcError {
  code: string;
  message: string;
}

type MemoryIpcResult<T> = { ok: true; data: T } | { ok: false; error: MemoryIpcError };

interface MemoryUiEntry {
  id: string;
  type: string;
  content: string;
  metadata?: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
}

interface MemoryViewerSnapshot {
  memories: MemoryUiEntry[];
  loading: boolean;
  error?: string;
}

interface MemorySearchRequest {
  query: string;
  type?: string;
  limit?: number;
}

interface MemoryUpdateRequest {
  id: string;
  content: string;
}

interface MemoryDeleteRequest {
  id: string;
}

// MemoryBridge is the compile-time mirror of the preload surface this page
// uses; only the memory domain is needed here.
interface MemoryBridge {
  memory: {
    list: (request?: { type?: string }) => Promise<MemoryIpcResult<MemoryViewerSnapshot>>;
    search: (request: MemorySearchRequest) => Promise<MemoryIpcResult<MemoryViewerSnapshot>>;
    update: (request: MemoryUpdateRequest) => Promise<MemoryIpcResult<MemoryUpdateRequest>>;
    delete: (request: MemoryDeleteRequest) => Promise<MemoryIpcResult<MemoryDeleteRequest>>;
    onUpdated: (cb: (snapshot: MemoryViewerSnapshot) => void) => () => void;
  };
}

function getMemoryBridge(): MemoryBridge | undefined {
  return (window as unknown as { jarvis?: MemoryBridge }).jarvis;
}

function memoryElement(tag: string, className?: string, text?: string): HTMLElement {
  const el = document.createElement(tag);
  if (className) {
    el.className = className;
  }
  if (text !== undefined) {
    el.textContent = text;
  }
  return el;
}

const MEMORY_TYPE_LABELS: Record<string, string> = {
  user_profile: "User memory",
  conversation: "Conversation",
  knowledge: "Knowledge",
  experience: "Experience",
};

// formatTimestamp renders a Go RFC3339 timestamp for display. Zero Go times
// (0001-01-01T00:00:00Z) render as a dash, since some record kinds carry no
// meaningful update time.
function memoryTimestamp(iso: string): string {
  if (!iso) {
    return "—";
  }
  const date = new Date(iso);
  if (Number.isNaN(date.getTime()) || date.getFullYear() <= 1) {
    return "—";
  }
  return date.toLocaleString();
}

function memoryMetaLine(entry: MemoryUiEntry): HTMLElement {
  const meta = memoryElement("div", "memory-meta");
  meta.appendChild(memoryElement("span", undefined, `Created ${memoryTimestamp(entry.createdAt)}`));
  meta.appendChild(memoryElement("span", undefined, `Updated ${memoryTimestamp(entry.updatedAt)}`));
  if (entry.metadata) {
    if (typeof entry.metadata.conversationId === "string") {
      meta.appendChild(memoryElement("span", undefined, `Conversation ${entry.metadata.conversationId}`));
    }
    if (typeof entry.metadata.role === "string") {
      meta.appendChild(memoryElement("span", undefined, `Role ${entry.metadata.role}`));
    }
  }
  return meta;
}

function memoryActionButton(label: string, className: string, onClick: () => void): HTMLButtonElement {
  const button = document.createElement("button");
  button.textContent = label;
  button.className = className;
  button.addEventListener("click", onClick);
  return button;
}

// renderMemoryEntry builds one record card. editContent is non-null while the
// record is in edit mode, so the card renders a textarea + Save/Cancel instead
// of the read-only content and actions.
function renderMemoryEntry(
  entry: MemoryUiEntry,
  editContent: string | null,
  onEdit: () => void,
  onSave: (content: string) => void,
  onCancelEdit: () => void,
  onDelete: () => void,
): HTMLElement {
  const card = memoryElement("div", "memory");

  const head = memoryElement("div", "memory-head");
  head.appendChild(memoryElement("span", `memory-type ${entry.type}`, MEMORY_TYPE_LABELS[entry.type] ?? entry.type));
  head.appendChild(memoryElement("span", "memory-id", entry.id));
  card.appendChild(head);

  if (editContent !== null) {
    const edit = memoryElement("div", "memory-edit");
    const textarea = document.createElement("textarea");
    textarea.value = editContent;
    const row = memoryElement("div", "memory-edit-row");
    row.appendChild(memoryActionButton("Save", "primary", () => onSave(textarea.value)));
    row.appendChild(memoryActionButton("Cancel", "", onCancelEdit));
    edit.appendChild(textarea);
    edit.appendChild(row);
    card.appendChild(edit);
    return card;
  }

  card.appendChild(memoryElement("div", "memory-content", entry.content));
  card.appendChild(memoryMetaLine(entry));

  const actions = memoryElement("div", "memory-actions");
  actions.appendChild(memoryActionButton("Edit", "", onEdit));
  actions.appendChild(memoryActionButton("Delete", "danger", onDelete));
  card.appendChild(actions);

  return card;
}

function renderMemoryList(
  root: HTMLElement,
  snapshot: MemoryViewerSnapshot,
  editingId: string | null,
  editValue: string,
  onEdit: (entry: MemoryUiEntry) => void,
  onSave: (entry: MemoryUiEntry, content: string) => void,
  onCancelEdit: () => void,
  onDelete: (entry: MemoryUiEntry) => void,
): void {
  root.replaceChildren();
  if (snapshot.memories.length === 0) {
    root.appendChild(
      memoryElement("div", "empty", snapshot.loading ? "Loading memory…" : "No memory records found."),
    );
    return;
  }
  for (const entry of snapshot.memories) {
    const isEditing = editingId === entry.id;
    root.appendChild(
      renderMemoryEntry(
        entry,
        isEditing ? editValue : null,
        () => onEdit(entry),
        (content: string) => onSave(entry, content),
        onCancelEdit,
        () => onDelete(entry),
      ),
    );
  }
}

document.addEventListener("DOMContentLoaded", () => {
  const jarvis = getMemoryBridge();
  const listEl = document.getElementById("list");
  const errorEl = document.getElementById("error");
  const subtitleEl = document.getElementById("subtitle");
  const searchEl = document.getElementById("search") as HTMLInputElement | null;
  const filterEl = document.getElementById("filter") as HTMLSelectElement | null;
  const searchButtonEl = document.getElementById("searchButton") as HTMLButtonElement | null;

  if (!jarvis || !listEl || !errorEl || !subtitleEl || !searchEl || !filterEl || !searchButtonEl) {
    if (errorEl) {
      errorEl.textContent = "Memory UI unavailable";
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

  let editingId: string | null = null;
  let editValue = "";

  const render = (snapshot: MemoryViewerSnapshot): void => {
    clearError();
    subtitleEl.textContent = snapshot.loading
      ? "Loading…"
      : `${snapshot.memories.length} record${snapshot.memories.length === 1 ? "" : "s"}`;
    renderMemoryList(
      listEl,
      snapshot,
      editingId,
      editValue,
      (entry) => {
        editingId = entry.id;
        editValue = entry.content;
        render(snapshot);
      },
      (entry, content) => {
        clearError();
        jarvis.memory
          .update({ id: entry.id, content })
          .then((result) => {
            if (result.ok) {
              editingId = null;
              editValue = "";
              return;
            }
            // The runtime rejected the edit; keep edit mode so the user's text
            // is not lost. The fresh snapshot (broadcast on jarvis:memory:updated)
            // re-renders on success.
            showError(`Edit failed: ${result.error.code} — ${result.error.message}`);
          })
          .catch((error: unknown) => {
            showError(`Edit failed: ${error instanceof Error ? error.message : String(error)}`);
          });
      },
      () => {
        editingId = null;
        editValue = "";
        render(snapshot);
      },
      (entry) => {
        clearError();
        if (!window.confirm(`Delete this memory record?\n\n${entry.content.slice(0, 120)}`)) {
          return;
        }
        jarvis.memory
          .delete({ id: entry.id })
          .then((result) => {
            if (result.ok) {
              return;
            }
            showError(`Delete failed: ${result.error.code} — ${result.error.message}`);
          })
          .catch((error: unknown) => {
            showError(`Delete failed: ${error instanceof Error ? error.message : String(error)}`);
          });
      },
    );
    if (snapshot.error) {
      showError(snapshot.error);
    }
  };

  // runQuery issues a search when the box has text, else a plain list scoped
  // to the current filter.
  const runQuery = (): void => {
    const query = searchEl.value.trim();
    const type = filterEl.value || undefined;
    clearError();
    searchButtonEl.disabled = true;
    const request = query === "" ? jarvis.memory.list({ type }) : jarvis.memory.search({ query, type });
    request
      .then((result) => {
        if (result.ok) {
          render(result.data);
        } else {
          showError(`Failed to load memory: ${result.error.code} — ${result.error.message}`);
        }
      })
      .catch((error: unknown) => {
        showError(`Failed to load memory: ${error instanceof Error ? error.message : String(error)}`);
      })
      .finally(() => {
        searchButtonEl.disabled = false;
      });
  };

  searchButtonEl.addEventListener("click", runQuery);
  searchEl.addEventListener("keydown", (event) => {
    if (event.key === "Enter") {
      runQuery();
    }
  });
  filterEl.addEventListener("change", runQuery);

  jarvis.memory.onUpdated(render);

  searchButtonEl.disabled = true;
  jarvis.memory
    .list()
    .then((result) => {
      if (result.ok) {
        render(result.data);
      } else {
        showError(`Failed to load memory: ${result.error.code} — ${result.error.message}`);
      }
    })
    .catch((error: unknown) => {
      showError(`Failed to load memory: ${error instanceof Error ? error.message : String(error)}`);
    })
    .finally(() => {
      searchButtonEl.disabled = false;
    });
});
