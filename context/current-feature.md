# Current Feature

## Status: Not Started

## Active Feature

No feature loaded. Use the feature workflow's `load` action to load the next spec (see `context/features/FEATURE_INDEX.md` and `docs/agents/JARVIS_BUILD_TRACKER.md`).

## History

### SPEC-0064 Desktop IPC Architecture (Completed)

Typed, secure Electron IPC across main <-> renderer: shared contract (`src/shared/ipc.ts` — channel allowlist + `IpcResult` envelope + payload validators + `JarvisBridge` type), main-process handler registration with allowlist enforcement, sandbox-compatible preload bridge (typed per-domain methods only, never raw `ipcRenderer`), and a plain-script renderer consuming the typed round-trips and pushes. Channels cover user commands, runtime status, tool approvals, and voice events. Verified: 7 unit tests (`npm test`), 12/12 Electron smoke checks, clean real launch, Go workspace unaffected. Fixed a latent SPEC-0063 renderer bug (CommonJS `exports` header in a sandboxed page). Details in `docs/agents/JARVIS_BUILD_TRACKER.md`.
