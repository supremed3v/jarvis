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

SPEC-0052 (Git Tool) is now `Completed` and merged to master — see History
below and its entry in `docs/agents/JARVIS_BUILD_TRACKER.md` for the full
record. This completes the entire Tools branch of Phase 4 Intelligence:
SPEC-0043 through SPEC-0052 are all now `Completed`.

Next candidate: SPEC-0053 (Audio Engine Interface) is the natural next
step — the first spec of the Voice branch, which
`docs/execution/JARVIS_MVP_SCOPE.md` and CLAUDE.md both call a core,
required MVP surface (voice-first, not optional, not deferrable behind a
desktop UI). SPEC-0054 (Microphone Capture System) and SPEC-0055 (Wake
Word Detection) follow it before SPEC-0056 (Speech To Text Provider) and
SPEC-0057 (Whisper Integration) continue the same branch. Research
(SPEC-0073 onward) remains blocked only on Search — browser automation is
now unblocked (SPEC-0051), but Search (SearXNG, ADR-0005) is still
outstanding. Which to pick up next is a product-priority call for whoever
loads the next feature.

## History

- 2026-08-03 SPEC-0052 Git Tool: loaded via `/feature load` (index read
  first per `actions/load.md`; dependencies resolved against SPEC-0043
  through SPEC-0051, all Completed — FEATURE_INDEX.md carries no per-spec
  Dependencies field yet), started on `feature/git-tool`. Implemented
  `services/core/tool_git.go`: five read-only Tools (`git.status`,
  `git.branch`, `git.log`, `git.diff`, `git.inspect`) sharing a single
  `git.read` permission category, each constructed with a
  `FilesystemRoots` allowlist reused as-is from the filesystem tool
  (rather than duplicated as a new type) so a `repoPath` input is
  resolved against it before any `git` process runs via
  `exec.CommandContext` (configurable timeout, default 30s). A shared
  `runGit` helper classifies failures into `TypeNotFound`/
  `TypeInvalidInput`/`TypeInternal`; `git.inspect`'s `headCommit`/
  `remoteURL` degrade to `""` rather than erroring on a repository with
  no commits or no `origin` remote. Added `tool_git_test.go`: tests run
  against real temporary git repositories (`git init -b main` + local
  committer config, not mocks, matching the package's established
  precedent), covering all three SPEC-0052 testing criteria (repository
  data loads, git commands execute safely, errors are handled) plus the
  allowlist/not-a-repo/cancellation/logging/permission-category checks
  every other Tool in this package already has. Reviewed against
  `docs/agents/CODE_REVIEW_PROTOCOL.md`: found and fixed a real security
  bug before completion — `git.diff`'s optional `ref` input was passed to
  `git diff` unguarded, so a value like `--output=<file>` (a real git
  option) let a caller write the diff to an arbitrary file, a write
  primitive smuggled through a Tool declared `git.read`-only, entirely
  bypassing the `FilesystemRoots` sandbox (the `path` input was already
  safe, placed after a `--` separator). Fixed by rejecting any `ref`
  beginning with `-` (no valid git ref can start with `-`), with a
  permanent regression test proving both the rejection and that no file
  is written. `go build`/`go vet`/`go test` clean across all 5 go.work
  modules via `scripts/go_all.ps1`. Marked Completed in
  `JARVIS_BUILD_TRACKER.md`, regenerated `FEATURE_INDEX.md`, and merged
  to master.
