// tool_browser.go implements SPEC-0051: the Browser Automation Tool -
// concrete SPEC-0043 Tools giving agents controlled browser access (page
// navigation, content extraction, screenshots, basic automation;
// JARVIS_MASTER_ARCHITECTURE.md's Tool System responsibility set), built on
// Playwright per ADR-0006's locked technology decision.
//
// Like tool_filesystem.go (SPEC-0049) and tool_terminal.go (SPEC-0050),
// permission and approval are already fully implemented by SPEC-0046's
// ToolExecutionEngine (tool_execution.go) and SPEC-0047/0048's
// PermissionChecker/ApprovalQueue (agent_permission.go, tool_approval.go) -
// a Tool need only declare its required Permissions categories and those
// layers enforce the rest before Execute ever runs. Unlike those two files,
// SPEC-0051 has no Security section naming a tool-local allowlist
// (FilesystemRoots' "Allowed paths" / AllowedCommands' "Command
// restrictions" equivalent), so none is invented here; the four Tools below
// are split by permission category alone, mirroring how
// terminal.exec/terminal.exec.privileged split the same underlying resource
// so an operator can grant "browser.automate" (arbitrary click/fill) more
// cautiously than read-only navigation/extraction/screenshot.
//
// Every Tool here is a self-contained, stateless call: it opens a new
// isolated BrowserContext+Page on a shared, already-running Browser
// (BrowserEngine), performs its one action, closes the context, and returns
// a structured result - the same "one independent unit of work per Execute"
// shape every other Tool in this package already has, with no cross-call
// browser session state for a caller to manage.
//
// playwright-go's API takes option structs with millisecond Timeout fields,
// not context.Context, so each op runs the actual Playwright calls on a
// goroutine and selects on ctx.Done() to honor tool.go's "Execute must
// respect ctx cancellation" contract even if a page hangs past its own
// Playwright-level timeout.
package core

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/mxschmitt/playwright-go"

	"jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
)

// defaultBrowserTimeout is the SPEC-0051 per-operation timeout applied when
// a browser Tool is constructed without WithBrowserToolTimeout, mirroring
// defaultTerminalTimeout's precedent (tool_terminal.go): long enough for an
// ordinary page load, short enough that a hung page cannot block a caller
// indefinitely.
const defaultBrowserTimeout = 30 * time.Second

// BrowserEngine holds the single Playwright driver process and Chromium
// Browser shared by every browser Tool constructed with it. Launching a
// Browser process is expensive relative to a single Execute call, so
// BrowserEngine is created once (by whoever wires up the Tool layer) and
// passed into each New*Tool constructor - the same "shared expensive
// resource, per-call isolation on top of it" split FilesystemRoots and
// AllowedCommands establish for their tools, except here the shared
// resource is a live process rather than static configuration.
type BrowserEngine struct {
	pw      *playwright.Playwright
	browser playwright.Browser
}

// browserEngineConfig holds the options NewBrowserEngine accepts.
type browserEngineConfig struct {
	headless bool
}

// BrowserEngineOption configures a BrowserEngine created by NewBrowserEngine.
type BrowserEngineOption func(*browserEngineConfig)

// WithBrowserEngineHeadful launches Chromium with a visible window instead
// of headless. Optional; intended for local debugging only - every
// production use is expected to run headless (the default).
func WithBrowserEngineHeadful() BrowserEngineOption {
	return func(c *browserEngineConfig) { c.headless = false }
}

// NewBrowserEngine starts the Playwright driver and launches a headless
// Chromium Browser, returning a BrowserEngine ready to back every browser
// Tool's constructor. The caller owns the returned BrowserEngine's
// lifecycle and must call Close when done with it. Returns a
// packages/errors error typed TypeUnavailable if the driver fails to start
// or the browser fails to launch - both indicate the local Playwright
// install (a prerequisite outside this package's control) is missing or
// broken, not a caller input error.
func NewBrowserEngine(opts ...BrowserEngineOption) (*BrowserEngine, error) {
	cfg := &browserEngineConfig{headless: true}
	for _, opt := range opts {
		opt(cfg)
	}

	pw, err := playwright.Run()
	if err != nil {
		return nil, errors.Wrap(err, errors.TypeUnavailable, "BROWSER_ENGINE_DRIVER_START_FAILED", "core.tool_browser",
			"starting the Playwright driver")
	}

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(cfg.headless),
	})
	if err != nil {
		_ = pw.Stop()
		return nil, errors.Wrap(err, errors.TypeUnavailable, "BROWSER_ENGINE_LAUNCH_FAILED", "core.tool_browser",
			"launching the Chromium browser")
	}

	return &BrowserEngine{pw: pw, browser: browser}, nil
}

