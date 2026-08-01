# Current Feature: SPEC-0011 Task Model

## Working In

Phase 3 (Execution) — packages/shared-types (extend existing `Task` struct) and/or a new services/core task package, per ADR-0007 (Tasks live in relational storage).

## Status

In Progress

## Goals

- Define the full Task data model: ID, Title, Description, Source, Priority, Status, Created timestamp, Updated timestamp, Parent task reference, Metadata.
- Support Task Sources: Voice input, Desktop input, Agent generated, Scheduled.
- Verify tasks can be created, serialize correctly, and preserve metadata.

## Dependencies

- SPEC-0001 Repository Foundation (status: Completed)
- SPEC-0002 Development Environment (status: Completed)
- SPEC-0003 Configuration System (status: Completed)
- SPEC-0004 Shared Types Package (status: Completed)
- SPEC-0005 Logging System (status: Completed)
- SPEC-0006 Error Handling System (status: Completed)
- SPEC-0007 Go Runtime Bootstrap (status: Completed)
- SPEC-0008 Dependency Container (status: Completed)
- SPEC-0009 Event Bus (status: Completed)
- SPEC-0010 Internal Message Protocol (status: Completed)

All of Phase 1 (Foundation) and Phase 2 (Runtime) are Completed per FEATURE_INDEX.md, so SPEC-0011 is unblocked as the first spec of Phase 3 (Execution).

## Notes

Specification:

context/features/SPEC-0011-task-model.md

Dependency resolution source: Implementation Order (Phase 3 Execution: 0011 Task Model -> 0012 State Machine -> 0013 Queue -> 0014 Workers -> 0015 Retry -> 0016 Scheduler) plus Dependency Graph (`Foundation -> Runtime -> Tasks`).

**Task struct overlap — resolved:** user chose "extend in place" (additive). `packages/shared-types/task.go`'s existing fields (`Type, Input, Result, Error`) were kept as-is; SPEC-0011's required fields were added alongside them: `Title, Description, Source (TaskSource), Priority (TaskPriority), ParentID, Metadata`.

`TaskSource` is a closed 4-value enum (`voice, desktop, agent, scheduled`) since SPEC-0011 itself enumerates these values. `TaskPriority` is a bare string type with **no** hardcoded constants — concrete levels (`LOW/NORMAL/HIGH/CRITICAL`) are explicitly SPEC-0013 Task Queue's "Priority Levels" section, not this spec's; this mirrors the SPEC-0004 precedent of `EventType` having no hardcoded constants since concrete event names belong to later specs.

ADR-0007 (Storage Architecture) places Tasks in the relational storage layer — relevant once persistence is implemented (likely a later spec), not for this data-model-only spec.

## Implementation Plan (executed)

1. Add `TaskSource` type + 4 constants and `TaskPriority` type (no constants) to `packages/shared-types/task.go`.
2. Extend `Task` struct with `Title, Description, Source, Priority, ParentID, Metadata` (all new optional fields `omitempty` except `Title`/`Source`, matching the spec's required-vs-optional shape).
3. Update `TestTask_JSONRoundTrip` to cover the new fields; add `TestTask_SourceValues` (enum non-empty/uniqueness, matching `TestTask_StatusValues`/`TestMessage_TypeValues` convention) and `TestTask_EmptyOptionalFieldsOmitted` (all new omitempty fields plus existing `input/result/error`).

Files modified: `packages/shared-types/task.go`, `packages/shared-types/types_test.go`. No new files; no services/agents/apps touched (data-model-only spec).

## Testing

`go build ./...`, `go vet ./...`, `go test ./... -v` clean across all 5 go.work modules (packages/config, packages/errors, packages/logger, packages/shared-types, services/core) — no regressions. `gofmt -l` flags every file in packages/shared-types, but `gofmt -d` confirms it's the pre-existing CRLF/LF line-ending artifact noted under SPEC-0008/SPEC-0007, not real formatting issues from this change.

## History

- 2026-08-01 load loaded SPEC-0011
- 2026-08-01 start: branch `feature/task-model` created; extended `Task`/added `TaskSource`/`TaskPriority` in packages/shared-types per SPEC-0011; tests added; all 5 modules build/vet/test clean. Status still In Progress — test/review/complete actions not yet run.
