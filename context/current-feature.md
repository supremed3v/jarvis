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

- 2026-08-01 SPEC-0013 Task Queue — Completed. Added `services/core/task_queue.go`: `Queue`, an in-memory, priority-ordered holding area for `*types.Task` (SPEC-0011), filling the "Adding tasks / Removing tasks / Priority ordering / Queue inspection / Worker consumption" requirements. Concrete `TaskPriority` values (`PriorityLow/Normal/High/Critical`) are defined here rather than in `packages/shared-types/task.go`, mirroring `eventbus.go`'s precedent of owning concrete enum constants outside the shape-only SPEC-0004 package — `task.go`'s own SPEC-0011 comment already deferred priority levels to this spec. `Queue` buckets tasks by priority tier behind a `sync.Mutex`: `Add` (rejects duplicate task IDs and unrecognized priorities; unset `Priority` defaults to `PriorityNormal` without mutating the caller's `Task`), `Next` (pops highest-tier, FIFO-within-tier, for worker consumption), `Remove` (dequeue by ID), `List` (priority-ordered inspection without removing), `Len`. Deliberately does not enforce `TaskStatus` transitions or integrate with `StateMachine` (SPEC-0012) and does not touch `container.go`'s `TaskManager` placeholder — out of this spec's scope, same rationale SPEC-0012 used. `go build ./...`, `go vet ./...`, `go test ./... -v` clean across all 5 go.work modules (12 new tests, including an 8-worker/200-task concurrent drain proving exactly-once delivery); `gofmt -l` clean on both new files. `go test -race` unavailable in this environment (no cgo toolchain), same constraint noted under SPEC-0005/0007/0009/0012. Built on feature/task-queue (off master, post SPEC-0012 merge), merged into master. Full rationale: see docs/agents/JARVIS_BUILD_TRACKER.md (SPEC-0013 row).

Earlier entries (SPEC-0001 through SPEC-0012): see docs/agents/JARVIS_BUILD_TRACKER.md and `git log` — this file's History section is reset on each feature completion rather than accumulated indefinitely.
