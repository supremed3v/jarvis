// settingsRenderer.ts drives the SPEC-0069 settings window (settings.html).
// Like renderer.ts, the sandboxed renderer runs this as a plain script with no
// module system, so the Settings shape and the jarvis bridge surface are
// mirrored here and only ever used at compile time; the authoritative models
// live in ../shared/settings.ts and ../shared/ipc.ts. The form reads the
// current settings over jarvis.settings.get() on load and submits the whole
// form as a settings object to jarvis.settings.save(), which rejects invalid
// values on the main-process side (SPEC-0069 testing criterion 3).
//
// Script-scope declarations are prefixed with Settings to avoid colliding with
// the other renderer script (renderer.ts), which shares this compilation unit's
// global scope.

interface SettingsIpcError {
  code: string;
  message: string;
}

type SettingsIpcResult<T> = { ok: true; data: T } | { ok: false; error: SettingsIpcError };

interface ModelSettings {
  provider: string;
  ollamaHost: string;
  ollamaPort: number;
  defaultModel: string;
  temperature: number;
  maxTokens: number;
}

interface VoiceSettings {
  wakeWordModelPath: string;
  sttModel: string;
  sttLanguage: string;
  sttDevice: string;
  ttsModel: string;
  audioDevice: string;
  sampleRate: number;
  ttsSampleRate: number;
}

interface ToolPermissions {
  filesystemEnabled: boolean;
  terminalEnabled: boolean;
  browserEnabled: boolean;
}

interface UserPreferences {
  startVoiceOnLaunch: boolean;
  uiLanguage: string;
}

interface AppBehavior {
  environment: string;
  logLevel: string;
}

interface Settings {
  model: ModelSettings;
  voice: VoiceSettings;
  permissions: ToolPermissions;
  preferences: UserPreferences;
  app: AppBehavior;
}

// SettingsBridge is the compile-time mirror of the preload surface this page
// uses; only the settings domain is needed here.
interface SettingsBridge {
  settings: {
    get: () => Promise<SettingsIpcResult<Settings>>;
    save: (settings: Settings) => Promise<SettingsIpcResult<Settings>>;
    onChanged: (cb: (settings: Settings) => void) => () => void;
  };
}

function getSettingsBridge(): SettingsBridge | undefined {
  return (window as unknown as { jarvis?: SettingsBridge }).jarvis;
}

// Fields are addressed by data-field="category.field", matching the Settings
// shape, so rendering and collecting stay mechanical.
const FIELD_PATH = "[data-field]";

function renderSettings(root: HTMLElement, settings: Settings): void {
  const inputs = Array.from(root.querySelectorAll<HTMLInputElement | HTMLSelectElement>(FIELD_PATH));
  for (const input of inputs) {
    const path = input.getAttribute("data-field");
    if (!path) {
      continue;
    }
    const value = lookup(settings, path);
    if (input.type === "checkbox") {
      input.checked = value === true;
    } else if (input.type === "number") {
      input.value = String(value);
    } else {
      input.value = String(value);
    }
  }
}

function lookup(settings: Settings, path: string): unknown {
  return path.split(".").reduce<unknown>((current, key) => {
    if (current === null || typeof current !== "object") {
      return undefined;
    }
    return (current as Record<string, unknown>)[key];
  }, settings);
}

function collectSettings(root: HTMLElement): Settings {
  const read = (category: string, field: string): unknown => {
    const input = root.querySelector<HTMLInputElement | HTMLSelectElement>(
      `[data-field="${category}.${field}"]`,
    );
    if (!input) {
      throw new Error(`settings form is missing field ${category}.${field}`);
    }
    if (input.type === "checkbox") {
      return input.checked;
    }
    if (input.type === "number") {
      return input.valueAsNumber;
    }
    return input.value;
  };

  return {
    model: {
      provider: String(read("model", "provider")),
      ollamaHost: String(read("model", "ollamaHost")),
      ollamaPort: Number(read("model", "ollamaPort")),
      defaultModel: String(read("model", "defaultModel")),
      temperature: Number(read("model", "temperature")),
      maxTokens: Number(read("model", "maxTokens")),
    },
    voice: {
      wakeWordModelPath: String(read("voice", "wakeWordModelPath")),
      sttModel: String(read("voice", "sttModel")),
      sttLanguage: String(read("voice", "sttLanguage")),
      sttDevice: String(read("voice", "sttDevice")),
      ttsModel: String(read("voice", "ttsModel")),
      audioDevice: String(read("voice", "audioDevice")),
      sampleRate: Number(read("voice", "sampleRate")),
      ttsSampleRate: Number(read("voice", "ttsSampleRate")),
    },
    permissions: {
      filesystemEnabled: Boolean(read("permissions", "filesystemEnabled")),
      terminalEnabled: Boolean(read("permissions", "terminalEnabled")),
      browserEnabled: Boolean(read("permissions", "browserEnabled")),
    },
    preferences: {
      startVoiceOnLaunch: Boolean(read("preferences", "startVoiceOnLaunch")),
      uiLanguage: String(read("preferences", "uiLanguage")),
    },
    app: {
      environment: String(read("app", "environment")),
      logLevel: String(read("app", "logLevel")),
    },
  };
}

document.addEventListener("DOMContentLoaded", () => {
  const jarvis = getSettingsBridge();
  const page = document.getElementById("page");
  const errorEl = document.getElementById("error");
  const saveBtn = document.getElementById("save");
  const savedHint = document.getElementById("saved-hint");

  if (!jarvis || !page || !errorEl || !saveBtn || !savedHint) {
    if (errorEl) {
      errorEl.textContent = "Settings UI unavailable";
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

  const showSaved = (): void => {
    savedHint.classList.remove("hidden");
    window.setTimeout(() => savedHint.classList.add("hidden"), 2000);
  };

  jarvis.settings.get().then((result) => {
    if (result.ok) {
      renderSettings(page, result.data);
    } else {
      showError(`Failed to load settings: ${result.error.code} — ${result.error.message}`);
    }
  });

  const saveButton = saveBtn as HTMLButtonElement;
  saveButton.addEventListener("click", () => {
    clearError();
    saveButton.disabled = true;
    let settings: Settings;
    try {
      settings = collectSettings(page);
    } catch (error) {
      saveButton.disabled = false;
      showError(error instanceof Error ? error.message : String(error));
      return;
    }
    jarvis.settings
      .save(settings)
      .then((result) => {
        if (result.ok) {
          showSaved();
        } else {
          showError(`Save failed: ${result.error.code} — ${result.error.message}`);
        }
      })
      .catch((error: unknown) => {
        showError(`Save failed: ${error instanceof Error ? error.message : String(error)}`);
      })
      .finally(() => {
        saveButton.disabled = false;
      });
  });
});
