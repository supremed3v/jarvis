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

- 2026-08-01 SPEC-0022 Agent Execution Loop — Completed. Added `services/core/agent_execution_loop.go`: `ExecutionLoop` filling the seven-stage cycle (Receive Task -> Analyze Context -> Create Plan -> Select Tools -> Execute Actions -> Evaluate Result -> Return Response) of SPEC-0022, the fourth spec of Phase 4 Intelligence, built directly on `Agent`/`Executor` (SPEC-0018/SPEC-0014) with no new dependency on any not-yet-built system. Four caller-supplied function types cover the stages no lower spec defines behavior for yet — there is still no LLM layer, Tool Registry, or Memory system: `ContextAnalyzer` (Analyze Context, optional, defaults to an empty map), `Planner` (Create Plan, required), `ToolCaller` (Execute Actions, required only if a `Plan` ever produces a `Step` naming a tool), and `ResultEvaluator` (Evaluate Result, optional, defaults to treating a non-nil `Step` error as the only failure). "Select Tools" is not a separate hook: a `Step.Tool` field the `Planner` sets when building the `Plan` is what `Execute Actions` reads, since tool selection is inseparable from planning which action a step performs — mirroring SPEC-0018's precedent that planning/tool-use stay internal to an Agent's own `Execute` rather than becoming separate contract surface. `ExecutionLoop.Run` has the exact `Executor`/`Agent.Execute` signature (`func(ctx, *types.Task) (map[string]any, error)`), so `loop.Run` passes directly to `NewWorker`/`NewAgentFromManifest` with no adapter, the same "no adapter needed" precedent SPEC-0018/SPEC-0019 already established. Failure handling is fail-fast (stops at the first Step whose evaluated outcome fails, matching `task_worker.go`'s existing `Executor`-failure precedent); every failure branch (cancellation, Analyze Context, Create Plan, or a Step) routes through one `fail` helper that wraps the cause with `packages/errors` `TypeInternal`/`EXECUTION_LOOP_*` (or `TypeCanceled`/`TypeTimeout`/`EXECUTION_LOOP_CANCELED` for a canceled `ctx`, mapped the same way `Worker.Stop` already maps `ctx.Err()`) plus `taskId`/`step`/`tool`/`stepIndex` context, logs it if a `Logger` is configured, and — critically for "failures return useful results" — `Run` always returns the accumulated `analysis`/`steps` response map alongside the error, not just on success. No `Container` slot was added (same reasoning as SPEC-0021: no pre-reserved placeholder exists for one). `go build ./...`, `go vet ./...`, `go test ./... -v` clean across all 5 go.work modules (12 new tests covering all three SPEC-0022 testing criteria — simple-task completion, tool execution with correct args/output capture, and failures returning useful/contextual results — plus plan/analysis-failure reporting, missing-`ToolCaller` handling, a `ResultEvaluator` rejecting an otherwise-successful step, an empty-`Plan` trivial success, context-cancellation stopping the loop before the next step, and a full `Worker`-driven integration test proving no adapter is needed). `gofmt -l` clean on both new files. Root `go build ./...` still fails with the same pre-existing go.work resolution error noted since SPEC-0009. Reviewed against `docs/agents/CODE_REVIEW_PROTOCOL.md` (Architecture/Code Quality/Security/Testing): found and fixed two Code Quality gaps — failure logging originally covered only Step failures, leaving Analyze Context/Create Plan failures unobserved despite the same `Logger` being configured (fixed by routing every failure branch through the shared `fail` helper); and the loop never checked `ctx` cancellation between stages, so a canceled context could still let a multi-step `Plan` keep calling `ToolCaller` for every remaining Step rather than stopping proactively like `Worker.loop`'s existing `ctx.Done()` check between iterations (fixed by adding explicit cancellation checks before `Run` starts and before each Step, with a regression test). Accepted one known, out-of-scope limitation: `ExecutionLoop` does not check `AgentMetadata.Permissions` before invoking a `ToolCaller`, since no permission-checking system exists anywhere in the codebase yet (SPEC-0018/SPEC-0019 already established Tools/Permissions as bare identifiers a future permission system owns the meaning of) — enforcing permissions here would be new, unrequested scope ahead of that system existing. Approved after fixes; re-verified `go build`/`go vet`/`go test ./... -v` clean and `gofmt -l` clean. Built on feature/agent-execution-loop (off master, post SPEC-0021 merge). Full rationale: see docs/agents/JARVIS_BUILD_TRACKER.md (SPEC-0022 row).

Earlier entries (SPEC-0001 through SPEC-0021): see docs/agents/JARVIS_BUILD_TRACKER.md and `git log` — this file's History section is reset on each feature completion rather than accumulated indefinitely.
