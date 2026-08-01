# Current Feature

## Overview
Not started — no feature currently loaded. Next candidate per docs/execution/JARVIS_IMPLEMENTATION_ORDER.md: SPEC-0030 (Streaming Response Handler), the next spec in Phase 4 Intelligence's LLM branch now that SPEC-0029 (Model Router) is Completed.

## Status
Not Started

## Goals
_None yet._

## Files Modified
_None yet._

## Notes
SPEC-0029 (Model Router) is now Completed (see docs/agents/JARVIS_BUILD_TRACKER.md, SPEC-0029 row, for full implementation/review rationale). SPEC-0030 (Streaming Response Handler) is unblocked and can consume `services/core`'s `Provider.Stream` (SPEC-0026/0027) alongside the new `ModelRouter.Route` (SPEC-0029) to pick a model before streaming a response.

## History

- 2026-08-02 SPEC-0029 Model Router — Completed. Added `services/core/model_router.go`: `ModelRouter.Route(ctx, RouteRequest)` resolves a `packages/config` `Model` in priority order — explicit `UserPreference`, then `ModelConfig.AgentModels[AgentType]` (SPEC-0028), then a router-side `taskModels` table (Task.Type -> `Models` key, covering the coding/conversation/reasoning examples), then `DefaultModel` — falling through rather than erroring on any dangling reference, and only failing if nothing resolves at all. Checks "model availability" via `Provider.ListModels` (SPEC-0026/0027) and falls back to `DefaultModel` (marking `RouteDecision.Fallback`) when the resolved model is absent, without treating a `ListModels` failure itself as unavailability. Every decision is logged via an optional `packages/logger.Logger`. Review pass found and fixed one architecture-fit gap: `Container` (SPEC-0008) gains a real slot for every LLM-branch service as its spec completes (`Provider` got one via SPEC-0026/0027) — `ModelRouter` had skipped that, fixed by adding `Router *ModelRouter` + `WithRouter` to `container.go`/`container_test.go`. `go build ./...`, `go vet ./...`, `go test ./... -v` clean across all 5 go.work modules (17 new tests across 9 functions covering all three SPEC-0029 testing criteria plus dangling-override, default-unavailable, and concurrency edge cases). Built on feature/model-router (off master, post SPEC-0028 merge).

Earlier entries (SPEC-0001 through SPEC-0028): see docs/agents/JARVIS_BUILD_TRACKER.md and `git log` — this file's History section is reset on each feature completion rather than accumulated indefinitely.
