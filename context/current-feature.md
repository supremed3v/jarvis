# Current Feature: Memory Viewer UI

## Working In

apps/desktop (Electron) — a renderer viewer over the memory data the Go
runtime already exposes (services/core/memory_interface.go SPEC-0034,
memory_storage.go SPEC-0035, conversation_memory.go SPEC-0036,
user_profile_memory.go SPEC-0037, knowledge_ingestion.go SPEC-0040),
following the SPEC-0064/0065/0067/0069/0070 pure-TS-shared-module +
main-process-store + sandboxed-window pattern.

## Status

Completed

## Goals

- Display user memories
- Display conversations
- Display knowledge entries
- Display memory metadata
- Support search
- Support filtering
- Support deletion
- Support editing where allowed

## Dependencies

- SPEC-0034 Memory Interface (status: Completed)
- SPEC-0035 Memory Storage Abstraction (status: Completed)
- SPEC-0036 Conversation Memory (status: Completed)
- SPEC-0037 User Profile Memory (status: Completed)
- SPEC-0040 Knowledge Ingestion (status: Completed)
- SPEC-0063 Electron Application Bootstrap (status: Completed)
- SPEC-0064 Desktop Ipc Architecture (status: Completed)
- SPEC-0065 Core Runtime Communication Bridge (status: Completed)

## Notes

Specification:

context/features/SPEC-0071-memory-viewer-ui.md

Index status at load time: Planned

Dependency resolution source: Implementation Order (Phase 5 Applications) +
Dependency Graph (Applications -> Memory, Desktop) + Requirements inference

Implemented design (confirmed at load): Go bridge-backed, runtime-authoritative.
The desktop never caches or persists memory; the runtime is the source of truth
and every change flows through the bridge.

- Go seam: services/core/memory_viewer.go — MemoryViewer interface
  (List/Search/Update/Delete) + CoreMemoryViewer, wired via WithViewerConversations
  / WithViewerProfile / WithViewerLister. List supplies the "show all" the
  SPEC-0034 Memory interface lacks (no list-all; MemoryQuery.Validate requires a
  non-empty Query); Search/Update/Delete delegate to the backing Memory.
  Update re-fetches the stored record and replaces only Content (Type/Metadata/
  timestamps preserved).
- Go bridge: services/core/ws_bridge.go — memory.list / memory.search /
  memory.update / memory.delete client frames, one memory.result ack frame,
  MemoryEntry wire view, WithBridgeMemory option, per-frame handlers. No viewer
  wired => memory.list answers an empty list, the rest answer MEMORY_DISABLED.
  Typed package-errors codes propagate onto the wire.
- Desktop: shared/runtime.ts memory frame types/builders/decoders; new
  shared/memory.ts MemoryUiStore + reducers; shared/ipc.ts jarvis:memory:*
  channels + validators + JarvisBridge surface; main/runtimeClient.ts
  listMemories/searchMemories/updateMemory/deleteMemory; main/ipc.ts + main.ts
  orchestration (memory window, refreshMemories, applyMemoryUpdate/Delete with
  jarvis:memory:updated broadcasts); tray "Memory" item; sandboxed renderer
  memory.html + memoryRenderer.ts (search box, type filter, edit/delete).
- devbridge leftover (services/core/cmd/devbridge/main.go) was deleted at load
  with user approval; it was a temporary SPEC-0070 dev harness outside any spec.

## History

- 2026-08-05 start Started implementation. Go side complete and fully tested:
  new services/core/memory_viewer.go (MemoryViewer seam + CoreMemoryViewer +
  WithViewer* options, listTypes display order, messageViewerMetadata) and
  services/core/ws_bridge.go additions (frame constants, MemoryEntry,
  WithBridgeMemory, handleMemoryList/Search/Update/Delete + sendMemory* helpers).
  New tests: memory_viewer_test.go (List grouping/filter/error, Search delegation,
  Update preserves type/metadata, Delete, validation) and ws_bridge_test.go
  memory frame tests (empty list without viewer, filtered list, invalid type,
  search match/scope/empty-query, MEMORY_DISABLED for all three control frames,
  update/delete ok + typed-error propagation). Full Go workspace clean
  (scripts/go_all.ps1 all). Desktop shared + main + renderer layers implemented:
  runtime.ts memory frames/decoders, new memory.ts store, ipc.ts channels+
  validators+JarvisBridge, runtimeClient.ts methods + memoryResult settle,
  main/ipc.ts handlers, main.ts memory window + refreshMemories +
  applyMemoryUpdate/applyMemoryDelete + tray, preload.ts, trayMenu/tray Memory
  item, memory.html + memoryRenderer.ts, package.json test list. Tests added for
  validators, frame builders/decoders, MemoryUiStore, runtimeClient memory
  methods, tray menu. npm test 136/136 passing (baseline was 108), npm run build
  clean. Remaining: review + completion (FEATURE_INDEX/build-tracker status
  flip, current-feature handoff).
- 2026-08-05 load loaded SPEC-0071 (context/features/SPEC-0071-memory-viewer-ui.md). Requirements: an interface for inspecting JARVIS memory — display user memories, conversations, knowledge entries, and memory metadata; support search, filtering, deletion, and editing where allowed; testing criteria are "memories load correctly", "search works", and "changes persist". All resolved prerequisites are Completed per docs/agents/JARVIS_BUILD_TRACKER.md / FEATURE_INDEX.md: the Memory layer data sources (SPEC-0034 Memory interface, SPEC-0035 StorageMemory/LocalStore/VectorStore, SPEC-0036 ConversationMemory, SPEC-0037 UserProfileMemory, SPEC-0040 KnowledgeIngestionPipeline) and the desktop stack (SPEC-0063 bootstrap, SPEC-0064 IPC, SPEC-0065 WebSocket bridge, plus Completed peers SPEC-0066-0070 for UI patterns). Working tree clean (HEAD fa8c55b) except one untracked leftover: services/core/cmd/devbridge/main.go — a temporary SPEC-0070 dev harness whose own header says "deleted after testing"; it was left behind, so start should either extend it to seed memory for end-to-end exercise or delete it (not part of any spec). Two facts shape the design direction: (1) the Container wires Memory only via WithMemory, with no default instance, and no runtime currently exposes memory over the bridge; (2) SPEC-0034's Memory interface has no "list all" — MemoryQuery.Validate() requires a non-empty Query, and only the façades offer bounded enumeration (SPEC-0036 RecentConversations, SPEC-0037 Facts). Design direction for start (to confirm with user): mirror SPEC-0070's Go bridge-backed pattern — additive SPEC-0065 frames (memory.list / memory.search / memory.update / memory.delete + ack frames), a new WithBridgeMemory option wiring a memory-viewer seam that supplies the list-all the Memory interface lacks, a pure-TS shared memory-view model + main-process store + sandboxed BrowserWindow, with deletion/editing persisted via the same bridge surface.
