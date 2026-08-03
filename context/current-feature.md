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

SPEC-0044 (Tool Manifest System) is now `Completed` and merged to master —
see History below and its entry in `docs/agents/JARVIS_BUILD_TRACKER.md`
for the full record. This continues the Tools branch of Phase 4
Intelligence (SPEC-0043 through SPEC-0052), with SPEC-0043 (Tool
Interface) and SPEC-0044 both now done.

Next candidate: SPEC-0045 (Tool Registry) is the natural next step in the
Tools branch — SPEC-0044's own build note names it as the next consumer,
the system that will hold and look up the `Tool` instances
`NewToolFromManifest` produces (the same registry role SPEC-0020 Agent
Registry already fills for `Agent`/SPEC-0018). SPEC-0056 (Speech To Text
Provider) remains available to continue the Voice branch after
SPEC-0053-0055, and is the previously-noted voice-first MVP priority per
`docs/execution/JARVIS_MVP_SCOPE.md` (voice is a core, required MVP
surface, not optional). Research (SPEC-0073 onward) remains blocked only
on Search/Browser, not on Tools or Memory. Which to pick up next is a
product-priority call for whoever loads the next feature.

## History

- 2026-08-03 SPEC-0044 Tool Manifest System: loaded via
  `scripts/setup_feature.ps1` (dependency manually resolved — FEATURE_INDEX.md
  carries no per-spec Dependencies field yet — via Requirements inference
  against the Tools sub-sequence and `services/core/tool.go`'s own build
  note; confirmed SPEC-0043 `Completed`), started on
  feature/tool-manifest-system, implemented `services/core/tool_manifest.go`
  (`ToolManifest`, `LoadToolManifest`, `Validate`, `Metadata`,
  `ToolExecutor`, `NewToolFromManifest`) + `tool_manifest_test.go` (6 tests
  covering all three SPEC-0044 testing criteria plus a minimal-manifest
  edge case and invalid-input rejection), reviewed (no issues found;
  verdict: Ready to complete), marked Completed in `JARVIS_BUILD_TRACKER.md`,
  and merged to master.
