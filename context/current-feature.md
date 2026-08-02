# Current Feature

## Overview

Not started — no feature currently loaded. Next candidate per
docs/execution/JARVIS_IMPLEMENTATION_ORDER.md: SPEC-0034 (Memory Interface),
the first spec of Phase 4 Intelligence's Memory branch, now that SPEC-0033
(Token Budget Manager) is Completed and the LLM branch (SPEC-0026 through
SPEC-0033) is entirely Completed.

## Status

Not Started

## Goals

_None yet._

## Files Modified

_None yet._

## Notes

SPEC-0033 (Token Budget Manager) is now Completed (see
docs/agents/JARVIS_BUILD_TRACKER.md, SPEC-0033 row, for full
implementation/review rationale), completing Phase 4 Intelligence's LLM
branch in full. SPEC-0034 (Memory Interface) is not causally unblocked by
SPEC-0033 the way SPEC-0033 was by SPEC-0032/SPEC-0028 — per
docs/execution/JARVIS_DEPENDENCY_GRAPH.md, Memory is a sibling branch of LLM
(both depend only on the completed Agent layer, SPEC-0018 through SPEC-0025),
not a downstream consumer of it. SPEC-0034 is simply the next spec in
implementation order, defining the abstraction layer (store/retrieve/
search/update/delete) the concrete Memory specs (SPEC-0035 onward: storage
abstraction, conversation memory, user profile memory, vector engine,
embedding pipeline) will implement against.

## History

- 2026-08-02 SPEC-0033 Token Budget Manager — Completed. Added
  `services/core/token_budget_manager.go`: `BudgetManager` resolves a
  per-agent token budget by combining `ModelConfig.ModelFor` (SPEC-0028)
  with `Provider.ListModels`'s `ModelInfo.ContextSize` (SPEC-0026/27) —
  falling back to a configurable default when no Provider is set or it
  reports no match — the exact resolution `WindowManager.Fit` (SPEC-0032)
  deliberately left unbuilt for this spec to own. `Record` accumulates
  per-agent cumulative input/output token usage and classifies it into
  `BudgetOK`/`BudgetWarning`/`BudgetExceeded` (default 80% warn threshold),
  logging (counts/status only, never content) when not OK; `Report`/`Reset`
  read/clear that state. `EstimateTokens` reuses SPEC-0032's own
  `TokenEstimator` type rather than redefining one. `ReduceContext` composes
  the resolved limit with a `WindowManager.Fit` call — the "context
  reduction strategies" requirement. `Container` gained
  `BudgetManager *BudgetManager` + `WithBudgetManager`, following the same
  real-slot treatment every LLM-branch spec has gotten as it completed.
  Reviewed against `docs/agents/CODE_REVIEW_PROTOCOL.md` (Architecture/Code
  Quality/Security/Testing): found and fixed two issues — (1) `resolveModel`
  was propagating `ModelConfig.ModelFor`'s bare `fmt.Errorf` unwrapped, the
  only services/core component that would return an error with no
  `packages/errors` `Type`; fixed via `errors.Wrap(..., TypeNotFound,
  "BUDGET_MANAGER_MODEL_UNRESOLVED", ...)`, matching `ModelRouter`'s
  `MODEL_ROUTER_NO_MODEL` precedent, plus a regression test. (2) a test
  named `..._LogsWarningOnlyWhenNotOK` never actually inspected log output;
  fixed using the same buffer-capture pattern
  `TestWindowManager_Fit_LogsTrimming` (SPEC-0032) established, plus a new
  `NoLoggerRunsSilently` test. Approved after fixes. `go build ./...`,
  `go vet ./...`, `go test ./...` clean across all 5 go.work modules;
  `gofmt -l` clean on both new files. Built on feature/token-budget-manager
  (off master, post SPEC-0032 merge).

Earlier entries (SPEC-0001 through SPEC-0033): see
docs/agents/JARVIS_BUILD_TRACKER.md and `git log` — this file's History
section is reset on each feature completion rather than accumulated
indefinitely.
