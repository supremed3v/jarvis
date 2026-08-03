// tool_git.go implements SPEC-0052: the Git Tool - concrete SPEC-0043 Tools
// giving agents controlled, read-only Git repository access (repository
// inspection, branch information, commit history, status checks, diff
// retrieval; JARVIS_MASTER_ARCHITECTURE.md's Tool System responsibility
// set, and the Developer Agent capabilities SPEC-0052's Purpose names).
//
// Like tool_filesystem.go (SPEC-0049) and tool_terminal.go (SPEC-0050),
// "Permission rules" and "User approvals" are already fully implemented by
// SPEC-0046's ToolExecutionEngine (tool_execution.go) and SPEC-0047/0048's
// PermissionChecker/ApprovalQueue (agent_permission.go, tool_approval.go) -
// a Tool need only declare its required Permissions categories and those
// layers enforce the rest before Execute ever runs. SPEC-0052's testing
// criterion "Git commands execute safely" is this file's equivalent of
// FilesystemRoots' "Allowed paths" / AllowedCommands' "Command
// restrictions": every git Tool here is constructed with a FilesystemRoots
// allowlist (reused as-is rather than duplicated as a new type, since "a set
// of directory trees an operation may run under" is exactly what
// FilesystemRoots already models) and every operation resolves its
// "repoPath" input against it before a git process is ever started.
//
// Unlike tool_filesystem.go's read/write split, every operation SPEC-0052
// names (inspection, branch info, commit history, status, diff) is
// read-only - none of them mutate the repository - so all five Tools below
// share a single "git.read" permission category rather than being split
// further.
package core

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
)

// defaultGitTimeout is the timeout applied to every git Tool constructed
// without WithGitToolTimeout, mirroring defaultTerminalTimeout/
// defaultBrowserTimeout's precedent (tool_terminal.go, tool_browser.go).
const defaultGitTimeout = 30 * time.Second

// gitRepoInputSchema is shared by every git Tool constructor whose only
// required input is the repository to operate on.
var gitRepoInputSchema = Schema{{Name: "repoPath", Type: "string", Required: true}}

// runGit runs `git <args...>` with its working directory set to repoPath
// (already resolved against the allowed roots by the caller), enforcing
// timeout the same way tool_terminal.go's Execute does. A non-zero exit or
// failure to start is reported as an error via gitCommandError - unlike
// tool_terminal.go's terminal.exec, every operation in this file is a query
// (status/log/diff/branch/inspect), so there is no meaningful "successful
// non-zero exit" result to hand back to the caller.
func runGit(ctx context.Context, timeout time.Duration, repoPath string, args ...string) (string, error) {
	runCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(runCtx, "git", args...)
	cmd.Dir = repoPath
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	if errType, timedOut := ctxErrType(runCtx); timedOut && runErr != nil {
		return "", errors.Wrap(runErr, errType, "GIT_EXECUTION_TIMEOUT", "core.tool_git",
			fmt.Sprintf("git %s timed out or was canceled", strings.Join(args, " "))).With("args", strings.Join(args, " "))
	}

	if runErr != nil {
		return "", gitCommandError(runErr, stderr.String(), args)
	}

	return stdout.String(), nil
}

// gitCommandError classifies a failed git invocation's stderr into the
// packages/errors Type that best matches it - TypeNotFound for "not a git
// repository", TypeInvalidInput for a bad ref/revision, TypeInternal
// otherwise - satisfying SPEC-0052 testing criterion 3, "Errors are
// handled", with a caller-actionable Type rather than one generic failure.
func gitCommandError(runErr error, stderrOutput string, args []string) error {
	msg := strings.TrimSpace(stderrOutput)
	if msg == "" {
		msg = runErr.Error()
	}

	errType := errors.TypeInternal
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "not a git repository"):
		errType = errors.TypeNotFound
	case strings.Contains(lower, "unknown revision"),
		strings.Contains(lower, "ambiguous argument"),
		strings.Contains(lower, "bad revision"),
		strings.Contains(lower, "does not have any commits"):
		errType = errors.TypeInvalidInput
	}

	return errors.Wrap(runErr, errType, "GIT_COMMAND_FAILED", "core.tool_git",
		fmt.Sprintf("running git %s: %s", strings.Join(args, " "), msg)).With("args", strings.Join(args, " "))
}

