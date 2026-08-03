package core

import (
	"bytes"
	"context"
	stderrors "errors"
	"strings"
	"testing"

	pkgerrors "jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
)

// newExecutionEngineTool builds a stubTool (tool_test.go) with the given ID,
// input schema, and required permission categories, and registers it in a
// fresh ToolRegistryStore.
func newExecutionEngineRegistry(t *testing.T, tools ...*stubTool) *ToolRegistryStore {
	t.Helper()
	registry := NewToolRegistry()
	for _, tool := range tools {
		if err := registry.Register(tool); err != nil {
			t.Fatalf("Register(%s) returned error: %v", tool.metadata.ID, err)
		}
	}
	return registry
}

// TestToolExecutionEngine_ValidToolsExecute verifies a registered tool with
// satisfied input and (where declared) an allowed permission runs and
// returns its structured result (SPEC-0046 testing criterion 1: "Valid
// tools execute").
func TestToolExecutionEngine_ValidToolsExecute(t *testing.T) {
	t.Run("no declared permissions needs no checker", func(t *testing.T) {
		tool := &stubTool{metadata: ToolMetadata{
			ID:          "echo",
			Name:        "Echo Tool",
			InputSchema: Schema{{Name: "payload", Type: "string", Required: true}},
		}}
		registry := newExecutionEngineRegistry(t, tool)

		engine, err := NewToolExecutionEngine(registry)
		if err != nil {
			t.Fatalf("NewToolExecutionEngine returned error: %v", err)
		}

		out, err := engine.Execute(context.Background(), "developer_agent", "echo", map[string]any{"payload": "hi"})
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if out["echo"] != "hi" {
			t.Errorf("Execute output = %#v, want echo=hi", out)
		}
	})

	t.Run("declared permission allowed by checker", func(t *testing.T) {
		tool := &stubTool{metadata: ToolMetadata{
			ID:          "read-file",
			Name:        "Read File",
			InputSchema: Schema{{Name: "path", Type: "string", Required: true}},
			Permissions: []string{"filesystem"},
		}}
		registry := newExecutionEngineRegistry(t, tool)
		checker := NewPermissionChecker(PermissionModel{
			"developer_agent": AgentPermissions{"filesystem": PermissionAllowed},
		})

		engine, err := NewToolExecutionEngine(registry, WithExecutionPermissionChecker(checker))
		if err != nil {
			t.Fatalf("NewToolExecutionEngine returned error: %v", err)
		}

		out, err := engine.Execute(context.Background(), "developer_agent", "read-file", map[string]any{"path": "/tmp/x", "payload": "ok"})
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if out["echo"] != "ok" {
			t.Errorf("Execute output = %#v, want echo=ok", out)
		}
	})
}

// TestToolExecutionEngine_InvalidInputsFail verifies a call missing a
// required input field fails before the tool's own Execute runs (SPEC-0046
// testing criterion 2: "Invalid inputs fail").
func TestToolExecutionEngine_InvalidInputsFail(t *testing.T) {
	tool := &stubTool{metadata: ToolMetadata{
		ID:          "needs-payload",
		Name:        "Needs Payload",
		InputSchema: Schema{{Name: "payload", Type: "string", Required: true}},
	}}
	registry := newExecutionEngineRegistry(t, tool)

	engine, err := NewToolExecutionEngine(registry)
	if err != nil {
		t.Fatalf("NewToolExecutionEngine returned error: %v", err)
	}

	_, err = engine.Execute(context.Background(), "developer_agent", "needs-payload", map[string]any{})
	if err == nil {
		t.Fatal("Execute() = nil error, want validation failure")
	}
	if !pkgerrors.HasCode(err, "TOOL_INPUT_MISSING_REQUIRED") {
		t.Errorf("missing code TOOL_INPUT_MISSING_REQUIRED: %v", err)
	}
	if !pkgerrors.Is(err, pkgerrors.TypeInvalidInput) {
		t.Errorf("error type = %v, want TypeInvalidInput", err)
	}
}

