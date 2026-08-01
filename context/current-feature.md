# Current Feature

## Working In

Not Started

## Status

Not Started

## Goals

-

## Dependencies

-

## Notes

-

## History

- 2026-08-01 SPEC-0023 Agent Context Builder — Completed. Added `services/core/agent_context_builder.go`: `ContextBuilder` filling the "context before agent execution" requirements of SPEC-0023, the fifth spec of Phase 4 Intelligence, built directly on `Agent`/`AgentMetadata` (SPEC-0018), `types.Task` (SPEC-0011), and `HistoryRecord` (`task_history.go`, SPEC-0017) with no new dependency on any not-yet-built system. `ContextInput` carries the six listed inputs (UserMessage, ConversationHistory, Memories, Task, AvailableTools, PreviousResults); `Memories` and `AvailableTools` are caller-resolved `[]string` identifiers rather than fetched by the builder itself, mirroring `AgentMetadata.Tools`/`MemoryAccess`'s existing bare-string precedent, since the Memory layer (SPEC-0034 onward) and Tool Registry (SPEC-0043..0045) are both still Planned; `ConversationHistory` is likewise pre-rendered `[]string` turns rather than a typed chat-message shape, since no such type exists yet (Conversation Memory, SPEC-0036, is also Planned). `Build` assembles a `Context` (ordered `[]ContextItem` plus `TotalSize`/`Truncated`) in a fixed `contextSectionOrder` matching SPEC-0023's own Requirements listing - "maintain ordering" therefore has one canonical answer independent of the order a caller populates `ContextInput`'s fields in - and silently omits any section left empty or blank ("avoid unnecessary context"). "Support token limits" is a `SizeEstimator` (`func(ContextItem) int`, defaulting to word count) plus an optional `WithMaxSize` budget: items are added in section order until the next one would exceed it, at which point building stops and every section that lost items is recorded in `Truncated` - deliberately not full token accounting or "prioritize important information" logic, both of which are SPEC-0032 Context Window Manager's and SPEC-0033 Token Budget Manager's job, also still Planned; this builder only needs to not silently produce an unbounded context, not replace those specs. No `Container` slot was added (same reasoning as SPEC-0021/0022: no pre-reserved placeholder exists for one). Designed as a natural fit for `ExecutionLoop`'s `ContextAnalyzer` hook (SPEC-0022's Analyze Context stage) but not wired into it, since no spec has asked for that integration yet. `go build ./...`, `go vet ./...`, `go test ./... -v` clean across all 5 go.work modules (10 new tests covering all three SPEC-0023 testing criteria - context generated correctly, required information included with correct ordering, and large contexts handled via size-limit truncation - plus empty-input/blank-entry omission, unlimited-by-default behavior, a custom `SizeEstimator`, truncation logging, and the `Task`/`HistoryRecord` integration boundaries). `gofmt -l` clean on both new files. Reviewed against `docs/agents/CODE_REVIEW_PROTOCOL.md` (Architecture/Code Quality/Security/Testing): found and fixed one Code Quality gap - `Build` called the configured `SizeEstimator` unconditionally, so `WithSizeEstimator(nil)` or a zero-value `ContextBuilder{}` (built without `NewContextBuilder`) would panic on the first item; fixed by falling back to the default word-count estimator whenever none is configured, with a regression test. Accepted one known, out-of-scope limitation, consistent with SPEC-0022's own accepted gap: `ContextBuilder` does not check `AgentMetadata.Permissions` before including a tool identifier in `AvailableTools`, since permission filtering is the caller's responsibility to have already applied before populating `ContextInput` - no permission-checking system exists anywhere in the codebase yet. Approved after fix; re-verified `go build`/`go vet`/`go test ./... -v` clean and `gofmt -l` clean. Built on feature/agent-context-builder (off master, post SPEC-0022 merge). Full rationale: see docs/agents/JARVIS_BUILD_TRACKER.md (SPEC-0023 row).

Earlier entries (SPEC-0001 through SPEC-0022): see docs/agents/JARVIS_BUILD_TRACKER.md and `git log` — this file's History section is reset on each feature completion rather than accumulated indefinitely.