// optionalStringInput extracts field from input as a string, returning "" if
// the field is absent or nil (the field is genuinely optional for every
// caller of this helper) or a packages/errors error typed TypeInvalidInput
// if it is present but not a string.
func optionalStringInput(input map[string]any, field string) (string, error) {
	v, ok := input[field]
	if !ok || v == nil {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", errors.New(errors.TypeInvalidInput, "GIT_INPUT_INVALID_FIELD", "core.tool_git",
			"input field must be a string").With("field", field)
	}
	return s, nil
}

// intInput extracts field from input as an int, returning def if the field
// is absent or nil, or a packages/errors error typed TypeInvalidInput if it
// is present but not a number. Accepts int, int64, and float64 so both a
// Go-constructed input map and a JSON-decoded one (which produces float64
// for numbers) work without an adapter.
func intInput(input map[string]any, field string, def int) (int, error) {
	v, ok := input[field]
	if !ok || v == nil {
		return def, nil
	}
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	default:
		return 0, errors.New(errors.TypeInvalidInput, "GIT_INPUT_INVALID_FIELD", "core.tool_git",
			"input field must be a number").With("field", field)
	}
}

// gitOp is one git Tool's actual behavior, run against an already-resolved
// repository path by gitTool.Execute.
type gitOp func(ctx context.Context, timeout time.Duration, repoPath string, input map[string]any) (map[string]any, error)

// gitTool is the Tool every constructor in this file produces: static
// Metadata plus a shared FilesystemRoots allowlist and an injected gitOp,
// mirroring filesystemTool/terminalTool/browserTool's metadata-plus-behavior
// split.
type gitTool struct {
	metadata ToolMetadata
	roots    FilesystemRoots
	timeout  time.Duration
	log      *logger.Logger
	op       gitOp
}

func (t *gitTool) Metadata() ToolMetadata { return t.metadata }

// Execute validates and resolves the required "repoPath" input against
// t.roots, then runs t.op against the resolved path. Execute must respect
// ctx cancellation, checked before any git process is started.
func (t *gitTool) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	if errType, canceled := ctxErrType(ctx); canceled {
		err := errors.Wrap(ctx.Err(), errType, "GIT_EXECUTION_CANCELED", "core.tool_git",
			fmt.Sprintf("%s canceled before running", t.metadata.ID)).With("toolId", t.metadata.ID)
		t.record(input, "canceled", err)
		return nil, err
	}

	repoPath, err := stringInput(input, "repoPath")
	if err != nil {
		t.record(input, "invalid_input", err)
		return nil, err
	}

	resolved, err := t.roots.Resolve(repoPath)
	if err != nil {
		t.record(input, "denied", err)
		return nil, err
	}

	out, err := t.op(ctx, t.timeout, resolved, input)
	if err != nil {
		t.record(input, "failed", err)
		return nil, err
	}
	t.record(input, "executed", nil)
	return out, nil
}

// record logs a single Execute outcome. A no-op if no Logger is configured.
func (t *gitTool) record(input map[string]any, outcome string, err error) {
	if t.log == nil {
		return
	}
	fields := map[string]any{"toolId": t.metadata.ID, "outcome": outcome}
	if repoPath, ok := input["repoPath"].(string); ok {
		fields["repoPath"] = repoPath
	}
	if err != nil {
		fields["error"] = err.Error()
		t.log.Error("git tool execution", fields)
		return
	}
	t.log.Info("git tool execution", fields)
}

// gitToolConfig holds the options every git Tool constructor in this file
// accepts.
type gitToolConfig struct {
	log     *logger.Logger
	timeout time.Duration
}

// GitToolOption configures a git Tool created by one of this file's New*
// constructors.
type GitToolOption func(*gitToolConfig)

// WithGitToolLogger attaches a Logger used to record every Execute outcome.
// Optional; a tool with no logger runs silently.
func WithGitToolLogger(log *logger.Logger) GitToolOption {
	return func(c *gitToolConfig) { c.log = log }
}

// WithGitToolTimeout overrides defaultGitTimeout, the time a git invocation
// is allowed to run before it is killed and GIT_EXECUTION_TIMEOUT is
// reported. A zero or negative d disables the timeout entirely, leaving
// cancellation to ctx alone.
func WithGitToolTimeout(d time.Duration) GitToolOption {
	return func(c *gitToolConfig) { c.timeout = d }
}

