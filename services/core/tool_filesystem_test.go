package core

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgerrors "jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
)

// TestFilesystemRoots_ResolveAllowsAndBlocks verifies FilesystemRoots
// resolves paths inside an allowed root and rejects paths outside every
// allowed root with a TypePermissionDenied error (SPEC-0049 Security:
// "Allowed paths", testing criterion 3: "Restricted paths are blocked").
func TestFilesystemRoots_ResolveAllowsAndBlocks(t *testing.T) {
	root := t.TempDir()
	roots, err := NewFilesystemRoots(root)
	if err != nil {
		t.Fatalf("NewFilesystemRoots returned error: %v", err)
	}

	t.Run("path inside root resolves", func(t *testing.T) {
		inside := filepath.Join(root, "file.txt")
		got, err := roots.Resolve(inside)
		if err != nil {
			t.Fatalf("Resolve returned error: %v", err)
		}
		if got != filepath.Clean(inside) {
			t.Errorf("Resolve = %q, want %q", got, filepath.Clean(inside))
		}
	})

	t.Run("root itself resolves", func(t *testing.T) {
		if _, err := roots.Resolve(root); err != nil {
			t.Fatalf("Resolve(root) returned error: %v", err)
		}
	})

	t.Run("sibling directory with shared prefix is blocked", func(t *testing.T) {
		// e.g. root "/allowed" must not admit "/allowed-other".
		sibling := root + "-other"
		if _, err := roots.Resolve(sibling); !pkgerrors.Is(err, pkgerrors.TypePermissionDenied) {
			t.Fatalf("Resolve(%q) error = %v, want TypePermissionDenied", sibling, err)
		}
	})

	t.Run("path outside root is blocked", func(t *testing.T) {
		outside := t.TempDir()
		if _, err := roots.Resolve(outside); !pkgerrors.Is(err, pkgerrors.TypePermissionDenied) {
			t.Fatalf("Resolve(%q) error = %v, want TypePermissionDenied", outside, err)
		}
	})
}

// TestNewFilesystemRoots_RequiresRoots verifies a filesystem tool cannot be
// constructed without at least one explicit allowed root, so "Allowed
// paths" fails closed rather than defaulting to unrestricted access.
func TestNewFilesystemRoots_RequiresRoots(t *testing.T) {
	if _, err := NewFilesystemRoots(); !pkgerrors.Is(err, pkgerrors.TypeInvalidInput) {
		t.Fatalf("NewFilesystemRoots() error = %v, want TypeInvalidInput", err)
	}
	if _, err := NewFilesystemRoots(""); !pkgerrors.Is(err, pkgerrors.TypeInvalidInput) {
		t.Fatalf("NewFilesystemRoots(\"\") error = %v, want TypeInvalidInput", err)
	}
}

// TestFilesystemReadTool_FilesCanBeRead verifies a file within the allowed
// roots can be read back correctly (SPEC-0049 testing criterion 1: "Files
// can be read").
func TestFilesystemReadTool_FilesCanBeRead(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "hello.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("seeding file: %v", err)
	}

	roots, err := NewFilesystemRoots(root)
	if err != nil {
		t.Fatalf("NewFilesystemRoots returned error: %v", err)
	}
	tool, err := NewFilesystemReadTool(roots)
	if err != nil {
		t.Fatalf("NewFilesystemReadTool returned error: %v", err)
	}

	out, err := tool.Execute(context.Background(), map[string]any{"path": path})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out["content"] != "hello world" {
		t.Errorf("Execute content = %#v, want %q", out["content"], "hello world")
	}
}

