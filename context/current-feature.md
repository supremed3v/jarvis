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

SPEC-0051 (Browser Automation Tool) is now `Completed` and merged to
master — see History below and its entry in
`docs/agents/JARVIS_BUILD_TRACKER.md` for the full record. This completes
the Tools branch of Phase 4 Intelligence started at SPEC-0043: SPEC-0043
through SPEC-0051 are all now `Completed`, except SPEC-0052 (Git Tool),
which remains `Planned`.

Next candidate: SPEC-0052 (Git Tool) is the natural next step — same shape
as SPEC-0049/SPEC-0050/SPEC-0051 (a concrete Tool built on the now-complete
Tool Interface/Manifest/Registry/Execution Engine/Permission
System/Approval Workflow layer), and its Requirements (repository
inspection, branch information, commit history, status checks, diff
retrieval) map onto read-only `git` subcommands — worth checking during
that spec's load/start whether it should build directly on SPEC-0050's
`terminal.exec` Tool (an `AllowedCommands` allowlist of `git` plus
argument-shape validation) rather than shelling out independently.
Completing SPEC-0052 would finish the entire Tools branch. SPEC-0056
(Speech To Text Provider) remains available to continue the Voice branch
after SPEC-0053-0055, and is the previously-noted voice-first MVP priority
per `docs/execution/JARVIS_MVP_SCOPE.md` (voice is a core, required MVP
surface, not optional). Research (SPEC-0073 onward) remains blocked only on
Search/Browser — browser is now unblocked by SPEC-0051, though Search
(SearXNG, ADR-0005) is still outstanding. Which to pick up next is a
product-priority call for whoever loads the next feature.

## History

- 2026-08-03 SPEC-0051 Browser Automation Tool: loaded via `/feature load`
  (index read first per `actions/load.md`; dependencies manually resolved —
  FEATURE_INDEX.md carries no per-spec Dependencies field yet — against
  SPEC-0045 through SPEC-0048, all Completed), started on
  feature/browser-automation-tool. Confirmed with user before proceeding:
  (1) the only viable Go Playwright binding is
  `github.com/mxschmitt/playwright-go` (the module
  `github.com/playwright-community/playwright-go` redirects to by declared
  path — a different identity than initially assumed, flagged and
  re-confirmed before installing anything), and (2) running its one-time
  `playwright install chromium` step (~297 MiB download to
  `%LOCALAPPDATA%\ms-playwright`). Implemented `services/core/tool_browser.go`
  (four Tools — `browser.navigate`, `browser.extract`, `browser.screenshot`,
  `browser.automate` — sharing a `BrowserEngine` that launches one headless
  Chromium instance reused across calls, each `Execute` opening an isolated
  `BrowserContext`+`Page` so every call stays self-contained and stateless;
  `browser.automate` declares its own permission category, mirroring
  `terminal.exec.privileged`, since it can arbitrarily interact with a
  page) + `tool_browser_test.go` (tests run against a real launched headless
  Chromium and a local `httptest` server, not mocks, matching
  tool_filesystem_test.go/tool_terminal_test.go's precedent; the package's
  one `TestMain`, in `tool_terminal_test.go`, now also owns the shared
  `BrowserEngine`'s lifecycle). Reviewed against
  `docs/agents/CODE_REVIEW_PROTOCOL.md`: found and fixed a real security
  bug before completion — `urlInput` accepted any URL scheme, so a
  `file://` URL let Chromium read arbitrary local files through the browser
  tool, completely bypassing `FilesystemRoots` (tool_filesystem.go), the
  allowlist this codebase otherwise uses as its whole model for scoping
  local file access; confirmed exploitable with a throwaway (uncommitted)
  test before fixing. Fixed by restricting `urlInput` to `http`/`https`
  schemes only (`TypePermissionDenied` otherwise), with a permanent
  regression test. Also checked ctx-cancellation racing against
  `BrowserEngine` context cleanup mid-navigation — no panic/crash observed,
  no fix needed, though `go test -race` remains unavailable in this
  environment (no C toolchain, same constraint noted since SPEC-0005/0007)
  so this wasn't independently verified under the race detector. 9 tests
  total, covering all three SPEC-0051 testing criteria (pages load, content
  extraction works, browser failures are handled) plus the scheme-rejection
  regression, screenshot PNG-magic-byte verification, automate fill+click,
  unsupported-action-type rejection, and nil-engine constructor rejection.
  `go build`/`go vet`/`go test` clean across all 5 go.work modules via
  `scripts/go_all.ps1`. Marked Completed in `JARVIS_BUILD_TRACKER.md`,
  regenerated `FEATURE_INDEX.md`, and merged to master.