// Close shuts down the shared Browser and stops the Playwright driver.
// Every browser Tool built on this BrowserEngine becomes unusable once
// Close returns.
func (e *BrowserEngine) Close() error {
	if err := e.browser.Close(); err != nil {
		return errors.Wrap(err, errors.TypeInternal, "BROWSER_ENGINE_CLOSE_FAILED", "core.tool_browser",
			"closing the Chromium browser")
	}
	if err := e.pw.Stop(); err != nil {
		return errors.Wrap(err, errors.TypeInternal, "BROWSER_ENGINE_STOP_FAILED", "core.tool_browser",
			"stopping the Playwright driver")
	}
	return nil
}

// newPage opens a fresh, isolated BrowserContext+Page on e's shared
// Browser. The returned closeFn tears both down and must be called by
// every caller, typically deferred, regardless of what the page is
// subsequently used for - this is what makes every browser Tool call
// self-contained rather than leaking browser state across Execute calls.
func (e *BrowserEngine) newPage() (playwright.Page, func(), error) {
	bctx, err := e.browser.NewContext()
	if err != nil {
		return nil, nil, errors.Wrap(err, errors.TypeInternal, "BROWSER_CONTEXT_CREATE_FAILED", "core.tool_browser",
			"creating a browser context")
	}

	page, err := bctx.NewPage()
	if err != nil {
		_ = bctx.Close()
		return nil, nil, errors.Wrap(err, errors.TypeInternal, "BROWSER_PAGE_CREATE_FAILED", "core.tool_browser",
			"creating a browser page")
	}

	return page, func() { _ = bctx.Close() }, nil
}

// browserOp is one browser Tool's actual behavior, run against a
// freshly-opened Page by browserTool.Execute. timeoutMS is the configured
// per-operation Playwright timeout (in milliseconds, the unit Playwright's
// own option structs use), passed through so an op can apply it to every
// Playwright call it makes (Goto, Click, Fill, ...).
type browserOp func(page playwright.Page, input map[string]any, timeoutMS float64) (map[string]any, error)

// browserTool is the Tool every constructor in this file produces: static
// Metadata plus a shared BrowserEngine and an injected browserOp, mirroring
// filesystemTool/terminalTool's metadata-plus-behavior split.
type browserTool struct {
	metadata ToolMetadata
	engine   *BrowserEngine
	timeout  time.Duration
	log      *logger.Logger
	op       browserOp
}

func (t *browserTool) Metadata() ToolMetadata { return t.metadata }

// Execute runs op against a fresh Page opened on t.engine, honoring ctx
// cancellation even though the underlying playwright-go calls do not accept
// a context.Context: op runs on a goroutine, and Execute selects on that
// goroutine's completion vs. ctx.Done(), returning a wrapped
// TypeCanceled/TypeTimeout error if ctx loses first (SPEC-0051's testing
// criterion "Browser failures are handled" covers both a canceled caller
// and an underlying Playwright failure).
func (t *browserTool) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	if errType, canceled := ctxErrType(ctx); canceled {
		err := errors.Wrap(ctx.Err(), errType, "BROWSER_EXECUTION_CANCELED", "core.tool_browser",
			fmt.Sprintf("%s canceled before running", t.metadata.ID)).With("toolId", t.metadata.ID)
		t.record(input, "canceled", err)
		return nil, err
	}

	page, closePage, err := t.engine.newPage()
	if err != nil {
		t.record(input, "page_create_failed", err)
		return nil, err
	}
	defer closePage()

	type opResult struct {
		out map[string]any
		err error
	}
	done := make(chan opResult, 1)
	go func() {
		out, err := t.op(page, input, float64(t.timeout.Milliseconds()))
		done <- opResult{out: out, err: err}
	}()

	select {
	case res := <-done:
		if res.err != nil {
			wrapped := errors.Wrap(res.err, errors.TypeInternal, "BROWSER_OPERATION_FAILED", "core.tool_browser",
				fmt.Sprintf("running %s", t.metadata.ID)).With("toolId", t.metadata.ID)
			t.record(input, "failed", wrapped)
			return nil, wrapped
		}
		t.record(input, "executed", nil)
		return res.out, nil
	case <-ctx.Done():
		errType, _ := ctxErrType(ctx)
		err := errors.Wrap(ctx.Err(), errType, "BROWSER_EXECUTION_CANCELED", "core.tool_browser",
			fmt.Sprintf("%s canceled while running", t.metadata.ID)).With("toolId", t.metadata.ID)
		t.record(input, "canceled", err)
		return nil, err
	}
}