// TestFilesystemReadTool_MissingFileIsNotFound verifies reading a
// non-existent path within the allowed roots reports TypeNotFound rather
// than a generic internal error.
func TestFilesystemReadTool_MissingFileIsNotFound(t *testing.T) {
	root := t.TempDir()
	roots, err := NewFilesystemRoots(root)
	if err != nil {
		t.Fatalf("NewFilesystemRoots returned error: %v", err)
	}
	tool, err := NewFilesystemReadTool(roots)
	if err != nil {
		t.Fatalf("NewFilesystemReadTool returned error: %v", err)
	}

	_, err = tool.Execute(context.Background(), map[string]any{"path": filepath.Join(root, "missing.txt")})
	if !pkgerrors.Is(err, pkgerrors.TypeNotFound) {
		t.Fatalf("Execute error = %v, want TypeNotFound", err)
	}
}

// TestFilesystemReadTool_RestrictedPathIsBlocked verifies a read outside
// the allowed roots is blocked before touching disk (SPEC-0049 testing
// criterion 3: "Restricted paths are blocked").
func TestFilesystemReadTool_RestrictedPathIsBlocked(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o644); err != nil {
		t.Fatalf("seeding file: %v", err)
	}

	roots, err := NewFilesystemRoots(root)
	if err != nil {
		t.Fatalf("NewFilesystemRoots returned error: %v", err)
	}
	tool, err := NewFilesystemReadTool(roots)
	if err != nil {
		t.Fatalf("NewFilesystemReadTool returned error: %v", err)
	}

	_, err = tool.Execute(context.Background(), map[string]any{"path": outsideFile})
	if !pkgerrors.Is(err, pkgerrors.TypePermissionDenied) {
		t.Fatalf("Execute error = %v, want TypePermissionDenied", err)
	}
}

// TestFilesystemWriteTool_FilesCanBeWritten verifies content can be written
// to a file within the allowed roots and read back (SPEC-0049 testing
// criterion 2: "Files can be written").
func TestFilesystemWriteTool_FilesCanBeWritten(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "out.txt")

	roots, err := NewFilesystemRoots(root)
	if err != nil {
		t.Fatalf("NewFilesystemRoots returned error: %v", err)
	}
	tool, err := NewFilesystemWriteTool(roots)
	if err != nil {
		t.Fatalf("NewFilesystemWriteTool returned error: %v", err)
	}

	out, err := tool.Execute(context.Background(), map[string]any{"path": path, "content": "written content"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out["bytesWritten"] != len("written content") {
		t.Errorf("Execute bytesWritten = %#v, want %d", out["bytesWritten"], len("written content"))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back written file: %v", err)
	}
	if string(data) != "written content" {
		t.Errorf("written file content = %q, want %q", data, "written content")
	}
}

// TestFilesystemWriteTool_RestrictedPathIsBlocked verifies a write outside
// the allowed roots is rejected and never touches disk (SPEC-0049 testing
// criterion 3: "Restricted paths are blocked").
func TestFilesystemWriteTool_RestrictedPathIsBlocked(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "should-not-exist.txt")

	roots, err := NewFilesystemRoots(root)
	if err != nil {
		t.Fatalf("NewFilesystemRoots returned error: %v", err)
	}
	tool, err := NewFilesystemWriteTool(roots)
	if err != nil {
		t.Fatalf("NewFilesystemWriteTool returned error: %v", err)
	}

	_, err = tool.Execute(context.Background(), map[string]any{"path": outsideFile, "content": "nope"})
	if !pkgerrors.Is(err, pkgerrors.TypePermissionDenied) {
		t.Fatalf("Execute error = %v, want TypePermissionDenied", err)
	}
	if _, statErr := os.Stat(outsideFile); !os.IsNotExist(statErr) {
		t.Errorf("blocked write should not have created %q", outsideFile)
	}
}

