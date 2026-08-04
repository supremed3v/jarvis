// settingsStore.ts is the SPEC-0069 settings store, owned by the Electron main
// process. It keeps the current Settings in memory, loads them from a JSON
// file at startup (defaults when the file is absent or corrupt, so a broken
// settings file can never prevent the app from starting), and persists every
// accepted update to that file — satisfying SPEC-0069's "settings save
// correctly", "settings load on restart", and "invalid values are rejected"
// testing criteria. It depends only on fs/path, so node:test can drive it
// against a temp directory without a running Electron instance; main.ts
// supplies the real file path (under app.getPath("userData")).

import * as fs from "fs";
import * as path from "path";
import { fail, ok, type IpcResult } from "../shared/ipc";
import {
  defaultSettings,
  mergeSettings,
  validateSettings,
  type Settings,
  type SettingsPatch,
} from "../shared/settings";

export class SettingsStore {
  private settings: Settings;
  private readonly filePath: string;
  private readonly listeners = new Set<(settings: Settings) => void>();

  constructor(filePath: string, initial: Settings = defaultSettings()) {
    this.filePath = filePath;
    this.settings = initial;
  }

  // load reads the persisted settings file, if any. A missing file, a file
  // that is not valid JSON, or a file whose values fail validation all leave
  // the store on the current settings (defaults on first start) rather than
  // aborting startup; a valid file is validated (fields absent from it fall
  // back to defaults) so values added by newer app versions still default
  // correctly.
  load(): Settings {
    let data: string;
    try {
      data = fs.readFileSync(this.filePath, "utf8");
    } catch {
      return this.settings;
    }
    let parsed: unknown;
    try {
      parsed = JSON.parse(data);
    } catch {
      return this.settings;
    }
    const result = validateSettings(parsed);
    if (result.ok) {
      this.settings = result.data;
    }
    return this.settings;
  }

  get(): Settings {
    return this.settings;
  }

  // update applies a partial patch onto the current settings, validates the
  // merged result, and — only when valid — persists it to disk and notifies
  // subscribers. An invalid patch returns an IpcResult failure without
  // mutating the store.
  update(patch: SettingsPatch): IpcResult<Settings> {
    const result = validateSettings(mergeSettings(this.settings, patch));
    if (!result.ok) {
      return result;
    }
    this.settings = result.data;
    try {
      fs.mkdirSync(path.dirname(this.filePath), { recursive: true });
      fs.writeFileSync(this.filePath, JSON.stringify(this.settings, null, 2) + "\n", "utf8");
    } catch (error) {
      return fail(
        "SETTINGS_WRITE_FAILED",
        `could not persist settings to ${this.filePath}: ${error instanceof Error ? error.message : String(error)}`,
      );
    }
    for (const listener of this.listeners) {
      listener(this.settings);
    }
    return ok(this.settings);
  }

  onChange(listener: (settings: Settings) => void): () => void {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  }
}
