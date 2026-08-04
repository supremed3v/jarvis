// settings.ts implements the renderer-facing settings model for SPEC-0069:
// the user configuration the desktop manages — model settings, voice
// settings, permissions, user preferences, and application behavior.
//
// The five categories mirror packages/config's Config shape (SPEC-0003), so a
// later spec can bridge desktop settings onto the Go runtime's config; this
// spec keeps settings desktop-local, per the load decision: a validated JSON
// file in the Electron userData directory owned by the main process
// (../main/settingsStore.ts) and exposed to the sandboxed renderer over the
// jarvis:settings:* IPC channels (SPEC-0064). Keeping the model, defaults,
// and validation here — pure TS with no electron/fs imports — makes it
// unit-testable in Node.
//
// Validation returns the same IpcResult envelope the SPEC-0064 validators use
// so the main-process store and IPC handlers can surface rejections uniformly;
// the dependency direction is settings -> ipc (values) while ipc -> settings
// is type-only, so there is no runtime import cycle.

import { fail, ok, type IpcResult } from "./ipc";

export interface ModelSettings {
  provider: string;
  ollamaHost: string;
  ollamaPort: number;
  defaultModel: string;
  temperature: number;
  maxTokens: number;
}

export interface VoiceSettings {
  wakeWordModelPath: string;
  sttModel: string;
  sttLanguage: string;
  sttDevice: string;
  ttsModel: string;
  audioDevice: string;
  sampleRate: number;
  ttsSampleRate: number;
}

export interface ToolPermissions {
  filesystemEnabled: boolean;
  terminalEnabled: boolean;
  browserEnabled: boolean;
}

export interface UserPreferences {
  startVoiceOnLaunch: boolean;
  uiLanguage: string;
}

export interface AppBehavior {
  environment: string;
  logLevel: string;
}

export interface Settings {
  model: ModelSettings;
  voice: VoiceSettings;
  permissions: ToolPermissions;
  preferences: UserPreferences;
  app: AppBehavior;
}

// SettingsPatch is a partial Settings whose categories, when present, are
// merged field-by-field over the current settings (see mergeSettings), so a
// caller can save a single field or category without supplying the rest.
export type SettingsPatch = {
  model?: Partial<ModelSettings>;
  voice?: Partial<VoiceSettings>;
  permissions?: Partial<ToolPermissions>;
  preferences?: Partial<UserPreferences>;
  app?: Partial<AppBehavior>;
};

// defaultSettings mirrors packages/config's Defaults() so the desktop and the
// Go runtime agree on the values a fresh install gets.
export function defaultSettings(): Settings {
  return {
    model: {
      provider: "ollama",
      ollamaHost: "127.0.0.1",
      ollamaPort: 11434,
      defaultModel: "general",
      temperature: 0.7,
      maxTokens: 0,
    },
    voice: {
      wakeWordModelPath: "models/hey_jarvis.onnx",
      sttModel: "base.en",
      sttLanguage: "en",
      sttDevice: "cpu",
      ttsModel: "models/jarvis-high",
      audioDevice: "default",
      sampleRate: 16000,
      ttsSampleRate: 22050,
    },
    permissions: {
      filesystemEnabled: false,
      terminalEnabled: false,
      browserEnabled: false,
    },
    preferences: {
      startVoiceOnLaunch: false,
      uiLanguage: "en",
    },
    app: {
      environment: "development",
      logLevel: "info",
    },
  };
}

// mergeSettings overlays a patch over current settings, one field at a time
// per category, so a caller saving a single category never silently resets the
// others.
export function mergeSettings(current: Settings, patch: SettingsPatch): Settings {
  return {
    model: { ...current.model, ...(patch.model ?? {}) },
    voice: { ...current.voice, ...(patch.voice ?? {}) },
    permissions: { ...current.permissions, ...(patch.permissions ?? {}) },
    preferences: { ...current.preferences, ...(patch.preferences ?? {}) },
    app: { ...current.app, ...(patch.app ?? {}) },
  };
}

const LOG_LEVELS: readonly string[] = ["debug", "info", "warn", "error"];
const STT_DEVICES: readonly string[] = ["cpu", "cuda"];

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function readField(record: Record<string, unknown>, key: string): { present: boolean; value: unknown } {
  return { present: Object.prototype.hasOwnProperty.call(record, key), value: record[key] };
}

