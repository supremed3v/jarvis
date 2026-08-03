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

SPEC-0049 (Filesystem Tool) is now `Completed` and merged to master — see
History below and its entry in `docs/agents/JARVIS_BUILD_TRACKER.md` for the
full record. This continues the Tools branch of Phase 4 Intelligence
(SPEC-0043 through SPEC-0052), with SPEC-0043 through SPEC-0049 all now
done.

Next candidate: SPEC-0050 (Terminal Tool) is the natural next step in the
Tools branch — same shape as SPEC-0049 (a concrete Tool built on the now-
complete Tool Interface/Manifest/Registry/Execution Engine/Permission
System/Approval Workflow layer), and SPEC-0049's own "Allowed paths"
pattern (`FilesystemRoots`) may be worth a look before designing SPEC-0050's
analogous command/argument restriction. SPEC-0056 (Speech To Text Provider)
remains available to continue the Voice branch after SPEC-0053-0055, and is
the previously-noted voice-first MVP priority per
`docs/execution/JARVIS_MVP_SCOPE.md` (voice is a core, required MVP surface,
not optional). Research (SPEC-0073 onward) remains blocked only on
Search/Browser, not on Tools or Memory. Which to pick up next is a
product-priority call for whoever loads the next feature.

## History

- 2026-08-03 SPEC-0049 Filesystem Tool: loaded via `/feature load` (index
  read first per `actions/load.md`; dependencies manually resolved —
  FEATURE_INDEX.md carries no per-spec Dependencies field yet — against
  SPEC-0043 through SPEC-0048, all Completed), started on
  feature/filesystem-tool, implemented `services/core/tool_filesystem.go`
  (`FilesystemRoots` "Allowed paths" allowlist enforcing SPEC-0049's
  Security requirement independently of the agent-level PermissionChecker,
  plus five Tools — `filesystem.read`/`.write`/`.list`/`.search`/
  `.metadata` — sharing one internal `filesystemTool` struct) +
  `tool_filesystem_test.go` (covering all three SPEC-0049 testing criteria
  plus list/search/metadata/cancellation/logging/permission-declaration
  edge cases), reviewed against `docs/agents/CODE_REVIEW_PROTOCOL.md`
  (architecture fit confirmed, no scope creep; two non-blocking hardening
  notes accepted as future-work observations — no symlink resolution in
  `FilesystemRoots.Resolve`, and case-sensitive path containment on
  Windows, both fail-safe rather than fail-open), marked Completed in
  `JARVIS_BUILD_TRACKER.md`, regenerated `FEATURE_INDEX.md`, and merged to
  master.
