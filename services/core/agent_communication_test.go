package core

import (
	"context"
	stderrors "errors"
	"sync"
	"testing"
	"time"

	"jarvis-pa/packages/errors"
	types "jarvis-pa/packages/shared-types"
)

// TestCommunicator_AgentsCanCommunicate verifies SPEC-0025's first testing
// criterion: a Request reaches the destination Agent's Execute and comes
// back as a valid response Message carrying its result.
func TestCommunicator_AgentsCanCommunicate(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&stubAgent{
		metadata: AgentMetadata{ID: "developer_agent", Name: "Developer Agent"},
		result:   map[string]any{"ok": true},
	}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	c, err := NewCommunicator(r)
	if err != nil {
		t.Fatalf("NewCommunicator returned error: %v", err)
	}

	task := &types.Task{ID: "task-1", Title: "do work"}
	msg, err := c.Request(context.Background(), "core_agent", "developer_agent", task)
	if err != nil {
		t.Fatalf("Request returned error: %v", err)
	}

	if msg.Type != types.MessageTypeAgentCommunication {
		t.Errorf("response Type = %v, want MessageTypeAgentCommunication", msg.Type)
	}
	if msg.Source != "developer_agent" || msg.Destination != "core_agent" {
		t.Errorf("response Source/Destination = %q/%q, want developer_agent/core_agent", msg.Source, msg.Destination)
	}
	if msg.Payload["kind"] != string(AgentMessageResponse) {
		t.Errorf("response kind = %v, want %q", msg.Payload["kind"], AgentMessageResponse)
	}
	if success, _ := msg.Payload["success"].(bool); !success {
		t.Errorf("response success = %v, want true", msg.Payload["success"])
	}
	result, _ := msg.Payload["result"].(map[string]any)
	if result["ok"] != true {
		t.Errorf("response result = %+v, want {ok: true}", result)
	}
}

// TestCommunicator_RequestUnknownAgent verifies a Request naming an
// unregistered agent fails with the Registry's own not-found error rather
// than a nil-pointer panic.
func TestCommunicator_RequestUnknownAgent(t *testing.T) {
	r := NewRegistry()
	c, err := NewCommunicator(r)
	if err != nil {
		t.Fatalf("NewCommunicator returned error: %v", err)
	}

	_, err = c.Request(context.Background(), "core_agent", "missing_agent", &types.Task{ID: "task-1"})
	if !errors.HasCode(err, "AGENT_REGISTRY_AGENT_NOT_FOUND") {
		t.Errorf("Request error = %v, want code AGENT_REGISTRY_AGENT_NOT_FOUND", err)
	}
}

