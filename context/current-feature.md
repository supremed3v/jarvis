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

- 2026-08-01 SPEC-0012 Task State Machine — Completed. Replaced `packages/shared-types/task.go`'s provisional 5-value `TaskStatus` (`pending/running/completed/failed/cancelled`, added under SPEC-0011 with a comment deferring the real enum to this spec) with the 8 states SPEC-0012 requires — `created, planning, queued, executing, waiting, failed, completed, cancelled` (lowercase, matching the `TaskSource`/`MessageType`/`AgentStatus` convention; confirmed with user since nothing outside shared-types referenced the old names). Added `TaskTransition` (TaskID/From/To/Timestamp) as a shared data shape for a single recorded state change, mirroring the `Event` precedent. Transition *validation* and *history tracking* — real behavior, which SPEC-0004 explicitly keeps out of packages/shared-types ("shapes only, no business logic") — live instead in new `services/core/task_state_machine.go`: a closed `validTransitions` adjacency table, `CanTransition(from,to)`, and `StateMachine` (`Transition(task,to)` validates+mutates+records via `packages/errors` `TypeInvalidInput`/`TASK_INVALID_TRANSITION` on rejection; `History(taskID)` returns a defensive copy). Placed in `services/core` per `JARVIS_MASTER_ARCHITECTURE.md` ("Core Runtime: task execution") and `container.go`'s existing `TaskManager` placeholder comment; `container.go` itself was not touched, since that slot covers the full multi-spec Task Execution layer, not SPEC-0012 alone. `go build ./...`, `go vet ./...`, `go test ./... -v` clean across all 5 go.work modules; a review pass caught the `StateMachine` doc comment overclaiming full concurrent-use safety (the `history` map is mutex-protected, but `Transition` mutates the caller's `*Task` outside the lock) — fixed by narrowing the doc comment. `go test -race` unavailable in this environment (no cgo toolchain), same constraint noted under SPEC-0005/0007/0009. Built on feature/task-state-machine (off feature/task-model), merged into master. Full rationale: see docs/agents/JARVIS_BUILD_TRACKER.md (SPEC-0012 row).

Earlier entries (SPEC-0001 through SPEC-0011): see docs/agents/JARVIS_BUILD_TRACKER.md and `git log` — this file's History section was not carried forward intact through the SPEC-0011 commit, so it is not reproduced here rather than reconstructed inexactly.