// newGitTool builds the gitTool common to every constructor below. It
// returns a packages/errors error typed TypeInvalidInput if roots is empty -
// a git tool cannot execute safely without at least one allowed repository
// root.
func newGitTool(metadata ToolMetadata, roots FilesystemRoots, op gitOp, opts []GitToolOption) (Tool, error) {
	if len(roots) == 0 {
		return nil, errors.New(errors.TypeInvalidInput, "GIT_TOOL_MISSING_ROOTS", "core.tool_git",
			fmt.Sprintf("cannot create %s without allowed repository roots", metadata.ID)).With("toolId", metadata.ID)
	}

	cfg := &gitToolConfig{timeout: defaultGitTimeout}
	for _, opt := range opts {
		opt(cfg)
	}

	return &gitTool{metadata: metadata, roots: roots, timeout: cfg.timeout, log: cfg.log, op: op}, nil
}

// NewGitStatusTool creates the "git.status" Tool: reports the current
// branch and working tree status (staged, unstaged, and untracked files) -
// SPEC-0052 testing criterion 1, "Repository data loads".
func NewGitStatusTool(roots FilesystemRoots, opts ...GitToolOption) (Tool, error) {
	return newGitTool(ToolMetadata{
		ID:          "git.status",
		Name:        "Git Status",
		Description: "Reports the current branch and working tree status (staged, unstaged, untracked files) for a repository within the allowed roots.",
		InputSchema: gitRepoInputSchema,
		OutputSchema: Schema{
			{Name: "branch", Type: "string", Required: true},
			{Name: "staged", Type: "array", Required: true},
			{Name: "unstaged", Type: "array", Required: true},
			{Name: "untracked", Type: "array", Required: true},
		},
		Permissions: []string{"git.read"},
	}, roots, gitStatus, opts)
}

func gitStatus(ctx context.Context, timeout time.Duration, repoPath string, _ map[string]any) (map[string]any, error) {
	out, err := runGit(ctx, timeout, repoPath, "status", "--porcelain=v1", "--branch")
	if err != nil {
		return nil, err
	}
	branch, staged, unstaged, untracked := parseGitStatusPorcelain(out)
	return map[string]any{
		"branch":    branch,
		"staged":    staged,
		"unstaged":  unstaged,
		"untracked": untracked,
	}, nil
}

// parseGitStatusPorcelain parses `git status --porcelain=v1 --branch`
// output into a branch name plus staged/unstaged/untracked path lists. Each
// status line is "XY path": X is the index (staged) status, Y is the
// worktree (unstaged) status, and "??" marks an untracked file; a path with
// both an index and worktree change (e.g. "MM") appears in both lists.
func parseGitStatusPorcelain(output string) (branch string, staged, unstaged, untracked []string) {
	staged = []string{}
	unstaged = []string{}
	untracked = []string{}

	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "## ") {
			branch = parseGitBranchHeader(line)
			continue
		}
		if len(line) < 3 {
			continue
		}
		code := line[:2]
		path := line[3:]
		if code == "??" {
			untracked = append(untracked, path)
			continue
		}
		if code[0] != ' ' {
			staged = append(staged, path)
		}
		if code[1] != ' ' {
			unstaged = append(unstaged, path)
		}
	}
	return
}

// parseGitBranchHeader extracts the branch name from a `git status
// --branch` "## " header line, stripping any "...upstream" tracking suffix
// or "[ahead N]" trailer. A detached HEAD header ("## HEAD (no branch)")
// yields "HEAD".
func parseGitBranchHeader(line string) string {
	rest := strings.TrimPrefix(line, "## ")
	if idx := strings.Index(rest, "..."); idx >= 0 {
		rest = rest[:idx]
	}
	if idx := strings.Index(rest, " "); idx >= 0 {
		rest = rest[:idx]
	}
	return rest
}

