# Current Feature

## Overview
Not started — no feature currently loaded. Next candidate per docs/execution/JARVIS_IMPLEMENTATION_ORDER.md: SPEC-0032 (Context Window Manager), the next spec in Phase 4 Intelligence's LLM branch now that SPEC-0031 (Prompt Template System) is Completed.

## Status
Not Started

## Goals
_None yet._

## Files Modified
_None yet._

## Notes
SPEC-0031 (Prompt Template System) is now Completed (see docs/agents/JARVIS_BUILD_TRACKER.md, SPEC-0031 row, for full implementation/review rationale). SPEC-0032 (Context Window Manager) and SPEC-0033 (Token Budget Manager) are the two specs `agent_context_builder.go` (SPEC-0023) and `prompt_template.go` (SPEC-0031) both explicitly deferred real token accounting to, and are unblocked now.

## History

- 2026-08-02 SPEC-0031 Prompt Template System — Completed. Added `services/core/prompt_template.go`: `PromptTemplate` (Name/Version/Kind/Body) plus `PromptTemplate.Render(PromptVariables) (string, error)` renders `Body` as a `text/template` with `missingkey=error`, producing the plain string a caller assigns to `GenerateRequest.Prompt` (SPEC-0026/0027) — it does not call into `Provider`/`StreamHandler` itself, keeping rendering and generation separate, caller-composed concerns. "System prompts" and "Agent instructions" are both just a `PromptTemplate` distinguished by `Kind` (`PromptKindSystem`/`PromptKindInstructions`), metadata only. "Dynamic variables" is `PromptVariables` (`UserContext`, `TaskInformation`, `Memories`, `AvailableTools`, `Instructions`, plus a free-form `Extra map[string]string`); `VariablesFromContext(Context) PromptVariables` bridges SPEC-0023's `ContextBuilder` output for the spec's own named example variables (`UserContext` = combined `ContextSectionUserMessage`+`ContextSectionConversationHistory`; `TaskInformation`/`Memories`/`AvailableTools` map to their same-named sections; `PreviousResults` deliberately left unmapped — no named variable for it in the spec). "Prompt versions" is `PromptRegistry`: keeps every registered `(Name, Version)` pair (`Register` rejects a duplicate version with a `packages/errors` `TypeAlreadyExists` error rather than overwriting), with `Get`/`Latest`/`Versions` (sorted ascending) to retrieve them. `Container` gained the same real-slot treatment every LLM-branch service gets as its spec completes: `PromptRegistry *PromptRegistry` + `WithPromptRegistry` in `container.go`/`container_test.go`. `go build ./...`, `go vet ./...`, `go test ./...` clean across all 5 go.work modules (23 new tests in `prompt_template_test.go`: SPEC-0031's three explicit testing criteria plus `Validate` field coverage, malformed/unknown-field template rejection, and the `VariablesFromContext` bridge's populated and empty-`Context` cases; 5x full-package rerun clean). Built on feature/prompt-template-system (off master, post SPEC-0030 merge).

Earlier entries (SPEC-0001 through SPEC-0030): see docs/agents/JARVIS_BUILD_TRACKER.md and `git log` — this file's History section is reset on each feature completion rather than accumulated indefinitely.