// record logs a single Execute outcome. A no-op if no Logger is configured.
func (t *browserTool) record(input map[string]any, outcome string, err error) {
	if t.log == nil {
		return
	}
	fields := map[string]any{"toolId": t.metadata.ID, "outcome": outcome}
	if url, ok := input["url"].(string); ok {
		fields["url"] = url
	}
	if err != nil {
		fields["error"] = err.Error()
		t.log.Error("browser tool execution", fields)
		return
	}
	t.log.Info("browser tool execution", fields)
}

// browserToolConfig holds the options every browser Tool constructor in
// this file accepts.
type browserToolConfig struct {
	log     *logger.Logger
	timeout time.Duration
}

// BrowserToolOption configures a browser Tool created by one of this file's
// New* constructors.
type BrowserToolOption func(*browserToolConfig)

// WithBrowserToolLogger attaches a Logger used to record every Execute
// outcome. Optional; a tool with no logger runs silently.
func WithBrowserToolLogger(log *logger.Logger) BrowserToolOption {
	return func(c *browserToolConfig) { c.log = log }
}

// WithBrowserToolTimeout overrides defaultBrowserTimeout, the per-operation
// timeout passed to every Playwright call an op makes. A zero or negative d
// disables the Playwright-level timeout, leaving cancellation to ctx alone.
func WithBrowserToolTimeout(d time.Duration) BrowserToolOption {
	return func(c *browserToolConfig) { c.timeout = d }
}

// newBrowserTool builds the browserTool common to every constructor below.
// It returns a packages/errors error typed TypeInvalidInput if engine is
// nil - a browser tool cannot operate without a running Browser to open
// pages on.
func newBrowserTool(metadata ToolMetadata, engine *BrowserEngine, op browserOp, opts []BrowserToolOption) (Tool, error) {
	if engine == nil {
		return nil, errors.New(errors.TypeInvalidInput, "BROWSER_TOOL_MISSING_ENGINE", "core.tool_browser",
			fmt.Sprintf("cannot create %s without a BrowserEngine", metadata.ID)).With("toolId", metadata.ID)
	}

	cfg := &browserToolConfig{timeout: defaultBrowserTimeout}
	for _, opt := range opts {
		opt(cfg)
	}

	return &browserTool{metadata: metadata, engine: engine, timeout: cfg.timeout, log: cfg.log, op: op}, nil
}

// urlInput extracts the required "url" field from input as a non-empty
// string, additionally enforcing that its scheme is http or https.
//
// Without this check a "browser.extract"/"browser.screenshot" call given a
// file:// URL reads the local filesystem through Chromium entirely outside
// FilesystemRoots (tool_filesystem.go) - the allowlist that is otherwise
// this codebase's whole model for controlling local file access - letting
// an agent permissioned only for browser tools read arbitrary files it was
// never granted filesystem.read for. Restricting every browser Tool to
// http(s) closes that bypass; SPEC-0051's stated capabilities (page
// navigation, content extraction, screenshots, basic automation on web
// pages) never called for file://, data:, or other local/non-network
// schemes in the first place.
func urlInput(input map[string]any) (string, error) {
	v, ok := input["url"]
	if !ok || v == nil {
		return "", errors.New(errors.TypeInvalidInput, "BROWSER_INPUT_MISSING_FIELD", "core.tool_browser",
			"required input field is missing").With("field", "url")
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", errors.New(errors.TypeInvalidInput, "BROWSER_INPUT_INVALID_FIELD", "core.tool_browser",
			"input field must be a non-empty string").With("field", "url")
	}

	parsed, err := url.Parse(s)
	if err != nil {
		return "", errors.Wrap(err, errors.TypeInvalidInput, "BROWSER_INPUT_INVALID_FIELD", "core.tool_browser",
			fmt.Sprintf("parsing url %q", s)).With("field", "url")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", errors.New(errors.TypePermissionDenied, "BROWSER_URL_SCHEME_NOT_ALLOWED", "core.tool_browser",
			fmt.Sprintf("url scheme %q is not allowed - only http and https are permitted", parsed.Scheme)).With("url", s)
	}

	return s, nil
}

