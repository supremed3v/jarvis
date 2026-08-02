# Current Feature

## Working In

Memory layer (Phase 4 Intelligence) — `services/core` (Go), alongside
`memory_interface.go` (SPEC-0034), `memory_storage.go` /
`memory_storage_local.go` (SPEC-0035), and `conversation_memory.go`
(SPEC-0036).

## Status

Loaded

## Goals

- Store user preferences (e.g. "User prefers Go over Python")
- Store personal information
- Store working style
- Store projects (e.g. "User works on Invoke Solutions")
- Store other important facts
- Support updating a fact so it replaces outdated information rather than
  accumulating duplicates — the spec's third testing criterion, and the
  part with no existing precedent: `ConversationMemory` (SPEC-0036) never
  needed "replace this fact by identity," only appending messages to a
  conversation thread.

## Dependencies

- SPEC-0034 Memory Interface (status: Completed) — `MemoryTypeUserProfile`
  already exists in `memory_interface.go`'s `MemoryType` enum, unused by
  any concrete feature until now.
- SPEC-0035 Memory Storage Abstraction (status: Completed) — `StorageMemory`
  routes `MemoryTypeUserProfile` records to whichever
  `MemoryStorageProvider` a caller configures, same as every other type.

## Notes

Specification: `context/features/SPEC-0037-user-profile-memory.md`.

Dependency resolution source: Implementation Order (Memory branch of Phase
4 Intelligence, after SPEC-0036) + requirements inference, same as
SPEC-0036's own load — `docs/execution/JARVIS_IMPLEMENTATION_ORDER.md` and
`docs/execution/JARVIS_DEPENDENCY_GRAPH.md` name "Memory" only at the
phase level, not per-spec.

Likely shape, by analogy to `conversation_memory.go`: a facade over
SPEC-0034's `Memory`, storing each fact as a
`MemoryRecord{Type: MemoryTypeUserProfile}`. Unlike `ConversationMemory`,
which only appends, this needs a stable identity per fact (e.g. a
category/key such as `preference:language`) so a later `Store` of the same
key updates in place instead of creating a duplicate — `Memory.Update`
already exists for replace-by-ID, but the caller-facing API needs to
resolve "the existing record for this key" to an ID first, which nothing
in SPEC-0034/0035/0036 currently does.

Not yet started — no code written, no branch created.

## History

- 2026-08-02 SPEC-0036 Conversation Memory — Completed. Implemented
  services/core/conversation_memory.go (`MessageRole`, `ConversationMessage`,
  `ConversationSummary`, `ConversationMemory` — a façade over SPEC-0034's
  `Memory` with `AddMessage`/`Conversation`/`RecentConversations`/`Search`/
  `PrepareContext`, keeping its own in-process index since `Memory.Search`
  has no notion of "distinct conversations"), 15 new tests, no changes to
  SPEC-0034/0035 contracts or `container.go`. `go build`/`vet`/`test` clean
  across all 5 go.work modules. Review pass found and fixed one gap
  (`ConversationSummary.LastMessageAt` was never populated) before
  completion. Merged feature/conversation-memory into master.