// TestFilesystemListTool_DirectoryListing verifies directory listing
// reports each immediate entry's name and kind.
func TestFilesystemListTool_DirectoryListing(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatalf("seeding file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "subdir"), 0o755); err != nil {
		t.Fatalf("seeding dir: %v", err)
	}

	roots, err := NewFilesystemRoots(root)
	if err != nil {
		t.Fatalf("NewFilesystemRoots returned error: %v", err)
	}
	tool, err := NewFilesystemListTool(roots)
	if err != nil {
		t.Fatalf("NewFilesystemListTool returned error: %v", err)
	}

	out, err := tool.Execute(context.Background(), map[string]any{"path": root})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	entries, ok := out["entries"].([]map[string]any)
	if !ok || len(entries) != 2 {
		t.Fatalf("Execute entries = %#v, want 2 entries", out["entries"])
	}

	seen := map[string]bool{}
	for _, e := range entries {
		name, _ := e["name"].(string)
		seen[name] = e["isDir"].(bool)
	}
	if isDir, ok := seen["a.txt"]; !ok || isDir {
		t.Errorf("expected a.txt as non-directory entry, got present=%v isDir=%v", ok, isDir)
	}
	if isDir, ok := seen["subdir"]; !ok || !isDir {
		t.Errorf("expected subdir as directory entry, got present=%v isDir=%v", ok, isDir)
	}
}

