package core

import (
	"bytes"
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pkgerrors "jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
	types "jarvis-pa/packages/shared-types"
)

func writePermissionModel(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "permissions.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	return path
}

// TestLoadPermissionModel_LoadsCorrectly verifies a well-formed permission
// table (matching SPEC-0024's own example) loads with every declared level.
func TestLoadPermissionModel_LoadsCorrectly(t *testing.T) {
	path := writePermissionModel(t, `
developer_agent:
  filesystem: allowed
  terminal: approval_required

research_agent:
  browser: allowed
  terminal: denied
`)

	model, err := LoadPermissionModel(path)
	if err != nil {
		t.Fatalf("LoadPermissionModel returned error: %v", err)
	}

	if model.Level("developer_agent", "filesystem") != PermissionAllowed {
		t.Errorf("developer_agent/filesystem = %v, want allowed", model.Level("developer_agent", "filesystem"))
	}
	if model.Level("developer_agent", "terminal") != PermissionApprovalRequired {
		t.Errorf("developer_agent/terminal = %v, want approval_required", model.Level("developer_agent", "terminal"))
	}
	if model.Level("research_agent", "browser") != PermissionAllowed {
		t.Errorf("research_agent/browser = %v, want allowed", model.Level("research_agent", "browser"))
	}
	if model.Level("research_agent", "terminal") != PermissionDenied {
		t.Errorf("research_agent/terminal = %v, want denied", model.Level("research_agent", "terminal"))
	}
}

// TestLoadPermissionModel_InvalidLevelFailsValidation verifies a level
// outside the three recognized values is rejected rather than silently
// loaded.
func TestLoadPermissionModel_InvalidLevelFailsValidation(t *testing.T) {
	path := writePermissionModel(t, `
developer_agent:
  filesystem: sometimes
`)

	_, err := LoadPermissionModel(path)
	if err == nil {
		t.Fatal("LoadPermissionModel() = nil, want error")
	}
	if !pkgerrors.HasCode(err, "PERMISSION_MODEL_INVALID_LEVEL") {
		t.Errorf("missing code PERMISSION_MODEL_INVALID_LEVEL: %v", err)
	}
}

// TestLoadPermissionModel_MalformedYAML verifies malformed YAML is reported
// as a distinct parse failure.
func TestLoadPermissionModel_MalformedYAML(t *testing.T) {
	path := writePermissionModel(t, "developer_agent: [unterminated\n")

	_, err := LoadPermissionModel(path)
	if !pkgerrors.HasCode(err, "PERMISSION_MODEL_PARSE_ERROR") {
		t.Errorf("missing code PERMISSION_MODEL_PARSE_ERROR: %v", err)
	}
}

// TestLoadPermissionModel_MissingFile verifies a missing permission model
// path is reported as a distinct not-found error rather than a parse
// failure.
func TestLoadPermissionModel_MissingFile(t *testing.T) {
	_, err := LoadPermissionModel(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if !pkgerrors.HasCode(err, "PERMISSION_MODEL_READ_FAILED") {
		t.Errorf("missing code PERMISSION_MODEL_READ_FAILED: %v", err)
	}
}

// TestPermissionModel_Level_UndeclaredDefaultsToDenied verifies an agent or
// category absent from the model fails closed rather than defaulting to
// allowed.
func TestPermissionModel_Level_UndeclaredDefaultsToDenied(t *testing.T) {
	model := PermissionModel{"developer_agent": AgentPermissions{"filesystem": PermissionAllowed}}

	if got := model.Level("developer_agent", "browser"); got != PermissionDenied {
		t.Errorf("undeclared category = %v, want denied", got)
	}
	if got := model.Level("unknown_agent", "filesystem"); got != PermissionDenied {
		t.Errorf("undeclared agent = %v, want denied", got)
	}
}

// TestPermissionChecker_Check_AllowedToolsExecute verifies an Allowed
// category returns nil (SPEC-0024's "allowed tools execute" criterion).
func TestPermissionChecker_Check_AllowedToolsExecute(t *testing.T) {
	model := PermissionModel{"developer_agent": AgentPermissions{"filesystem": PermissionAllowed}}
	checker := NewPermissionChecker(model)

	if err := checker.Check(context.Background(), "developer_agent", "filesystem"); err != nil {
		t.Errorf("Check() = %v, want nil", err)
	}
}

// TestPermissionChecker_Check_RestrictedToolsAreBlocked verifies a Denied
// category, and an undeclared agent/category, are both rejected with
// TypePermissionDenied (SPEC-0024's "restricted tools are blocked"
// criterion).
func TestPermissionChecker_Check_RestrictedToolsAreBlocked(t *testing.T) {
	model := PermissionModel{"research_agent": AgentPermissions{"terminal": PermissionDenied}}
	checker := NewPermissionChecker(model)

	tests := []struct {
		name     string
		agentID  string
		category string
	}{
		{"explicit deny", "research_agent", "terminal"},
		{"undeclared category", "research_agent", "filesystem"},
		{"undeclared agent", "unknown_agent", "terminal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checker.Check(context.Background(), tt.agentID, tt.category)
			if err == nil {
				t.Fatal("Check() = nil, want error")
			}
			if !pkgerrors.HasCode(err, "AGENT_PERMISSION_DENIED") {
				t.Errorf("missing code AGENT_PERMISSION_DENIED: %v", err)
			}
			if !pkgerrors.Is(err, pkgerrors.TypePermissionDenied) {
				t.Errorf("Type = %v, want TypePermissionDenied", err)
			}
		})
	}
}

