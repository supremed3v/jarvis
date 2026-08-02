package core

import (
	"context"
	"errors"
	"testing"

	pkgerr "jarvis-pa/packages/errors"
	types "jarvis-pa/packages/shared-types"
)

// stubTool is a minimal Tool implementation used to verify the SPEC-0043
// contract can be implemented. It declares a small InputSchema, requires one
// permission, and returns its input verbatim on success.
type stubTool struct {
	metadata ToolMetadata
	err      error
}

func (t *stubTool) Metadata() ToolMetadata { return t.metadata }

func (t *stubTool) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if t.err != nil {
		return nil, t.err
	}
	return map[string]any{"echo": input["payload"]}, nil
}

// TestTool_InterfaceCanBeImplemented verifies a concrete type can satisfy the
// Tool interface and that its Metadata and Execute methods behave as declared
// (SPEC-0043 testing criterion 1: "Tool interface can be implemented").
func TestTool_InterfaceCanBeImplemented(t *testing.T) {
	var tool Tool = &stubTool{
		metadata: ToolMetadata{
			ID:           "tool-1",
			Name:         "Stub Tool",
			Description:  "A stub tool used for testing the Tool interface contract.",
			InputSchema:  Schema{{Name: "payload", Type: "string", Required: true}},
			OutputSchema: Schema{{Name: "echo", Type: "string", Required: true}},
			Permissions:  []string{"filesystem.read"},
		},
	}

	got := tool.Metadata()
	if got.ID != "tool-1" || got.Name != "Stub Tool" {
		t.Errorf("Metadata() = %+v, want ID=tool-1 Name=%q", got, "Stub Tool")
	}

	out, err := tool.Execute(context.Background(), map[string]any{"payload": "hello"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out["echo"] != "hello" {
		t.Errorf("Execute output = %#v, want echo=hello", out)
	}
}

// TestTool_MetadataValidatesCorrectly verifies ToolMetadata.Validate enforces
// the spec's required identity fields (Tool ID and Tool Name) and reports the
// first missing one via a typed packages/errors error (SPEC-0043 testing
// criterion 2: "Tool metadata validates correctly").
func TestTool_MetadataValidatesCorrectly(t *testing.T) {
	tests := []struct {
		name     string
		metadata ToolMetadata
		wantCode string
	}{
		{
			name: "valid",
			metadata: ToolMetadata{
				ID:          "tool-1",
				Name:        "Tool One",
				Description: "does a thing",
			},
		},
		{
			name:     "missing ID",
			metadata: ToolMetadata{Name: "Tool One"},
			wantCode: "TOOL_METADATA_MISSING_ID",
		},
		{
			name:     "missing name",
			metadata: ToolMetadata{ID: "tool-1"},
			wantCode: "TOOL_METADATA_MISSING_NAME",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.metadata.Validate()
			if tt.wantCode == "" {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error with code %s", tt.wantCode)
			}
			if !pkgerr.HasCode(err, tt.wantCode) {
				t.Errorf("missing code %s: %v", tt.wantCode, err)
			}
			if !pkgerr.Is(err, pkgerr.TypeInvalidInput) {
				t.Errorf("error type = %v, want TypeInvalidInput", err)
			}
		})
	}
}

// TestTool_ExecutionFollowsContract verifies Tool.Execute honors both halves
// of the SPEC-0043 contract: it returns a structured result on success, and
// it reports failures safely by returning the error (rather than panicking)
// when its underlying action fails (SPEC-0043 testing criterion 3: "Tool
// execution follows contract"). It also asserts the spec's "Validate inputs"
// responsibility: a missing required input field surfaces as a typed error
// before the tool's own action runs.
func TestTool_ExecutionFollowsContract(t *testing.T) {
	t.Run("success returns structured result", func(t *testing.T) {
		tool := &stubTool{
			metadata: ToolMetadata{ID: "echo", Name: "Echo Tool"},
		}
		out, err := tool.Execute(context.Background(), map[string]any{"payload": "ping"})
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if out["echo"] != "ping" {
			t.Errorf("Execute output = %#v, want echo=ping", out)
		}
	})

	t.Run("failure reports error safely rather than panicking", func(t *testing.T) {
		sentinelErr := errors.New("boom")
		tool := &stubTool{
			metadata: ToolMetadata{ID: "failing", Name: "Failing Tool"},
			err:      sentinelErr,
		}
		_, err := tool.Execute(context.Background(), map[string]any{})
		if err == nil {
			t.Fatal("Execute returned nil error, want sentinel")
		}
		if !errors.Is(err, sentinelErr) {
			t.Errorf("Execute err = %v, want it to wrap the sentinel error", err)
		}
	})

	t.Run("cancellation is respected", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		tool := &stubTool{metadata: ToolMetadata{ID: "ctx", Name: "Ctx Tool"}}
		_, err := tool.Execute(ctx, map[string]any{})
		if err == nil {
			t.Fatal("Execute returned nil, want ctx cancellation error")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Execute err = %v, want context.Canceled", err)
		}
	})
}

