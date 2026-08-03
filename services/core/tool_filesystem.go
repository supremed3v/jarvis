// tool_filesystem.go implements SPEC-0049: the Filesystem Tool - concrete
// SPEC-0043 Tools giving agents controlled filesystem access (file reading,
// file writing, directory listing, file searching, metadata retrieval;
// JARVIS_MASTER_ARCHITECTURE.md's Tool System "Filesystem access"
// responsibility).
//
// SPEC-0049's Security section names three things a filesystem tool must
// respect: Allowed paths, Permission rules, and User approvals. The latter
// two are already fully implemented by SPEC-0046's ToolExecutionEngine
// (tool_execution.go) and SPEC-0047/0048's PermissionChecker/ApprovalQueue
// (agent_permission.go, tool_approval.go) - a Tool need only declare its
// required Permissions categories (as tool.go's own doc comment already
// illustrates with the "filesystem.read" example) and those layers enforce
// the rest before Execute ever runs. "Allowed paths" has no existing home:
// PermissionChecker's PermissionModel is a category-level allow/deny table,
// not a path allowlist. FilesystemRoots fills that gap as a tool-local,
// defense-in-depth boundary every operation resolves its target path
// against.
//
// Read/list/search/metadata declare "filesystem.read"; write declares
// "filesystem.write". They are kept as five separate Tools (rather than one
// tool with an "operation" field) so an agent permissioned only for
// filesystem.read cannot reach filesystem.write through the same tool ID -
// ToolExecutionEngine.checkPermissions requires every category a tool
// declares, so a single combined tool would force every caller, including
// read-only ones, to hold filesystem.write too.
package core

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
)

// FilesystemRoots is the SPEC-0049 "Allowed paths" allowlist: the set of
// directory trees a filesystem Tool is permitted to operate under. It is
// the tool-local counterpart to SPEC-0024's category-level PermissionModel
// - a second, independent boundary checked on every operation regardless
// of what the agent-level permission check already decided.
type FilesystemRoots []string

// NewFilesystemRoots resolves each of roots to an absolute, cleaned path and
// returns the resulting FilesystemRoots. It returns a packages/errors error
// typed TypeInvalidInput if roots is empty or contains an empty string -
// a filesystem tool with no allowed roots would either be useless (deny
// everything) or, if roots defaulted to unrestricted, defeat the "Allowed
// paths" requirement entirely; requiring at least one explicit root fails
// closed by construction instead.
func NewFilesystemRoots(roots ...string) (FilesystemRoots, error) {
	if len(roots) == 0 {
		return nil, errors.New(errors.TypeInvalidInput, "FILESYSTEM_ROOTS_EMPTY", "core.tool_filesystem",
			"at least one allowed root path is required")
	}

	resolved := make(FilesystemRoots, len(roots))
	for i, root := range roots {
		if root == "" {
			return nil, errors.New(errors.TypeInvalidInput, "FILESYSTEM_ROOTS_EMPTY_ENTRY", "core.tool_filesystem",
				"allowed root path must not be empty").With("index", i)
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, errors.Wrap(err, errors.TypeInvalidInput, "FILESYSTEM_ROOTS_INVALID", "core.tool_filesystem",
				fmt.Sprintf("resolving allowed root %q", root)).With("root", root)
		}
		resolved[i] = filepath.Clean(abs)
	}
	return resolved, nil
}

// Resolve reports the absolute, cleaned form of path if it falls inside one
// of r's allowed roots (the root itself, or anything below it), or a
// packages/errors error typed TypePermissionDenied otherwise - SPEC-0049
// testing criterion 3, "Restricted paths are blocked".
func (r FilesystemRoots) Resolve(path string) (string, error) {
	if path == "" {
		return "", errors.New(errors.TypeInvalidInput, "FILESYSTEM_PATH_EMPTY", "core.tool_filesystem",
			"path must not be empty")
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", errors.Wrap(err, errors.TypeInvalidInput, "FILESYSTEM_PATH_INVALID", "core.tool_filesystem",
			fmt.Sprintf("resolving path %q", path)).With("path", path)
	}
	clean := filepath.Clean(abs)

	for _, root := range r {
		if clean == root || strings.HasPrefix(clean, root+string(filepath.Separator)) {
			return clean, nil
		}
	}
	return "", errors.New(errors.TypePermissionDenied, "FILESYSTEM_PATH_NOT_ALLOWED", "core.tool_filesystem",
		fmt.Sprintf("path %q is outside the allowed roots", path)).With("path", clean)
}

