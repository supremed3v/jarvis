package core

import (
	"context"
	"testing"
	"time"

	"jarvis-pa/packages/errors"
	types "jarvis-pa/packages/shared-types"
)

// stubAgent is a minimal Agent implementation used to verify the SPEC-0018
// contract can be implemented and driven by the runtime.
type stubAgent struct {
	metadata AgentMetadata
	result   map[string]any
	err      error
}

func (a *stubAgent) Metadata() AgentMetadata { return a.metadata }

func (a *stubAgent) Execute(ctx context.Context, task *types.Task) (map[string]any, error) {
	if a.err != nil {
		return nil, a.err
	}
	return a.result, nil
}

// TestAgent_InterfaceCanBeImplemented verifies a concrete type can satisfy
// the Agent interface and that its Metadata and Execute methods behave as
// declared.
func TestAgent_InterfaceCanBeImplemented(t *testing.T) {
	var agent Agent = &stubAgent{
		metadata: AgentMetadata{ID: "agent-1", Name: "Stub Agent"},
		result:   map[string]any{"ok": true},
	}

	if got := agent.Metadata(); got.ID != "agent-1" || got.Name != "Stub Agent" {
		t.Errorf("Metadata() = %+v, want ID=agent-1 Name=%q", got, "Stub Agent")
	}

	result, err := agent.Execute(context.Background(), &types.Task{ID: "task-1"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result["ok"] != true {
		t.Errorf("Execute result = %+v, want ok=true", result)
	}
}

// TestAgent_RuntimeCanExecuteSampleAgent verifies a Worker (SPEC-0014) can
// drive an Agent's Execute method directly, with no adapter, exercising the
// same Queue/StateMachine/EventBus wiring real Task execution uses.
func TestAgent_RuntimeCanExecuteSampleAgent(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()

	agent := &stubAgent{
		metadata: AgentMetadata{ID: "agent-1", Name: "Sample Agent"},
		result:   map[string]any{"summary": "done"},
	}

	task := &types.Task{ID: "task-1", Source: types.TaskSourceAgent, Type: "test", Status: types.TaskStatusQueued}
	if err := q.Add(task); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	w := NewWorker("worker-1", q, sm, bus, agent.Execute, WithPollInterval(time.Millisecond))
	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer w.Stop(context.Background())

	waitFor(t, time.Second, func() bool { return task.Status == types.TaskStatusCompleted })

	if task.Result["summary"] != "done" {
		t.Errorf("task.Result = %+v, want summary=done", task.Result)
	}
}

// TestAgentMetadata_Validate verifies AgentMetadata's declared identity
// fields are required and reported individually.
func TestAgentMetadata_Validate(t *testing.T) {
	tests := []struct {
		name     string
		metadata AgentMetadata
		wantCode string
	}{
		{
			name:     "valid",
			metadata: AgentMetadata{ID: "agent-1", Name: "Agent One"},
		},
		{
			name:     "missing ID",
			metadata: AgentMetadata{Name: "Agent One"},
			wantCode: "AGENT_METADATA_MISSING_ID",
		},
		{
			name:     "missing name",
			metadata: AgentMetadata{ID: "agent-1"},
			wantCode: "AGENT_METADATA_MISSING_NAME",
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
			if !errors.HasCode(err, tt.wantCode) {
				t.Errorf("missing code %s: %v", tt.wantCode, err)
			}
			if !errors.Is(err, errors.TypeInvalidInput) {
				t.Errorf("error type = %v, want TypeInvalidInput", err)
			}
		})
	}
}
