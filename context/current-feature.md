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

- 2026-08-01 SPEC-0016 Background Scheduler — Completed. Added `services/core/task_scheduler.go`: `Scheduler`, filling the "One-time tasks" / "Recurring tasks" / "Delayed execution" requirements by producing `Task`s on a schedule rather than executing them — it drives each fired `Task` through `StateMachine` (SPEC-0012) `Created->Planning->Queued` transitions and hands it to `Queue.Add` (SPEC-0013) for an existing `Worker` (SPEC-0014) to pick up, so `Worker`/`Queue`/`StateMachine`/`RetryManager` (SPEC-0011-0015) needed no changes. Three constructors build a `Schedule` around a caller-supplied `TaskFactory func() *types.Task`: `ScheduleOnce(id, at, factory)`, `ScheduleAfter(id, delay, factory)`, `ScheduleEvery(id, interval, factory)` — the `scheduleKind` discriminant is unexported so callers can't construct a mismatched kind/duration pair directly. `Start`/`Stop` follow `Worker`'s own lifecycle idiom (context-derived cancellation, `SCHEDULER_STOP_INCOMPLETE`/`SCHEDULER_ALREADY_STARTED`); a background `time.Ticker` (`WithSchedulerTick`-configurable) drives `fireDue`, which advances/removes due entries under the Scheduler's own mutex before firing any of them outside that lock, so a concurrent `Cancel` can never race a firing already in flight. Publishes new `TASK_SCHEDULED`/`SCHEDULED_TASK_FIRED`/`SCHEDULED_TASK_CANCELED` events on the `EventBus` (SPEC-0009). A firing that fails (bad transition, or `Queue.Add` rejecting a duplicate task ID from a buggy `TaskFactory`) is logged and dropped rather than retried inline. `container.go`'s `TaskManager` placeholder was left untouched, for the same reason SPEC-0012-0015 left it alone. `go build ./...`, `go vet ./...`, `go test ./... -v` clean across all 5 go.work modules (16 new tests: one-time/delayed/recurring firing, cancellation, event emission, input validation, duplicate-ID/unknown-cancel rejection, double-start/idempotent-stop, dynamic scheduling against an already-running Scheduler, failed-firing resilience, and a full `Scheduler`->`Queue`->`Worker` integration test). `go test -race` unavailable in this environment (no cgo toolchain), same constraint noted under SPEC-0005/0007/0009/0012/0013/0014/0015. Built on feature/background-scheduler (off master, post SPEC-0015 merge). Full rationale: see docs/agents/JARVIS_BUILD_TRACKER.md (SPEC-0016 row).

Earlier entries (SPEC-0001 through SPEC-0015): see docs/agents/JARVIS_BUILD_TRACKER.md and `git log` — this file's History section is reset on each feature completion rather than accumulated indefinitely.
