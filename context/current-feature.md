# Current Feature: Git Tool

## Working In

services/core (Tools layer, same module as the completed Filesystem/
Terminal/Browser Automation tools — SPEC-0049/0050/0051 all live in
services/core/tool_*.go). Follow that pattern: tool_git.go implementing
the Tool interface from SPEC-0043, registered via the SPEC-0045 registry
and gated by the SPEC-0047/0048 permission/approval system.

## Status

In Progress

## Goals

- Repository inspection (`git.inspect`)
- Branch information (`git.branch`)
- Commit history (`git.log`)
- Status checks (`git.status`)
- Diff retrieval (`git.diff`)

## Dependencies

FEATURE_INDEX.md carries no explicit Dependencies field for this entry;
resolved via Implementation Order sequence (Tools layer) + Requirements
inference. All are Completed per FEATURE_INDEX.md / JARVIS_BUILD_TRACKER.md:

- SPEC-0043 Tool Interface (status: Completed)
- SPEC-0044 Tool Manifest System (status: Completed)
- SPEC-0045 Tool Registry (status: Completed)
- SPEC-0046 Tool Execution Engine (status: Completed)
- SPEC-0047 Tool Permission System (status: Completed)
- SPEC-0048 Tool Approval Workflow (status: Completed)
- SPEC-0049 Filesystem Tool (status: Completed, precedent implementation)
- SPEC-0050 Terminal Tool (status: Completed, precedent implementation)
- SPEC-0051 Browser Automation Tool (status: Completed, immediate predecessor in sequence)

No blocked or unimplemented prerequisites — clear to proceed to
`/feature start`.

## Notes

Specification:

context/features/SPEC-0052-git-tool.md

Index status at load time: Planned

Dependency resolution source: Implementation Order (Tools layer sequence)
+ Requirements inference (git operations imply the existing Tool
Interface/Registry/Execution/Permission stack, same as prior tools).

Related specs: (none declared in FEATURE_INDEX.md)

## Implementation Plan (as executed)

1. Branch `feature/git-tool` created off `master`.
2. `services/core/tool_git.go`: five read-only Tools sharing a single
   `git.read` permission category (no mutating git operations are in
   scope), each constructed with a `FilesystemRoots` allowlist reused
   as-is from tool_filesystem.go (repo paths are just another directory
   allowlist — no new type invented) so every `repoPath` input is
   resolved against it before `git` ever runs:
   - `git.status` — branch + staged/unstaged/untracked via
     `git status --porcelain=v1 --branch`
   - `git.branch` — local branches + current via `git branch --list`
   - `git.log` — commit history (hash/author/date/message) via
     `git log --pretty=format:...`, optional `limit` (default 20)
   - `git.diff` — diff via `git diff [ref] [-- path]`, ref/path optional
   - `git.inspect` — root/currentBranch/headCommit/remoteURL via
     `rev-parse`/`symbolic-ref`/`remote get-url`; headCommit and
     remoteURL degrade to `""` rather than erroring on a repo with no
     commits or no `origin` remote
   - Shared `runGit` helper runs `git` via `exec.CommandContext` with a
     configurable timeout (default 30s) and classifies failures into
     `TypeNotFound` ("not a git repository"), `TypeInvalidInput` (bad
     ref/revision), or `TypeInternal` (SPEC-0052 testing criterion 3,
     "Errors are handled").
3. `services/core/tool_git_test.go`: integration tests against real
   temporary git repositories (`git init -b main` + local committer
   config), covering all three testing criteria plus the allowlist/
   cancellation/logging/permission-category precedents every other Tool
   in this package already has.
4. Verified with `scripts/go_all.ps1 all` — all 5 workspace modules
   build/vet/test clean.

## History

- 2026-08-03 09:08 setup_feature.ps1 loaded SPEC-0052 (SPEC-0052-git-tool.md)
- 2026-08-03 load resolved dependencies (SPEC-0043–0051, all Completed) and confirmed services/core as implementation target
- 2026-08-03 start created feature/git-tool branch, implemented tool_git.go (5 Tools: git.status/branch/log/diff/inspect) + tool_git_test.go, status set to In Progress; full workspace build/vet/test clean
- 2026-08-03 review found and fixed a flag-injection issue in git.diff's "ref" input (a leading "-" let a caller smuggle git options like "--output=<file>", an arbitrary file write through a Tool declared git.read-only); added TestGitDiffTool_ReturnsDiff/a_ref_that_looks_like_a_flag_is_rejected_rather_than_executed regression test; re-ran go_all.ps1 all clean. Verdict: Ready to complete.
