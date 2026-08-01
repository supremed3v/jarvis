package types

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEvent_JSONRoundTrip(t *testing.T) {
	want := Event{
		ID:        "evt-1",
		Type:      EventType("task.completed"),
		Source:    "task-worker",
		Timestamp: time.Now().UTC().Truncate(time.Second),
		Payload:   map[string]any{"taskId": "task-1"},
	}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal(Event) returned error: %v", err)
	}

	var got Event
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal(Event) returned error: %v", err)
	}

	if got.ID != want.ID || got.Type != want.Type || got.Source != want.Source {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if !got.Timestamp.Equal(want.Timestamp) {
		t.Errorf("Timestamp = %v, want %v", got.Timestamp, want.Timestamp)
	}
	if got.Payload["taskId"] != want.Payload["taskId"] {
		t.Errorf("Payload[taskId] = %v, want %v", got.Payload["taskId"], want.Payload["taskId"])
	}
}

func TestTask_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	want := Task{
		ID:          "task-1",
		Title:       "Research JARVIS competitors",
		Description: "Summarize similar local-first assistant projects",
		Source:      TaskSourceAgent,
		Priority:    TaskPriority("high"),
		Type:        "research",
		Status:      TaskStatusExecuting,
		Input:       map[string]any{"query": "jarvis"},
		ParentID:    "task-0",
		Metadata:    map[string]any{"origin": "planner"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal(Task) returned error: %v", err)
	}

	var got Task
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal(Task) returned error: %v", err)
	}

	if got.ID != want.ID || got.Title != want.Title || got.Description != want.Description {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if got.Source != want.Source || got.Priority != want.Priority || got.ParentID != want.ParentID {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if got.Type != want.Type || got.Status != want.Status {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) || !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Errorf("timestamps = (%v, %v), want (%v, %v)", got.CreatedAt, got.UpdatedAt, want.CreatedAt, want.UpdatedAt)
	}
	if got.Input["query"] != want.Input["query"] {
		t.Errorf("Input[query] = %v, want %v", got.Input["query"], want.Input["query"])
	}
	if got.Metadata["origin"] != want.Metadata["origin"] {
		t.Errorf("Metadata[origin] = %v, want %v", got.Metadata["origin"], want.Metadata["origin"])
	}
}

func TestTask_SourceValues(t *testing.T) {
	sources := []TaskSource{
		TaskSourceVoice,
		TaskSourceDesktop,
		TaskSourceAgent,
		TaskSourceScheduled,
	}
	seen := map[TaskSource]bool{}
	for _, s := range sources {
		if s == "" {
			t.Errorf("TaskSource constant is empty")
		}
		if seen[s] {
			t.Errorf("duplicate TaskSource value %q", s)
		}
		seen[s] = true
	}
}

func TestTask_EmptyOptionalFieldsOmitted(t *testing.T) {
	task := Task{ID: "task-1", Title: "Do a thing", Source: TaskSourceDesktop, Status: TaskStatusCreated}
	data, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("Marshal(Task) returned error: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal into map returned error: %v", err)
	}
	for _, field := range []string{"description", "priority", "input", "result", "error", "parentId", "metadata"} {
		if _, ok := raw[field]; ok {
			t.Errorf("expected %s to be omitted when empty, got %v", field, raw[field])
		}
	}
}

func TestTask_StatusValues(t *testing.T) {
	statuses := []TaskStatus{
		TaskStatusCreated,
		TaskStatusPlanning,
		TaskStatusQueued,
		TaskStatusExecuting,
		TaskStatusWaiting,
		TaskStatusFailed,
		TaskStatusCompleted,
		TaskStatusCancelled,
	}
	seen := map[TaskStatus]bool{}
	for _, s := range statuses {
		if s == "" {
			t.Errorf("TaskStatus constant is empty")
		}
		if seen[s] {
			t.Errorf("duplicate TaskStatus value %q", s)
		}
		seen[s] = true
	}
}