// goto navigates page to url with the given Playwright timeout and returns
// the resulting Response, or an error if navigation failed to complete.
func gotoURL(page playwright.Page, url string, timeoutMS float64) (playwright.Response, error) {
	resp, err := page.Goto(url, playwright.PageGotoOptions{Timeout: playwright.Float(timeoutMS)})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// browserInputSchema is shared by every browser Tool constructor: every one
// takes at minimum a required "url".
var browserInputSchema = Schema{{Name: "url", Type: "string", Required: true}}

// NewBrowserNavigateTool creates the "browser.navigate" Tool: loads url and
// reports its final title and HTTP status - SPEC-0051 testing criterion
// "Pages load".
func NewBrowserNavigateTool(engine *BrowserEngine, opts ...BrowserToolOption) (Tool, error) {
	return newBrowserTool(ToolMetadata{
		ID:          "browser.navigate",
		Name:        "Browser Navigate",
		Description: "Navigates to a URL and reports the resulting page title and HTTP status.",
		InputSchema: browserInputSchema,
		OutputSchema: Schema{
			{Name: "url", Type: "string", Required: true},
			{Name: "title", Type: "string", Required: true},
			{Name: "statusCode", Type: "integer", Required: true},
		},
		Permissions: []string{"browser.navigate"},
	}, engine, browserNavigate, opts)
}

func browserNavigate(page playwright.Page, input map[string]any, timeoutMS float64) (map[string]any, error) {
	url, err := urlInput(input)
	if err != nil {
		return nil, err
	}

	resp, err := gotoURL(page, url, timeoutMS)
	if err != nil {
		return nil, err
	}

	title, err := page.Title()
	if err != nil {
		return nil, err
	}

	statusCode := 0
	if resp != nil {
		statusCode = resp.Status()
	}

	return map[string]any{"url": url, "title": title, "statusCode": statusCode}, nil
}

// NewBrowserExtractTool creates the "browser.extract" Tool: navigates to url
// and returns both the page's visible text and full HTML - SPEC-0051
// testing criterion "Content extraction works".
func NewBrowserExtractTool(engine *BrowserEngine, opts ...BrowserToolOption) (Tool, error) {
	return newBrowserTool(ToolMetadata{
		ID:          "browser.extract",
		Name:        "Browser Extract",
		Description: "Navigates to a URL and extracts the page's visible text and full HTML.",
		InputSchema: browserInputSchema,
		OutputSchema: Schema{
			{Name: "url", Type: "string", Required: true},
			{Name: "title", Type: "string", Required: true},
			{Name: "text", Type: "string", Required: true},
			{Name: "html", Type: "string", Required: true},
		},
		Permissions: []string{"browser.extract"},
	}, engine, browserExtract, opts)
}

func browserExtract(page playwright.Page, input map[string]any, timeoutMS float64) (map[string]any, error) {
	url, err := urlInput(input)
	if err != nil {
		return nil, err
	}

	if _, err := gotoURL(page, url, timeoutMS); err != nil {
		return nil, err
	}

	return extractPageContent(page, url)
}

// extractPageContent reads the current page's title, visible body text, and
// full HTML - the shared result shape browser.extract and browser.automate
// both return.
func extractPageContent(page playwright.Page, url string) (map[string]any, error) {
	title, err := page.Title()
	if err != nil {
		return nil, err
	}

	text, err := page.InnerText("body")
	if err != nil {
		return nil, err
	}

	html, err := page.Content()
	if err != nil {
		return nil, err
	}

	return map[string]any{"url": url, "title": title, "text": text, "html": html}, nil
}

// NewBrowserScreenshotTool creates the "browser.screenshot" Tool: navigates
// to url and returns a base64-encoded PNG screenshot.
func NewBrowserScreenshotTool(engine *BrowserEngine, opts ...BrowserToolOption) (Tool, error) {
	return newBrowserTool(ToolMetadata{
		ID:          "browser.screenshot",
		Name:        "Browser Screenshot",
		Description: "Navigates to a URL and captures a screenshot as a base64-encoded PNG.",
		InputSchema: Schema{
			{Name: "url", Type: "string", Required: true},
			{Name: "fullPage", Type: "boolean", Required: false},
		},
		OutputSchema: Schema{
			{Name: "url", Type: "string", Required: true},
			{Name: "imageBase64", Type: "string", Required: true},
			{Name: "format", Type: "string", Required: true},
		},
		Permissions: []string{"browser.screenshot"},
	}, engine, browserScreenshot, opts)
}

func browserScreenshot(page playwright.Page, input map[string]any, timeoutMS float64) (map[string]any, error) {
	url, err := urlInput(input)
	if err != nil {
		return nil, err
	}

	if _, err := gotoURL(page, url, timeoutMS); err != nil {
		return nil, err
	}

	fullPage := false
	if v, ok := input["fullPage"]; ok && v != nil {
		b, ok := v.(bool)
		if !ok {
			return nil, errors.New(errors.TypeInvalidInput, "BROWSER_INPUT_INVALID_FIELD", "core.tool_browser",
				"input field must be a boolean").With("field", "fullPage")
		}
		fullPage = b
	}

	img, err := page.Screenshot(playwright.PageScreenshotOptions{
		FullPage: playwright.Bool(fullPage),
		Timeout:  playwright.Float(timeoutMS),
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"url":         url,
		"imageBase64": base64.StdEncoding.EncodeToString(img),
		"format":      "png",
	}, nil
}

// browserAction is one step of a "browser.automate" Tool's actions input:
// either a "click" or a "fill" (with value) against selector.
type browserAction struct {
	Type     string
	Selector string
	Value    string
}

// parseBrowserActions extracts and validates the required "actions" input
// field of a "browser.automate" call. It returns a packages/errors error
// typed TypeInvalidInput - naming the first structurally invalid entry, or
// an unsupported action Type - before any action runs, so a request
// containing one bad step never partially executes.
func parseBrowserActions(input map[string]any) ([]browserAction, error) {
	raw, ok := input["actions"]
	if !ok || raw == nil {
		return nil, errors.New(errors.TypeInvalidInput, "BROWSER_INPUT_MISSING_FIELD", "core.tool_browser",
			"required input field is missing").With("field", "actions")
	}
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil, errors.New(errors.TypeInvalidInput, "BROWSER_INPUT_INVALID_FIELD", "core.tool_browser",
			"input field must be a non-empty array").With("field", "actions")
	}

	actions := make([]browserAction, len(items))
	for i, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, errors.New(errors.TypeInvalidInput, "BROWSER_INPUT_INVALID_FIELD", "core.tool_browser",
				"each action must be an object").With("field", "actions").With("index", i)
		}

		actionType, _ := m["type"].(string)
		selector, _ := m["selector"].(string)
		if selector == "" {
			return nil, errors.New(errors.TypeInvalidInput, "BROWSER_INPUT_INVALID_FIELD", "core.tool_browser",
				"action is missing a non-empty selector").With("field", "actions").With("index", i)
		}

		switch actionType {
		case "click":
			actions[i] = browserAction{Type: actionType, Selector: selector}
		case "fill":
			value, _ := m["value"].(string)
			actions[i] = browserAction{Type: actionType, Selector: selector, Value: value}
		default:
			return nil, errors.New(errors.TypeInvalidInput, "BROWSER_ACTION_UNSUPPORTED_TYPE", "core.tool_browser",
				fmt.Sprintf("unsupported action type %q", actionType)).With("field", "actions").With("index", i)
		}
	}
	return actions, nil
}