// stringInput extracts field from input as a non-empty string. Tool.Execute
// implementations are still responsible for their own input validation
// (tool.go's "Validate inputs" Tool Responsibility) even though
// ToolExecutionEngine also runs ValidateToolInput before Execute, since
// Execute can be invoked directly without going through the engine.
func stringInput(input map[string]any, field string) (string, error) {
	v, ok := input[field]
	if !ok || v == nil {
		return "", errors.New(errors.TypeInvalidInput, "FILESYSTEM_INPUT_MISSING_FIELD", "core.tool_filesystem",
			"required input field is missing").With("field", field)
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", errors.New(errors.TypeInvalidInput, "FILESYSTEM_INPUT_INVALID_FIELD", "core.tool_filesystem",
			"input field must be a non-empty string").With("field", field)
	}
	return s, nil
}

// notExistOrInternal wraps a filesystem error as TypeNotFound if it reports
// a missing path, or TypeInternal otherwise.
func notExistOrInternal(err error, code, path string) error {
	if os.IsNotExist(err) {
		return errors.Wrap(err, errors.TypeNotFound, code+"_NOT_FOUND", "core.tool_filesystem",
			fmt.Sprintf("path %q does not exist", path)).With("path", path)
	}
	return errors.Wrap(err, errors.TypeInternal, code+"_FAILED", "core.tool_filesystem",
		fmt.Sprintf("operating on path %q", path)).With("path", path)
}

// filesystemOp is one filesystem Tool's actual behavior, run against an
// already-resolved-safe input by filesystemTool.Execute.
type filesystemOp func(ctx context.Context, roots FilesystemRoots, input map[string]any) (map[string]any, error)

// filesystemTool is the Tool every constructor in this file produces:
// static Metadata plus an injected filesystemOp, mirroring
// tool_manifest.go's manifestTool split between declared identity and
// supplied behavior. roots is consulted by op itself (every op resolves its
// own path argument(s) via roots.Resolve).
type filesystemTool struct {
	metadata ToolMetadata
	roots    FilesystemRoots
	log      *logger.Logger
	op       filesystemOp
}

func (t *filesystemTool) Metadata() ToolMetadata { return t.metadata }

func (t *filesystemTool) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	if errType, canceled := ctxErrType(ctx); canceled {
		err := errors.Wrap(ctx.Err(), errType, "FILESYSTEM_EXECUTION_CANCELED", "core.tool_filesystem",
			fmt.Sprintf("%s canceled before running", t.metadata.ID)).With("toolId", t.metadata.ID)
		t.record(input, "canceled", err)
		return nil, err
	}

	out, err := t.op(ctx, t.roots, input)
	if err != nil {
		t.record(input, "failed", err)
		return nil, err
	}
	t.record(input, "executed", nil)
	return out, nil
}

// record logs a single Execute outcome. A no-op if no Logger is configured.
func (t *filesystemTool) record(input map[string]any, outcome string, err error) {
	if t.log == nil {
		return
	}
	fields := map[string]any{"toolId": t.metadata.ID, "outcome": outcome}
	if path, ok := input["path"].(string); ok {
		fields["path"] = path
	}
	if err != nil {
		fields["error"] = err.Error()
		t.log.Error("filesystem tool execution", fields)
		return
	}
	t.log.Info("filesystem tool execution", fields)
}

// filesystemToolConfig holds the options every filesystem Tool constructor
// in this file accepts.
type filesystemToolConfig struct {
	log *logger.Logger
}

// FilesystemToolOption configures a filesystem Tool created by one of this
// file's New* constructors.
type FilesystemToolOption func(*filesystemToolConfig)

// WithFilesystemToolLogger attaches a Logger used to record every Execute
// outcome. Optional; a tool with no logger runs silently.
func WithFilesystemToolLogger(log *logger.Logger) FilesystemToolOption {
	return func(c *filesystemToolConfig) { c.log = log }
}

