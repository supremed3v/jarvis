# Current Feature: Browser Automation Tool

## Working In

services/core (Go) — the Tool layer is implemented here as tool_*.go files
(tool.go, tool_registry.go, tool_execution.go, tool_approval.go,
tool_filesystem.go, tool_terminal.go), not in services/tools, which remains
an empty .gitkeep scaffold per CLAUDE.md. Expect a new tool_browser.go (+
tool_browser_test.go) following the same pattern as SPEC-0049/SPEC-0050,
implementing the Tool interface (tool.go) and registering via
tool_registry.go. Playwright is the locked technology choice (ADR-0006);
services/core currently has one third-party dependency
(gopkg.in/yaml.v3) — adding a Playwright Go binding
(github.com/playwright-community/playwright-go) would be a second, and
Playwright itself requires a browser binary install step, which is worth
flagging explicitly before /feature start since it's a new kind of
dependency for this module.

## Status

In Progress

## Goals

- Page navigation
- Content extraction
- Screenshots
- Basic automation

## Dependencies

- SPEC-0045 Tool Registry (status: Completed)
- SPEC-0046 Tool Execution Engine (status: Completed)
- SPEC-0047 Tool Permission System (status: Completed)
- SPEC-0048 Tool Approval Workflow (status: Completed)

## Notes

Specification:

context/features/SPEC-0051-browser-automation-tool.md

Index status at load time: Planned

Dependency resolution source: Requirements inference (FEATURE_INDEX.md
carries no explicit Dependencies/Related fields yet — see load.md Step 4
fallback chain) + JARVIS_DEPENDENCY_GRAPH.md's "Critical Dependencies"
(Agents/Research both list Tools as a prerequisite, confirming this sits in
the Tools layer, not a leaf spec). SPEC-0045-0048 are the general Tool
framework (interface, registry, execution engine, permission, approval)
that SPEC-0049 (Filesystem Tool) and SPEC-0050 (Terminal Tool) were both
built directly on top of (see services/core/tool_terminal.go); Browser
Automation Tool is the next tool in that same sequence per
JARVIS_IMPLEMENTATION_ORDER.md and FEATURE_INDEX.md's SPEC-0045..0053
ordering, so the same four prerequisites apply. ADR-0006 (Playwright) is
the locked technology decision referenced by this spec's "Technology"
section.

Related specs: (none declared in FEATURE_INDEX.md)

## History

- 2026-08-03 08:24 setup_feature.ps1 loaded SPEC-0051 (SPEC-0051-browser-automation-tool.md)
- 2026-08-03 load resolved dependencies (SPEC-0045..0048, all Completed) via requirements inference and updated Working In / Notes
- 2026-08-03 start: created branch feature/browser-automation-tool; added
  github.com/mxschmitt/playwright-go dependency (confirmed with user - the
  module github.com/playwright-community/playwright-go redirects to this
  declared path) and ran its one-time `playwright install chromium` step
  (confirmed with user - ~297 MiB network download to
  %LOCALAPPDATA%\ms-playwright); implemented services/core/tool_browser.go
  (browser.navigate/extract/screenshot/automate, BrowserEngine) and
  services/core/tool_browser_test.go (8 tests against a real headless
  Chromium + local httptest fixtures); `go build`/`go vet`/`go test` clean
  across all 5 go.work modules via scripts/go_all.ps1; updated
  docs/agents/JARVIS_BUILD_TRACKER.md (SPEC-0051 -> Completed) and
  regenerated context/features/FEATURE_INDEX.md. Not yet committed - status
  left "In Progress" pending /feature test, /feature review, and/or
  /feature complete (which owns committing/pushing/merging per
  actions/complete.md's "Never: Auto commit").
- 2026-08-03 review: found and fixed a real security bug - browser.extract/
  navigate/screenshot accepted file:// URLs, letting Chromium read arbitrary
  local files and bypass FilesystemRoots (tool_filesystem.go) entirely,
  confirmed exploitable via a throwaway (not committed) test reading a temp
  file's contents through browser.extract. Fixed by restricting urlInput to
  http/https schemes only (TypePermissionDenied otherwise); added
  TestBrowserExtractTool_RejectsNonHTTPScheme as a permanent regression
  test (9 browser tests total now). Also checked ctx-cancellation racing
  against BrowserEngine context cleanup mid-navigation (slow local server +
  200ms timeout) - no panic/crash observed, no fix needed there, though
  go test -race is unavailable in this environment so this wasn't verified
  under the race detector. go build/vet/test clean across all 5 modules
  after the fix. Updated JARVIS_BUILD_TRACKER.md's SPEC-0051 entry to
  describe the finding and fix. Verdict: Ready to complete.
