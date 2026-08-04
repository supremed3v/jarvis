// agentStore.ts is the SPEC-0070 agent store, owned by the Electron main
// process. It records the user's per-agent enable/disable intent and persists
// it to a JSON file (a map of agent id -> enabled flag), so the dashboard's
// enable state survives restarts. Only explicitly-toggled agents are written;
// an agent that has never been toggled reads as enabled by default.
//
// The store is deliberately decoupled from the runtime lifecycle: toggling
// also drives agent.start / agent.stop bridge frames as a best-effort control
// (see main.ts and main/ipc.ts), but the local flag is authoritative for what
// the dashboard renders and is what persists. It depends only on fs/path, so
// node:test can drive it against a temp directory without a running Electron
// instance.

import * as fs from "fs";
import * as path from "path";
import { fail, ok, type IpcResult } from "../shared/ipc";
import type { AgentEnabledPatch } from "../shared/ipc";

export class AgentStore {
  private readonly enabledById = new Map<string, boolean>();
  private readonly filePath: string;

  constructor(filePath: string) {
    this.filePath = filePath;
  }

  // load reads the persisted enabled map, if any. A missing or corrupt file
  // leaves the store empty (everything enabled by default) rather than
  // aborting startup.
  load(): void {
    let data: string;
    try {
      data = fs.readFileSync(this.filePath, "utf8");
    } catch {
      return;
    }
    let parsed: unknown;
    try {
      parsed = JSON.parse(data);
    } catch {
      return;
    }
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
      return;
    }
    for (const [id, enabled] of Object.entries(parsed as Record<string, unknown>)) {
      if (typeof enabled === "boolean") {
        this.enabledById.set(id, enabled);
      }
    }
  }

  // isEnabled returns whether the agent is currently enabled, defaulting to
  // enabled for agents the store has never seen.
  isEnabled(id: string): boolean {
    return this.enabledById.get(id) ?? true;
  }

  // set records the flag for one agent and persists the map. On write failure
  // the in-memory value is still updated so the session remains consistent,
  // but the caller is told the write did not persist.
  set(id: string, enabled: boolean): IpcResult<AgentEnabledPatch> {
    this.enabledById.set(id, enabled);
    try {
      fs.mkdirSync(path.dirname(this.filePath), { recursive: true });
      fs.writeFileSync(this.filePath, JSON.stringify(Object.fromEntries(this.enabledById), null, 2) + "\n", "utf8");
    } catch (error) {
      return fail(
        "AGENT_STORE_WRITE_FAILED",
        `could not persist agent state to ${this.filePath}: ${error instanceof Error ? error.message : String(error)}`,
      );
    }
    return ok({ id, enabled });
  }
}
