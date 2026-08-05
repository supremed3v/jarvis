import type { RuntimeEvent } from "./runtime";

export const LogCategory = {
  system: "system",
  agent: "agent",
  tool: "tool",
  error: "error",
} as const;

export type LogCategory = (typeof LogCategory)[keyof typeof LogCategory];

const LOG_CATEGORY_SET: ReadonlySet<string> = new Set<string>(Object.values(LogCategory));

export function isLogCategory(value: string): value is LogCategory {
  return LOG_CATEGORY_SET.has(value);
}

export const LogCategoryLabel: Record<LogCategory, string> = {
  system: "System",
  agent: "Agent",
  tool: "Tool",
  error: "Error",
};

export interface LogEntry {
  id: string;
  category: LogCategory;
  eventType: string;
  source: string;
  message: string;
  timestamp: number;
  payload?: unknown;
}

export interface LogsViewerState {
  entries: LogEntry[];
  loading: boolean;
  error?: string;
}

const MAX_LOG_ENTRIES = 500;

let nextLogId = 1;

export function categorizeEvent(eventType: string): LogCategory {
  const lower = eventType.toLowerCase();
  if (lower.includes("error") || lower.includes("fail") || lower.includes("panic")) {
    return LogCategory.error;
  }
  if (lower.includes("agent")) {
    return LogCategory.agent;
  }
  if (lower.includes("tool") || lower.includes("approval")) {
    return LogCategory.tool;
  }
  return LogCategory.system;
}

export function eventToLogEntry(event: RuntimeEvent): LogEntry {
  const id = `log-${nextLogId++}`;
  const category = categorizeEvent(event.eventType);
  const message = formatEventMessage(event);
  return {
    id,
    category,
    eventType: event.eventType,
    source: event.source ?? "",
    message,
    timestamp: event.timestamp,
    payload: event.payload,
  };
}

function formatEventMessage(event: RuntimeEvent): string {
  if (typeof event.payload === "string") {
    return event.payload;
  }
  if (typeof event.payload === "object" && event.payload !== null) {
    const record = event.payload as Record<string, unknown>;
    if (typeof record.message === "string") {
      return record.message;
    }
    if (typeof record.error === "string") {
      return record.error;
    }
  }
  return event.eventType;
}

export function createLogsViewer(): LogsViewerState {
  return { entries: [], loading: false };
}

export function addLogEntry(state: LogsViewerState, entry: LogEntry): LogsViewerState {
  const entries = [entry, ...state.entries];
  if (entries.length > MAX_LOG_ENTRIES) {
    entries.length = MAX_LOG_ENTRIES;
  }
  return { ...state, entries };
}

export function setLogsLoading(state: LogsViewerState, loading: boolean): LogsViewerState {
  return { ...state, loading };
}

export function setLogsError(state: LogsViewerState, error?: string): LogsViewerState {
  return { ...state, error };
}

export function clearLogs(state: LogsViewerState): LogsViewerState {
  return { ...state, entries: [] };
}

export class LogUiStore {
  private snapshot: LogsViewerState;

  constructor() {
    this.snapshot = createLogsViewer();
  }

  get current(): LogsViewerState {
    return this.snapshot;
  }

  addEntry(entry: LogEntry): LogsViewerState {
    this.snapshot = addLogEntry(this.snapshot, entry);
    return this.snapshot;
  }

  ingestEvent(event: RuntimeEvent): LogEntry {
    const entry = eventToLogEntry(event);
    this.addEntry(entry);
    return entry;
  }

  setLoading(loading: boolean): LogsViewerState {
    this.snapshot = setLogsLoading(this.snapshot, loading);
    return this.snapshot;
  }

  setError(error?: string): LogsViewerState {
    this.snapshot = setLogsError(this.snapshot, error);
    return this.snapshot;
  }

  clear(): LogsViewerState {
    this.snapshot = clearLogs(this.snapshot);
    return this.snapshot;
  }
}
