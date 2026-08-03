// tool_terminal.go implements SPEC-0050: the Terminal Tool - controlled
// terminal command execution (JARVIS_MASTER_ARCHITECTURE.md's Tool System
// "Terminal access" responsibility), giving agents the ability to run shell
// commands and get back captured output, exit codes, and failures.
//
// Like tool_filesystem.go (SPEC-0049), "Permission rules" and "User
// approvals" are already fully implemented by SPEC-0046's
// ToolExecutionEngine (tool_execution.go) and SPEC-0047/0048's
// PermissionChecker/ApprovalQueue (agent_permission.go, tool_approval.go) -
// a Tool need only declare its required Permissions categories and those
// layers enforce the rest before Execute ever runs.
//
// SPEC-0050's Security section additionally names "Command restrictions",
// which - like FilesystemRoots' "Allowed paths" - has no existing home:
// PermissionChecker's PermissionModel is a category-level allow/deny table,
// not a command allowlist. AllowedCommands fills that gap the same way
// FilesystemRoots does: a tool-local, defense-in-depth boundary every
// invocation is checked against.
//
// SPEC-0050's testing criteria distinguish "Safe commands execute" from
// "Dangerous commands require approval". Because ToolExecutionEngine checks
// permission categories once, from a Tool's static Metadata, before Execute
// ever sees the input, a single Tool cannot vary its approval requirement
// per invocation. This file therefore splits the capability into two Tools,
// mirroring tool_filesystem.go's read/write split (same underlying
// resource, different Permissions category so access can be scoped
// differently per agent):
//
//   - "terminal.exec": restricted to an explicit AllowedCommands allowlist
//     (the "safe" set); declares category "terminal.exec", intended to be
//     PermissionAllowed for agents trusted to run those commands.
//   - "terminal.exec.privileged": no allowlist restriction (the "dangerous"
//     set - any command); declares category "terminal.exec.privileged",
//     intended to be configured PermissionApprovalRequired in the
//     PermissionModel so every invocation is routed through SPEC-0048's
//     ApprovalQueue by the already-generic PermissionChecker flow, with no
//     special-casing needed here.
package core

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
)

// defaultTerminalTimeout is the SPEC-0050 "Execution timeout" applied when a
// terminal Tool is constructed without WithTerminalToolTimeout: long enough
// for ordinary commands, short enough that a hung process cannot block a
// caller indefinitely.
const defaultTerminalTimeout = 30 * time.Second

// AllowedCommands is the SPEC-0050 "Command restrictions" allowlist: the set
// of command names a "terminal.exec" Tool is permitted to run. It is the
// tool-local counterpart to SPEC-0024's category-level PermissionModel - a
// second, independent boundary checked on every invocation regardless of
// what the agent-level permission check already decided, mirroring
// FilesystemRoots (tool_filesystem.go).
type AllowedCommands []string

// NewAllowedCommands returns commands as an AllowedCommands allowlist. It
// returns a packages/errors error typed TypeInvalidInput if commands is
// empty or contains an empty string - a terminal tool with no allowed
// commands would either be useless (deny everything) or, if it defaulted to
// unrestricted, defeat the "Command restrictions" requirement entirely;
// requiring at least one explicit command fails closed by construction
// instead, the same precedent NewFilesystemRoots sets for allowed roots.
func NewAllowedCommands(commands ...string) (AllowedCommands, error) {
	if len(commands) == 0 {
		return nil, errors.New(errors.TypeInvalidInput, "TERMINAL_ALLOWED_COMMANDS_EMPTY", "core.tool_terminal",
			"at least one allowed command is required")
	}

	allowed := make(AllowedCommands, len(commands))
	for i, c := range commands {
		if c == "" {
			return nil, errors.New(errors.TypeInvalidInput, "TERMINAL_ALLOWED_COMMANDS_EMPTY_ENTRY", "core.tool_terminal",
				"allowed command must not be empty").With("index", i)
		}
		allowed[i] = c
	}
	return allowed, nil
}

// Check reports nil if command is in a's allowlist, or a packages/errors
// error typed TypePermissionDenied otherwise - SPEC-0050 testing criterion
// 1 ("Safe commands execute") requires membership; anything else is
// rejected before a process is ever started.
func (a AllowedCommands) Check(command string) error {
	for _, c := range a {
		if c == command {
			return nil
		}
	}
	return errors.New(errors.TypePermissionDenied, "TERMINAL_COMMAND_NOT_ALLOWED", "core.tool_terminal",
		fmt.Sprintf("command %q is not in the allowed command list", command)).With("command", command)
}