// TestFilesystemSearchTool_MatchesPattern verifies search finds files
// nested under the given path whose name matches the glob pattern, and
// ignores non-matching files.
func TestFilesystemSearchTool_MatchesPattern(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatalf("seeding dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "match.log"), []byte(""), 0o644); err != nil {
		t.Fatalf("seeding file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "also.log"), []byte(""), 0o644); err != nil {
		t.Fatalf("seeding file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "skip.txt"), []byte(""), 0o644); err != nil {
		t.Fatalf("seeding file: %v", err)
	}

	roots, err := NewFilesystemRoots(root)
	if err != nil {
		t.Fatalf("NewFilesystemRoots returned error: %v", err)
	}
	tool, err := NewFilesystemSearchTool(roots)
	if err != nil {
		t.Fatalf("NewFilesystemSearchTool returned error: %v", err)
	}

	out, err := tool.Execute(context.Background(), map[string]any{"path": root, "pattern": "*.log"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	matches, ok := out["matches"].([]string)
	if !ok || len(matches) != 2 {
		t.Fatalf("Execute matches = %#v, want 2 matches", out["matches"])
	}
	for _, m := range matches {
		if !strings.HasSuffix(m, ".log") {
			t.Errorf("match %q does not end in .log", m)
		}
	}
}

// TestFilesystemMetadataTool_RetrievesMetadata verifies metadata retrieval
// reports size, directory flag, and a non-zero modification time.
func TestFilesystemMetadataTool_RetrievesMetadata(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	if err := os.WriteFile(path, []byte("1234"), 0o644); err != nil {
		t.Fatalf("seeding file: %v", err)
	}

	roots, err := NewFilesystemRoots(root)
	if err != nil {
		t.Fatalf("NewFilesystemRoots returned error: %v", err)
	}
	tool, err := NewFilesystemMetadataTool(roots)
	if err != nil {
		t.Fatalf("NewFilesystemMetadataTool returned error: %v", err)
	}

	out, err := tool.Execute(context.Background(), map[string]any{"path": path})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out["size"] != int64(4) {
		t.Errorf("Execute size = %#v, want 4", out["size"])
	}
	if out["isDir"] != false {
		t.Errorf("Execute isDir = %#v, want false", out["isDir"])
	}
	modTime, ok := out["modTime"].(time.Time)
	if !ok || modTime.IsZero() {
		t.Errorf("Execute modTime = %#v, want non-zero time.Time", out["modTime"])
	}
}

// TestFilesystemTool_RespectsContextCancellation verifies Execute reports a
// cancellation error rather than running when ctx is already canceled
// (Tool.Execute's "must respect ctx cancellation" contract, tool.go).
func TestFilesystemTool_RespectsContextCancellation(t *testing.T) {
	root := t.TempDir()
	roots, err := NewFilesystemRoots(root)
	if err != nil {
		t.Fatalf("NewFilesystemRoots returned error: %v", err)
	}
	tool, err := NewFilesystemReadTool(roots)
	if err != nil {
		t.Fatalf("NewFilesystemReadTool returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = tool.Execute(ctx, map[string]any{"path": filepath.Join(root, "file.txt")})
	if !pkgerrors.Is(err, pkgerrors.TypeCanceled) {
		t.Fatalf("Execute error = %v, want TypeCanceled", err)
	}
}

// TestFilesystemTool_LogsOutcomes verifies a configured Logger records both
// successful and failed Execute outcomes.
func TestFilesystemTool_LogsOutcomes(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatalf("seeding file: %v", err)
	}

	var buf bytes.Buffer
	log := logger.New("test", logger.WithOutput(&buf))

	roots, err := NewFilesystemRoots(root)
	if err != nil {
		t.Fatalf("NewFilesystemRoots returned error: %v", err)
	}
	tool, err := NewFilesystemReadTool(roots, WithFilesystemToolLogger(log))
	if err != nil {
		t.Fatalf("NewFilesystemReadTool returned error: %v", err)
	}

	if _, err := tool.Execute(context.Background(), map[string]any{"path": path}); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"path": filepath.Join(root, "missing.txt")}); err == nil {
		t.Fatalf("Execute on missing file returned no error")
	}

	logged := buf.String()
	if !strings.Contains(logged, "executed") {
		t.Errorf("log output missing successful outcome: %s", logged)
	}
	if !strings.Contains(logged, "failed") {
		t.Errorf("log output missing failed outcome: %s", logged)
	}
}

// TestNewFilesystemTool_RequiresRoots verifies every constructor in this
// file refuses to create a Tool with no allowed roots.
func TestNewFilesystemTool_RequiresRoots(t *testing.T) {
	constructors := map[string]func(FilesystemRoots) (Tool, error){
		"read":     func(r FilesystemRoots) (Tool, error) { return NewFilesystemReadTool(r) },
		"write":    func(r FilesystemRoots) (Tool, error) { return NewFilesystemWriteTool(r) },
		"list":     func(r FilesystemRoots) (Tool, error) { return NewFilesystemListTool(r) },
		"search":   func(r FilesystemRoots) (Tool, error) { return NewFilesystemSearchTool(r) },
		"metadata": func(r FilesystemRoots) (Tool, error) { return NewFilesystemMetadataTool(r) },
	}
	for name, ctor := range constructors {
		t.Run(name, func(t *testing.T) {
			if _, err := ctor(nil); !pkgerrors.Is(err, pkgerrors.TypeInvalidInput) {
				t.Fatalf("%s constructor with nil roots error = %v, want TypeInvalidInput", name, err)
			}
		})
	}
}

// TestFilesystemTools_DeclarePermissionCategories verifies each tool
// declares the permission category tool.go's own example maps to (read
// operations require filesystem.read, write requires filesystem.write) -
// so SPEC-0046's ToolExecutionEngine enforces them via SPEC-0047's
// PermissionChecker without a read-only agent needing write access.
func TestFilesystemTools_DeclarePermissionCategories(t *testing.T) {
	root := t.TempDir()
	roots, err := NewFilesystemRoots(root)
	if err != nil {
		t.Fatalf("NewFilesystemRoots returned error: %v", err)
	}

	readTool, _ := NewFilesystemReadTool(roots)
	writeTool, _ := NewFilesystemWriteTool(roots)
	listTool, _ := NewFilesystemListTool(roots)
	searchTool, _ := NewFilesystemSearchTool(roots)
	metadataTool, _ := NewFilesystemMetadataTool(roots)

	for _, tc := range []struct {
		name string
		tool Tool
		want string
	}{
		{"read", readTool, "filesystem.read"},
		{"write", writeTool, "filesystem.write"},
		{"list", listTool, "filesystem.read"},
		{"search", searchTool, "filesystem.read"},
		{"metadata", metadataTool, "filesystem.read"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			perms := tc.tool.Metadata().Permissions
			if len(perms) != 1 || perms[0] != tc.want {
				t.Errorf("%s Permissions = %v, want [%s]", tc.name, perms, tc.want)
			}
		})
	}
}
