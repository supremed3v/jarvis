import { test } from "node:test";
import assert from "node:assert";
import * as fs from "fs";
import * as os from "os";
import * as path from "path";
import { defaultSettings, type Settings } from "../shared/settings";
import { SettingsStore } from "./settingsStore";

function tempFile(): string {
  return path.join(fs.mkdtempSync(path.join(os.tmpdir(), "jarvis-settings-")), "settings.json");
}

function readFile(filePath: string): unknown {
  return JSON.parse(fs.readFileSync(filePath, "utf8"));
}

test("store starts on defaults and loads them when no file exists", () => {
  const store = new SettingsStore(tempFile());
  const loaded = store.load();
  assert.deepStrictEqual(loaded, defaultSettings());
  assert.deepStrictEqual(store.get(), defaultSettings());
});

test("update persists settings that survive a reload", () => {
  const filePath = tempFile();
  const store = new SettingsStore(filePath);
  store.load();

  const patch: Partial<Settings> = {
    model: { ...defaultSettings().model, temperature: 0.1, defaultModel: "coding" },
    preferences: { ...defaultSettings().preferences, startVoiceOnLaunch: true },
  };
  const result = store.update(patch);
  assert.strictEqual(result.ok, true);
  if (result.ok) {
    assert.strictEqual(result.data.model.temperature, 0.1);
    assert.strictEqual(result.data.preferences.startVoiceOnLaunch, true);
  }

  const reloaded = new SettingsStore(filePath);
  reloaded.load();
  assert.strictEqual(reloaded.get().model.temperature, 0.1);
  assert.strictEqual(reloaded.get().model.defaultModel, "coding");
  assert.strictEqual(reloaded.get().preferences.startVoiceOnLaunch, true);
});

test("update rejects invalid values without persisting", () => {
  const filePath = tempFile();
  const store = new SettingsStore(filePath);
  store.load();

  const invalid = store.update({ model: { ...defaultSettings().model, ollamaPort: 99999 } });
  assert.strictEqual(invalid.ok, false);
  if (!invalid.ok) {
    assert.strictEqual(invalid.error.code, "INVALID_SETTINGS");
    assert.match(invalid.error.message, /ollamaPort/);
  }
  assert.deepStrictEqual(store.get(), defaultSettings());
  assert.strictEqual(fs.existsSync(filePath), false);
});

test("update after a rejected patch still accepts a valid one", () => {
  const filePath = tempFile();
  const store = new SettingsStore(filePath);
  store.load();

  assert.strictEqual(store.update({ model: { ...defaultSettings().model, temperature: -1 } }).ok, false);
  const valid = store.update({ model: { ...defaultSettings().model, temperature: 1.5 } });
  assert.strictEqual(valid.ok, true);
  if (valid.ok) {
    assert.strictEqual(valid.data.model.temperature, 1.5);
  }
});

test("update writes a valid JSON file to disk", () => {
  const filePath = tempFile();
  const store = new SettingsStore(filePath);
  store.load();
  const result = store.update({ app: { ...defaultSettings().app, logLevel: "error" } });
  assert.strictEqual(result.ok, true);
  const onDisk = readFile(filePath) as Settings;
  assert.strictEqual(onDisk.app.logLevel, "error");
});

test("load falls back to defaults on a corrupt settings file", () => {
  const filePath = tempFile();
  fs.writeFileSync(filePath, "{ not json", "utf8");
  const store = new SettingsStore(filePath);
  assert.deepStrictEqual(store.load(), defaultSettings());
});

test("load falls back to defaults on a settings file with invalid values", () => {
  const filePath = tempFile();
  fs.writeFileSync(filePath, JSON.stringify({ model: { ollamaPort: 0 } }), "utf8");
  const store = new SettingsStore(filePath);
  assert.deepStrictEqual(store.load(), defaultSettings());
});

test("load merges a valid file over defaults for missing fields", () => {
  const filePath = tempFile();
  fs.writeFileSync(filePath, JSON.stringify({ model: { temperature: 0.3 } }), "utf8");
  const store = new SettingsStore(filePath);
  const loaded = store.load();
  assert.strictEqual(loaded.model.temperature, 0.3);
  assert.strictEqual(loaded.model.ollamaPort, defaultSettings().model.ollamaPort);
});

test("onChange notifies subscribers on a successful update and not on a rejection", () => {
  const store = new SettingsStore(tempFile());
  store.load();
  const seen: Settings[] = [];
  store.onChange((settings) => seen.push(settings));

  store.update({ app: { ...defaultSettings().app, environment: "production" } });
  assert.strictEqual(seen.length, 1);
  assert.strictEqual(seen[0].app.environment, "production");

  store.update({ model: { ...defaultSettings().model, ollamaPort: 99999 } });
  assert.strictEqual(seen.length, 1);
});

test("onChange unsubscribes when the returned handler is called", () => {
  const store = new SettingsStore(tempFile());
  store.load();
  let count = 0;
  const off = store.onChange(() => {
    count += 1;
  });
  off();
  store.update({ app: { ...defaultSettings().app, logLevel: "warn" } });
  assert.strictEqual(count, 0);
});
