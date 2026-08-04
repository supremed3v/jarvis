import { test } from "node:test";
import assert from "node:assert";
import {
  MemoryType,
  MemoryUiStore,
  applyMemoryEdit,
  createMemoryViewer,
  isMemoryType,
  reduceMemoryList,
  removeMemory,
  setMemoryError,
  setMemoryLoading,
} from "./memory";
import type { MemoryEntry } from "./memory";

const entry = (id: string, content: string, type = "knowledge"): MemoryEntry => ({
  id,
  type,
  content,
  createdAt: "2026-08-04T20:00:00Z",
  updatedAt: "2026-08-04T20:00:00Z",
});

test("MemoryType values match the SPEC-0034 memory type strings", () => {
  assert.deepStrictEqual(Object.values(MemoryType), ["user_profile", "conversation", "knowledge", "experience"]);
  for (const type of Object.values(MemoryType)) {
    assert.strictEqual(isMemoryType(type), true, `expected ${type} to be a memory type`);
  }
  for (const value of ["telepathy", "", "memory", "constructor"]) {
    assert.strictEqual(isMemoryType(value), false, `expected ${value} to be rejected`);
  }
});

test("reduceMemoryList replaces the whole listing with the runtime snapshot", () => {
  const initial = reduceMemoryList(createMemoryViewer(), { memories: [entry("a", "one")] });
  const next = reduceMemoryList(initial, { memories: [entry("b", "two")] });
  assert.deepStrictEqual(next.memories.map((e) => e.id), ["b"]);
  assert.strictEqual(next.loading, false);
  assert.strictEqual(next.error, undefined);
});

test("reduceMemoryList clears loading and error", () => {
  const state = setMemoryError(setMemoryLoading(createMemoryViewer(), true), "boom");
  const next = reduceMemoryList(state, { memories: [] });
  assert.strictEqual(next.loading, false);
  assert.strictEqual(next.error, undefined);
});

test("applyMemoryEdit replaces only the edited record's content", () => {
  const state = reduceMemoryList(createMemoryViewer(), {
    memories: [entry("a", "one"), entry("b", "two")],
  });
  const next = applyMemoryEdit(state, "a", "ONE");
  assert.strictEqual(next.memories.find((e) => e.id === "a")?.content, "ONE");
  assert.strictEqual(next.memories.find((e) => e.id === "b")?.content, "two");
  assert.strictEqual(next.memories.length, 2);
});

test("applyMemoryEdit leaves the state unchanged for an unknown id", () => {
  const state = reduceMemoryList(createMemoryViewer(), { memories: [entry("a", "one")] });
  const next = applyMemoryEdit(state, "ghost", "x");
  assert.strictEqual(next.memories.length, 1);
  assert.strictEqual(next.memories[0].content, "one");
});

test("removeMemory drops the record", () => {
  const state = reduceMemoryList(createMemoryViewer(), {
    memories: [entry("a", "one"), entry("b", "two")],
  });
  const next = removeMemory(state, "a");
  assert.deepStrictEqual(next.memories.map((e) => e.id), ["b"]);
});

test("setMemoryLoading and setMemoryError update their fields", () => {
  const loading = setMemoryLoading(createMemoryViewer(), true);
  assert.strictEqual(loading.loading, true);
  const errored = setMemoryError(loading, "boom");
  assert.strictEqual(errored.error, "boom");
  const cleared = setMemoryError(errored, undefined);
  assert.strictEqual(cleared.error, undefined);
});

test("MemoryUiStore reduces remote snapshots and local edits onto one state", () => {
  const store = new MemoryUiStore();
  assert.deepStrictEqual(store.current, createMemoryViewer());

  store.reduceList({ memories: [entry("a", "one"), entry("b", "two")] });
  assert.strictEqual(store.current.memories.length, 2);

  store.applyEdit("a", "ONE");
  assert.strictEqual(store.current.memories.find((e) => e.id === "a")?.content, "ONE");

  store.remove("b");
  assert.deepStrictEqual(store.current.memories.map((e) => e.id), ["a"]);

  store.setLoading(true);
  assert.strictEqual(store.current.loading, true);
  store.setError("boom");
  assert.strictEqual(store.current.error, "boom");
});
