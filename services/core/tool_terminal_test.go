package core

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	pkgerrors "jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
)

// TestMain intercepts a special re-invocation of this test binary as a
// helper subprocess, so terminal Tool tests can exercise real process
// execution (stdout/stderr capture, exit codes, hangs) without depending on
// OS-specific executables like "echo" (a shell builtin, not a standalone
// executable, on Windows).
func TestMain(m *testing.M) {
	if os.Getenv("JARVIS_TERMINAL_TEST_HELPER") == "1" {
		os.Exit(runTerminalTestHelper())
	}
	os.Exit(m.Run())
}

// runTerminalTestHelper implements the terminal test helper's behavior,
// selected by JARVIS_TERMINAL_TEST_MODE.
func runTerminalTestHelper() int {
	switch os.Getenv("JARVIS_TERMINAL_TEST_MODE") {
	case "echo":
		msg := "hello-terminal-tool"
		if len(os.Args) > 1 {
			msg = strings.Join(os.Args[1:], " ")
		}
		fmt.Fprint(os.Stdout, msg)
		return 0
	case "fail":
		fmt.Fprint(os.Stderr, "boom")
		return 3
	case "sleep":
		time.Sleep(5 * time.Second)
		return 0
	default:
		return 99
	}
}

// withTerminalHelperEnv sets the environment variables that route a
// re-exec of this test binary into runTerminalTestHelper with the given
// mode, and returns a cleanup func restoring the previous environment.
func withTerminalHelperEnv(t *testing.T, mode string) {
	t.Helper()
	t.Setenv("JARVIS_TERMINAL_TEST_HELPER", "1")
	t.Setenv("JARVIS_TERMINAL_TEST_MODE", mode)
}

// TestNewAllowedCommands_RequiresCommands verifies a "terminal.exec" Tool
// cannot be constructed without at least one explicit allowed command, so
// "Command restrictions" fails closed rather than defaulting to
// unrestricted execution.
func TestNewAllowedCommands_RequiresCommands(t *testing.T) {
	if _, err := NewAllowedCommands(); !pkgerrors.Is(err, pkgerrors.TypeInvalidInput) {
		t.Fatalf("NewAllowedCommands() error = %v, want TypeInvalidInput", err)
	}
	if _, err := NewAllowedCommands(""); !pkgerrors.Is(err, pkgerrors.TypeInvalidInput) {
		t.Fatalf("NewAllowedCommands(\"\") error = %v, want TypeInvalidInput", err)
	}
	if _, err := NewTerminalExecTool(nil); !pkgerrors.Is(err, pkgerrors.TypeInvalidInput) {
		t.Fatalf("NewTerminalExecTool(nil) error = %v, want TypeInvalidInput", err)
	}
}

// TestAllowedCommands_Check verifies AllowedCommands.Check permits an exact
// match and rejects anything else with TypePermissionDenied.
func TestAllowedCommands_Check(t *testing.T) {
	allowed, err := NewAllowedCommands("git", "ls")
	if err != nil {
		t.Fatalf("NewAllowedCommands returned error: %v", err)
	}

	if err := allowed.Check("git"); err != nil {
		t.Errorf("Check(git) = %v, want nil", err)
	}
	if err := allowed.Check("rm"); !pkgerrors.Is(err, pkgerrors.TypePermissionDenied) {
		t.Errorf("Check(rm) = %v, want TypePermissionDenied", err)
	}
}

// TestTerminalExecTool_SafeCommandsExecute verifies a command within the
// configured allowlist runs and its stdout, stderr, and exit code are
// captured (SPEC-0050 testing criterion 1: "Safe commands execute", and the
// "Process output capture"/"Exit code handling" requirements).
func TestTerminalExecTool_SafeCommandsExecute(t *testing.T) {
	withTerminalHelperEnv(t, "echo")
	self := os.Args[0]

	allowed, err := NewAllowedCommands(self)
	if err != nil {
		t.Fatalf("NewAllowedCommands returned error: %v", err)
	}
	tool, err := NewTerminalExecTool(allowed)
	if err != nil {
		t.Fatalf("NewTerminalExecTool returned error: %v", err)
	}

	out, err := tool.Execute(context.Background(), map[string]any{"command": self})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out["exitCode"] != 0 {
		t.Errorf("exitCode = %v, want 0", out["exitCode"])
	}
	if out["stdout"] != "hello-terminal-tool" {
		t.Errorf("stdout = %q, want %q", out["stdout"], "hello-terminal-tool")
	}
	if out["stderr"] != "" {
		t.Errorf("stderr = %q, want empty", out["stderr"])
	}
}

