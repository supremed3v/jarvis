# Current Feature

## Status: Not Started

## Active Feature

No feature loaded. Use the feature workflow's `load` action to load the next spec (see `context/features/FEATURE_INDEX.md` and `docs/agents/JARVIS_BUILD_TRACKER.md`).

## History

### SPEC-0065 Core Runtime Communication Bridge (Completed)

WebSocket transport between the Electron desktop and the Go runtime, fulfilling SPEC-0065's four requirements (sending tasks to core, receiving events, streaming responses, runtime status updates) via the `container.go` `WSBridge` slot. Go: `services/core/ws_bridge.go` (`Bridge` implements `WSBridge` + `runtime.Dependency`) — JSON frame protocol on `127.0.0.1:42321/ws`, batch/streaming/default-agent command dispatch, EventBus event forwarding, SPEC-0048 ApprovalQueue polling, status pushes on transition + on connect, typed `packages/errors`. Desktop: `src/shared/runtime.ts` wire protocol, `src/main/runtimeClient.ts` client (reconnect, injectable socket), IPC channels `runtime:event`/`command:stream`/`command:result`, real handlers replacing SPEC-0064 stubs. New dependency: `coder/websocket v1.8.15`. Verified: 18 Go bridge tests (full core suite green, a server-push race fixed with a connect barrier), 36 TS tests (Node 20 & 24), `go_all.ps1` build+vet+test clean, `npm run build` clean. Details in `docs/agents/JARVIS_BUILD_TRACKER.md`.
