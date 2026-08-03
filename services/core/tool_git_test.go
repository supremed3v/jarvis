package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgerrors "jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
)

// runGitSetupCommand runs `git <args...>` in dir and fails the test if it
// does not exit zero - a small helper used only to seed fixture repositories
// for the tests below, distinct from the runGit production helper this file
// exercises.
func runGitSetupCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

// initGitRepo creates a fresh repository (branch "main") with a committer
// identity configured locally, so tests do not depend on the host's global
// git config.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGitSetupCommand(t, dir, "init", "-b", "main")
	runGitSetupCommand(t, dir, "config", "user.email", "jarvis-test@example.com")
	runGitSetupCommand(t, dir, "config", "user.name", "JARVIS Test")
	return dir
}

// commitFile writes name/content into dir and commits it with message,
// seeding fixture history for the tests below.
func commitFile(t *testing.T, dir, name, content, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("writing file: %v", err)
	}
	runGitSetupCommand(t, dir, "add", name)
	runGitSetupCommand(t, dir, "commit", "-m", message)
}

func normalizePath(p string) string {
	return strings.ToLower(filepath.ToSlash(p))
}

// TestNewGitTool_RequiresRoots verifies every git Tool constructor fails
// closed without at least one allowed repository root, mirroring
// tool_filesystem.go/tool_terminal.go's "Allowed paths"/"Command
// restrictions" precedent for this file's own safety boundary.
func TestNewGitTool_RequiresRoots(t *testing.T) {
	if _, err := NewGitStatusTool(nil); !pkgerrors.Is(err, pkgerrors.TypeInvalidInput) {
		t.Fatalf("NewGitStatusTool(nil) error = %v, want TypeInvalidInput", err)
	}
	if _, err := NewGitStatusTool(FilesystemRoots{}); !pkgerrors.Is(err, pkgerrors.TypeInvalidInput) {
		t.Fatalf("NewGitStatusTool(empty) error = %v, want TypeInvalidInput", err)
	}
}

// TestGitTool_RepoPathOutsideRootsIsBlocked verifies a repoPath outside
// every allowed root is rejected before any git process runs (SPEC-0052
// testing criterion 2, "Git commands execute safely").
func TestGitTool_RepoPathOutsideRootsIsBlocked(t *testing.T) {
	allowedRoot := t.TempDir()
	outside := initGitRepo(t)

	roots, err := NewFilesystemRoots(allowedRoot)
	if err != nil {
		t.Fatalf("NewFilesystemRoots returned error: %v", err)
	}
	tool, err := NewGitStatusTool(roots)
	if err != nil {
		t.Fatalf("NewGitStatusTool returned error: %v", err)
	}

	_, err = tool.Execute(context.Background(), map[string]any{"repoPath": outside})
	if !pkgerrors.Is(err, pkgerrors.TypePermissionDenied) {
		t.Fatalf("Execute error = %v, want TypePermissionDenied", err)
	}
}

// TestGitTool_NotAGitRepositoryReportsNotFound verifies a path within the
// allowed roots that is not itself a git repository fails with TypeNotFound
// rather than a generic error (SPEC-0052 testing criterion 3, "Errors are
// handled").
func TestGitTool_NotAGitRepositoryReportsNotFound(t *testing.T) {
	dir := t.TempDir()
	roots, err := NewFilesystemRoots(dir)
	if err != nil {
		t.Fatalf("NewFilesystemRoots returned error: %v", err)
	}
	tool, err := NewGitStatusTool(roots)
	if err != nil {
		t.Fatalf("NewGitStatusTool returned error: %v", err)
	}

	_, err = tool.Execute(context.Background(), map[string]any{"repoPath": dir})
	if !pkgerrors.Is(err, pkgerrors.TypeNotFound) {
		t.Fatalf("Execute error = %v, want TypeNotFound", err)
	}
}

