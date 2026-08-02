# Current Feature: Token Budget Manager

## Working In

services/core (Phase 4 Intelligence, LLM branch — 7th spec after SPEC-0026
through SPEC-0032), alongside `context_window_manager.go` (SPEC-0032) and
`packages/config` (SPEC-0028).

## Status

In Progress

## Goals

- Track input tokens, output tokens, context usage, and model limits
- Support token estimation, budget warnings, and context reduction strategies
- Resolve what a `WindowManager.Fit(Context, budget int)` (SPEC-0032) budget
  should actually be for a given model/agent — the resolution SPEC-0032
  explicitly left unbuilt for this spec to own

## Dependencies

- SPEC-0028 Model Configuration System (status: Completed) — `packages/config`
  `Model.MaxTokens` caps generated output length only, not input context size
- SPEC-0032 Context Window Manager (status: Completed) — `services/core`
  `WindowManager.Fit(Context, budget int) (Context, Usage)` takes budget as a
  caller-supplied int; this spec owns resolving that int (model lookup,
  defaults, per-agent overrides)

Both dependencies are Completed — SPEC-0033 is unblocked.

## Notes

Specification:

context/features/SPEC-0033-token-budget-manager.md

Dependency resolution source: JARVIS_BUILD_TRACKER.md (SPEC-0032 and SPEC-0028
row text) — `JARVIS_IMPLEMENTATION_ORDER.md`/`JARVIS_DEPENDENCY_GRAPH.md` only
resolve to the phase level ("Phase 4 Intelligence: LLM"), not per-spec, so
Step 4's fallback chain (build tracker + spec Requirements inference) was
used instead of an explicit Dependencies field.

Index status at load time (FEATURE_INDEX.md): Planned. No `Related` field
present in the index entry.

## Files Modified

- `services/core/token_budget_manager.go` (new) — `BudgetManager`: `Limit`
  (resolves an agent's model via `ModelConfig.ModelFor`, SPEC-0028, then its
  context window size via `Provider.ListModels`'s `ModelInfo.ContextSize`,
  SPEC-0026/27, falling back to a configurable default), `EstimateTokens`
  (reuses SPEC-0032's `TokenEstimator`), `Record`/`Report`/`Reset` (per-agent
  cumulative input/output token tracking plus `BudgetStatus`
  ok/warning/exceeded classification, logged when not ok), `ReduceContext`
  (composes `Limit` with a `WindowManager.Fit`, SPEC-0032).
- `services/core/token_budget_manager_test.go` (new) — covers all three
  SPEC-0033 testing criteria plus edge cases (nil-option fallback, per-agent
  isolation, `Limit` fallback chain, unresolvable-model errors).
- `services/core/container.go` — added `BudgetManager *BudgetManager` slot +
  `WithBudgetManager` `ContainerOption`, following the same real-slot
  treatment every LLM-branch spec (SPEC-0026 onward) has gotten as it
  completed.
- `services/core/container_test.go` — extended the unwired-slots-default-to-nil
  and options-wire-stub-slots tests to cover the new slot.

## History

- 2026-08-02 load loaded SPEC-0033 (SPEC-0033-token-budget-manager.md)
- 2026-08-02 start implemented `BudgetManager` on branch
  `feature/token-budget-manager`; `go build ./...`, `go vet ./...`,
  `go test ./...` clean across all 5 go.work modules via
  `scripts/go_all.ps1`; `gofmt -l` clean on both new files (one genuine
  non-CRLF alignment issue in the test file found and fixed via
  `gofmt -w`); `container.go`/`container_test.go` still flag under `gofmt -l`
  but only the same pre-existing CRLF/LF artifact noted since SPEC-0007.
- 2026-08-02 review — reviewed against `docs/agents/CODE_REVIEW_PROTOCOL.md`
  (Architecture/Code Quality/Security/Testing): architecture confirmed (sits
  on `ModelConfig.ModelFor`/SPEC-0028, `Provider.ListModels`/SPEC-0026-27,
  `WindowManager.Fit`/SPEC-0032 without modifying any of them beyond the
  additive `Container` slot; no new third-party dependency); no security
  concerns (`Record`'s warning log carries only counts/status, never prompt
  content, matching `StreamHandler`'s SPEC-0030 precedent). Found and fixed
  two issues before approval: (1) Code Quality — `resolveModel` propagated
  `ModelConfig.ModelFor`'s bare `fmt.Errorf` unchanged, the only
  services/core component that would have returned an error with no
  `packages/errors` `Type` for a caller to branch on; fixed by wrapping it
  via `errors.Wrap(..., TypeNotFound, "BUDGET_MANAGER_MODEL_UNRESOLVED", ...)`
  matching `ModelRouter`'s `MODEL_ROUTER_NO_MODEL` precedent (SPEC-0029),
  plus a regression test asserting `errors.Is(err, errors.TypeNotFound)`.
  (2) Testing — `TestBudgetManager_Record_LogsWarningOnlyWhenNotOK` asserted
  only that `Record` didn't error/panic, never actually inspecting log
  output despite its name; fixed using the same `logger.New("test",
  logger.WithOutput(&buf))` + buffer-content-assertion pattern
  `TestWindowManager_Fit_LogsTrimming` (SPEC-0032) already established,
  plus a new `TestBudgetManager_Record_NoLoggerRunsSilently` covering the
  nil-logger path the original test's name had claimed to cover instead.
  Re-verified `go build`/`go vet`/`go test ./...` clean across all 5
  go.work modules and `gofmt -l` clean on both new files after both fixes.
  All three SPEC-0033 testing criteria (usage tracked, limits trigger
  correctly, reduction strategies execute) verified present and passing.
  Approved after fixes.

Earlier entries (SPEC-0001 through SPEC-0032): see docs/agents/JARVIS_BUILD_TRACKER.md
and `git log` — this file's History section is reset on each feature
completion rather than accumulated indefinitely.