// NewBrowserAutomateTool creates the "browser.automate" Tool: navigates to
// url, then runs a sequence of click/fill actions against it, returning the
// page's resulting title/text/html - SPEC-0051's "Basic automation"
// requirement. Because this Tool can arbitrarily interact with a page
// (unlike the read-only navigate/extract/screenshot Tools), it declares a
// distinct "browser.automate" permission category so an operator can
// configure it PermissionApprovalRequired, the same way
// terminal.exec.privileged is (tool_terminal.go).
func NewBrowserAutomateTool(engine *BrowserEngine, opts ...BrowserToolOption) (Tool, error) {
	return newBrowserTool(ToolMetadata{
		ID:          "browser.automate",
		Name:        "Browser Automate",
		Description: "Navigates to a URL and runs a sequence of click/fill actions, requiring human approval via the configured Permission Model.",
		InputSchema: Schema{
			{Name: "url", Type: "string", Required: true},
			{Name: "actions", Type: "array", Required: true},
		},
		OutputSchema: Schema{
			{Name: "url", Type: "string", Required: true},
			{Name: "title", Type: "string", Required: true},
			{Name: "text", Type: "string", Required: true},
			{Name: "html", Type: "string", Required: true},
		},
		Permissions: []string{"browser.automate"},
	}, engine, browserAutomate, opts)
}

func browserAutomate(page playwright.Page, input map[string]any, timeoutMS float64) (map[string]any, error) {
	url, err := urlInput(input)
	if err != nil {
		return nil, err
	}
	actions, err := parseBrowserActions(input)
	if err != nil {
		return nil, err
	}

	if _, err := gotoURL(page, url, timeoutMS); err != nil {
		return nil, err
	}

	for _, a := range actions {
		switch a.Type {
		case "click":
			if err := page.Click(a.Selector, playwright.PageClickOptions{Timeout: playwright.Float(timeoutMS)}); err != nil {
				return nil, err
			}
		case "fill":
			if err := page.Fill(a.Selector, a.Value, playwright.PageFillOptions{Timeout: playwright.Float(timeoutMS)}); err != nil {
				return nil, err
			}
		}
	}

	return extractPageContent(page, page.URL())
}
