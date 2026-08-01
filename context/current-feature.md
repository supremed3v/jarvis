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

- 2026-08-01 SPEC-0017 Task History Storage — Completed. Added `services/core/task_history.go`: `HistoryStore`, filling the "Task execution timeline" / "Status changes" / "Tool executions" / "Errors" / "Results" requirements — the final piece of the Task Execution layer `container.go`'s `TaskManager` placeholder comment names (SPEC-0011..0017). Rather than a new write path, `HistoryStore` subscribes to the lifecycle events the layer already publishes (`TASK_SCHEDULED`/`SCHEDULED_TASK_FIRED`/`SCHEDULED_TASK_CANCELED`, `TASK_STARTED`/`TASK_COMPLETED`/`TASK_FAILED`, `TASK_RETRY_SCHEDULED`) via `EventBus` (SPEC-0009) and records each one keyed by the event's `taskId` payload field, so `Queue`/`StateMachine`/`RetryManager`/`Scheduler` (SPEC-0011-0013/0015/0016) needed no changes; the event type set is overridable via `WithHistoryEventTypes` so a later Task-scoped producer can be historized without changing `HistoryStore` itself. One existing file needed a small change: `task_worker.go`'s `EventTaskCompleted` publish previously omitted the task's result, so a `"result": task.Result` payload key was added (additive, not a schema change - `Event.Payload` is already `map[string]any`). `History(taskID)` sorts by each event's own `Timestamp` rather than delivery order, since different event types are delivered on independent per-subscription goroutines and aren't guaranteed to arrive in publish order. A review pass caught and fixed one real bug before completion: `record()` originally stored `event.Payload` by reference rather than copying it, so a "persisted" `HistoryRecord` was actually still aliased to the same mutable map the publisher held - fixed with a shallow copy at record time plus a regression test. `container.go`'s `TaskManager` placeholder was left untouched, for the same reason SPEC-0012-0016 left it alone; `HistoryStore` is in-memory, matching every other Task Execution component's existing precedent, since no relational storage dependency exists anywhere in the repo yet despite ADR-0007 nominally calling Task data "Relational" - a gap that's pre-existing across the whole layer, not introduced here. `go build ./...`, `go vet ./...`, `go test ./... -v` clean across all 5 go.work modules (12 new/updated tests covering happy paths, errors, edge cases, payload non-aliasing, and integration through real `Worker`/`RetryManager`/`Scheduler`). `go test -race` unavailable in this environment (no cgo toolchain), same constraint noted under SPEC-0005/0007/0009/0012/0013/0014/0015/0016. Built on feature/task-history-storage (off master, post SPEC-0016 merge). Full rationale: see docs/agents/JARVIS_BUILD_TRACKER.md (SPEC-0017 row).

Earlier entries (SPEC-0001 through SPEC-0016): see docs/agents/JARVIS_BUILD_TRACKER.md and `git log` — this file's History section is reset on each feature completion rather than accumulated indefinitely.