// TestGitTool_RespectsContextCancellation verifies a pre-canceled ctx is
// reported as TypeCanceled before any git process is started, matching
// every other Tool in this package's precedent.
func TestGitTool_RespectsContextCancellation(t *testing.T) {
	dir := initGitRepo(t)
	roots, err := NewFilesystemRoots(dir)
	if err != nil {
		t.Fatalf("NewFilesystemRoots returned error: %v", err)
	}
	tool, err := NewGitStatusTool(roots)
	if err != nil {
		t.Fatalf("NewGitStatusTool returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = tool.Execute(ctx, map[string]any{"repoPath": dir})
	if !pkgerrors.Is(err, pkgerrors.TypeCanceled) {
		t.Fatalf("Execute error = %v, want TypeCanceled", err)
	}
}

// TestGitTool_ExecutionIsLogged verifies a configured Logger records every
// Execute outcome, mirroring tool_terminal.go's "Execution logging"
// precedent.
func TestGitTool_ExecutionIsLogged(t *testing.T) {
	dir := initGitRepo(t)
	commitFile(t, dir, "a.txt", "hello", "initial commit")

	roots, err := NewFilesystemRoots(dir)
	if err != nil {
		t.Fatalf("NewFilesystemRoots returned error: %v", err)
	}
	var buf strings.Builder
	log := logger.New("test", logger.WithOutput(&buf))
	tool, err := NewGitStatusTool(roots, WithGitToolLogger(log))
	if err != nil {
		t.Fatalf("NewGitStatusTool returned error: %v", err)
	}

	if _, err := tool.Execute(context.Background(), map[string]any{"repoPath": dir}); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(buf.String(), "git tool execution") {
		t.Errorf("log output = %q, want it to contain %q", buf.String(), "git tool execution")
	}
}

// TestGitStatusTool_ReportsWorkingTreeState verifies "git.status" reports
// the current branch and correctly buckets staged, unstaged, and untracked
// files (SPEC-0052 testing criterion 1, "Repository data loads").
func TestGitStatusTool_ReportsWorkingTreeState(t *testing.T) {
	dir := initGitRepo(t)
	commitFile(t, dir, "committed.txt", "v1", "initial commit")

	// Unstaged modification to a tracked file.
	if err := os.WriteFile(filepath.Join(dir, "committed.txt"), []byte("v2"), 0o644); err != nil {
		t.Fatalf("modifying file: %v", err)
	}
	// Staged new file.
	if err := os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("new"), 0o644); err != nil {
		t.Fatalf("writing file: %v", err)
	}
	runGitSetupCommand(t, dir, "add", "staged.txt")
	// Untracked new file.
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("new"), 0o644); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	roots, err := NewFilesystemRoots(dir)
	if err != nil {
		t.Fatalf("NewFilesystemRoots returned error: %v", err)
	}
	tool, err := NewGitStatusTool(roots)
	if err != nil {
		t.Fatalf("NewGitStatusTool returned error: %v", err)
	}

	out, err := tool.Execute(context.Background(), map[string]any{"repoPath": dir})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if out["branch"] != "main" {
		t.Errorf("branch = %v, want %q", out["branch"], "main")
	}
	assertContainsString(t, "staged", out["staged"], "staged.txt")
	assertContainsString(t, "unstaged", out["unstaged"], "committed.txt")
	assertContainsString(t, "untracked", out["untracked"], "untracked.txt")
}

// assertContainsString fails the test if got (expected to be a []string) does
// not contain want.
func assertContainsString(t *testing.T, label string, got any, want string) {
	t.Helper()
	list, ok := got.([]string)
	if !ok {
		t.Fatalf("%s = %v (%T), want []string", label, got, got)
	}
	for _, s := range list {
		if s == want {
			return
		}
	}
	t.Errorf("%s = %v, want it to contain %q", label, list, want)
}