// NewGitBranchTool creates the "git.branch" Tool: lists local branches and
// reports which one is current - SPEC-0052's "Branch information"
// requirement.
func NewGitBranchTool(roots FilesystemRoots, opts ...GitToolOption) (Tool, error) {
	return newGitTool(ToolMetadata{
		ID:          "git.branch",
		Name:        "Git Branch",
		Description: "Lists local branches and reports the current branch for a repository within the allowed roots.",
		InputSchema: gitRepoInputSchema,
		OutputSchema: Schema{
			{Name: "current", Type: "string", Required: true},
			{Name: "branches", Type: "array", Required: true},
		},
		Permissions: []string{"git.read"},
	}, roots, gitBranch, opts)
}

func gitBranch(ctx context.Context, timeout time.Duration, repoPath string, _ map[string]any) (map[string]any, error) {
	out, err := runGit(ctx, timeout, repoPath, "branch", "--list")
	if err != nil {
		return nil, err
	}
	current, branches := parseGitBranchList(out)
	return map[string]any{"current": current, "branches": branches}, nil
}

// parseGitBranchList parses `git branch --list` output into the current
// branch name (the line prefixed "* ") and the full list of branch names.
func parseGitBranchList(output string) (current string, branches []string) {
	branches = []string{}
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		isCurrent := strings.HasPrefix(line, "* ")
		name := strings.TrimSpace(strings.TrimPrefix(line, "* "))
		if isCurrent {
			current = name
		}
		branches = append(branches, name)
	}
	return
}

// NewGitLogTool creates the "git.log" Tool: retrieves commit history
// (hash, author, date, message) - SPEC-0052's "Commit history" requirement.
// The optional "limit" input caps how many commits are returned (default
// 20).
func NewGitLogTool(roots FilesystemRoots, opts ...GitToolOption) (Tool, error) {
	return newGitTool(ToolMetadata{
		ID:          "git.log",
		Name:        "Git Log",
		Description: "Retrieves commit history (hash, author, date, message) for a repository within the allowed roots.",
		InputSchema: Schema{
			{Name: "repoPath", Type: "string", Required: true},
			{Name: "limit", Type: "integer", Required: false},
		},
		OutputSchema: Schema{{Name: "commits", Type: "array", Required: true}},
		Permissions:  []string{"git.read"},
	}, roots, gitLog, opts)
}

// gitLogFieldSep is the ASCII unit separator used to delimit fields within a
// single `git log --pretty=format:...` line - a byte that cannot legally
// appear in a commit hash, author name, ISO date, or (in practice) a commit
// subject, so splitting on it is safe without escaping.
const gitLogFieldSep = "\x1f"

func gitLog(ctx context.Context, timeout time.Duration, repoPath string, input map[string]any) (map[string]any, error) {
	limit, err := intInput(input, "limit", 20)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, errors.New(errors.TypeInvalidInput, "GIT_INPUT_INVALID_FIELD", "core.tool_git",
			"input field must be a positive number").With("field", "limit")
	}

	out, err := runGit(ctx, timeout, repoPath, "log", fmt.Sprintf("-n%d", limit),
		"--pretty=format:%H"+gitLogFieldSep+"%an"+gitLogFieldSep+"%aI"+gitLogFieldSep+"%s")
	if err != nil {
		return nil, err
	}

	commits := []map[string]any{}
	if strings.TrimSpace(out) != "" {
		for _, line := range strings.Split(out, "\n") {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, gitLogFieldSep, 4)
			if len(parts) != 4 {
				continue
			}
			commits = append(commits, map[string]any{
				"hash":    parts[0],
				"author":  parts[1],
				"date":    parts[2],
				"message": parts[3],
			})
		}
	}
	return map[string]any{"commits": commits}, nil
}

// NewGitDiffTool creates the "git.diff" Tool: retrieves a diff, optionally
// scoped to a ref and/or path - SPEC-0052's "Diff retrieval" requirement.
func NewGitDiffTool(roots FilesystemRoots, opts ...GitToolOption) (Tool, error) {
	return newGitTool(ToolMetadata{
		ID:          "git.diff",
		Name:        "Git Diff",
		Description: "Retrieves a diff, optionally scoped to a ref and/or path, for a repository within the allowed roots.",
		InputSchema: Schema{
			{Name: "repoPath", Type: "string", Required: true},
			{Name: "ref", Type: "string", Required: false},
			{Name: "path", Type: "string", Required: false},
		},
		OutputSchema: Schema{{Name: "diff", Type: "string", Required: true}},
		Permissions:  []string{"git.read"},
	}, roots, gitDiff, opts)
}

