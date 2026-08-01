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

- 2026-08-01 SPEC-0014 Task Worker System — Completed. Added `services/core/task_worker.go`: `Worker`, filling the "Pull tasks from queue / Execute assigned work / Update task status / Report failures / Emit events" requirements by tying together `Queue` (SPEC-0013), `StateMachine` (SPEC-0012), and `EventBus` (SPEC-0009) for the first time. `Start`/`Stop` follow `Bus`'s own lifecycle idiom (context-derived cancellation, `Stop` racing a `done` channel against the caller's `ctx`, returning `packages/errors` `TypeTimeout`/`TypeCanceled` `WORKER_STOP_INCOMPLETE` on timeout — mirroring `EVENTBUS_CLOSE_INCOMPLETE`); `Start` rejects a second concurrent start (`WORKER_ALREADY_STARTED`), `Stop` is idempotent. The loop polls `Queue.Next()` (configurable interval, default 20ms) and runs each task through a caller-supplied `Executor func(ctx, *types.Task) (map[string]any, error)`, transitioning Queued->Executing->Completed/Failed via `StateMachine` and publishing `TASK_STARTED`/`TASK_COMPLETED`/`TASK_FAILED` events; a task that can't validly reach Executing is reported failed without ever calling the `Executor`. `container.go`'s `TaskManager` placeholder was left untouched, same rationale SPEC-0012/0013 used. `go build ./...`, `go vet ./...`, `go test ./... -v`, `gofmt -l` clean across all 5 go.work modules (10 new tests). `go test -race` unavailable in this environment (no cgo toolchain), same constraint noted under SPEC-0005/0007/0009/0012/0013. Built on feature/task-worker-system (off master, post SPEC-0013 merge), merged into master. Full rationale: see docs/agents/JARVIS_BUILD_TRACKER.md (SPEC-0014 row).

Earlier entries (SPEC-0001 through SPEC-0013): see docs/agents/JARVIS_BUILD_TRACKER.md and `git log` — this file's History section is reset on each feature completion rather than accumulated indefinitely.
