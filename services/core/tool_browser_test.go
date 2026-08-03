package core

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	pkgerrors "jarvis-pa/packages/errors"
)

// sharedBrowserEngine is the single BrowserEngine every test in this file
// runs against. Launching Chromium takes roughly a second, so it's launched
// once for the whole package (see TestMain in tool_terminal_test.go, which
// owns the one TestMain a package may define) rather than per test.
var sharedBrowserEngine *BrowserEngine

// setupSharedBrowserEngine launches sharedBrowserEngine. Called from
// tool_terminal_test.go's TestMain before m.Run().
func setupSharedBrowserEngine() error {
	engine, err := NewBrowserEngine()
	if err != nil {
		return err
	}
	sharedBrowserEngine = engine
	return nil
}

// teardownSharedBrowserEngine closes sharedBrowserEngine. Called from
// tool_terminal_test.go's TestMain after m.Run().
func teardownSharedBrowserEngine() error {
	return sharedBrowserEngine.Close()
}

// newBrowserTestServer starts an httptest.Server serving small fixed HTML
// fixtures: "/" (known text + a link to "/other"), "/other" (a distinct
// marker page), and "/form" (a text input and a button that, on click,
// writes the input's value into a result element - for browser.automate).
func newBrowserTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>Home</title></head><body>
			<h1>hello-browser-tool</h1>
			<a id="next" href="/other">go</a>
		</body></html>`)
	})
	mux.HandleFunc("/other", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>Other</title></head><body><p>other-page-marker</p></body></html>`)
	})
	mux.HandleFunc("/form", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>Form</title></head><body>
			<input id="name" type="text" />
			<button id="submit" onclick="document.getElementById('result').innerText = document.getElementById('name').value">submit</button>
			<div id="result"></div>
		</body></html>`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestBrowserNavigateTool_LoadsPage verifies "browser.navigate" loads a
// page and reports its title and a 200 status - SPEC-0051 testing criterion
// "Pages load".
func TestBrowserNavigateTool_LoadsPage(t *testing.T) {
	srv := newBrowserTestServer(t)
	tool, err := NewBrowserNavigateTool(sharedBrowserEngine)
	if err != nil {
		t.Fatalf("NewBrowserNavigateTool returned error: %v", err)
	}

	out, err := tool.Execute(context.Background(), map[string]any{"url": srv.URL + "/"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out["title"] != "Home" {
		t.Errorf("title = %v, want %q", out["title"], "Home")
	}
	if out["statusCode"] != 200 {
		t.Errorf("statusCode = %v, want 200", out["statusCode"])
	}
}

// TestBrowserExtractTool_ExtractsContent verifies "browser.extract" returns
// both visible text and full HTML for a page - SPEC-0051 testing criterion
// "Content extraction works".
func TestBrowserExtractTool_ExtractsContent(t *testing.T) {
	srv := newBrowserTestServer(t)
	tool, err := NewBrowserExtractTool(sharedBrowserEngine)
	if err != nil {
		t.Fatalf("NewBrowserExtractTool returned error: %v", err)
	}

	out, err := tool.Execute(context.Background(), map[string]any{"url": srv.URL + "/"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	text, _ := out["text"].(string)
	if !strings.Contains(text, "hello-browser-tool") {
		t.Errorf("text = %q, want it to contain %q", text, "hello-browser-tool")
	}
	html, _ := out["html"].(string)
	if !strings.Contains(html, "hello-browser-tool") {
		t.Errorf("html = %q, want it to contain %q", html, "hello-browser-tool")
	}
}

// TestBrowserScreenshotTool_ReturnsValidPNG verifies "browser.screenshot"
// returns a base64-encoded PNG (identified by its magic bytes) for a loaded
// page.
func TestBrowserScreenshotTool_ReturnsValidPNG(t *testing.T) {
	srv := newBrowserTestServer(t)
	tool, err := NewBrowserScreenshotTool(sharedBrowserEngine)
	if err != nil {
		t.Fatalf("NewBrowserScreenshotTool returned error: %v", err)
	}

	out, err := tool.Execute(context.Background(), map[string]any{"url": srv.URL + "/"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out["format"] != "png" {
		t.Errorf("format = %v, want %q", out["format"], "png")
	}

	imgB64, _ := out["imageBase64"].(string)
	if imgB64 == "" {
		t.Fatal("imageBase64 is empty")
	}
	img, err := base64.StdEncoding.DecodeString(imgB64)
	if err != nil {
		t.Fatalf("base64 decode failed: %v", err)
	}
	pngMagic := []byte{0x89, 'P', 'N', 'G'}
	if len(img) < len(pngMagic) || string(img[:len(pngMagic)]) != string(pngMagic) {
		t.Errorf("screenshot bytes do not start with the PNG magic header")
	}
}

// TestBrowserAutomateTool_FillAndClick verifies "browser.automate" runs a
// fill then a click in order and returns the page's resulting state -
// SPEC-0051's "Basic automation" requirement.
func TestBrowserAutomateTool_FillAndClick(t *testing.T) {
	srv := newBrowserTestServer(t)
	tool, err := NewBrowserAutomateTool(sharedBrowserEngine)
	if err != nil {
		t.Fatalf("NewBrowserAutomateTool returned error: %v", err)
	}

	out, err := tool.Execute(context.Background(), map[string]any{
		"url": srv.URL + "/form",
		"actions": []any{
			map[string]any{"type": "fill", "selector": "#name", "value": "jarvis"},
			map[string]any{"type": "click", "selector": "#submit"},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	text, _ := out["text"].(string)
	if !strings.Contains(text, "jarvis") {
		t.Errorf("text = %q, want it to contain %q", text, "jarvis")
	}
}

// TestBrowserAutomateTool_RejectsUnsupportedActionType verifies an unknown
// action "type" is rejected with TypeInvalidInput before any action runs -
// the input containing a valid "click" first should not execute it once an
// unsupported step is found.
func TestBrowserAutomateTool_RejectsUnsupportedActionType(t *testing.T) {
	srv := newBrowserTestServer(t)
	tool, err := NewBrowserAutomateTool(sharedBrowserEngine)
	if err != nil {
		t.Fatalf("NewBrowserAutomateTool returned error: %v", err)
	}

	_, err = tool.Execute(context.Background(), map[string]any{
		"url": srv.URL + "/form",
		"actions": []any{
			map[string]any{"type": "hover", "selector": "#name"},
		},
	})
	if !pkgerrors.Is(err, pkgerrors.TypeInvalidInput) {
		t.Fatalf("Execute error = %v, want TypeInvalidInput", err)
	}
}

// TestBrowserNavigateTool_UnreachableURLIsHandled verifies a connection
// failure is reported as a wrapped TypeInternal error rather than a panic
// or an unstructured failure - SPEC-0051 testing criterion "Browser
// failures are handled".
func TestBrowserNavigateTool_UnreachableURLIsHandled(t *testing.T) {
	tool, err := NewBrowserNavigateTool(sharedBrowserEngine, WithBrowserToolTimeout(3*time.Second))
	if err != nil {
		t.Fatalf("NewBrowserNavigateTool returned error: %v", err)
	}

	_, err = tool.Execute(context.Background(), map[string]any{"url": "http://127.0.0.1:1/"})
	if !pkgerrors.Is(err, pkgerrors.TypeInternal) {
		t.Fatalf("Execute error = %v, want TypeInternal", err)
	}
}

// TestBrowserExtractTool_RejectsNonHTTPScheme verifies a "file://" URL is
// rejected with TypePermissionDenied rather than navigated to - a
// file:// URL would otherwise let a browser Tool read arbitrary local
// files through Chromium, entirely bypassing FilesystemRoots
// (tool_filesystem.go), the allowlist an agent's filesystem.read
// permission is supposed to be scoped by.
func TestBrowserExtractTool_RejectsNonHTTPScheme(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "secret-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("top-secret"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	tool, err := NewBrowserExtractTool(sharedBrowserEngine)
	if err != nil {
		t.Fatalf("NewBrowserExtractTool returned error: %v", err)
	}

	fileURL := "file:///" + strings.ReplaceAll(f.Name(), "\\", "/")
	_, err = tool.Execute(context.Background(), map[string]any{"url": fileURL})
	if !pkgerrors.Is(err, pkgerrors.TypePermissionDenied) {
		t.Fatalf("Execute(%q) error = %v, want TypePermissionDenied", fileURL, err)
	}
}

// TestBrowserNavigateTool_CanceledContext verifies Execute honors ctx
// cancellation, returning TypeCanceled rather than running the navigation -
// SPEC-0051 testing criterion "Browser failures are handled".
func TestBrowserNavigateTool_CanceledContext(t *testing.T) {
	srv := newBrowserTestServer(t)
	tool, err := NewBrowserNavigateTool(sharedBrowserEngine)
	if err != nil {
		t.Fatalf("NewBrowserNavigateTool returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = tool.Execute(ctx, map[string]any{"url": srv.URL + "/"})
	if !pkgerrors.Is(err, pkgerrors.TypeCanceled) {
		t.Fatalf("Execute error = %v, want TypeCanceled", err)
	}
}

// TestNewBrowserNavigateTool_RequiresEngine verifies a browser Tool cannot
// be constructed without a BrowserEngine.
func TestNewBrowserNavigateTool_RequiresEngine(t *testing.T) {
	if _, err := NewBrowserNavigateTool(nil); !pkgerrors.Is(err, pkgerrors.TypeInvalidInput) {
		t.Fatalf("NewBrowserNavigateTool(nil) error = %v, want TypeInvalidInput", err)
	}
}