// TestTerminalExecTool_RestrictedCommandIsBlocked verifies a command
// outside the configured allowlist is rejected before any process starts
// (SPEC-0050 Security: "Command restrictions").
func TestTerminalExecTool_RestrictedCommandIsBlocked(t *testing.T) {
	allowed, err := NewAllowedCommands("some-allowed-command")
	if err != nil {
		t.Fatalf("NewAllowedCommands returned error: %v", err)
	}
	tool, err := NewTerminalExecTool(allowed)
	if err != nil {
		t.Fatalf("NewTerminalExecTool returned error: %v", err)
	}

	_, err = tool.Execute(context.Background(), map[string]any{"command": "not-allowed"})
	if !pkgerrors.Is(err, pkgerrors.TypePermissionDenied) {
		t.Fatalf("Execute error = %v, want TypePermissionDenied", err)
	}
}

// TestTerminalExecTool_FailuresAreCaptured verifies a command that starts
// but exits non-zero returns its exit code and stderr in the result rather
// than as a Go error, and a command that never starts is reported as a Go
// error (SPEC-0050 testing criterion 3: "Failures are captured").
func TestTerminalExecTool_FailuresAreCaptured(t *testing.T) {
	t.Run("non-zero exit is captured in the result", func(t *testing.T) {
		withTerminalHelperEnv(t, "fail")
		self := os.Args[0]

		allowed, err := NewAllowedCommands(self)
		if err != nil {
			t.Fatalf("NewAllowedCommands returned error: %v", err)
		}
		tool, err := NewTerminalExecTool(allowed)
		if err != nil {
			t.Fatalf("NewTerminalExecTool returned error: %v", err)
		}

		out, err := tool.Execute(context.Background(), map[string]any{"command": self})
		if err != nil {
			t.Fatalf("Execute returned error: %v, want a captured result instead", err)
		}
		if out["exitCode"] != 3 {
			t.Errorf("exitCode = %v, want 3", out["exitCode"])
		}
		stderr, _ := out["stderr"].(string)
		if !strings.Contains(stderr, "boom") {
			t.Errorf("stderr = %q, want to contain %q", stderr, "boom")
		}
	})

	t.Run("a command that cannot start is a Go error", func(t *testing.T) {
		const missing = "jarvis-terminal-tool-test-nonexistent-command"
		allowed, err := NewAllowedCommands(missing)
		if err != nil {
			t.Fatalf("NewAllowedCommands returned error: %v", err)
		}
		tool, err := NewTerminalExecTool(allowed)
		if err != nil {
			t.Fatalf("NewTerminalExecTool returned error: %v", err)
		}

		_, err = tool.Execute(context.Background(), map[string]any{"command": missing})
		if !pkgerrors.Is(err, pkgerrors.TypeInternal) {
			t.Fatalf("Execute error = %v, want TypeInternal", err)
		}
	})
}

// TestTerminalExecTool_ExecutionTimeout verifies a command still running
// past the configured timeout is killed and reported as TypeTimeout
// (SPEC-0050 Requirements: "Execution timeout").
func TestTerminalExecTool_ExecutionTimeout(t *testing.T) {
	withTerminalHelperEnv(t, "sleep")
	self := os.Args[0]

	allowed, err := NewAllowedCommands(self)
	if err != nil {
		t.Fatalf("NewAllowedCommands returned error: %v", err)
	}
	tool, err := NewTerminalExecTool(allowed, WithTerminalToolTimeout(50*time.Millisecond))
	if err != nil {
		t.Fatalf("NewTerminalExecTool returned error: %v", err)
	}

	_, err = tool.Execute(context.Background(), map[string]any{"command": self})
	if !pkgerrors.Is(err, pkgerrors.TypeTimeout) {
		t.Fatalf("Execute error = %v, want TypeTimeout", err)
	}
}

