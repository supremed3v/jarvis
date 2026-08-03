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

SPEC-0050 (Terminal Tool) is now `Completed` and merged to master — see
History below and its entry in `docs/agents/JARVIS_BUILD_TRACKER.md` for the
full record. This continues the Tools branch of Phase 4 Intelligence
(SPEC-0043 through SPEC-0052), with SPEC-0043 through SPEC-0050 all now
done.

Next candidate: SPEC-0052 (Git Tool) is the natural next step in the Tools
branch — same shape as SPEC-0049/SPEC-0050 (a concrete Tool built on the
now-complete Tool Interface/Manifest/Registry/Execution Engine/Permission
System/Approval Workflow layer), and its Requirements (repository
inspection, branch information, commit history, status checks, diff
retrieval) map onto read-only `git` subcommands that could even be built
directly on top of SPEC-0050's new `terminal.exec` Tool (an `AllowedCommands`
allowlist of `git` plus argument-shape validation) rather than shelling out
independently — worth confirming during that spec's load/start. SPEC-0051
(Browser Automation Tool) is also Planned and would complete the Tools
branch, but requires Playwright (ADR-0006) integration, which has no
existing wiring anywhere in this pure-Go workspace yet — a bigger,
cross-stack undertaking than SPEC-0052, so likely worth deferring until
Node/Electron-side infrastructure exists (see SPEC-0063+ Application layer)
or is otherwise justified. SPEC-0056 (Speech To Text Provider) remains
available to continue the Voice branch after SPEC-0053-0055, and is the
previously-noted voice-first MVP priority per
`docs/execution/JARVIS_MVP_SCOPE.md` (voice is a core, required MVP surface,
not optional). Research (SPEC-0073 onward) remains blocked only on
Search/Browser, not on Tools or Memory. Which to pick up next is a
product-priority call for whoever loads the next feature.

## History

- 2026-08-03 SPEC-0050 Terminal Tool: loaded via `/feature load` (index
  read first per `actions/load.md`; dependencies manually resolved —
  FEATURE_INDEX.md carries no per-spec Dependencies field yet — against
  SPEC-0043 through SPEC-0048, all Completed), started on
  feature/terminal-tool, implemented `services/core/tool_terminal.go` (two
  Tools — `terminal.exec` with an `AllowedCommands` allowlist for the "safe"
  command set, and `terminal.exec.privileged` with no allowlist, declaring a
  separate permission category so it can be configured
  `PermissionApprovalRequired` for the "dangerous" set — since
  `ToolExecutionEngine` checks a Tool's declared Permissions once from
  static Metadata before Execute runs, not per-invocation, a single Tool
  can't vary its approval requirement by command) + `tool_terminal_test.go`
  (covering all three SPEC-0050 testing criteria, including an end-to-end
  approval-flow integration test through a real `ToolExecutionEngine`+
  `PermissionChecker`+`ApprovalQueue`, plus args-forwarding, cancellation,
  logging, and permission-declaration edge cases added during review to
  match SPEC-0049's own coverage depth), reviewed against
  `docs/agents/CODE_REVIEW_PROTOCOL.md` (architecture fit confirmed, no
  scope creep; two non-blocking hardening notes accepted as future-work
  observations, mirroring SPEC-0049's own — command-name-only restriction,
  case-sensitive matching), marked Completed in `JARVIS_BUILD_TRACKER.md`,
  regenerated `FEATURE_INDEX.md`, and merged to master.