// TestCommunicator_RequestExecuteFailure verifies a Request whose
// destination Agent's Execute fails still returns a validated response
// Message reporting the failure, alongside the original error.
func TestCommunicator_RequestExecuteFailure(t *testing.T) {
	r := NewRegistry()
	execErr := stderrors.New("boom")
	if err := r.Register(&stubAgent{
		metadata: AgentMetadata{ID: "developer_agent", Name: "Developer Agent"},
		err:      execErr,
	}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	c, err := NewCommunicator(r)
	if err != nil {
		t.Fatalf("NewCommunicator returned error: %v", err)
	}

	msg, err := c.Request(context.Background(), "core_agent", "developer_agent", &types.Task{ID: "task-1"})
	if err == nil || err.Error() != "boom" {
		t.Errorf("Request error = %v, want boom", err)
	}
	if success, _ := msg.Payload["success"].(bool); success {
		t.Errorf("response success = %v, want false", msg.Payload["success"])
	}
	if msg.Payload["error"] != "boom" {
		t.Errorf("response error = %v, want boom", msg.Payload["error"])
	}
}

// TestCommunicator_RequestRejectsInvalidResponse verifies SPEC-0025's
// second testing criterion: an invalid response never reaches the caller
// as a Message - Request instead reports the validation failure.
func TestCommunicator_RequestRejectsInvalidResponse(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&stubAgent{
		metadata: AgentMetadata{ID: "developer_agent", Name: "Developer Agent"},
		result:   map[string]any{},
	}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	c, err := NewCommunicator(r, WithCommunicatorValidator(func(AgentResponse) error {
		return errors.New(errors.TypeInvalidInput, "TEST_ALWAYS_INVALID", "core.agentcommunication_test", "always invalid")
	}))
	if err != nil {
		t.Fatalf("NewCommunicator returned error: %v", err)
	}

	_, err = c.Request(context.Background(), "core_agent", "developer_agent", &types.Task{ID: "task-1"})
	if !errors.HasCode(err, "TEST_ALWAYS_INVALID") {
		t.Errorf("Request error = %v, want code TEST_ALWAYS_INVALID", err)
	}
}

// TestCommunicator_NilValidatorFallsBackToDefault verifies
// WithCommunicatorValidator(nil) does not leave the Communicator with no
// validation function (and so does not panic on the next Request) - it
// falls back to ValidateAgentResponse, mirroring ContextBuilder's own
// nil-SizeEstimator fallback (agent_context_builder.go, SPEC-0023).
func TestCommunicator_NilValidatorFallsBackToDefault(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&stubAgent{
		metadata: AgentMetadata{ID: "developer_agent", Name: "Developer Agent"},
		result:   map[string]any{"ok": true},
	}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	c, err := NewCommunicator(r, WithCommunicatorValidator(nil))
	if err != nil {
		t.Fatalf("NewCommunicator returned error: %v", err)
	}

	msg, err := c.Request(context.Background(), "core_agent", "developer_agent", &types.Task{ID: "task-1"})
	if err != nil {
		t.Fatalf("Request returned error: %v", err)
	}
	if success, _ := msg.Payload["success"].(bool); !success {
		t.Errorf("response success = %v, want true", msg.Payload["success"])
	}
}

// TestCommunicator_DelegationWorksCorrectly verifies SPEC-0025's third
// testing criterion: Delegate hands the Task to the destination Agent,
// tags it with the delegation chain, and returns a successful response.
func TestCommunicator_DelegationWorksCorrectly(t *testing.T) {
	r := NewRegistry()

	var gotTask *types.Task
	if err := r.Register(&recordingAgent{
		metadata: AgentMetadata{ID: "developer_agent", Name: "Developer Agent"},
		record:   func(task *types.Task) { gotTask = task },
		result:   map[string]any{"delegated": true},
	}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	c, err := NewCommunicator(r)
	if err != nil {
		t.Fatalf("NewCommunicator returned error: %v", err)
	}

	task := &types.Task{ID: "task-1", Title: "build feature"}
	msg, err := c.Delegate(context.Background(), "core_agent", "developer_agent", task)
	if err != nil {
		t.Fatalf("Delegate returned error: %v", err)
	}

	if gotTask == nil {
		t.Fatal("destination agent never received the delegated task")
	}
	if gotTask.Metadata["delegatedFrom"] != "core_agent" || gotTask.Metadata["delegatedTo"] != "developer_agent" {
		t.Errorf("task.Metadata = %+v, want delegatedFrom=core_agent delegatedTo=developer_agent", gotTask.Metadata)
	}
	if success, _ := msg.Payload["success"].(bool); !success {
		t.Errorf("response success = %v, want true", msg.Payload["success"])
	}
}

// TestValidateAgentResponse verifies each way an AgentResponse can be
// malformed is rejected, and a well-formed response of either outcome is
// accepted.
func TestValidateAgentResponse(t *testing.T) {
	cases := []struct {
		name     string
		resp     AgentResponse
		wantCode string
	}{
		{"missing request id", AgentResponse{AgentID: "a", Success: true}, "AGENT_RESPONSE_MISSING_REQUEST_ID"},
		{"missing agent id", AgentResponse{RequestID: "r", Success: true}, "AGENT_RESPONSE_MISSING_AGENT_ID"},
		{"success with error", AgentResponse{RequestID: "r", AgentID: "a", Success: true, Error: "boom"}, "AGENT_RESPONSE_INVALID_STATE"},
		{"failure without error", AgentResponse{RequestID: "r", AgentID: "a", Success: false}, "AGENT_RESPONSE_INVALID_STATE"},
		{"valid success", AgentResponse{RequestID: "r", AgentID: "a", Success: true}, ""},
		{"valid failure", AgentResponse{RequestID: "r", AgentID: "a", Success: false, Error: "boom"}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAgentResponse(tc.resp)
			if tc.wantCode == "" {
				if err != nil {
					t.Errorf("ValidateAgentResponse(%+v) = %v, want nil", tc.resp, err)
				}
				return
			}
			if !errors.HasCode(err, tc.wantCode) {
				t.Errorf("ValidateAgentResponse(%+v) = %v, want code %s", tc.resp, err, tc.wantCode)
			}
		})
	}
}

// TestCommunicator_BroadcastsStatusAndErrorEvents verifies status updates
// and error reports are published on the configured EventBus.
func TestCommunicator_BroadcastsStatusAndErrorEvents(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&stubAgent{
		metadata: AgentMetadata{ID: "developer_agent", Name: "Developer Agent"},
		err:      stderrors.New("boom"),
	}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	bus := NewBus()
	var mu sync.Mutex
	var kinds []string
	unsubscribe := bus.Subscribe(EventAgentMessage, func(event types.Event) {
		kind, _ := event.Payload["kind"].(string)
		mu.Lock()
		kinds = append(kinds, kind)
		mu.Unlock()
	})
	defer unsubscribe()

	c, err := NewCommunicator(r, WithCommunicatorEventBus(bus))
	if err != nil {
		t.Fatalf("NewCommunicator returned error: %v", err)
	}

	if _, err := c.Request(context.Background(), "core_agent", "developer_agent", &types.Task{ID: "task-1"}); err == nil {
		t.Fatal("Request() = nil error, want boom")
	}

	waitFor(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(kinds) >= 3
	})

	mu.Lock()
	defer mu.Unlock()
	wantKinds := map[string]bool{
		string(AgentMessageStatusUpdate): false,
		string(AgentMessageErrorReport):  false,
	}
	for _, k := range kinds {
		if _, ok := wantKinds[k]; ok {
			wantKinds[k] = true
		}
	}
	for k, seen := range wantKinds {
		if !seen {
			t.Errorf("never observed a broadcast %q event; got kinds=%v", k, kinds)
		}
	}
}

// recordingAgent is an Agent implementation used to verify Delegate hands
// the actual *types.Task (with any mutations, e.g. the delegation-chain
// Metadata) through to the destination Agent's Execute.
type recordingAgent struct {
	metadata AgentMetadata
	record   func(task *types.Task)
	result   map[string]any
}

func (a *recordingAgent) Metadata() AgentMetadata { return a.metadata }

func (a *recordingAgent) Execute(ctx context.Context, task *types.Task) (map[string]any, error) {
	a.record(task)
	return a.result, nil
}
