# Current Feature: Terminal Tool

## Working In

services/core (same module as SPEC-0043 through SPEC-0049's Tool layer —
tool.go, tool_manifest.go, tool_registry.go, tool_execution.go,
tool_approval.go, tool_filesystem.go). SPEC-0050 is the next concrete Tool
in this module, following the tool_filesystem.go pattern
(filesystemTool-style struct implementing the Tool interface, registered
against the Tool Registry, permission/approval-checked via the existing
Permission System and Approval Workflow).

## Status

In Progress

## Goals

- Command execution
- Process output capture
- Exit code handling
- Execution timeout
- Command restrictions (security)
- Approval requirements (security)
- Execution logging (security)

## Dependencies

- SPEC-0043 Tool Interface (status: Completed)
- SPEC-0044 Tool Manifest System (status: Completed)
- SPEC-0045 Tool Registry (status: Completed)
- SPEC-0046 Tool Execution Engine (status: Completed)
- SPEC-0047 Tool Permission System (status: Completed)
- SPEC-0048 Tool Approval Workflow (status: Completed)

All six resolved dependencies are Completed per FEATURE_INDEX.md — no
blockers. SPEC-0049 (Filesystem Tool) is not a hard dependency but is the
closest sibling implementation and worth reviewing for the established
concrete-Tool pattern (its `FilesystemRoots` allowlist is the analogue for
this spec's "Command restrictions" requirement).

## Notes

Specification:

context/features/SPEC-0050-terminal-tool.md

Index status at load time: Planned

Dependency resolution source: FEATURE_INDEX.md carries no per-spec
Dependencies field yet, so resolved manually per load.md Step 4 —
JARVIS_IMPLEMENTATION_ORDER.md places SPEC-0050 in the same Tools branch of
Phase 4 Intelligence as SPEC-0043 through SPEC-0049 (JARVIS_DEPENDENCY_GRAPH.md
has no SPEC-level entries, only phase-level: Tasks -> Agents -> Tools);
requirements text ("Command execution", "Command restrictions", "Approval
requirements", "Execution logging") maps directly onto the existing Tool
Interface / Permission System / Approval Workflow specs already implemented
in services/core.

Related specs: SPEC-0049 Filesystem Tool (sibling concrete Tool, same layer,
Completed) — pattern reference only, not a dependency.

## History

- 2026-08-03 08:02 setup_feature.ps1 loaded SPEC-0050 (SPEC-0050-terminal-tool.md)
- 2026-08-03 /feature load: read FEATURE_INDEX.md first (per actions/load.md
  Step 2), then SPEC-0050-terminal-tool.md; resolved dependencies manually
  against SPEC-0043 through SPEC-0048 (all Completed, per Step 4's
  Implementation Order + Requirements-inference fallback, since
  FEATURE_INDEX.md has no Dependencies field); no blockers found.
- 2026-08-03 /feature start: set Status to In Progress, created branch
  feature/terminal-tool, implemented `services/core/tool_terminal.go` +
  `tool_terminal_test.go`. Design: since ToolExecutionEngine checks a Tool's
  Permissions once from static Metadata before Execute runs (not per input),
  a single Tool can't vary its approval requirement by command, so the
  capability is split into two Tools mirroring tool_filesystem.go's
  read/write split — `terminal.exec` (AllowedCommands allowlist, the "safe"
  set, testing criterion 1) and `terminal.exec.privileged` (unrestricted,
  meant to be configured PermissionApprovalRequired so every call routes
  through the existing PermissionChecker/ApprovalQueue flow, testing
  criterion 2). Command/process handling uses `exec.CommandContext` with a
  configurable timeout (default 30s); a non-zero exit is returned as a
  captured result (exitCode/stdout/stderr), not a Go error, while a command
  that never starts (unknown executable, timeout, cancellation) is a typed
  error (testing criterion 3). Execution logging mirrors filesystemTool's
  Logger-based `record` method. Tests use a TestMain-based re-exec of the
  test binary itself (env-var-selected helper modes: echo/fail/sleep) for
  cross-platform process behavior instead of OS-specific executables like
  `echo` (a shell builtin, not a standalone exe, on Windows), plus an
  integration test wiring ToolExecutionEngine + PermissionChecker +
  ApprovalQueue to verify the privileged tool actually blocks on approval.
  `go build`/`go vet`/`go test` clean across the whole workspace via
  `scripts/go_all.ps1`.
- 2026-08-03 /feature review: read current-feature.md, reviewed the diff
  (services/core/tool_terminal.go + tool_terminal_test.go, new/untracked -
  no commit yet) against SPEC-0050 and docs/agents/CODE_REVIEW_PROTOCOL.md.
  Architecture: no changes to Tool/ToolExecutionEngine/PermissionChecker/
  ApprovalQueue - only two new concrete Tools built on them, same shape as
  tool_filesystem.go; no ARCHITECTURE_CHANGE_PROTOCOL.md trigger. Scope:
  confined to services/core, no unrelated changes. Security: permissions,
  approval routing, and command restrictions all confirmed correctly wired;
  exec.CommandContext (no shell) means no shell-metacharacter injection via
  args. Testing: found gaps against tool_filesystem_test.go's own precedent
  (no cancellation test, no permission-declaration test, no args-forwarding
  test) and closed them - added
  TestTerminalExecTool_RespectsContextCancellation,
  TestTerminalTools_DeclarePermissionCategories,
  TestTerminalExecTool_ArgsAreForwarded (both []string and []any input
  shapes), and TestTerminalExecTool_InvalidArgsTypeIsRejected. Re-ran
  `scripts/go_all.ps1 all` after the additions - all 5 modules clean.
  Two non-blocking hardening notes accepted as future work, mirroring
  SPEC-0049's own accepted notes: (1) AllowedCommands restricts the command
  name only, not its arguments - an allowlisted command can still be
  invoked with arbitrary flags/args, the same trust boundary
  FilesystemRoots accepts for paths; (2) command matching is case-sensitive,
  same Windows caveat tool_filesystem.go already accepted for path
  comparison. Verdict: Ready to complete.
