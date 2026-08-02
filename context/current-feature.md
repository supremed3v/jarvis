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

SPEC-0041 (Memory Retrieval System) is now `Completed` and merged to master —
see History below and its entry in `docs/agents/JARVIS_BUILD_TRACKER.md` for
the full record.

Next candidate: SPEC-0042 (Memory Consolidation Engine) is the natural
continuation of the Memory branch of Phase 4 Intelligence — it sits directly
on top of the `Memory`/`VectorStore`/`MemoryRetriever` work SPEC-0034 through
SPEC-0041 just completed, and would be the spec that starts actually
computing/updating the `MemoryRecord.Metadata["importance"]` convention
`MemoryRetriever` (SPEC-0041) reads but does not itself populate. SPEC-0056/
0057 (STT Provider interface, Whisper Integration) also remain unblocked and
were the previously-noted voice-first MVP priority per
`docs/execution/JARVIS_MVP_SCOPE.md`; the Tools layer (SPEC-0043-0052) is
unblocked as well. Which to pick up next is a product-priority call for
whoever loads the next feature.

## History

- 2026-08-03 SPEC-0041 Memory Retrieval System: loaded, started on
  feature/memory-retrieval-system, implemented `services/core/memory_retrieval.go`
  (`MemoryRetriever`/`RetrievalRequest`) + `memory_retrieval_test.go` (9 tests),
  wired into `container.go`, reviewed (verdict: Ready to complete), marked
  Completed in `JARVIS_BUILD_TRACKER.md`, and merged to master.
