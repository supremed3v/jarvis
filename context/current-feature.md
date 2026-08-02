# Current Feature: SPEC-0036 Conversation Memory

## Working In

Memory layer (Phase 4 Intelligence) — `services/core` (Go), alongside
`memory_interface.go` (SPEC-0034) and `memory_storage.go` /
`memory_storage_local.go` (SPEC-0035).

## Status

Completed

## Goals

- Store conversations, messages, roles, timestamps, metadata, and related
  tasks
- Support loading recent conversations
- Support searching previous conversations
- Support context preparation (assembling recent/relevant messages for an
  agent's context window)

## Dependencies

- SPEC-0034 Memory Interface (status: Completed)
- SPEC-0035 Memory Storage Abstraction (status: Completed)
- SPEC-0011 Task Model (status: Completed) — needed for the "related tasks"
  requirement

## Notes

Specification:

context/features/SPEC-0036-conversation-memory.md

Dependency resolution source: Implementation Order + Requirements inference
(Phase 4 places Memory after Agents/LLM; the spec's own requirements name
conversation storage keyed to conversations/messages/roles/timestamps/
metadata/related tasks, which routes through SPEC-0034's `Memory`
abstraction and SPEC-0035's `MemoryStorageProvider`/`StorageMemory` routing
rather than a new storage layer — conversation memory is relational in
character per ADR-0007, so it should ride the `LocalStore` provider
introduced in SPEC-0035, not the vector store).

Conversation memory is one of `MemoryType`'s four supported kinds per
SPEC-0034 (Conversation, User Profile, Knowledge, Experience) — this spec
implements the Conversation Memory type concretely on top of the existing
`Memory`/`MemoryStorageProvider` interfaces, it does not redefine them.

## Implementation Plan

Affected area: `services/core` only (no other module touched).

Files to create:
- `services/core/conversation_memory.go` — `MessageRole` (user/assistant/
  system), `ConversationMessage` (ID, ConversationID, Role, Content,
  RelatedTaskIDs, Metadata, CreatedAt) with `Validate()`, and
  `ConversationMemory`, a façade over the existing `Memory` interface
  (SPEC-0034) that:
  - `AddMessage` — stores a message as a `MemoryRecord{Type:
    MemoryTypeConversation}` via `Memory.Store`, encoding
    conversationID/role/relatedTaskIDs into `MemoryRecord.Metadata`, then
    `Memory.Retrieve`s it back so returned timestamps are provider-assigned;
    updates an in-process index (conversationID -> ordered message IDs, and
    a recency-ordered list of conversation IDs) the same way
    `memory_storage_local.go`'s `LocalStore` keeps its own map alongside
    satisfying its interface.
  - `Conversation` — returns all messages for one conversation ID in
    chronological order ("conversations can be retrieved").
  - `RecentConversations` — returns up to N conversation summaries ordered
    by most recent activity ("loading recent conversations"). Uses the
    in-process index rather than `Memory.Search`, since `MemoryQuery.Query`
    is required non-empty and local substring search isn't suited to
    listing distinct conversations.
  - `Search` — delegates to `Memory.Search` scoped to
    `MemoryTypeConversation` for free-text lookup ("searching previous
    conversations").
  - `PrepareContext` — returns the trailing N messages of a conversation in
    chronological order ("context preparation").
- `services/core/conversation_memory_test.go` — covers the three Testing
  criteria: messages persist, conversations can be retrieved, context
  loading works, plus validation and multi-conversation isolation.

No changes to `memory_interface.go` / `memory_storage.go` /
`memory_storage_local.go` (SPEC-0034/0035 contracts) or `container.go` —
`ConversationMemory` is constructed by wrapping whatever `Memory` a caller
already has (e.g. `container.Memory`), matching how SPEC-0023's
`ContextBuilder` currently takes conversation history as a caller-resolved
input.

## History

- 2026-08-02 load loaded SPEC-0036
- 2026-08-02 start — status set to In Progress, branch
  feature/conversation-memory created, implementation plan recorded above.
- 2026-08-02 start — implemented `services/core/conversation_memory.go`
  (`MessageRole`, `ConversationMessage`, `ConversationSummary`,
  `ConversationMemory` with `AddMessage`/`Conversation`/
  `RecentConversations`/`Search`/`PrepareContext`) and
  `conversation_memory_test.go` (14 new test cases covering all three
  Testing criteria plus validation and multi-conversation isolation).
  `scripts/go_all.ps1 all` clean across all 5 go.work modules (one
  pre-existing flaky test, `TestWorker_FailsTerminallyAfterMaxAttempts` in
  `task_worker_test.go`, unrelated to this change — reproduces intermittently
  on master too). Not yet merged; run `/jarvis-feature test`,
  `/jarvis-feature review`, then `/jarvis-feature complete` to finish.
- 2026-08-02 review — found and fixed one real defect:
  `ConversationSummary.LastMessageAt` was declared and documented but never
  populated (always the zero time). Added a `lastMessageAt` index alongside
  `byConversation`, threaded through `touchIndex`, and set from
  `AddMessage`'s provider-assigned `rec.CreatedAt`; `RecentConversations`
  now fills it in. Added a regression test for it. Also fixed a gofmt
  misalignment in the `metaConversationID`/`metaRole`/`metaRelatedTaskIDs`
  const block introduced during implementation. Re-ran
  `scripts/go_all.ps1 all` clean across all 5 modules after the fixes.
  Goals, architecture compliance (no changes outside `services/core`,
  no SPEC-0034/0035 contract changes), scope control, error handling, and
  test coverage all check out. Verdict: Ready to complete.
- 2026-08-02 complete — updated `docs/agents/JARVIS_BUILD_TRACKER.md`
  (SPEC-0036 row: Completed) and regenerated
  `context/features/FEATURE_INDEX.md` via `scripts/generate_feature_index.ps1`
  (SPEC-0036 now shows Completed). `scripts/check_dependencies.ps1` and
  `scripts/validate_structure.ps1` clean. Final
  `scripts/go_all.ps1 all` clean across all 5 go.work modules. Status set
  to Completed; awaiting commit approval, then merge into master.