// newFilesystemTool builds the filesystemTool common to every constructor
// below. It returns a packages/errors error typed TypeInvalidInput if roots
// is empty - a filesystem tool cannot enforce "Allowed paths" without at
// least one configured root.
func newFilesystemTool(metadata ToolMetadata, roots FilesystemRoots, op filesystemOp, opts []FilesystemToolOption) (Tool, error) {
	if len(roots) == 0 {
		return nil, errors.New(errors.TypeInvalidInput, "FILESYSTEM_TOOL_MISSING_ROOTS", "core.tool_filesystem",
			fmt.Sprintf("cannot create %s without allowed roots", metadata.ID)).With("toolId", metadata.ID)
	}

	cfg := &filesystemToolConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	return &filesystemTool{metadata: metadata, roots: roots, log: cfg.log, op: op}, nil
}

// NewFilesystemReadTool creates the "filesystem.read" Tool: reads the file
// at the given path and returns its contents.
func NewFilesystemReadTool(roots FilesystemRoots, opts ...FilesystemToolOption) (Tool, error) {
	return newFilesystemTool(ToolMetadata{
		ID:           "filesystem.read",
		Name:         "Filesystem Read",
		Description:  "Reads the contents of a file within the allowed filesystem roots.",
		InputSchema:  Schema{{Name: "path", Type: "string", Required: true}},
		OutputSchema: Schema{{Name: "content", Type: "string", Required: true}},
		Permissions:  []string{"filesystem.read"},
	}, roots, filesystemRead, opts)
}

func filesystemRead(_ context.Context, roots FilesystemRoots, input map[string]any) (map[string]any, error) {
	path, err := stringInput(input, "path")
	if err != nil {
		return nil, err
	}
	resolved, err := roots.Resolve(path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, notExistOrInternal(err, "FILESYSTEM_READ", resolved)
	}

	return map[string]any{"path": resolved, "content": string(data)}, nil
}

// NewFilesystemWriteTool creates the "filesystem.write" Tool: writes the
// given content to the file at path within the allowed roots, creating or
// truncating it as needed.
func NewFilesystemWriteTool(roots FilesystemRoots, opts ...FilesystemToolOption) (Tool, error) {
	return newFilesystemTool(ToolMetadata{
		ID:          "filesystem.write",
		Name:        "Filesystem Write",
		Description: "Writes content to a file within the allowed filesystem roots.",
		InputSchema: Schema{
			{Name: "path", Type: "string", Required: true},
			{Name: "content", Type: "string", Required: true},
		},
		OutputSchema: Schema{{Name: "bytesWritten", Type: "integer", Required: true}},
		Permissions:  []string{"filesystem.write"},
	}, roots, filesystemWrite, opts)
}

func filesystemWrite(_ context.Context, roots FilesystemRoots, input map[string]any) (map[string]any, error) {
	path, err := stringInput(input, "path")
	if err != nil {
		return nil, err
	}
	content, ok := input["content"]
	if !ok || content == nil {
		return nil, errors.New(errors.TypeInvalidInput, "FILESYSTEM_INPUT_MISSING_FIELD", "core.tool_filesystem",
			"required input field is missing").With("field", "content")
	}
	contentStr, ok := content.(string)
	if !ok {
		return nil, errors.New(errors.TypeInvalidInput, "FILESYSTEM_INPUT_INVALID_FIELD", "core.tool_filesystem",
			"input field must be a string").With("field", "content")
	}

	resolved, err := roots.Resolve(path)
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(resolved, []byte(contentStr), 0o644); err != nil {
		return nil, notExistOrInternal(err, "FILESYSTEM_WRITE", resolved)
	}

	return map[string]any{"path": resolved, "bytesWritten": len(contentStr)}, nil
}

// NewFilesystemListTool creates the "filesystem.list" Tool: lists the
// immediate contents of a directory within the allowed roots.
func NewFilesystemListTool(roots FilesystemRoots, opts ...FilesystemToolOption) (Tool, error) {
	return newFilesystemTool(ToolMetadata{
		ID:           "filesystem.list",
		Name:         "Filesystem List",
		Description:  "Lists the immediate contents of a directory within the allowed filesystem roots.",
		InputSchema:  Schema{{Name: "path", Type: "string", Required: true}},
		OutputSchema: Schema{{Name: "entries", Type: "array", Required: true}},
		Permissions:  []string{"filesystem.read"},
	}, roots, filesystemList, opts)
}

