# Current Feature

## Working In

Not specified — no feature currently loaded.

## Status

Not Started

## Goals

_None yet._

## Dependencies

_None yet._

## Notes

Next candidate per docs/execution/JARVIS_IMPLEMENTATION_ORDER.md: SPEC-0038
(Vector Memory Engine), continuing the Memory branch of Phase 4 Intelligence
now that SPEC-0037 (User Profile Memory) is Completed.

## History

- 2026-08-02 SPEC-0037 User Profile Memory — Completed. Implemented
  services/core/user_profile_memory.go (`ProfileCategory`, `ProfileFact`,
  `UserProfileMemory` — a façade over SPEC-0034's `Memory` with
  `Remember`/`Fact`/`Facts`, keeping an in-process Key -> record ID index
  so a second `Remember` under a known Key calls `Memory.Update` in place
  instead of storing a duplicate), 17 new tests, no changes to
  SPEC-0034/0035/0036 contracts or `container.go`. `go build`/`vet`/`test`
  clean across all 5 go.work modules. Review pass found and fixed one gap
  (`Remember`'s check-then-act on the Key index wasn't atomic, risking an
  orphaned record under concurrent calls on the same new Key) before
  completion. Merged feature/user-profile-memory into master.