// TestTerminalExecTool_ExecutionIsLogged verifies a configured Logger
// records every Execute outcome (SPEC-0050 Security: "Execution logging").
func TestTerminalExecTool_ExecutionIsLogged(t *testing.T) {
	withTerminalHelperEnv(t, "echo")
	self := os.Args[0]

	allowed, err := NewAllowedCommands(self)
	if err != nil {
		t.Fatalf("NewAllowedCommands returned error: %v", err)
	}

	var buf strings.Builder
	log := logger.New("test", logger.WithOutput(&buf))
	tool, err := NewTerminalExecTool(allowed, WithTerminalToolLogger(log))
	if err != nil {
		t.Fatalf("NewTerminalExecTool returned error: %v", err)
	}

	if _, err := tool.Execute(context.Background(), map[string]any{"command": self}); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !strings.Contains(buf.String(), "terminal tool execution") {
		t.Errorf("log output = %q, want it to contain %q", buf.String(), "terminal tool execution")
	}
}

// TestTerminalExecTool_RespectsContextCancellation verifies a pre-canceled
// ctx is reported as TypeCanceled before any process is started, matching
// tool_filesystem.go's TestFilesystemTool_RespectsContextCancellation
// precedent for every Tool in this package.
func TestTerminalExecTool_RespectsContextCancellation(t *testing.T) {
	allowed, err := NewAllowedCommands("irrelevant")
	if err != nil {
		t.Fatalf("NewAllowedCommands returned error: %v", err)
	}
	tool, err := NewTerminalExecTool(allowed)
	if err != nil {
		t.Fatalf("NewTerminalExecTool returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = tool.Execute(ctx, map[string]any{"command": "irrelevant"})
	if !pkgerrors.Is(err, pkgerrors.TypeCanceled) {
		t.Fatalf("Execute error = %v, want TypeCanceled", err)
	}
}

// TestTerminalExecTool_ArgsAreForwarded verifies the optional "args" input
// field reaches the executed process, accepting both a []string and a
// []any shape (the latter being what a JSON-decoded array typically
// produces).
func TestTerminalExecTool_ArgsAreForwarded(t *testing.T) {
	withTerminalHelperEnv(t, "echo")
	self := os.Args[0]

	allowed, err := NewAllowedCommands(self)
	if err != nil {
		t.Fatalf("NewAllowedCommands returned error: %v", err)
	}
	tool, err := NewTerminalExecTool(allowed)
	if err != nil {
		t.Fatalf("NewTerminalExecTool returned error: %v", err)
	}

	t.Run("[]string", func(t *testing.T) {
		out, err := tool.Execute(context.Background(), map[string]any{
			"command": self,
			"args":    []string{"foo", "bar"},
		})
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if out["stdout"] != "foo bar" {
			t.Errorf("stdout = %q, want %q", out["stdout"], "foo bar")
		}
	})

	t.Run("[]any", func(t *testing.T) {
		out, err := tool.Execute(context.Background(), map[string]any{
			"command": self,
			"args":    []any{"foo", "bar"},
		})
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if out["stdout"] != "foo bar" {
			t.Errorf("stdout = %q, want %q", out["stdout"], "foo bar")
		}
	})
}

// TestTerminalExecTool_InvalidArgsTypeIsRejected verifies a non-array
// "args" field, and a non-string element within it, are both reported as
// TypeInvalidInput rather than silently coerced or passed through.
func TestTerminalExecTool_InvalidArgsTypeIsRejected(t *testing.T) {
	allowed, err := NewAllowedCommands("irrelevant")
	if err != nil {
		t.Fatalf("NewAllowedCommands returned error: %v", err)
	}
	tool, err := NewTerminalExecTool(allowed)
	if err != nil {
		t.Fatalf("NewTerminalExecTool returned error: %v", err)
	}

	t.Run("non-array args", func(t *testing.T) {
		_, err := tool.Execute(context.Background(), map[string]any{"command": "irrelevant", "args": 123})
		if !pkgerrors.Is(err, pkgerrors.TypeInvalidInput) {
			t.Fatalf("Execute error = %v, want TypeInvalidInput", err)
		}
	})

	t.Run("non-string element", func(t *testing.T) {
		_, err := tool.Execute(context.Background(), map[string]any{"command": "irrelevant", "args": []any{123}})
		if !pkgerrors.Is(err, pkgerrors.TypeInvalidInput) {
			t.Fatalf("Execute error = %v, want TypeInvalidInput", err)
		}
	})
}

// TestTerminalTools_DeclarePermissionCategories verifies terminal.exec and
// terminal.exec.privileged each declare exactly the permission category
// their name implies, so an operator can configure the two independently in
// the PermissionModel (e.g. terminal.exec allowed, terminal.exec.privileged
// approval-required) without one category leaking into the other.
func TestTerminalTools_DeclarePermissionCategories(t *testing.T) {
	allowed, err := NewAllowedCommands("git")
	if err != nil {
		t.Fatalf("NewAllowedCommands returned error: %v", err)
	}
	execTool, err := NewTerminalExecTool(allowed)
	if err != nil {
		t.Fatalf("NewTerminalExecTool returned error: %v", err)
	}
	privilegedTool := NewTerminalPrivilegedExecTool()

	for _, tc := range []struct {
		name string
		tool Tool
		want string
	}{
		{"exec", execTool, "terminal.exec"},
		{"privileged", privilegedTool, "terminal.exec.privileged"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			perms := tc.tool.Metadata().Permissions
			if len(perms) != 1 || perms[0] != tc.want {
				t.Errorf("Permissions = %v, want [%s]", perms, tc.want)
			}
		})
	}
}

// TestTerminalPrivilegedExecTool_DangerousCommandsRequireApproval verifies
// terminal.exec.privileged, wired through ToolExecutionEngine with a
// PermissionModel that marks its category ApprovalRequired, blocks until a
// human resolves the pending ApprovalQueue request (SPEC-0050 testing
// criterion 2: "Dangerous commands require approval").
func TestTerminalPrivilegedExecTool_DangerousCommandsRequireApproval(t *testing.T) {
	withTerminalHelperEnv(t, "echo")
	self := os.Args[0]

	tool := NewTerminalPrivilegedExecTool()
	registry := NewToolRegistry()
	if err := registry.Register(tool); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	queue := NewApprovalQueue()
	model := PermissionModel{
		"developer_agent": AgentPermissions{"terminal.exec.privileged": PermissionApprovalRequired},
	}
	checker := NewPermissionChecker(model, WithApprovalFunc(queue.AsApprovalFunc()))

	engine, err := NewToolExecutionEngine(registry, WithExecutionPermissionChecker(checker))
	if err != nil {
		t.Fatalf("NewToolExecutionEngine returned error: %v", err)
	}

	type result struct {
		out map[string]any
		err error
	}
	resultCh := make(chan result, 1)
	go func() {
		out, err := engine.Execute(context.Background(), "developer_agent", "terminal.exec.privileged",
			map[string]any{"command": self})
		resultCh <- result{out, err}
	}()

	select {
	case <-resultCh:
		t.Fatal("Execute returned before approval was resolved")
	case <-time.After(20 * time.Millisecond):
	}

	id := waitForPending(t, queue)
	if err := queue.Resolve(id, true); err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	res := <-resultCh
	if res.err != nil {
		t.Fatalf("Execute returned error: %v", res.err)
	}
	if res.out["stdout"] != "hello-terminal-tool" {
		t.Errorf("stdout = %v, want %q", res.out["stdout"], "hello-terminal-tool")
	}
}

// TestTerminalPrivilegedExecTool_RejectedApprovalDenies verifies a rejected
// approval stops the command from running at all.
func TestTerminalPrivilegedExecTool_RejectedApprovalDenies(t *testing.T) {
	tool := NewTerminalPrivilegedExecTool()
	registry := NewToolRegistry()
	if err := registry.Register(tool); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	queue := NewApprovalQueue()
	model := PermissionModel{
		"developer_agent": AgentPermissions{"terminal.exec.privileged": PermissionApprovalRequired},
	}
	checker := NewPermissionChecker(model, WithApprovalFunc(queue.AsApprovalFunc()))

	engine, err := NewToolExecutionEngine(registry, WithExecutionPermissionChecker(checker))
	if err != nil {
		t.Fatalf("NewToolExecutionEngine returned error: %v", err)
	}

	type result struct {
		err error
	}
	resultCh := make(chan result, 1)
	go func() {
		_, err := engine.Execute(context.Background(), "developer_agent", "terminal.exec.privileged",
			map[string]any{"command": "irrelevant"})
		resultCh <- result{err}
	}()

	id := waitForPending(t, queue)
	if err := queue.Resolve(id, false); err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	res := <-resultCh
	if !pkgerrors.Is(res.err, pkgerrors.TypePermissionDenied) {
		t.Fatalf("Execute error = %v, want TypePermissionDenied", res.err)
	}
}