// TestToolExecutionEngine_ErrorsAreReturnedCorrectly verifies every distinct
// failure mode in the flow surfaces a typed, diagnosable error rather than
// panicking or masking the cause (SPEC-0046 testing criterion 3: "Errors are
// returned correctly").
func TestToolExecutionEngine_ErrorsAreReturnedCorrectly(t *testing.T) {
	t.Run("unregistered tool", func(t *testing.T) {
		registry := newExecutionEngineRegistry(t)
		engine, err := NewToolExecutionEngine(registry)
		if err != nil {
			t.Fatalf("NewToolExecutionEngine returned error: %v", err)
		}

		_, err = engine.Execute(context.Background(), "developer_agent", "missing", map[string]any{})
		if !pkgerrors.HasCode(err, "TOOL_REGISTRY_TOOL_NOT_FOUND") {
			t.Errorf("missing code TOOL_REGISTRY_TOOL_NOT_FOUND: %v", err)
		}
		if !pkgerrors.Is(err, pkgerrors.TypeNotFound) {
			t.Errorf("error type = %v, want TypeNotFound", err)
		}
	})

	t.Run("declared permission denied by checker", func(t *testing.T) {
		tool := &stubTool{metadata: ToolMetadata{
			ID:          "call-out",
			Name:        "Call Out",
			Permissions: []string{"network"},
		}}
		registry := newExecutionEngineRegistry(t, tool)
		checker := NewPermissionChecker(PermissionModel{
			"developer_agent": AgentPermissions{"network": PermissionDenied},
		})

		engine, err := NewToolExecutionEngine(registry, WithExecutionPermissionChecker(checker))
		if err != nil {
			t.Fatalf("NewToolExecutionEngine returned error: %v", err)
		}

		_, err = engine.Execute(context.Background(), "developer_agent", "call-out", map[string]any{})
		if !pkgerrors.HasCode(err, "AGENT_PERMISSION_DENIED") {
			t.Errorf("missing code AGENT_PERMISSION_DENIED: %v", err)
		}
		if !pkgerrors.Is(err, pkgerrors.TypePermissionDenied) {
			t.Errorf("error type = %v, want TypePermissionDenied", err)
		}
	})

	t.Run("declared permission with no checker configured fails closed", func(t *testing.T) {
		tool := &stubTool{metadata: ToolMetadata{
			ID:          "call-out",
			Name:        "Call Out",
			Permissions: []string{"network"},
		}}
		registry := newExecutionEngineRegistry(t, tool)

		engine, err := NewToolExecutionEngine(registry)
		if err != nil {
			t.Fatalf("NewToolExecutionEngine returned error: %v", err)
		}

		_, err = engine.Execute(context.Background(), "developer_agent", "call-out", map[string]any{})
		if !pkgerrors.HasCode(err, "TOOL_EXECUTION_NO_PERMISSION_CHECKER") {
			t.Errorf("missing code TOOL_EXECUTION_NO_PERMISSION_CHECKER: %v", err)
		}
		if !pkgerrors.Is(err, pkgerrors.TypePermissionDenied) {
			t.Errorf("error type = %v, want TypePermissionDenied", err)
		}
	})

	t.Run("tool execution failure is wrapped, not swallowed", func(t *testing.T) {
		sentinelErr := stderrors.New("boom")
		tool := &stubTool{
			metadata: ToolMetadata{ID: "failing", Name: "Failing Tool"},
			err:      sentinelErr,
		}
		registry := newExecutionEngineRegistry(t, tool)

		engine, err := NewToolExecutionEngine(registry)
		if err != nil {
			t.Fatalf("NewToolExecutionEngine returned error: %v", err)
		}

		_, err = engine.Execute(context.Background(), "developer_agent", "failing", map[string]any{})
		if !pkgerrors.HasCode(err, "TOOL_EXECUTION_FAILED") {
			t.Errorf("missing code TOOL_EXECUTION_FAILED: %v", err)
		}
		if !stderrors.Is(err, sentinelErr) {
			t.Errorf("Execute err = %v, want it to wrap the sentinel error", err)
		}
	})

	t.Run("canceled context is rejected before dispatch", func(t *testing.T) {
		tool := &stubTool{metadata: ToolMetadata{ID: "echo", Name: "Echo Tool"}}
		registry := newExecutionEngineRegistry(t, tool)

		engine, err := NewToolExecutionEngine(registry)
		if err != nil {
			t.Fatalf("NewToolExecutionEngine returned error: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err = engine.Execute(ctx, "developer_agent", "echo", map[string]any{})
		if !pkgerrors.HasCode(err, "TOOL_EXECUTION_CANCELED") {
			t.Errorf("missing code TOOL_EXECUTION_CANCELED: %v", err)
		}
		if !pkgerrors.Is(err, pkgerrors.TypeCanceled) {
			t.Errorf("error type = %v, want TypeCanceled", err)
		}
	})
}

// TestNewToolExecutionEngine_RequiresRegistry verifies a nil registry is
// rejected at construction rather than deferred to a later Execute panic.
func TestNewToolExecutionEngine_RequiresRegistry(t *testing.T) {
	_, err := NewToolExecutionEngine(nil)
	if !pkgerrors.HasCode(err, "TOOL_EXECUTION_ENGINE_MISSING_REGISTRY") {
		t.Errorf("missing code TOOL_EXECUTION_ENGINE_MISSING_REGISTRY: %v", err)
	}
}

// TestToolExecutionEngine_LogsEveryOutcome verifies both a successful and a
// failed Execute are logged with their outcome, when a Logger is configured.
func TestToolExecutionEngine_LogsEveryOutcome(t *testing.T) {
	tool := &stubTool{metadata: ToolMetadata{
		ID:          "echo",
		Name:        "Echo Tool",
		InputSchema: Schema{{Name: "payload", Type: "string", Required: true}},
	}}
	registry := newExecutionEngineRegistry(t, tool)

	var buf bytes.Buffer
	log := logger.New("test", logger.WithOutput(&buf))

	engine, err := NewToolExecutionEngine(registry, WithToolExecutionEngineLogger(log))
	if err != nil {
		t.Fatalf("NewToolExecutionEngine returned error: %v", err)
	}

	if _, err := engine.Execute(context.Background(), "developer_agent", "echo", map[string]any{"payload": "hi"}); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if _, err := engine.Execute(context.Background(), "developer_agent", "echo", map[string]any{}); err == nil {
		t.Fatal("Execute() = nil error, want validation failure")
	}

	out := buf.String()
	for _, want := range []string{`"outcome":"executed"`, `"outcome":"validation_failed"`} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %s; got %s", want, out)
		}
	}
}