// TestPermissionChecker_Check_ApprovalRequired verifies ApprovalRequired
// grants or denies based on the configured ApprovalFunc's outcome.
func TestPermissionChecker_Check_ApprovalRequired(t *testing.T) {
	model := PermissionModel{"developer_agent": AgentPermissions{"terminal": PermissionApprovalRequired}}

	t.Run("granted", func(t *testing.T) {
		checker := NewPermissionChecker(model, WithApprovalFunc(func(ctx context.Context, agentID, category string) (bool, error) {
			return true, nil
		}))
		if err := checker.Check(context.Background(), "developer_agent", "terminal"); err != nil {
			t.Errorf("Check() = %v, want nil", err)
		}
	})

	t.Run("declined", func(t *testing.T) {
		checker := NewPermissionChecker(model, WithApprovalFunc(func(ctx context.Context, agentID, category string) (bool, error) {
			return false, nil
		}))
		err := checker.Check(context.Background(), "developer_agent", "terminal")
		if !pkgerrors.HasCode(err, "AGENT_PERMISSION_DENIED") {
			t.Errorf("missing code AGENT_PERMISSION_DENIED: %v", err)
		}
	})

	t.Run("approval func error", func(t *testing.T) {
		approvalErr := stderrors.New("prompt unavailable")
		checker := NewPermissionChecker(model, WithApprovalFunc(func(ctx context.Context, agentID, category string) (bool, error) {
			return false, approvalErr
		}))
		err := checker.Check(context.Background(), "developer_agent", "terminal")
		if !pkgerrors.HasCode(err, "PERMISSION_APPROVAL_FAILED") {
			t.Errorf("missing code PERMISSION_APPROVAL_FAILED: %v", err)
		}
	})

	t.Run("no approver configured fails closed", func(t *testing.T) {
		checker := NewPermissionChecker(model)
		err := checker.Check(context.Background(), "developer_agent", "terminal")
		if !pkgerrors.HasCode(err, "AGENT_PERMISSION_DENIED") {
			t.Errorf("missing code AGENT_PERMISSION_DENIED: %v", err)
		}
	})
}

// TestPermissionChecker_Check_LogsEveryOutcome verifies allowed, denied,
// and approval-required checks are all logged (SPEC-0024's "permission
// checks are logged" criterion).
func TestPermissionChecker_Check_LogsEveryOutcome(t *testing.T) {
	model := PermissionModel{
		"developer_agent": AgentPermissions{
			"filesystem": PermissionAllowed,
			"terminal":   PermissionApprovalRequired,
			"network":    PermissionDenied,
		},
	}

	var buf bytes.Buffer
	log := logger.New("test", logger.WithOutput(&buf))
	checker := NewPermissionChecker(model,
		WithPermissionCheckerLogger(log),
		WithApprovalFunc(func(ctx context.Context, agentID, category string) (bool, error) { return true, nil }),
	)

	checker.Check(context.Background(), "developer_agent", "filesystem")
	checker.Check(context.Background(), "developer_agent", "terminal")
	checker.Check(context.Background(), "developer_agent", "network")

	out := buf.String()
	for _, want := range []string{`"decision":"allowed"`, `"decision":"approved"`, `"decision":"denied"`} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %s; got %s", want, out)
		}
	}
}