func gitDiff(ctx context.Context, timeout time.Duration, repoPath string, input map[string]any) (map[string]any, error) {
	ref, err := optionalStringInput(input, "ref")
	if err != nil {
		return nil, err
	}
	// A ref beginning with "-" would be parsed by git as an option rather
	// than a revision - e.g. "--output=<file>" makes `git diff` write its
	// output to an arbitrary file, a write primitive smuggled through a Tool
	// declared "git.read"-only, entirely outside the FilesystemRoots
	// allowlist. No valid git ref (branch, tag, or commit hash) can begin
	// with "-" (git-check-ref-format disallows it), so rejecting it here is
	// both safe and sufficient - SPEC-0052 testing criterion 2, "Git
	// commands execute safely".
	if strings.HasPrefix(ref, "-") {
		return nil, errors.New(errors.TypeInvalidInput, "GIT_INPUT_INVALID_FIELD", "core.tool_git",
			"input field must not begin with '-'").With("field", "ref")
	}
	path, err := optionalStringInput(input, "path")
	if err != nil {
		return nil, err
	}

	args := []string{"diff"}
	if ref != "" {
		args = append(args, ref)
	}
	if path != "" {
		args = append(args, "--", path)
	}

	out, err := runGit(ctx, timeout, repoPath, args...)
	if err != nil {
		return nil, err
	}
	return map[string]any{"diff": out}, nil
}

// NewGitInspectTool creates the "git.inspect" Tool: reports repository-level
// information (root path, current branch, HEAD commit, remote URL) -
// SPEC-0052's "Repository inspection" requirement. headCommit and remoteURL
// are reported as empty strings, not errors, when unavailable (a repository
// with no commits yet, or no configured "origin" remote) - the branch and
// root are still known and the operation as a whole did not fail.
func NewGitInspectTool(roots FilesystemRoots, opts ...GitToolOption) (Tool, error) {
	return newGitTool(ToolMetadata{
		ID:          "git.inspect",
		Name:        "Git Inspect",
		Description: "Reports repository-level information (root path, current branch, HEAD commit, remote URL) for a repository within the allowed roots.",
		InputSchema: gitRepoInputSchema,
		OutputSchema: Schema{
			{Name: "root", Type: "string", Required: true},
			{Name: "currentBranch", Type: "string", Required: true},
			{Name: "headCommit", Type: "string", Required: true},
			{Name: "remoteURL", Type: "string", Required: true},
		},
		Permissions: []string{"git.read"},
	}, roots, gitInspect, opts)
}

func gitInspect(ctx context.Context, timeout time.Duration, repoPath string, _ map[string]any) (map[string]any, error) {
	root, err := runGit(ctx, timeout, repoPath, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}

	// "symbolic-ref --short HEAD" resolves the branch name from HEAD's
	// symbolic ref alone, so it works even on a repository with zero
	// commits (unlike "rev-parse --abbrev-ref HEAD", which requires HEAD to
	// resolve to a commit and fails with "ambiguous argument" otherwise).
	// It only fails for a detached HEAD, which "rev-parse --abbrev-ref HEAD"
	// - the fallback here - resolves to the commit hash's short form.
	branch := ""
	if out, branchErr := runGit(ctx, timeout, repoPath, "symbolic-ref", "--short", "HEAD"); branchErr == nil {
		branch = strings.TrimSpace(out)
	} else if out, branchErr := runGit(ctx, timeout, repoPath, "rev-parse", "--abbrev-ref", "HEAD"); branchErr == nil {
		branch = strings.TrimSpace(out)
	}

	headCommit := ""
	if out, headErr := runGit(ctx, timeout, repoPath, "rev-parse", "HEAD"); headErr == nil {
		headCommit = strings.TrimSpace(out)
	}

	remoteURL := ""
	if out, remoteErr := runGit(ctx, timeout, repoPath, "remote", "get-url", "origin"); remoteErr == nil {
		remoteURL = strings.TrimSpace(out)
	}

	return map[string]any{
		"root":          strings.TrimSpace(root),
		"currentBranch": strings.TrimSpace(branch),
		"headCommit":    headCommit,
		"remoteURL":     remoteURL,
	}, nil
}