func TestAgent_JSONRoundTrip(t *testing.T) {
	want := Agent{
		ID:           "agent-1",
		Name:         "core-agent",
		Type:         "core",
		Status:       AgentStatusIdle,
		Capabilities: []string{"chat", "planning"},
	}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal(Agent) returned error: %v", err)
	}

	var got Agent
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal(Agent) returned error: %v", err)
	}

	if got.ID != want.ID || got.Name != want.Name || got.Type != want.Type || got.Status != want.Status {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if len(got.Capabilities) != len(want.Capabilities) || got.Capabilities[0] != want.Capabilities[0] {
		t.Errorf("Capabilities = %v, want %v", got.Capabilities, want.Capabilities)
	}
}

func TestTool_JSONRoundTrip(t *testing.T) {
	want := Tool{
		Name:        "filesystem.read",
		Description: "Read a file from disk",
		Parameters:  map[string]any{"path": "string"},
		Permissions: []string{"filesystem:read"},
	}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal(Tool) returned error: %v", err)
	}

	var got Tool
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal(Tool) returned error: %v", err)
	}

	if got.Name != want.Name || got.Description != want.Description {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if got.Parameters["path"] != want.Parameters["path"] {
		t.Errorf("Parameters[path] = %v, want %v", got.Parameters["path"], want.Parameters["path"])
	}
	if len(got.Permissions) != 1 || got.Permissions[0] != want.Permissions[0] {
		t.Errorf("Permissions = %v, want %v", got.Permissions, want.Permissions)
	}
}

func TestEvent_EmptyPayloadOmitted(t *testing.T) {
	e := Event{ID: "evt-1", Type: "x", Source: "y", Timestamp: time.Now()}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal(Event) returned error: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal into map returned error: %v", err)
	}
	if _, ok := raw["payload"]; ok {
		t.Errorf("expected payload to be omitted when empty, got %v", raw["payload"])
	}
}

func TestAgent_EmptyCapabilitiesOmitted(t *testing.T) {
	a := Agent{ID: "agent-1", Name: "core-agent", Type: "core", Status: AgentStatusIdle}
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("Marshal(Agent) returned error: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal into map returned error: %v", err)
	}
	if _, ok := raw["capabilities"]; ok {
		t.Errorf("expected capabilities to be omitted when empty, got %v", raw["capabilities"])
	}
}

func TestTool_EmptyParametersAndPermissionsOmitted(t *testing.T) {
	tool := Tool{Name: "noop", Description: "does nothing"}
	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("Marshal(Tool) returned error: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal into map returned error: %v", err)
	}
	if _, ok := raw["parameters"]; ok {
		t.Errorf("expected parameters to be omitted when empty, got %v", raw["parameters"])
	}
	if _, ok := raw["permissions"]; ok {
		t.Errorf("expected permissions to be omitted when empty, got %v", raw["permissions"])
	}
}

func TestTask_ZeroValueRoundTrip(t *testing.T) {
	var want Task

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal(zero Task) returned error: %v", err)
	}

	var got Task
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal(zero Task) returned error: %v", err)
	}
	if got.ID != "" || got.Status != "" || got.Input != nil {
		t.Errorf("got %+v, want zero value", got)
	}
}

func TestUnmarshal_MalformedJSONFailsWithoutPanic(t *testing.T) {
	malformed := []byte(`{not valid json`)

	var e Event
	if err := json.Unmarshal(malformed, &e); err == nil {
		t.Errorf("Unmarshal(Event) with malformed JSON returned no error")
	}

	var task Task
	if err := json.Unmarshal(malformed, &task); err == nil {
		t.Errorf("Unmarshal(Task) with malformed JSON returned no error")
	}

	var agent Agent
	if err := json.Unmarshal(malformed, &agent); err == nil {
		t.Errorf("Unmarshal(Agent) with malformed JSON returned no error")
	}

	var tool Tool
	if err := json.Unmarshal(malformed, &tool); err == nil {
		t.Errorf("Unmarshal(Tool) with malformed JSON returned no error")
	}

	var message Message
	if err := json.Unmarshal(malformed, &message); err == nil {
		t.Errorf("Unmarshal(Message) with malformed JSON returned no error")
	}
}

