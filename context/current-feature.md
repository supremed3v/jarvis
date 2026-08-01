# Current Feature

## Overview
SPEC-0032 — Context Window Manager (`context/features/SPEC-0032-context-window-manager.md`), loaded and ready to start. Fifth spec of Phase 4 Intelligence's LLM branch (after SPEC-0026 Provider, SPEC-0027 Ollama Integration, SPEC-0028 Model Configuration, SPEC-0029 Model Router, SPEC-0030 Streaming Response Handler, SPEC-0031 Prompt Template System, all Completed). Manages information sent to LLMs within context limits: conversation history, memory retrieval, tool outputs, agent instructions. Must prioritize important information, remove unnecessary context, and track token usage; verified by context staying within limits, important information surviving trimming, and large conversations being handled.

## Status
Reviewed, approved after fix — not yet merged (`/jarvis-feature complete` still pending).

## Goals
- Real token accounting, replacing the word-count `SizeEstimator` stand-in `agent_context_builder.go` (SPEC-0023) currently uses as a placeholder for this spec. — Done via `TokenEstimator`/`defaultTokenEstimator`.
- Prioritization logic for what to keep/drop when a conversation exceeds the window, beyond SPEC-0023's existing "stop adding once over budget" truncation. — Done via `ContextPriority`/`PriorityFunc`/`defaultPriority` plus recency tiebreak within a tier.
- Track and expose token usage (also relevant to SPEC-0033 Token Budget Manager, the next spec, which SPEC-0032 unblocks). — Done via `Usage{Budget, Used, BySection}`.

## Files Modified
- `services/core/context_window_manager.go` (new) — `WindowManager.Fit(Context, budget) (Context, Usage)`.
- `services/core/context_window_manager_test.go` (new) — SPEC-0032's three testing criteria plus nil-option/logging/custom-func edge cases.
- `services/core/container.go` — added `WindowManager *WindowManager` slot + `WithWindowManager`.
- `services/core/container_test.go` — extended unwired-slots-default-to-nil and options-wire-stub-slots tests for the new slot.

## Notes
Directly builds on `services/core/agent_context_builder.go` (SPEC-0023 `ContextBuilder`/`Context`/`ContextItem`/`SizeEstimator`) and interacts with `services/core/prompt_template.go` (SPEC-0031 `PromptVariables`/`VariablesFromContext`) and `services/core/llm_provider.go` (SPEC-0026 `GenerateRequest`/`GenerateResponse`, neither of which currently carries token-count metadata). SPEC-0023's package doc comment explicitly defers "tracking real token usage and prioritizing what to keep under a model's actual limit" to this spec and SPEC-0033. `packages/config`'s `Model.MaxTokens` (SPEC-0028) is the existing per-model token-limit config field this spec should read rather than duplicate. No spec-internal "Dependencies" section or explicit implementation-order/dependency-graph entry exists beyond its Phase 4 Intelligence / LLM-branch placement and FEATURE_INDEX.md Status (`Planned`, confirmed 2026-08-02) — order inferred from SPEC-0023's and SPEC-0031's own forward references to SPEC-0032.

## History

- 2026-08-02 SPEC-0032 Context Window Manager — Implemented and reviewed (approved after fix; not yet merged). Added `services/core/context_window_manager.go`: `WindowManager.Fit(Context, budget int) (Context, Usage)` trims a SPEC-0023 `Context` to a token budget, keeping the highest-`ContextPriority` items first and, within a tier, later (more recent) items over earlier ones. `TokenEstimator` defaults to a ~4-chars/token heuristic; `PriorityFunc` defaults to ranking `UserMessage`/`Task` critical, `Memories` high, `ConversationHistory`/`AvailableTools` normal, `PreviousResults` low. `Usage{Budget, Used, BySection}` covers "track token usage". Deliberately takes budget as a caller-supplied int rather than resolving it from `Model.MaxTokens` (SPEC-0028 caps output length, not input context size) — that resolution is left to SPEC-0033 Token Budget Manager. `Container` gained `WindowManager *WindowManager` + `WithWindowManager`. Reviewed against `docs/agents/CODE_REVIEW_PROTOCOL.md` (Architecture/Code Quality/Security/Testing): found and fixed one real Code Quality bug — `Fit`'s trimming branch rebuilt `Truncated` from scratch instead of merging with the incoming `Context.Truncated`, silently losing record of sections an upstream `ContextBuilder` had already dropped entirely (nothing left in `Items` for `Fit` to notice); fixed by seeding `droppedSections` from `c.Truncated` before the kept/dropped scan, plus a regression test (`TestWindowManager_Fit_PreservesUpstreamTruncation`). Approved after fix. `go build`/`go vet`/`go test` clean across all 5 go.work modules; `gofmt -l` clean on both new files. Built on feature/context-window-manager (off master, post SPEC-0031 merge). Still pending: merge to master (`/jarvis-feature complete`).

Earlier entries (SPEC-0001 through SPEC-0031): see docs/agents/JARVIS_BUILD_TRACKER.md and `git log` — this file's History section is reset on each feature completion rather than accumulated indefinitely.