// validateSettings normalizes unknown input into a Settings: fields that are
// present are validated strictly (an invalid value rejects the whole input),
// fields that are absent fall back to their default. The returned Settings is
// always fully populated, so callers can trust it after a ok result.
export function validateSettings(input: unknown): IpcResult<Settings> {
  if (!isRecord(input)) {
    return fail("INVALID_SETTINGS", "Settings must be an object");
  }

  const d = defaultSettings();
  const model = isRecord(input.model) ? input.model : {};
  const voice = isRecord(input.voice) ? input.voice : {};
  const permissions = isRecord(input.permissions) ? input.permissions : {};
  const preferences = isRecord(input.preferences) ? input.preferences : {};
  const app = isRecord(input.app) ? input.app : {};

  const reject = (message: string): IpcResult<Settings> => fail("INVALID_SETTINGS", message);

  const providerField = readField(model, "provider");
  const provider = providerField.present ? providerField.value : d.model.provider;
  if (typeof provider !== "string" || provider.trim().length === 0) {
    return reject("model.provider must be a non-empty string");
  }
  if (provider !== "ollama") {
    return reject(`model.provider must be "ollama" (the only active provider per ADR-0004), got ${JSON.stringify(provider)}`);
  }

  const ollamaHostField = readField(model, "ollamaHost");
  const ollamaHost = ollamaHostField.present ? ollamaHostField.value : d.model.ollamaHost;
  if (typeof ollamaHost !== "string" || ollamaHost.trim().length === 0) {
    return reject("model.ollamaHost must be a non-empty string");
  }

  const ollamaPortField = readField(model, "ollamaPort");
  const ollamaPort = ollamaPortField.present ? ollamaPortField.value : d.model.ollamaPort;
  if (typeof ollamaPort !== "number" || !Number.isInteger(ollamaPort) || ollamaPort < 1 || ollamaPort > 65535) {
    return reject(`model.ollamaPort must be an integer between 1 and 65535, got ${String(ollamaPort)}`);
  }

  const defaultModelField = readField(model, "defaultModel");
  const defaultModel = defaultModelField.present ? defaultModelField.value : d.model.defaultModel;
  if (typeof defaultModel !== "string" || defaultModel.trim().length === 0) {
    return reject("model.defaultModel must be a non-empty string");
  }

  const temperatureField = readField(model, "temperature");
  const temperature = temperatureField.present ? temperatureField.value : d.model.temperature;
  if (typeof temperature !== "number" || Number.isNaN(temperature) || temperature < 0 || temperature > 2) {
    return reject(`model.temperature must be a number between 0 and 2, got ${String(temperature)}`);
  }

  const maxTokensField = readField(model, "maxTokens");
  const maxTokens = maxTokensField.present ? maxTokensField.value : d.model.maxTokens;
  if (typeof maxTokens !== "number" || !Number.isInteger(maxTokens) || maxTokens < 0) {
    return reject(`model.maxTokens must be a non-negative integer, got ${String(maxTokens)}`);
  }

  const wakeWordModelPathField = readField(voice, "wakeWordModelPath");
  const wakeWordModelPath = wakeWordModelPathField.present ? wakeWordModelPathField.value : d.voice.wakeWordModelPath;
  if (typeof wakeWordModelPath !== "string" || wakeWordModelPath.trim().length === 0) {
    return reject("voice.wakeWordModelPath must be a non-empty string");
  }

  const sttModelField = readField(voice, "sttModel");
  const sttModel = sttModelField.present ? sttModelField.value : d.voice.sttModel;
  if (typeof sttModel !== "string" || sttModel.trim().length === 0) {
    return reject("voice.sttModel must be a non-empty string");
  }

  const sttLanguageField = readField(voice, "sttLanguage");
  const sttLanguage = sttLanguageField.present ? sttLanguageField.value : d.voice.sttLanguage;
  if (typeof sttLanguage !== "string") {
    return reject("voice.sttLanguage must be a string (empty lets Whisper auto-detect)");
  }

  const sttDeviceField = readField(voice, "sttDevice");
  const sttDevice = sttDeviceField.present ? sttDeviceField.value : d.voice.sttDevice;
  if (typeof sttDevice !== "string" || !STT_DEVICES.includes(sttDevice)) {
    return reject(`voice.sttDevice must be one of ${STT_DEVICES.join(", ")}, got ${String(sttDevice)}`);
  }

  const ttsModelField = readField(voice, "ttsModel");
  const ttsModel = ttsModelField.present ? ttsModelField.value : d.voice.ttsModel;
  if (typeof ttsModel !== "string" || ttsModel.trim().length === 0) {
    return reject("voice.ttsModel must be a non-empty string");
  }

  const audioDeviceField = readField(voice, "audioDevice");
  const audioDevice = audioDeviceField.present ? audioDeviceField.value : d.voice.audioDevice;
  if (typeof audioDevice !== "string" || audioDevice.trim().length === 0) {
    return reject("voice.audioDevice must be a non-empty string");
  }

  const sampleRateField = readField(voice, "sampleRate");
  const sampleRate = sampleRateField.present ? sampleRateField.value : d.voice.sampleRate;
  if (typeof sampleRate !== "number" || !Number.isInteger(sampleRate) || sampleRate < 1) {
    return reject(`voice.sampleRate must be a positive integer, got ${String(sampleRate)}`);
  }

  const ttsSampleRateField = readField(voice, "ttsSampleRate");
  const ttsSampleRate = ttsSampleRateField.present ? ttsSampleRateField.value : d.voice.ttsSampleRate;
  if (typeof ttsSampleRate !== "number" || !Number.isInteger(ttsSampleRate) || ttsSampleRate < 1) {
    return reject(`voice.ttsSampleRate must be a positive integer, got ${String(ttsSampleRate)}`);
  }

  const readBoolean = (source: Record<string, unknown>, key: string, fallback: boolean): { ok: boolean; value?: boolean; message?: string } => {
    const field = readField(source, key);
    const value = field.present ? field.value : fallback;
    if (typeof value !== "boolean") {
      return { ok: false, message: `${key} must be a boolean, got ${String(value)}` };
    }
    return { ok: true, value };
  };

  const filesystem = readBoolean(permissions, "filesystemEnabled", d.permissions.filesystemEnabled);
  if (!filesystem.ok) return reject(filesystem.message ?? "");
  const terminal = readBoolean(permissions, "terminalEnabled", d.permissions.terminalEnabled);
  if (!terminal.ok) return reject(terminal.message ?? "");
  const browser = readBoolean(permissions, "browserEnabled", d.permissions.browserEnabled);
  if (!browser.ok) return reject(browser.message ?? "");

  const startVoiceOnLaunch = readBoolean(preferences, "startVoiceOnLaunch", d.preferences.startVoiceOnLaunch);
  if (!startVoiceOnLaunch.ok) return reject(startVoiceOnLaunch.message ?? "");

  const uiLanguageField = readField(preferences, "uiLanguage");
  const uiLanguage = uiLanguageField.present ? uiLanguageField.value : d.preferences.uiLanguage;
  if (typeof uiLanguage !== "string" || uiLanguage.trim().length === 0) {
    return reject("preferences.uiLanguage must be a non-empty string");
  }

  const environmentField = readField(app, "environment");
  const environment = environmentField.present ? environmentField.value : d.app.environment;
  if (typeof environment !== "string" || environment.trim().length === 0) {
    return reject("app.environment must be a non-empty string");
  }

  const logLevelField = readField(app, "logLevel");
  const logLevel = logLevelField.present ? logLevelField.value : d.app.logLevel;
  if (typeof logLevel !== "string" || !LOG_LEVELS.includes(logLevel)) {
    return reject(`app.logLevel must be one of ${LOG_LEVELS.join(", ")}, got ${String(logLevel)}`);
  }

  return ok({
    model: {
      provider,
      ollamaHost,
      ollamaPort,
      defaultModel,
      temperature,
      maxTokens,
    },
    voice: {
      wakeWordModelPath,
      sttModel,
      sttLanguage,
      sttDevice,
      ttsModel,
      audioDevice,
      sampleRate,
      ttsSampleRate,
    },
    permissions: {
      filesystemEnabled: filesystem.value ?? false,
      terminalEnabled: terminal.value ?? false,
      browserEnabled: browser.value ?? false,
    },
    preferences: {
      startVoiceOnLaunch: startVoiceOnLaunch.value ?? false,
      uiLanguage,
    },
    app: {
      environment,
      logLevel,
    },
  });
}
