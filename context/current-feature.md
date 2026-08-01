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

- 2026-08-01 SPEC-0015 Task Retry System — Completed. Added `services/core/task_retry.go`: `RetryPolicy`/`RetryManager`, filling the "Retry count / Retry delay / Maximum attempts / Failure reasons" requirements and the "retries must avoid infinite loops" rule by tracking per-task attempt counts and failure reasons in `RetryManager` (mirroring `StateMachine.history`'s pattern) rather than on `types.Task`, so SPEC-0011/0012/0013 needed no changes. `task_worker.go` gained an opt-in `WithRetryManager` option: a failed task is retried via the existing Executing->Waiting->(re-queued)->Executing cycle (publishing the new `TASK_RETRY_SCHEDULED` event) until `MaxAttempts` is reached, then falls through to SPEC-0014's original `Failed` + `TASK_FAILED` path unchanged; a `Worker` with no `RetryManager` configured keeps its original fail-on-first-error behavior. `Worker.Stop` now also waits for in-flight retry goroutines before returning. `go build ./...`, `go vet ./...`, `go test ./... -v` clean across all 5 go.work modules (71/71 tests pass, 7 new). `go test -race` unavailable in this environment (no cgo toolchain), same constraint noted under SPEC-0005/0007/0009/0012/0013/0014. Built on feature/task-retry-system (off master, post SPEC-0014 merge), merged into master. Full rationale: see docs/agents/JARVIS_BUILD_TRACKER.md (SPEC-0015 row).

Earlier entries (SPEC-0001 through SPEC-0014): see docs/agents/JARVIS_BUILD_TRACKER.md and `git log` — this file's History section is reset on each feature completion rather than accumulated indefinitely.
