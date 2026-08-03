# Current Feature: Filesystem Tool

## Working In

services/core (tool_filesystem.go + tool_filesystem_test.go), following the
pattern set by SPEC-0043-0048 (tool.go, tool_manifest.go, tool_registry.go,
tool_execution.go, tool_approval.go all live in services/core, not the empty
services/tools scaffold).

## Status

Implemented (pending /feature test and /feature review)

## Branch

feature/filesystem-tool

## Implementation Plan

1. `services/core/tool_filesystem.go`:
   - `FilesystemRoots` type ("Allowed paths" - Security requirement):
     resolves each configured root to an absolute, cleaned path at
     construction; `Resolve(path)` rejects (TypePermissionDenied) any path
     that does not resolve inside one of those roots - satisfies testing
     criterion 3 ("Restricted paths are blocked") independent of the
     agent-level PermissionChecker.
   - Five separate `Tool` implementations (one per capability, matching
     ToolMetadata.Permissions' own "filesystem.read" / "filesystem.write"
     example in tool.go rather than one combined tool - so a read-only
     agent permission grant can't accidentally gate on a write permission
     it doesn't have): filesystem.read, filesystem.write, filesystem.list,
     filesystem.search, filesystem.metadata. All five share one internal
     `filesystemTool` struct (metadata + FilesystemRoots + injected op
     func + optional logger), mirroring tool_manifest.go's
     metadata+injected-behavior split.
   - Permission rules / user approvals: satisfied for free by declaring
     each tool's `Permissions` field - SPEC-0046's ToolExecutionEngine and
     SPEC-0047/0048's PermissionChecker/ApprovalQueue already enforce
     those before Execute runs; this spec does not reimplement that layer.
2. `services/core/tool_filesystem_test.go`: table-driven tests per the
   SPEC-0049 testing section (files can be read, files can be written,
   restricted paths are blocked) plus list/search/metadata coverage and
   ctx-cancellation, using `t.TempDir()` as the allowed root.
3. Run `scripts/go_all.ps1` (build/vet/test) for services/core.

## Goals

- File reading
- File writing
- Directory listing
- File searching
- Metadata retrieval
- Enforce allowed paths, permission rules, and user approvals (via the
  existing Tool Permission System / Tool Approval Workflow)

## Dependencies

- SPEC-0043 Tool Interface (status: Completed)
- SPEC-0044 Tool Manifest System (status: Completed)
- SPEC-0045 Tool Registry (status: Completed)
- SPEC-0046 Tool Execution Engine (status: Completed)
- SPEC-0047 Tool Permission System (status: Completed)
- SPEC-0048 Tool Approval Workflow (status: Completed)

## Notes

Specification:

context/features/SPEC-0049-filesystem-tool.md

Index status at load time: Planned

Dependency resolution source: JARVIS_IMPLEMENTATION_ORDER.md (Phase 4 Tools
sequence) + FEATURE_INDEX.md sequential ordering (SPEC-0043-0048 immediately
precede SPEC-0049 within the Tools category, all status Completed) +
Requirements inference (Security section's "Permission rules" / "User
approvals" map directly to SPEC-0047 / SPEC-0048).

FEATURE_INDEX.md does not carry explicit Dependencies/Related fields per
spec (generator limitation noted in the load skill) - the above was resolved
manually per Step 4 of actions/load.md rather than taken from
setup_feature.ps1's output alone.

## History

- 2026-08-03 07:42 setup_feature.ps1 loaded SPEC-0049 (SPEC-0049-filesystem-tool.md)
- 2026-08-03 load action: manually resolved dependencies (Step 4) since
  FEATURE_INDEX.md has no per-spec Dependencies field; confirmed
  services/core as the working directory by inspecting existing
  tool_*.go files from SPEC-0043-0048.
- 2026-08-03 start action: created branch feature/filesystem-tool;
  implemented all 5 goals in services/core/tool_filesystem.go
  (FilesystemRoots allowed-path allowlist + filesystem.read/write/list/
  search/metadata Tools) with tests in
  services/core/tool_filesystem_test.go; scripts/go_all.ps1 (build, vet,
  test) passes clean across all 5 workspace modules.