// TestGitBranchTool_ListsBranchesAndCurrent verifies "git.branch" reports
// every local branch and correctly identifies the current one (SPEC-0052's
// "Branch information" requirement).
func TestGitBranchTool_ListsBranchesAndCurrent(t *testing.T) {
	dir := initGitRepo(t)
	commitFile(t, dir, "a.txt", "v1", "initial commit")
	runGitSetupCommand(t, dir, "branch", "feature")

	roots, err := NewFilesystemRoots(dir)
	if err != nil {
		t.Fatalf("NewFilesystemRoots returned error: %v", err)
	}
	tool, err := NewGitBranchTool(roots)
	if err != nil {
		t.Fatalf("NewGitBranchTool returned error: %v", err)
	}

	out, err := tool.Execute(context.Background(), map[string]any{"repoPath": dir})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if out["current"] != "main" {
		t.Errorf("current = %v, want %q", out["current"], "main")
	}
	assertContainsString(t, "branches", out["branches"], "main")
	assertContainsString(t, "branches", out["branches"], "feature")
}

// TestGitLogTool_ReturnsCommitHistory verifies "git.log" returns commits
// newest-first with the requested fields, honoring the optional "limit"
// (SPEC-0052's "Commit history" requirement).
func TestGitLogTool_ReturnsCommitHistory(t *testing.T) {
	dir := initGitRepo(t)
	commitFile(t, dir, "a.txt", "v1", "first commit")
	commitFile(t, dir, "a.txt", "v2", "second commit")

	roots, err := NewFilesystemRoots(dir)
	if err != nil {
		t.Fatalf("NewFilesystemRoots returned error: %v", err)
	}
	tool, err := NewGitLogTool(roots)
	if err != nil {
		t.Fatalf("NewGitLogTool returned error: %v", err)
	}

	t.Run("default limit returns all commits, newest first", func(t *testing.T) {
		out, err := tool.Execute(context.Background(), map[string]any{"repoPath": dir})
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		commits, ok := out["commits"].([]map[string]any)
		if !ok || len(commits) != 2 {
			t.Fatalf("commits = %v, want 2 entries", out["commits"])
		}
		if commits[0]["message"] != "second commit" {
			t.Errorf("commits[0].message = %v, want %q", commits[0]["message"], "second commit")
		}
		if commits[1]["message"] != "first commit" {
			t.Errorf("commits[1].message = %v, want %q", commits[1]["message"], "first commit")
		}
		if hash, ok := commits[0]["hash"].(string); !ok || len(hash) != 40 {
			t.Errorf("commits[0].hash = %v, want a 40-character hash", commits[0]["hash"])
		}
	})

	t.Run("limit caps the number of commits returned", func(t *testing.T) {
		out, err := tool.Execute(context.Background(), map[string]any{"repoPath": dir, "limit": 1})
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		commits, ok := out["commits"].([]map[string]any)
		if !ok || len(commits) != 1 {
			t.Fatalf("commits = %v, want 1 entry", out["commits"])
		}
		if commits[0]["message"] != "second commit" {
			t.Errorf("commits[0].message = %v, want %q", commits[0]["message"], "second commit")
		}
	})
}

// TestGitLogTool_InvalidLimitIsRejected verifies a non-numeric or
// non-positive "limit" is rejected as TypeInvalidInput rather than silently
// coerced.
func TestGitLogTool_InvalidLimitIsRejected(t *testing.T) {
	dir := initGitRepo(t)
	commitFile(t, dir, "a.txt", "v1", "initial commit")

	roots, err := NewFilesystemRoots(dir)
	if err != nil {
		t.Fatalf("NewFilesystemRoots returned error: %v", err)
	}
	tool, err := NewGitLogTool(roots)
	if err != nil {
		t.Fatalf("NewGitLogTool returned error: %v", err)
	}

	for name, limit := range map[string]any{"non-number": "many", "zero": 0, "negative": -1} {
		t.Run(name, func(t *testing.T) {
			_, err := tool.Execute(context.Background(), map[string]any{"repoPath": dir, "limit": limit})
			if !pkgerrors.Is(err, pkgerrors.TypeInvalidInput) {
				t.Fatalf("Execute error = %v, want TypeInvalidInput", err)
			}
		})
	}
}

