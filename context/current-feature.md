# Current Feature

## Working In

Not specified — no feature currently loaded.

## Status

Not Started

## Goals

_None yet._

## Dependencies

_None yet._

## Notes

Next candidate per docs/execution/JARVIS_IMPLEMENTATION_ORDER.md: SPEC-0037
(User Profile Memory), continuing the Memory branch of Phase 4 Intelligence
now that SPEC-0036 (Conversation Memory) is Completed.

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