func filesystemList(_ context.Context, roots FilesystemRoots, input map[string]any) (map[string]any, error) {
	path, err := stringInput(input, "path")
	if err != nil {
		return nil, err
	}
	resolved, err := roots.Resolve(path)
	if err != nil {
		return nil, err
	}

	dirEntries, err := os.ReadDir(resolved)
	if err != nil {
		return nil, notExistOrInternal(err, "FILESYSTEM_LIST", resolved)
	}

	entries := make([]map[string]any, 0, len(dirEntries))
	for _, e := range dirEntries {
		info, err := e.Info()
		if err != nil {
			return nil, notExistOrInternal(err, "FILESYSTEM_LIST", filepath.Join(resolved, e.Name()))
		}
		entries = append(entries, map[string]any{
			"name":  e.Name(),
			"isDir": e.IsDir(),
			"size":  info.Size(),
		})
	}

	return map[string]any{"path": resolved, "entries": entries}, nil
}

// NewFilesystemSearchTool creates the "filesystem.search" Tool: recursively
// searches a directory within the allowed roots for files whose base name
// matches a filepath.Match glob pattern.
func NewFilesystemSearchTool(roots FilesystemRoots, opts ...FilesystemToolOption) (Tool, error) {
	return newFilesystemTool(ToolMetadata{
		ID:          "filesystem.search",
		Name:        "Filesystem Search",
		Description: "Recursively searches a directory within the allowed filesystem roots for files matching a glob pattern.",
		InputSchema: Schema{
			{Name: "path", Type: "string", Required: true},
			{Name: "pattern", Type: "string", Required: true},
		},
		OutputSchema: Schema{{Name: "matches", Type: "array", Required: true}},
		Permissions:  []string{"filesystem.read"},
	}, roots, filesystemSearch, opts)
}

func filesystemSearch(ctx context.Context, roots FilesystemRoots, input map[string]any) (map[string]any, error) {
	path, err := stringInput(input, "path")
	if err != nil {
		return nil, err
	}
	pattern, err := stringInput(input, "pattern")
	if err != nil {
		return nil, err
	}
	resolved, err := roots.Resolve(path)
	if err != nil {
		return nil, err
	}

	var matches []string
	walkErr := filepath.WalkDir(resolved, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			return nil
		}
		ok, err := filepath.Match(pattern, d.Name())
		if err != nil {
			return err
		}
		if ok {
			matches = append(matches, p)
		}
		return nil
	})
	if walkErr != nil {
		if walkErr == ctx.Err() {
			errType, _ := ctxErrType(ctx)
			return nil, errors.Wrap(walkErr, errType, "FILESYSTEM_SEARCH_CANCELED", "core.tool_filesystem",
				fmt.Sprintf("search under %q canceled", resolved)).With("path", resolved)
		}
		return nil, notExistOrInternal(walkErr, "FILESYSTEM_SEARCH", resolved)
	}

	sort.Strings(matches)
	return map[string]any{"path": resolved, "pattern": pattern, "matches": matches}, nil
}

// NewFilesystemMetadataTool creates the "filesystem.metadata" Tool:
// retrieves metadata (size, directory flag, modification time, mode) for a
// path within the allowed roots.
func NewFilesystemMetadataTool(roots FilesystemRoots, opts ...FilesystemToolOption) (Tool, error) {
	return newFilesystemTool(ToolMetadata{
		ID:           "filesystem.metadata",
		Name:         "Filesystem Metadata",
		Description:  "Retrieves metadata for a file or directory within the allowed filesystem roots.",
		InputSchema:  Schema{{Name: "path", Type: "string", Required: true}},
		OutputSchema: Schema{{Name: "size", Type: "integer", Required: true}},
		Permissions:  []string{"filesystem.read"},
	}, roots, filesystemMetadata, opts)
}

func filesystemMetadata(_ context.Context, roots FilesystemRoots, input map[string]any) (map[string]any, error) {
	path, err := stringInput(input, "path")
	if err != nil {
		return nil, err
	}
	resolved, err := roots.Resolve(path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return nil, notExistOrInternal(err, "FILESYSTEM_METADATA", resolved)
	}

	return map[string]any{
		"path":    resolved,
		"name":    info.Name(),
		"size":    info.Size(),
		"isDir":   info.IsDir(),
		"modTime": info.ModTime(),
		"mode":    info.Mode().String(),
	}, nil
}
