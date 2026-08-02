# Current Feature

## Working In

Memory layer (Phase 4 Intelligence) — `services/core` (Go), alongside
`memory_interface.go` (SPEC-0034), `memory_storage.go` /
`memory_storage_local.go` (SPEC-0035), and `conversation_memory.go`
(SPEC-0036).

## Status

Reviewed, approved after fix — not yet merged (`/jarvis-feature complete`
still pending).

## Goals

- Store user preferences (e.g. "User prefers Go over Python") — Done via
  `ProfileCategoryPreference`.
- Store personal information — Done via `ProfileCategoryPersonalInfo`.
- Store working style — Done via `ProfileCategoryWorkingStyle`.
- Store projects (e.g. "User works on Invoke Solutions") — Done via
  `ProfileCategoryProject`.
- Store other important facts — Done via `ProfileCategoryFact`.
- Support updating a fact so it replaces outdated information rather than
  accumulating duplicates — the spec's third testing criterion, and the
  part with no existing precedent: `ConversationMemory` (SPEC-0036) never
  needed "replace this fact by identity," only appending messages to a
  conversation thread. Done via `ProfileFact.Key` + `Remember`'s
  Key -> record ID index, calling `Memory.Update` in place when a Key is
  already remembered.

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

Reviewed against `docs/agents/CODE_REVIEW_PROTOCOL.md` (Architecture /
Code Quality / Security / Testing). Still pending: merge to master
(`/jarvis-feature complete`).

## History

- 2026-08-02 SPEC-0037 User Profile Memory — Implemented and reviewed
  (approved after fix; not yet merged). Added
  `services/core/user_profile_memory.go`: `ProfileCategory`
  (`preference`/`personal_info`/`working_style`/`project`/`fact`,
  matching the five categories the spec lists), `ProfileFact`
  (Key/Category/Content/Metadata/timestamps), and `UserProfileMemory` —
  a façade over SPEC-0034's `Memory` with `Remember`/`Fact`/`Facts`,
  storing each fact as a `MemoryRecord{Type: MemoryTypeUserProfile}` (a
  type SPEC-0034 already defined but nothing used until now). `Remember`
  keeps an in-process Key -> record ID index (same map+mutex approach
  `ConversationMemory` and `memory_storage_local.go`'s `LocalStore`
  already use) so a second `Remember` under an already-known Key calls
  `Memory.Update` in place instead of `Memory.Store`-ing a duplicate —
  satisfying "updates replace outdated information" without any
  free-text matching. No changes to SPEC-0034/0035/0036 contracts or
  `container.go` — `UserProfileMemory` wraps whatever `Memory` a caller
  already has, same as `ConversationMemory`. Reviewed against
  `docs/agents/CODE_REVIEW_PROTOCOL.md`: found and fixed one real Code
  Quality bug — `Remember`'s check-then-act on `byKey` (does this Key
  already exist?) released the mutex before acting on the answer,
  deciding `Store` vs. `Update` outside the lock; two concurrent
  `Remember` calls on the same new Key could both see "not found," both
  `Store`, and permanently orphan whichever record lost the final index
  write. Fixed by holding the mutex across the whole read-decide-write
  sequence, plus a regression test
  (`TestUserProfileMemory_Remember_ConcurrentSameKeyConverges`). Approved
  after fix. 17 tests total, covering all three SPEC-0037 testing
  criteria plus validation, `Facts()` ordering, and the concurrency fix.
  `go build`/`vet`/`test` clean across all 5 go.work modules; `gofmt -l`
  clean on both new files. Built on feature/user-profile-memory (off
  master, post SPEC-0036 merge). Still pending: merge to master
  (`/jarvis-feature complete`).

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