// TestValidateToolInput verifies the ValidateToolInput helper enforces a
// Tool's declared InputSchema against an input, satisfying the spec's
// "Validate inputs" responsibility at runtime (as opposed to the static
// metadata validation tested above).
func TestValidateToolInput(t *testing.T) {
	tool := &stubTool{
		metadata: ToolMetadata{
			ID:   "needs-payload",
			Name: "Needs Payload",
			InputSchema: Schema{
				{Name: "payload", Type: "string", Required: true},
				{Name: "optional", Type: "string", Required: false},
			},
		},
	}

	t.Run("satisfied input returns nil", func(t *testing.T) {
		if err := ValidateToolInput(tool, map[string]any{"payload": "x"}); err != nil {
			t.Errorf("ValidateToolInput = %v, want nil for satisfied schema", err)
		}
	})

	t.Run("missing required input surfaces typed error", func(t *testing.T) {
		err := ValidateToolInput(tool, map[string]any{"optional": "y"})
		if err == nil {
			t.Fatal("ValidateToolInput = nil, want missing-required error")
		}
		if !pkgerr.HasCode(err, "TOOL_INPUT_MISSING_REQUIRED") {
			t.Errorf("ValidateToolInput err = %v, want code TOOL_INPUT_MISSING_REQUIRED", err)
		}
		if !pkgerr.Is(err, pkgerr.TypeInvalidInput) {
			t.Errorf("ValidateToolInput err type = %v, want TypeInvalidInput", err)
		}
		// The helper attaches the missing field as context for diagnostics.
		if e, ok := err.(*pkgerr.Error); !ok || e.Context["field"] != "payload" {
			t.Errorf("ValidateToolInput err context field = %v, want %q", e, "payload")
		}
	})

	t.Run("nil input surfaces typed error for required fields", func(t *testing.T) {
		err := ValidateToolInput(tool, map[string]any{"payload": nil})
		if err == nil {
			t.Fatal("ValidateToolInput = nil, want error for nil-valued required field")
		}
		if !pkgerr.HasCode(err, "TOOL_INPUT_MISSING_REQUIRED") {
			t.Errorf("ValidateToolInput err = %v, want code TOOL_INPUT_MISSING_REQUIRED", err)
		}
	})
}

// TestSchema_Validate verifies that an empty Schema (a free-form input/output
// contract) accepts anything - mirroring the loose map[string]any contract the
// existing ToolCaller seam (agent_execution_loop.go) already uses so a Tool
// with no declared schema can be wired straight into it.
func TestSchema_Validate(t *testing.T) {
	t.Run("empty schema accepts any input", func(t *testing.T) {
		s := Schema{}
		if err := s.Validate(map[string]any{"anything": "ok"}); err != nil {
			t.Errorf("empty Schema.Validate returned %v, want nil", err)
		}
	})

	t.Run("optional fields do not require presence", func(t *testing.T) {
		s := Schema{{Name: "optional", Type: "string", Required: false}}
		if err := s.Validate(map[string]any{}); err != nil {
			t.Errorf("Schema with only optional fields = %v, want nil", err)
		}
	})
}

// TestTool_IntegratesWithExecutionLoop verifies the integration boundary
// tool.go's doc comments claim: a Tool's Execute method value requires only a
// small, name-dropping wrapper (the kind SPEC-0045's Registry will provide)
// to satisfy ExecutionLoop's existing ToolCaller seam (agent_execution_loop.go,
// SPEC-0022) - no signature adapter beyond that. This mirrors the precedent
// set by SPEC-0018/SPEC-0022's own integration tests proving their contracts
// drive real runtime components end-to-end, not just in isolation.
func TestTool_IntegratesWithExecutionLoop(t *testing.T) {
	tool := &stubTool{
		metadata: ToolMetadata{
			ID:          "echo",
			Name:        "Echo Tool",
			InputSchema: Schema{{Name: "payload", Type: "string", Required: true}},
		},
	}

	// The only "adapter" needed: drop the tool-name argument ToolCaller
	// receives, since a Tool already knows its own identity via Metadata().
	caller := func(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
		return tool.Execute(ctx, input)
	}

	loop, err := NewExecutionLoop(
		func(ctx context.Context, task *types.Task, analysis map[string]any) (Plan, error) {
			return Plan{Steps: []Step{
				{Name: "call-echo", Tool: tool.Metadata().ID, Input: map[string]any{"payload": "via-loop"}},
			}}, nil
		},
		WithToolCaller(caller),
	)
	if err != nil {
		t.Fatalf("NewExecutionLoop returned error: %v", err)
	}

	task := &types.Task{ID: "task-1"}
	result, err := loop.Run(context.Background(), task)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got := result["result"].(map[string]any)["echo"]; got != "via-loop" {
		t.Errorf("Run result[echo] = %#v, want %q", got, "via-loop")
	}
}