// TestPermissionEnforcedToolCaller_BlocksAndAllows verifies the
// ExecutionLoop ToolCaller wrapper enforces the checker before delegating
// to the wrapped caller.
func TestPermissionEnforcedToolCaller_BlocksAndAllows(t *testing.T) {
	model := PermissionModel{"developer_agent": AgentPermissions{"filesystem": PermissionAllowed}}
	checker := NewPermissionChecker(model)

	var called bool
	next := func(ctx context.Context, tool string, input map[string]any) (map[string]any, error) {
		called = true
		return map[string]any{"ok": true}, nil
	}

	caller := PermissionEnforcedToolCaller(checker, "developer_agent", next)

	if _, err := caller(context.Background(), "filesystem", nil); err != nil {
		t.Errorf("allowed tool: Check() = %v, want nil", err)
	}
	if !called {
		t.Error("allowed tool: wrapped ToolCaller was not invoked")
	}

	called = false
	_, err := caller(context.Background(), "terminal", nil)
	if !pkgerrors.HasCode(err, "AGENT_PERMISSION_DENIED") {
		t.Errorf("blocked tool: missing code AGENT_PERMISSION_DENIED: %v", err)
	}
	if called {
		t.Error("blocked tool: wrapped ToolCaller must not be invoked")
	}
}

// TestPermissionModel_Level_DeclaredAgentEmptyCategories verifies an agent
// present in the model but with no categories declared still fails closed,
// distinct from an agent entirely absent from the model.
func TestPermissionModel_Level_DeclaredAgentEmptyCategories(t *testing.T) {
	model := PermissionModel{"developer_agent": AgentPermissions{}}
	if got := model.Level("developer_agent", "filesystem"); got != PermissionDenied {
		t.Errorf("agent with no declared categories = %v, want denied", got)
	}
}

// TestPermissionModel_Validate_DirectCall verifies Validate accepts a
// multi-agent, multi-category model built directly in Go (not just one
// loaded via LoadPermissionModel).
func TestPermissionModel_Validate_DirectCall(t *testing.T) {
	model := PermissionModel{
		"developer_agent": AgentPermissions{"filesystem": PermissionAllowed, "terminal": PermissionApprovalRequired},
		"research_agent":  AgentPermissions{"browser": PermissionAllowed, "terminal": PermissionDenied},
	}
	if err := model.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

// TestExecutionLoop_Run_EnforcesPermissions verifies
// PermissionEnforcedToolCaller integrates with the real SPEC-0022
// ExecutionLoop: a Plan step naming a permitted tool executes, and a step
// naming a denied tool stops the loop with AGENT_PERMISSION_DENIED, proving
// the enforcement point actually sits on the loop's Execute Actions stage
// rather than only working in isolation.
func TestExecutionLoop_Run_EnforcesPermissions(t *testing.T) {
	model := PermissionModel{
		"developer_agent": AgentPermissions{"filesystem": PermissionAllowed, "network": PermissionDenied},
	}
	checker := NewPermissionChecker(model)

	rawCaller := func(ctx context.Context, tool string, input map[string]any) (map[string]any, error) {
		return map[string]any{"tool": tool}, nil
	}
	enforced := PermissionEnforcedToolCaller(checker, "developer_agent", rawCaller)

	planner := func(ctx context.Context, task *types.Task, analysis map[string]any) (Plan, error) {
		return Plan{Steps: []Step{
			{Name: "read-file", Tool: "filesystem"},
			{Name: "call-out", Tool: "network"},
		}}, nil
	}

	loop, err := NewExecutionLoop(planner, WithToolCaller(enforced))
	if err != nil {
		t.Fatalf("NewExecutionLoop returned error: %v", err)
	}

	result, err := loop.Run(context.Background(), &types.Task{ID: "task-1"})
	if err == nil {
		t.Fatal("Run() = nil error, want failure at the denied network step")
	}
	if !pkgerrors.HasCode(err, "AGENT_PERMISSION_DENIED") {
		t.Errorf("missing code AGENT_PERMISSION_DENIED: %v", err)
	}

	steps, ok := result["steps"].([]map[string]any)
	if !ok || len(steps) != 2 {
		t.Fatalf("result[steps] = %#v, want 2 recorded steps (one succeeded, one denied)", result["steps"])
	}
	if steps[0]["error"] != "" {
		t.Errorf("step 0 (filesystem, allowed) recorded error %v, want none", steps[0]["error"])
	}
	if steps[1]["error"] == "" {
		t.Error("step 1 (network, denied) recorded no error, want AGENT_PERMISSION_DENIED")
	}
}