// TestGitDiffTool_ReturnsDiff verifies "git.diff" reports an unstaged
// change and correctly scopes to a ref and/or path (SPEC-0052's "Diff
// retrieval" requirement).
func TestGitDiffTool_ReturnsDiff(t *testing.T) {
	dir := initGitRepo(t)
	commitFile(t, dir, "a.txt", "line one\n", "initial commit")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("line one\nline two\n"), 0o644); err != nil {
		t.Fatalf("modifying file: %v", err)
	}

	roots, err := NewFilesystemRoots(dir)
	if err != nil {
		t.Fatalf("NewFilesystemRoots returned error: %v", err)
	}
	tool, err := NewGitDiffTool(roots)
	if err != nil {
		t.Fatalf("NewGitDiffTool returned error: %v", err)
	}

	out, err := tool.Execute(context.Background(), map[string]any{"repoPath": dir})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	diff, ok := out["diff"].(string)
	if !ok || !strings.Contains(diff, "line two") {
		t.Errorf("diff = %q, want it to contain %q", diff, "line two")
	}

	t.Run("scoped to a non-matching path returns no diff", func(t *testing.T) {
		out, err := tool.Execute(context.Background(), map[string]any{"repoPath": dir, "path": "other.txt"})
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if diff, _ := out["diff"].(string); diff != "" {
			t.Errorf("diff = %q, want empty", diff)
		}
	})

	t.Run("non-string ref is rejected", func(t *testing.T) {
		_, err := tool.Execute(context.Background(), map[string]any{"repoPath": dir, "ref": 123})
		if !pkgerrors.Is(err, pkgerrors.TypeInvalidInput) {
			t.Fatalf("Execute error = %v, want TypeInvalidInput", err)
		}
	})

	t.Run("a ref that looks like a flag is rejected rather than executed", func(t *testing.T) {
		// "--output=<file>" is a real git diff option that writes the diff
		// to an arbitrary file - if this reached exec.Command unfiltered it
		// would be a write primitive smuggled through a Tool declared
		// "git.read"-only, entirely outside the FilesystemRoots allowlist.
		outside := filepath.Join(t.TempDir(), "escaped.txt")
		_, err := tool.Execute(context.Background(), map[string]any{
			"repoPath": dir,
			"ref":      "--output=" + outside,
		})
		if !pkgerrors.Is(err, pkgerrors.TypeInvalidInput) {
			t.Fatalf("Execute error = %v, want TypeInvalidInput", err)
		}
		if _, statErr := os.Stat(outside); !os.IsNotExist(statErr) {
			t.Fatalf("expected %q to not exist, stat error = %v", outside, statErr)
		}
	})
}