// terminalTool is the Tool both constructors in this file produce: static
// Metadata plus an optional AllowedCommands allowlist (nil means
// unrestricted - used by the privileged tool) and an execution timeout.
type terminalTool struct {
	metadata ToolMetadata
	allowed  AllowedCommands
	timeout  time.Duration
	log      *logger.Logger
}

func (t *terminalTool) Metadata() ToolMetadata { return t.metadata }

// Execute runs the SPEC-0050 flow: validate input, check command
// restrictions (if this Tool is allowlisted), run the command with the
// configured timeout, and capture its exit code, stdout, and stderr.
//
// A command that starts but exits non-zero is not treated as an Execute
// error - its exit code and output are still meaningful results the caller
// should see, satisfying "Exit code handling" and "Process output capture"
// together with "Failures are captured" (a failing command is a captured
// result, not a broken invocation). A command that cannot be started at all
// (unknown executable, timeout, cancellation) is reported as a Go error,
// since no result was produced for that invocation.
func (t *terminalTool) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	if errType, canceled := ctxErrType(ctx); canceled {
		err := errors.Wrap(ctx.Err(), errType, "TERMINAL_EXECUTION_CANCELED", "core.tool_terminal",
			fmt.Sprintf("%s canceled before running", t.metadata.ID)).With("toolId", t.metadata.ID)
		t.record(input, "canceled", err)
		return nil, err
	}

	command, err := stringInput(input, "command")
	if err != nil {
		t.record(input, "invalid_input", err)
		return nil, err
	}

	if t.allowed != nil {
		if err := t.allowed.Check(command); err != nil {
			t.record(input, "denied", err)
			return nil, err
		}
	}

	args, err := stringSliceInput(input, "args")
	if err != nil {
		t.record(input, "invalid_input", err)
		return nil, err
	}

	runCtx := ctx
	if t.timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, t.timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(runCtx, command, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	if errType, timedOut := ctxErrType(runCtx); timedOut && runErr != nil {
		err := errors.Wrap(runErr, errType, "TERMINAL_EXECUTION_TIMEOUT", "core.tool_terminal",
			fmt.Sprintf("command %q timed out or was canceled", command)).With("command", command)
		t.record(input, "timeout", err)
		return nil, err
	}

	exitCode := 0
	if runErr != nil {
		exitErr, ok := runErr.(*exec.ExitError)
		if !ok {
			wrapped := errors.Wrap(runErr, errors.TypeInternal, "TERMINAL_START_FAILED", "core.tool_terminal",
				fmt.Sprintf("starting command %q", command)).With("command", command)
			t.record(input, "start_failed", wrapped)
			return nil, wrapped
		}
		exitCode = exitErr.ExitCode()
	}

	t.record(input, "executed", nil)
	return map[string]any{
		"exitCode": exitCode,
		"stdout":   stdout.String(),
		"stderr":   stderr.String(),
	}, nil
}

// record logs a single Execute outcome. A no-op if no Logger is configured.
func (t *terminalTool) record(input map[string]any, outcome string, err error) {
	if t.log == nil {
		return
	}
	fields := map[string]any{"toolId": t.metadata.ID, "outcome": outcome}
	if command, ok := input["command"].(string); ok {
		fields["command"] = command
	}
	if err != nil {
		fields["error"] = err.Error()
		t.log.Error("terminal tool execution", fields)
		return
	}
	t.log.Info("terminal tool execution", fields)
}

// stringSliceInput extracts field from input as a []string. A missing or
// nil field returns a nil slice (the field is optional for every terminal
// Tool that uses it - "args" may be legitimately empty). Accepts both
// []string and []any (the shape a JSON-decoded array typically takes) so
// callers built on either representation work without an adapter.
func stringSliceInput(input map[string]any, field string) ([]string, error) {
	v, ok := input[field]
	if !ok || v == nil {
		return nil, nil
	}
	if s, ok := v.([]string); ok {
		return s, nil
	}
	raw, ok := v.([]any)
	if !ok {
		return nil, errors.New(errors.TypeInvalidInput, "TERMINAL_INPUT_INVALID_FIELD", "core.tool_terminal",
			"input field must be an array of strings").With("field", field)
	}
	out := make([]string, len(raw))
	for i, item := range raw {
		s, ok := item.(string)
		if !ok {
			return nil, errors.New(errors.TypeInvalidInput, "TERMINAL_INPUT_INVALID_FIELD", "core.tool_terminal",
				"input field must be an array of strings").With("field", field).With("index", i)
		}
		out[i] = s
	}
	return out, nil
}

