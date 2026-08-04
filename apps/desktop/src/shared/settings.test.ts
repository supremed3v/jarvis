import { test } from "node:test";
import assert from "node:assert";
import {
  defaultSettings,
  mergeSettings,
  validateSettings,
  type Settings,
} from "./settings";

test("defaults mirror the SPEC-0003 config defaults", () => {
  const d = defaultSettings();
  assert.strictEqual(d.model.provider, "ollama");
  assert.strictEqual(d.model.ollamaHost, "127.0.0.1");
  assert.strictEqual(d.model.ollamaPort, 11434);
  assert.strictEqual(d.model.defaultModel, "general");
  assert.strictEqual(d.model.temperature, 0.7);
  assert.strictEqual(d.model.maxTokens, 0);
  assert.strictEqual(d.voice.sttDevice, "cpu");
  assert.strictEqual(d.voice.sampleRate, 16000);
  assert.strictEqual(d.voice.ttsSampleRate, 22050);
  assert.strictEqual(d.permissions.filesystemEnabled, false);
  assert.strictEqual(d.preferences.startVoiceOnLaunch, false);
  assert.strictEqual(d.app.logLevel, "info");
});

test("validateSettings accepts a complete valid settings object", () => {
  const input: Settings = {
    model: {
      provider: "ollama",
      ollamaHost: "127.0.0.1",
      ollamaPort: 11434,
      defaultModel: "coding",
      temperature: 0.2,
      maxTokens: 2048,
    },
    voice: {
      wakeWordModelPath: "models/hey_jarvis.onnx",
      sttModel: "small",
      sttLanguage: "",
      sttDevice: "cuda",
      ttsModel: "models/jarvis-high",
      audioDevice: "default",
      sampleRate: 16000,
      ttsSampleRate: 22050,
    },
    permissions: { filesystemEnabled: true, terminalEnabled: false, browserEnabled: true },
    preferences: { startVoiceOnLaunch: true, uiLanguage: "en" },
    app: { environment: "production", logLevel: "warn" },
  };
  const result = validateSettings(input);
  assert.strictEqual(result.ok, true);
  if (result.ok) {
    assert.deepStrictEqual(result.data, input);
  }
});

test("validateSettings fills missing fields from defaults", () => {
  const result = validateSettings({});
  assert.strictEqual(result.ok, true);
  if (result.ok) {
    assert.deepStrictEqual(result.data, defaultSettings());
  }
});

test("validateSettings rejects non-object input", () => {
  for (const input of [null, 42, "settings", [1, 2, 3]]) {
    const result = validateSettings(input);
    assert.strictEqual(result.ok, false, `expected ${JSON.stringify(input)} to be rejected`);
    if (!result.ok) {
      assert.strictEqual(result.error.code, "INVALID_SETTINGS");
      assert.ok(result.error.message.length > 0);
    }
  }
});

test("validateSettings rejects invalid model values", () => {
  const invalid = [
    { model: { provider: "openai" } },
    { model: { provider: "" } },
    { model: { provider: 42 } },
    { model: { ollamaHost: "" } },
    { model: { ollamaHost: 7 } },
    { model: { ollamaPort: 0 } },
    { model: { ollamaPort: 70000 } },
    { model: { ollamaPort: 11434.5 } },
    { model: { ollamaPort: "11434" } },
    { model: { defaultModel: "" } },
    { model: { temperature: -0.1 } },
    { model: { temperature: 2.1 } },
    { model: { temperature: "0.7" } },
    { model: { maxTokens: -1 } },
    { model: { maxTokens: 100.5 } },
  ];
  for (const patch of invalid) {
    const result = validateSettings({ ...defaultSettings(), ...patch });
    assert.strictEqual(result.ok, false, `expected ${JSON.stringify(patch)} to be rejected`);
  }
});

test("validateSettings rejects invalid voice values", () => {
  const invalid = [
    { voice: { sttModel: "" } },
    { voice: { sttDevice: "gpu" } },
    { voice: { sttDevice: 1 } },
    { voice: { ttsModel: "" } },
    { voice: { audioDevice: "" } },
    { voice: { wakeWordModelPath: "" } },
    { voice: { sampleRate: 0 } },
    { voice: { sampleRate: 8000.5 } },
    { voice: { ttsSampleRate: -1 } },
    { voice: { sttLanguage: 5 } },
  ];
  for (const patch of invalid) {
    const result = validateSettings({ ...defaultSettings(), ...patch });
    assert.strictEqual(result.ok, false, `expected ${JSON.stringify(patch)} to be rejected`);
  }
});

test("validateSettings rejects invalid permissions, preferences, and app values", () => {
  const invalid = [
    { permissions: { filesystemEnabled: "yes" } },
    { permissions: { terminalEnabled: 1 } },
    { permissions: { browserEnabled: null } },
    { preferences: { startVoiceOnLaunch: "true" } },
    { preferences: { uiLanguage: "" } },
    { app: { environment: "" } },
    { app: { logLevel: "verbose" } },
    { app: { logLevel: 5 } },
  ];
  for (const patch of invalid) {
    const result = validateSettings({ ...defaultSettings(), ...patch });
    assert.strictEqual(result.ok, false, `expected ${JSON.stringify(patch)} to be rejected`);
  }
});

test("validateSettings rejects unknown top-level shape but keeps defaults", () => {
  const result = validateSettings({ bogus: true });
  assert.strictEqual(result.ok, true);
  if (result.ok) {
    assert.deepStrictEqual(result.data, defaultSettings());
  }
});

test("mergeSettings overlays patches field-by-field per category", () => {
  const current = defaultSettings();
  const merged = mergeSettings(current, {
    model: { temperature: 0.1, maxTokens: 512 },
    preferences: { startVoiceOnLaunch: true },
  });
  assert.strictEqual(merged.model.temperature, 0.1);
  assert.strictEqual(merged.model.maxTokens, 512);
  assert.strictEqual(merged.model.ollamaPort, current.model.ollamaPort);
  assert.strictEqual(merged.preferences.startVoiceOnLaunch, true);
  assert.strictEqual(merged.preferences.uiLanguage, current.preferences.uiLanguage);
  assert.deepStrictEqual(merged.voice, current.voice);
  assert.deepStrictEqual(merged.app, current.app);
});
