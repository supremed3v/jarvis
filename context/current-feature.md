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

SPEC-0045 (Tool Registry) is now `Completed` and merged to master — see
History below and its entry in `docs/agents/JARVIS_BUILD_TRACKER.md` for
the full record. This continues the Tools branch of Phase 4 Intelligence
(SPEC-0043 through SPEC-0052), with SPEC-0043 (Tool Interface), SPEC-0044
(Tool Manifest System), and SPEC-0045 all now done.

Next candidate: SPEC-0046 (Tool Execution Engine) is the natural next step
in the Tools branch — it's the execution layer (validate input, check
permission, execute, return result/error) that runs the `Tool`s
SPEC-0045's `ToolRegistry` now holds, and its Requirements name Permission
checking explicitly, which is where SPEC-0024's `PermissionChecker` (left
undelegated by both SPEC-0043 and SPEC-0044) is expected to finally get
wired in. SPEC-0056 (Speech To Text Provider) remains available to
continue the Voice branch after SPEC-0053-0055, and is the previously-noted
voice-first MVP priority per `docs/execution/JARVIS_MVP_SCOPE.md` (voice
is a core, required MVP surface, not optional). Research (SPEC-0073
onward) remains blocked only on Search/Browser, not on Tools or Memory.
Which to pick up next is a product-priority call for whoever loads the
next feature.

## History

- 2026-08-03 SPEC-0045 Tool Registry: loaded via `scripts/setup_feature.ps1`
  (dependencies manually resolved — FEATURE_INDEX.md carries no per-spec
  Dependencies field yet — against SPEC-0044's own build note naming Tool
  Registry as its next consumer; confirmed SPEC-0043/SPEC-0044 both
  Completed), started on feature/tool-registry, implemented
  `services/core/tool_registry.go` (`ToolRegistry` interface —
  Register/Lookup/Remove/List/IsAvailable — and its in-memory
  `ToolRegistryStore`, mirroring SPEC-0020's AgentRegistry/Registry pattern
  plus an IsAvailable method for the "Tool availability checks"
  requirement) + `tool_registry_test.go` (8 tests covering all three
  SPEC-0045 testing criteria plus not-found/remove/empty-list/concurrency
  edge cases), replaced the `Container.ToolRegistry` `interface{}`
  placeholder in `container.go` with the real interface and updated
  `container_test.go` accordingly, reviewed (no issues found; verdict:
  Approved, ready to complete), marked Completed in
  `JARVIS_BUILD_TRACKER.md`, and merged to master.
