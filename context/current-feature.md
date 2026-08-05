# Current Feature

SPEC-0072: Runtime Logs Viewer

## Status

Implementation Complete

## Spec

`context/features/SPEC-0072-runtime-logs-viewer.md`

## Summary

Create a desktop UI for viewing JARVIS runtime activity: system logs, agent activity, tool execution, and errors — with filtering, searching, and live updates.

## Dependencies (all Completed)

- SPEC-0065 WSBridge (runtime event transport)
- SPEC-0067 Voice Interface UI, SPEC-0068 System Tray, SPEC-0069 Settings UI, SPEC-0070 Agent Dashboard, SPEC-0071 Memory Viewer (established desktop UI patterns)

## Requirements

1. Display system logs, agent activity, tool execution, and errors
2. Category filtering (system/agent/tool/error)
3. Text search across event type, message, and source
4. Live updates as runtime events arrive
5. Clear logs action
6. Payload inspection toggle per entry

## Testing Criteria

1. Logs appear (entries accumulate from runtime events)
2. Filters work (category dropdown + text search)
3. Live updates function (new events broadcast to logs window)

## Files Created

- `apps/desktop/src/shared/logs.ts` — LogEntry, LogsViewerState, LogUiStore (ring buffer of 500 entries), event categorization
- `apps/desktop/src/shared/logs.test.ts` — 16 unit tests covering categorization, accumulation, capping, and store behavior
- `apps/desktop/src/renderer/logs.html` — HTML page matching existing UI style (dark theme, cyan accent)
- `apps/desktop/src/renderer/logsRenderer.ts` — renderer script with client-side filtering/searching, payload toggle, live updates

## Files Modified

- `apps/desktop/src/shared/ipc.ts` — added `logs` IPC channel group (list, clear, updated) and `logs` domain on JarvisBridge
- `apps/desktop/src/main/preload.ts` — added logs channels and bridge surface
- `apps/desktop/src/main/ipc.ts` — added LogUiStore parameter and logs:list/logs:clear handlers
- `apps/desktop/src/main/main.ts` — added logsStore, logsWindow management, event ingestion into logs store with broadcast, tray handler wiring
- `apps/desktop/src/main/tray.ts` — added `logs` to TrayHandlers and dispatch
- `apps/desktop/src/main/trayMenu.ts` — added "Logs" menu entry and `logs` to TrayMenuItemId
- `apps/desktop/src/main/trayMenu.test.ts` — updated to include the new "logs" entry

## History

- 2026-08-05 loaded SPEC-0072 Runtime Logs Viewer
- 2026-08-05 implemented: created shared/logs.ts (LogEntry model, LogUiStore with 500-entry ring buffer, event categorization into system/agent/tool/error), renderer/logs.html + logsRenderer.ts (dark-themed UI with category filter, text search, payload inspection, live updates), wired IPC channels (logs:list, logs:clear, logs:updated), preload bridge, main-process event ingestion and window management, tray menu entry. TypeScript type-check clean. All 152 desktop tests pass (16 new logs tests + all existing tests).