func TestMessage_JSONRoundTrip(t *testing.T) {
	want := Message{
		ID:          "msg-1",
		Timestamp:   time.Now().UTC().Truncate(time.Second),
		Source:      "core-agent",
		Destination: "developer-agent",
		Type:        MessageTypeAgentCommunication,
		Payload:     map[string]any{"task": "review-pr"},
	}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal(Message) returned error: %v", err)
	}

	var got Message
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal(Message) returned error: %v", err)
	}

	if got.ID != want.ID || got.Source != want.Source || got.Destination != want.Destination || got.Type != want.Type {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if !got.Timestamp.Equal(want.Timestamp) {
		t.Errorf("Timestamp = %v, want %v", got.Timestamp, want.Timestamp)
	}
	if got.Payload["task"] != want.Payload["task"] {
		t.Errorf("Payload[task] = %v, want %v", got.Payload["task"], want.Payload["task"])
	}
}

func TestMessage_TypeValues(t *testing.T) {
	types := []MessageType{
		MessageTypeAgentCommunication,
		MessageTypeToolRequest,
		MessageTypeEventNotification,
	}
	seen := map[MessageType]bool{}
	for _, mt := range types {
		if mt == "" {
			t.Errorf("MessageType constant is empty")
		}
		if seen[mt] {
			t.Errorf("duplicate MessageType value %q", mt)
		}
		seen[mt] = true
	}
}

func TestMessage_EmptyDestinationAndPayloadOmitted(t *testing.T) {
	m := Message{
		ID:        "msg-1",
		Timestamp: time.Now(),
		Source:    "event-bus",
		Type:      MessageTypeEventNotification,
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal(Message) returned error: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal into map returned error: %v", err)
	}
	if _, ok := raw["destination"]; ok {
		t.Errorf("expected destination to be omitted when empty, got %v", raw["destination"])
	}
	if _, ok := raw["payload"]; ok {
		t.Errorf("expected payload to be omitted when empty, got %v", raw["payload"])
	}
}

// TestUnknownFieldsAreIgnored guards the wire-compatibility boundary between
// services: a producer sending a newer field set must not break an older
// consumer built against this package.
func TestUnknownFieldsAreIgnored(t *testing.T) {
	data := []byte(`{"id":"task-1","type":"research","status":"created","futureField":"unexpected"}`)

	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		t.Fatalf("Unmarshal with unknown field returned error: %v", err)
	}
	if task.ID != "task-1" || task.Status != TaskStatusCreated {
		t.Errorf("got %+v, want ID=task-1 Status=created", task)
	}
}

func TestTaskTransition_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	want := TaskTransition{
		TaskID:    "task-1",
		From:      TaskStatusQueued,
		To:        TaskStatusExecuting,
		Timestamp: now,
	}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal(TaskTransition) returned error: %v", err)
	}

	var got TaskTransition
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal(TaskTransition) returned error: %v", err)
	}
	if got.TaskID != want.TaskID || got.From != want.From || got.To != want.To {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if !got.Timestamp.Equal(want.Timestamp) {
		t.Errorf("Timestamp = %v, want %v", got.Timestamp, want.Timestamp)
	}
}

// TestEnumTypesEncodeAsPlainStrings guards the wire format for the
// string-based enum types: other services (e.g. non-Go consumers) decode
// these as plain JSON strings, not nested objects.
func TestEnumTypesEncodeAsPlainStrings(t *testing.T) {
	data, err := json.Marshal(struct {
		Status AgentStatus `json:"status"`
	}{Status: AgentStatusRunning})
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	if string(data) != `{"status":"running"}` {
		t.Errorf("got %s, want {\"status\":\"running\"}", data)
	}
}
