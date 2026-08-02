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

SPEC-0043 (Tool Interface) is now `Completed` and merged to master — see
History below and its entry in `docs/agents/JARVIS_BUILD_TRACKER.md` for
the full record. This starts the Tools branch of Phase 4 Intelligence
(SPEC-0043 through SPEC-0052).

Next candidate: SPEC-0044 (Tool Manifest System) is the natural next step
in the Tools branch — SPEC-0043's own build note names it as the spec that
must produce a runnable `Tool` from a loaded manifest, the same
manifest-on-top-of-an-interface pattern SPEC-0019 (Agent Manifest System)
already established for `Agent`/SPEC-0018. SPEC-0056 (Speech To Text
Provider) remains available to continue the Voice branch after
SPEC-0053-0055, and is the previously-noted voice-first MVP priority per
`docs/execution/JARVIS_MVP_SCOPE.md` (voice is a core, required MVP
surface, not optional). Research (SPEC-0073 onward) remains blocked only
on Search/Browser, not on Tools or Memory. Which to pick up next is a
product-priority call for whoever loads the next feature.

## History

- 2026-08-03 SPEC-0043 Tool Interface: loaded (dependencies manually
  resolved via Implementation Order + Dependency Graph + build-tracker
  cross-reference, since FEATURE_INDEX.md carries no per-spec Dependencies
  field yet — confirmed SPEC-0018/SPEC-0024 both `Completed`), started on
  feature/tool-interface, implemented `services/core/tool.go`
  (`ToolMetadata`, `Schema`/`SchemaField`, `Tool` interface,
  `ValidateToolInput` helper) + `tool_test.go` (11 tests including an
  end-to-end integration test proving a `Tool` drives a real
  `ExecutionLoop` via its `ToolCaller` seam with only a name-dropping
  wrapper), reviewed (one real gofmt misalignment found and fixed;
  verdict: Ready to complete), marked Completed in
  `JARVIS_BUILD_TRACKER.md`, and merged to master.