// terminalToolConfig holds the options both terminal Tool constructors in
// this file accept.
type terminalToolConfig struct {
	log     *logger.Logger
	timeout time.Duration
}

// TerminalToolOption configures a terminal Tool created by one of this
// file's New* constructors.
type TerminalToolOption func(*terminalToolConfig)

// WithTerminalToolLogger attaches a Logger used to record every Execute
// outcome. Optional; a tool with no logger runs silently.
func WithTerminalToolLogger(log *logger.Logger) TerminalToolOption {
	return func(c *terminalToolConfig) { c.log = log }
}

// WithTerminalToolTimeout overrides defaultTerminalTimeout, the SPEC-0050
// "Execution timeout" a command is allowed to run for before it is killed
// and TERMINAL_EXECUTION_TIMEOUT is reported. A zero or negative d disables
// the timeout entirely, leaving cancellation to ctx alone.
func WithTerminalToolTimeout(d time.Duration) TerminalToolOption {
	return func(c *terminalToolConfig) { c.timeout = d }
}

// terminalInputSchema and terminalOutputSchema are shared by both terminal
// Tool constructors: same input/output shape regardless of whether the
// command is allowlist-restricted.
var (
	terminalInputSchema = Schema{
		{Name: "command", Type: "string", Required: true},
		{Name: "args", Type: "array", Required: false},
	}
	terminalOutputSchema = Schema{
		{Name: "exitCode", Type: "integer", Required: true},
		{Name: "stdout", Type: "string", Required: true},
		{Name: "stderr", Type: "string", Required: true},
	}
)

// newTerminalTool builds the terminalTool common to both constructors below.
func newTerminalTool(metadata ToolMetadata, allowed AllowedCommands, opts []TerminalToolOption) Tool {
	cfg := &terminalToolConfig{timeout: defaultTerminalTimeout}
	for _, opt := range opts {
		opt(cfg)
	}
	return &terminalTool{metadata: metadata, allowed: allowed, timeout: cfg.timeout, log: cfg.log}
}

// NewTerminalExecTool creates the "terminal.exec" Tool: runs a command from
// the given AllowedCommands allowlist and captures its output, exit code,
// and failures. It returns a packages/errors error typed TypeInvalidInput
// if allowed is empty - this Tool cannot enforce "Command restrictions"
// without at least one permitted command.
func NewTerminalExecTool(allowed AllowedCommands, opts ...TerminalToolOption) (Tool, error) {
	if len(allowed) == 0 {
		return nil, errors.New(errors.TypeInvalidInput, "TERMINAL_TOOL_MISSING_ALLOWED_COMMANDS", "core.tool_terminal",
			"cannot create terminal.exec without an allowed command list")
	}
	return newTerminalTool(ToolMetadata{
		ID:           "terminal.exec",
		Name:         "Terminal Execute",
		Description:  "Executes a command from the configured allowlist and captures its output, exit code, and failures.",
		InputSchema:  terminalInputSchema,
		OutputSchema: terminalOutputSchema,
		Permissions:  []string{"terminal.exec"},
	}, allowed, opts), nil
}

// NewTerminalPrivilegedExecTool creates the "terminal.exec.privileged"
// Tool: runs any command, with no allowlist restriction. It declares the
// "terminal.exec.privileged" permission category so an operator can
// configure it PermissionApprovalRequired in the PermissionModel - SPEC-0050
// testing criterion 2, "Dangerous commands require approval", enforced
// entirely by the existing PermissionChecker/ApprovalQueue flow with no
// special-casing in this Tool.
func NewTerminalPrivilegedExecTool(opts ...TerminalToolOption) Tool {
	return newTerminalTool(ToolMetadata{
		ID:           "terminal.exec.privileged",
		Name:         "Terminal Execute (Privileged)",
		Description:  "Executes any command with no allowlist restriction. Requires human approval via the configured Permission Model.",
		InputSchema:  terminalInputSchema,
		OutputSchema: terminalOutputSchema,
		Permissions:  []string{"terminal.exec.privileged"},
	}, nil, opts)
}
