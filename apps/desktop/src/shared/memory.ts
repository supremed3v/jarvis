// memory.ts implements the renderer-facing memory-viewer view model for
// SPEC-0071's Memory Viewer UI.
//
// The main process fetches the runtime's memory records over the SPEC-0065
// bridge (memory.list / memory.search -> memory.result), applies edits and
// deletions (memory.update / memory.delete -> memory.result), and pushes the
// resulting snapshot to the renderer over the jarvis:memory:updated IPC channel
// (SPEC-0064); the renderer renders the snapshot as-is. Keeping the reducer
// here rather than in the sandboxed renderer script makes it unit-testable in
// Node.
//
// The type values map onto the SPEC-0034 MemoryType values reported by the
// core runtime (services/core/memory_interface.go).

import type { MemoryListResult } from "./runtime";

export const MemoryType = {
  userProfile: "user_profile",
  conversation: "conversation",
  knowledge: "knowledge",
  experience: "experience",
} as const;

export type MemoryType = (typeof MemoryType)[keyof typeof MemoryType];

const MEMORY_TYPE_SET: ReadonlySet<string> = new Set<string>(Object.values(MemoryType));

export function isMemoryType(value: string): value is MemoryType {
  return MEMORY_TYPE_SET.has(value);
}

// MemoryTypeLabel is the display label the viewer's filter control shows for
// each memory type.
export const MemoryTypeLabel: Record<MemoryType, string> = {
  user_profile: "User memories",
  conversation: "Conversations",
  knowledge: "Knowledge",
  experience: "Experience",
};

// MemoryEntry is one memory record as the viewer renders it: the
// runtime-reported SPEC-0071 MemoryEntry (shared/runtime.ts) plus nothing - the
// viewer's snapshot is the runtime's view, since the runtime is authoritative
// for what memory contains.
export interface MemoryEntry {
  id: string;
  type: string;
  content: string;
  metadata?: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
}

export interface MemoryViewerState {
  memories: MemoryEntry[];
  loading: boolean;
  error?: string;
}

export function createMemoryViewer(): MemoryViewerState {
  return { memories: [], loading: false };
}

// reduceMemoryList merges a runtime memory.result into the current state. The
// runtime is authoritative for which records exist and their content, so a
// refresh replaces the whole listing.
export function reduceMemoryList(state: MemoryViewerState, list: MemoryListResult): MemoryViewerState {
  return { ...state, memories: list.memories, loading: false, error: undefined };
}

// setMemoryLoading tracks in-flight bridge requests so the renderer can show
// progress.
export function setMemoryLoading(state: MemoryViewerState, loading: boolean): MemoryViewerState {
  return { ...state, loading };
}

export function setMemoryError(state: MemoryViewerState, error?: string): MemoryViewerState {
  return { ...state, error };
}

// applyMemoryEdit replaces the content of one record after the runtime
// acknowledged a memory.update, so the renderer reflects the persisted change
// immediately. A record that is not currently listed leaves the state
// unchanged.
export function applyMemoryEdit(state: MemoryViewerState, id: string, content: string): MemoryViewerState {
  return {
    ...state,
    memories: state.memories.map((entry) => (entry.id === id ? { ...entry, content } : entry)),
  };
}

// removeMemory drops one record after the runtime acknowledged a memory.delete.
export function removeMemory(state: MemoryViewerState, id: string): MemoryViewerState {
  return { ...state, memories: state.memories.filter((entry) => entry.id !== id) };
}

// MemoryUiStore owns a single memory-viewer snapshot, reducing remote memory
// lists and local edits onto it as they arrive (one instance per main
// process).
export class MemoryUiStore {
  private snapshot: MemoryViewerState;

  constructor() {
    this.snapshot = createMemoryViewer();
  }

  get current(): MemoryViewerState {
    return this.snapshot;
  }

  reduceList(list: MemoryListResult): MemoryViewerState {
    this.snapshot = reduceMemoryList(this.snapshot, list);
    return this.snapshot;
  }

  setLoading(loading: boolean): MemoryViewerState {
    this.snapshot = setMemoryLoading(this.snapshot, loading);
    return this.snapshot;
  }

  setError(error?: string): MemoryViewerState {
    this.snapshot = setMemoryError(this.snapshot, error);
    return this.snapshot;
  }

  applyEdit(id: string, content: string): MemoryViewerState {
    this.snapshot = applyMemoryEdit(this.snapshot, id, content);
    return this.snapshot;
  }

  remove(id: string): MemoryViewerState {
    this.snapshot = removeMemory(this.snapshot, id);
    return this.snapshot;
  }
}
