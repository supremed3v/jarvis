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

SPEC-0046 (Tool Execution Engine) is now `Completed` and merged to master —
see History below and its entry in `docs/agents/JARVIS_BUILD_TRACKER.md` for
the full record. This continues the Tools branch of Phase 4 Intelligence
(SPEC-0043 through SPEC-0052), with SPEC-0043 (Tool Interface), SPEC-0044
(Tool Manifest System), SPEC-0045 (Tool Registry), and SPEC-0046 all now
done.

Next candidate: SPEC-0047 (Tool Permission System) is the natural next step
in the Tools branch — SPEC-0046's `ToolExecutionEngine` already reuses
SPEC-0024's `PermissionChecker` per declared category ad hoc (checking
`ToolMetadata.Permissions` one category at a time, failing closed if no
checker is configured), and SPEC-0047 is expected to formalize that into a
proper permission system. Worth resolving as part of SPEC-0047: the final
review of SPEC-0046 noted a latent, unreconciled divergence between this
category-list-keyed checking and SPEC-0022's existing
`PermissionEnforcedToolCaller` (`agent_permission.go`), which instead keys a
check by the tool's own name — neither is wired to the other today, but
SPEC-0047 is the natural place to settle on one model. SPEC-0056 (Speech To
Text Provider) remains available to continue the Voice branch after
SPEC-0053-0055, and is the previously-noted voice-first MVP priority per
`docs/execution/JARVIS_MVP_SCOPE.md` (voice is a core, required MVP surface,
not optional). Research (SPEC-0073 onward) remains blocked only on
Search/Browser, not on Tools or Memory. Which to pick up next is a
product-priority call for whoever loads the next feature.

## History

- 2026-08-03 SPEC-0046 Tool Execution Engine: loaded via
  `scripts/setup_feature.ps1` (dependencies manually resolved —
  FEATURE_INDEX.md carries no per-spec Dependencies field yet — against
  SPEC-0043/SPEC-0044/SPEC-0045, all Completed; flagged SPEC-0047 Tool
  Permission System, next in sequence, as not yet implemented and not a
  blocker), started on feature/tool-execution-engine, implemented
  `services/core/tool_execution.go` (`ToolExecutionEngine.Execute` sequences
  `ToolRegistry.Lookup` -> `ValidateToolInput` -> `PermissionChecker.Check`
  per declared permission category -> `Tool.Execute`, failing closed if a
  tool declares permissions but no checker is configured) +
  `tool_execution_test.go` (11 tests covering all three SPEC-0046 testing
  criteria plus nil-registry/logging/integration edge cases, 100.0%
  statement coverage), reviewed via an 8-angle automated review (code-review
  skill, medium effort: zero correctness bugs found; four non-blocking
  design notes accepted as future-work observations), marked Completed in
  `JARVIS_BUILD_TRACKER.md`, and merged to master.