// TestToolExecutionEngine_IntegratesWithRegistryAndPermissionChecker
// verifies the engine composes the real SPEC-0045 ToolRegistry and SPEC-0024
// PermissionChecker end-to-end, not just against test doubles: one agent is
// allowed to run a permissioned tool, another is denied.
func TestToolExecutionEngine_IntegratesWithRegistryAndPermissionChecker(t *testing.T) {
	tool := &stubTool{metadata: ToolMetadata{
		ID:          "read-file",
		Name:        "Read File",
		InputSchema: Schema{{Name: "payload", Type: "string", Required: true}},
		Permissions: []string{"filesystem"},
	}}
	registry := newExecutionEngineRegistry(t, tool)
	checker := NewPermissionChecker(PermissionModel{
		"developer_agent": AgentPermissions{"filesystem": PermissionAllowed},
		"research_agent":  AgentPermissions{"filesystem": PermissionDenied},
	})

	engine, err := NewToolExecutionEngine(registry, WithExecutionPermissionChecker(checker))
	if err != nil {
		t.Fatalf("NewToolExecutionEngine returned error: %v", err)
	}

	if _, err := engine.Execute(context.Background(), "developer_agent", "read-file", map[string]any{"payload": "ok"}); err != nil {
		t.Errorf("developer_agent Execute() = %v, want nil", err)
	}

	_, err = engine.Execute(context.Background(), "research_agent", "read-file", map[string]any{"payload": "ok"})
	if !pkgerrors.HasCode(err, "AGENT_PERMISSION_DENIED") {
		t.Errorf("research_agent missing code AGENT_PERMISSION_DENIED: %v", err)
	}
}
