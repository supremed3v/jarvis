# Current Feature

## Overview
Not started — no feature currently loaded. Next candidate per docs/execution/JARVIS_IMPLEMENTATION_ORDER.md: SPEC-0033 (Token Budget Manager), the next spec in Phase 4 Intelligence's LLM branch now that SPEC-0032 (Context Window Manager) is Completed.

## Status
Not Started

## Goals
_None yet._

## Files Modified
_None yet._

## Notes
SPEC-0032 (Context Window Manager) is now Completed (see docs/agents/JARVIS_BUILD_TRACKER.md, SPEC-0032 row, for full implementation/review rationale). SPEC-0033 (Token Budget Manager) is unblocked: it owns resolving what a `WindowManager.Fit(Context, budget int)` (SPEC-0032) budget should actually be for a given model/agent — `packages/config`'s `Model.MaxTokens` (SPEC-0028) caps generated output length only, not input context size, so that resolution (model lookup, defaults, per-agent overrides) was deliberately left unbuilt for this spec to own.

## History

- 2026-08-02 SPEC-0032 Context Window Manager — Completed. Added `services/core/context_window_manager.go`: `WindowManager.Fit(Context, budget int) (Context, Usage)` sits directly on top of `ContextBuilder`/`Context`/`ContextItem` (SPEC-0023) without modifying them, providing the real token-accounting/prioritization layer `agent_context_builder.go`'s own package doc comment explicitly deferred to this spec. "Prioritize important information" is `ContextPriority` (`Low`/`Normal`/`High`/`Critical` — named `ContextPriority` rather than `Priority` to avoid colliding with `task_queue.go`'s existing `TaskPriority` constants, SPEC-0013) plus a `PriorityFunc` defaulting to ranking `UserMessage`/`Task` critical, `Memories` high, `ConversationHistory`/`AvailableTools` normal, `PreviousResults` low; within a tier, later (more recently added) items are kept over earlier ones. "Remove unnecessary context" is `Fit`'s greedy fill in priority-then-recency order, skipping (not aborting on) any item that doesn't fit so one oversized item can't starve smaller lower-priority ones. "Track token usage" is `Usage{Budget, Used, BySection}`, returned alongside the trimmed `Context` on every call. `TokenEstimator` defaults to a ~4-chars/token heuristic, a stand-in for a real tokenizer exactly as SPEC-0023's own word-count `defaultSizeEstimator` was. `Fit` deliberately takes budget as a caller-supplied int rather than resolving it from `Model.MaxTokens` (SPEC-0028 caps output length, not input context size) — that resolution is left to SPEC-0033 Token Budget Manager. `Container` gained `WindowManager *WindowManager` + `WithWindowManager`. Reviewed against `docs/agents/CODE_REVIEW_PROTOCOL.md` (Architecture/Code Quality/Security/Testing): found and fixed one real Code Quality bug — `Fit`'s trimming branch rebuilt `Truncated` from scratch instead of merging with the incoming `Context.Truncated`, silently losing record of sections an upstream `ContextBuilder` had already dropped entirely; fixed by seeding `droppedSections` from `c.Truncated`, plus regression test `TestWindowManager_Fit_PreservesUpstreamTruncation`. Approved after fix. `go build ./...`, `go vet ./...`, `go test ./...` clean across all 5 go.work modules; `gofmt -l` clean on both new files. Built on feature/context-window-manager (off master, post SPEC-0031 merge).

Earlier entries (SPEC-0001 through SPEC-0032): see docs/agents/JARVIS_BUILD_TRACKER.md and `git log` — this file's History section is reset on each feature completion rather than accumulated indefinitely.
