import { test } from "node:test";
import assert from "node:assert";
import * as fs from "fs";
import * as os from "os";
import * as path from "path";
import { AgentStore } from "./agentStore";

function tempFile(): string {
  return path.join(fs.mkdtempSync(path.join(os.tmpdir(), "jarvis-agents-")), "agents.json");
}

function readFile(filePath: string): unknown {
  return JSON.parse(fs.readFileSync(filePath, "utf8"));
}

test("untouched agents default to enabled and nothing is written", () => {
  const filePath = tempFile();
  const store = new AgentStore(filePath);
  store.load();
  assert.strictEqual(store.isEnabled("core-agent"), true);
  assert.strictEqual(fs.existsSync(filePath), false);
});

test("set persists a disabled flag that survives a reload", () => {
  const filePath = tempFile();
  const store = new AgentStore(filePath);
  store.load();

  const result = store.set("research-agent", false);
  assert.strictEqual(result.ok, true);
  if (result.ok) {
    assert.deepStrictEqual(result.data, { id: "research-agent", enabled: false });
  }
  assert.strictEqual(store.isEnabled("research-agent"), false);
  assert.strictEqual(store.isEnabled("core-agent"), true);

  const reloaded = new AgentStore(filePath);
  reloaded.load();
  assert.strictEqual(reloaded.isEnabled("research-agent"), false);
  assert.strictEqual(reloaded.isEnabled("core-agent"), true);
});

test("set writes a valid JSON map to disk", () => {
  const filePath = tempFile();
  const store = new AgentStore(filePath);
  store.load();
  store.set("core-agent", false);
  store.set("research-agent", true);
  const onDisk = readFile(filePath) as Record<string, unknown>;
  assert.deepStrictEqual(onDisk, { "core-agent": false, "research-agent": true });
});

test("load ignores a corrupt file", () => {
  const filePath = tempFile();
  fs.writeFileSync(filePath, "{ not json", "utf8");
  const store = new AgentStore(filePath);
  store.load();
  assert.strictEqual(store.isEnabled("core-agent"), true);
});

test("load ignores a file with invalid values and keeps only booleans", () => {
  const filePath = tempFile();
  fs.writeFileSync(filePath, JSON.stringify({ "core-agent": false, "bad-agent": "yes", "other": 7 }), "utf8");
  const store = new AgentStore(filePath);
  store.load();
  assert.strictEqual(store.isEnabled("core-agent"), false);
  assert.strictEqual(store.isEnabled("bad-agent"), true);
  assert.strictEqual(store.isEnabled("other"), true);
});

test("re-enabling an agent overwrites the persisted flag", () => {
  const filePath = tempFile();
  const store = new AgentStore(filePath);
  store.load();
  store.set("core-agent", false);
  store.set("core-agent", true);
  assert.strictEqual(store.isEnabled("core-agent"), true);
  const reloaded = new AgentStore(filePath);
  reloaded.load();
  assert.strictEqual(reloaded.isEnabled("core-agent"), true);
});
