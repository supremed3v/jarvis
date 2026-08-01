# Current Feature

## Overview
SPEC-0032 — Context Window Manager (`context/features/SPEC-0032-context-window-manager.md`), loaded and ready to start. Fifth spec of Phase 4 Intelligence's LLM branch (after SPEC-0026 Provider, SPEC-0027 Ollama Integration, SPEC-0028 Model Configuration, SPEC-0029 Model Router, SPEC-0030 Streaming Response Handler, SPEC-0031 Prompt Template System, all Completed). Manages information sent to LLMs within context limits: conversation history, memory retrieval, tool outputs, agent instructions. Must prioritize important information, remove unnecessary context, and track token usage; verified by context staying within limits, important information surviving trimming, and large conversations being handled.

## Status
Loaded

## Goals
- Real token accounting, replacing the word-count `SizeEstimator` stand-in `agent_context_builder.go` (SPEC-0023) currently uses as a placeholder for this spec.
- Prioritization logic for what to keep/drop when a conversation exceeds the window, beyond SPEC-0023's existing "stop adding once over budget" truncation.
- Track and expose token usage (also relevant to SPEC-0033 Token Budget Manager, the next spec, which SPEC-0032 unblocks).

## Files Modified
_None yet._

## Notes
Directly builds on `services/core/agent_context_builder.go` (SPEC-0023 `ContextBuilder`/`Context`/`ContextItem`/`SizeEstimator`) and interacts with `services/core/prompt_template.go` (SPEC-0031 `PromptVariables`/`VariablesFromContext`) and `services/core/llm_provider.go` (SPEC-0026 `GenerateRequest`/`GenerateResponse`, neither of which currently carries token-count metadata). SPEC-0023's package doc comment explicitly defers "tracking real token usage and prioritizing what to keep under a model's actual limit" to this spec and SPEC-0033. `packages/config`'s `Model.MaxTokens` (SPEC-0028) is the existing per-model token-limit config field this spec should read rather than duplicate. No spec-internal "Dependencies" section or explicit implementation-order/dependency-graph entry exists beyond its Phase 4 Intelligence / LLM-branch placement and FEATURE_INDEX.md Status (`Planned`, confirmed 2026-08-02) — order inferred from SPEC-0023's and SPEC-0031's own forward references to SPEC-0032.

## History

Earlier entries (SPEC-0001 through SPEC-0031): see docs/agents/JARVIS_BUILD_TRACKER.md and `git log` — this file's History section is reset on each feature completion rather than accumulated indefinitely.

## History

- 2026-08-02 SPEC-0031 Prompt Template System — Completed. Added `services/core/prompt_template.go`: `PromptTemplate` (Name/Version/Kind/Body) plus `PromptTemplate.Render(PromptVariables) (string, error)` renders `Body` as a `text/template` with `missingkey=error`, producing the plain string a caller assigns to `GenerateRequest.Prompt` (SPEC-0026/0027) — it does not call into `Provider`/`StreamHandler` itself, keeping rendering and generation separate, caller-composed concerns. "System prompts" and "Agent instructions" are both just a `PromptTemplate` distinguished by `Kind` (`PromptKindSystem`/`PromptKindInstructions`), metadata only. "Dynamic variables" is `PromptVariables` (`UserContext`, `TaskInformation`, `Memories`, `AvailableTools`, `Instructions`, plus a free-form `Extra map[string]string`); `VariablesFromContext(Context) PromptVariables` bridges SPEC-0023's `ContextBuilder` output for the spec's own named example variables (`UserContext` = combined `ContextSectionUserMessage`+`ContextSectionConversationHistory`; `TaskInformation`/`Memories`/`AvailableTools` map to their same-named sections; `PreviousResults` deliberately left unmapped — no named variable for it in the spec). "Prompt versions" is `PromptRegistry`: keeps every registered `(Name, Version)` pair (`Register` rejects a duplicate version with a `packages/errors` `TypeAlreadyExists` error rather than overwriting), with `Get`/`Latest`/`Versions` (sorted ascending) to retrieve them. `Container` gained the same real-slot treatment every LLM-branch service gets as its spec completes: `PromptRegistry *PromptRegistry` + `WithPromptRegistry` in `container.go`/`container_test.go`. `go build ./...`, `go vet ./...`, `go test ./...` clean across all 5 go.work modules (23 new tests in `prompt_template_test.go`: SPEC-0031's three explicit testing criteria plus `Validate` field coverage, malformed/unknown-field template rejection, and the `VariablesFromContext` bridge's populated and empty-`Context` cases; 5x full-package rerun clean). Built on feature/prompt-template-system (off master, post SPEC-0030 merge).

Earlier entries (SPEC-0001 through SPEC-0030): see docs/agents/JARVIS_BUILD_TRACKER.md and `git log` — this file's History section is reset on each feature completion rather than accumulated indefinitely.