// TestGitInspectTool_ReportsRepositoryInfo verifies "git.inspect" reports
// root/branch/HEAD/remote for a populated repository, and degrades
// headCommit/remoteURL to "" (not an error) when a repository has no
// commits or no configured remote (SPEC-0052's "Repository inspection"
// requirement).
func TestGitInspectTool_ReportsRepositoryInfo(t *testing.T) {
	t.Run("populated repository", func(t *testing.T) {
		dir := initGitRepo(t)
		commitFile(t, dir, "a.txt", "v1", "initial commit")
		runGitSetupCommand(t, dir, "remote", "add", "origin", "https://example.com/repo.git")

		roots, err := NewFilesystemRoots(dir)
		if err != nil {
			t.Fatalf("NewFilesystemRoots returned error: %v", err)
		}
		tool, err := NewGitInspectTool(roots)
		if err != nil {
			t.Fatalf("NewGitInspectTool returned error: %v", err)
		}

		out, err := tool.Execute(context.Background(), map[string]any{"repoPath": dir})
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}

		root, _ := out["root"].(string)
		if normalizePath(root) != normalizePath(dir) {
			t.Errorf("root = %q, want %q", root, dir)
		}
		if out["currentBranch"] != "main" {
			t.Errorf("currentBranch = %v, want %q", out["currentBranch"], "main")
		}
		if headCommit, ok := out["headCommit"].(string); !ok || len(headCommit) != 40 {
			t.Errorf("headCommit = %v, want a 40-character hash", out["headCommit"])
		}
		if out["remoteURL"] != "https://example.com/repo.git" {
			t.Errorf("remoteURL = %v, want %q", out["remoteURL"], "https://example.com/repo.git")
		}
	})

	t.Run("empty repository with no remote", func(t *testing.T) {
		dir := initGitRepo(t)

		roots, err := NewFilesystemRoots(dir)
		if err != nil {
			t.Fatalf("NewFilesystemRoots returned error: %v", err)
		}
		tool, err := NewGitInspectTool(roots)
		if err != nil {
			t.Fatalf("NewGitInspectTool returned error: %v", err)
		}

		out, err := tool.Execute(context.Background(), map[string]any{"repoPath": dir})
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if out["headCommit"] != "" {
			t.Errorf("headCommit = %v, want empty", out["headCommit"])
		}
		if out["remoteURL"] != "" {
			t.Errorf("remoteURL = %v, want empty", out["remoteURL"])
		}
	})
}

// TestGitTools_DeclarePermissionCategory verifies every git Tool declares
// exactly the "git.read" permission category, since every operation in this
// file is read-only.
func TestGitTools_DeclarePermissionCategory(t *testing.T) {
	dir := initGitRepo(t)
	roots, err := NewFilesystemRoots(dir)
	if err != nil {
		t.Fatalf("NewFilesystemRoots returned error: %v", err)
	}

	constructors := map[string]func(FilesystemRoots, ...GitToolOption) (Tool, error){
		"git.status":  NewGitStatusTool,
		"git.branch":  NewGitBranchTool,
		"git.log":     NewGitLogTool,
		"git.diff":    NewGitDiffTool,
		"git.inspect": NewGitInspectTool,
	}
	for id, ctor := range constructors {
		t.Run(id, func(t *testing.T) {
			tool, err := ctor(roots)
			if err != nil {
				t.Fatalf("constructor returned error: %v", err)
			}
			meta := tool.Metadata()
			if meta.ID != id {
				t.Errorf("ID = %q, want %q", meta.ID, id)
			}
			if len(meta.Permissions) != 1 || meta.Permissions[0] != "git.read" {
				t.Errorf("Permissions = %v, want [git.read]", meta.Permissions)
			}
		})
	}
}

// TestGitTool_ExecutionTimeout verifies a vanishingly small configured
// timeout causes an otherwise-normal git invocation to be killed and
// reported as TypeTimeout, mirroring tool_terminal.go's "Execution timeout"
// precedent.
func TestGitTool_ExecutionTimeout(t *testing.T) {
	dir := initGitRepo(t)
	commitFile(t, dir, "a.txt", "v1", "initial commit")

	roots, err := NewFilesystemRoots(dir)
	if err != nil {
		t.Fatalf("NewFilesystemRoots returned error: %v", err)
	}
	tool, err := NewGitStatusTool(roots, WithGitToolTimeout(1*time.Nanosecond))
	if err != nil {
		t.Fatalf("NewGitStatusTool returned error: %v", err)
	}

	_, err = tool.Execute(context.Background(), map[string]any{"repoPath": dir})
	if !pkgerrors.Is(err, pkgerrors.TypeTimeout) {
		t.Fatalf("Execute error = %v, want TypeTimeout", err)
	}
}
