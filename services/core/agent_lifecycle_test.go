package core

import (
	"context"
	"errors"
	"testing"

	pkgerrors "jarvis-pa/packages/errors"
	types "jarvis-pa/packages/shared-types"
)

// lifecycleStubAgent extends stubAgent with optional Init/Cleanup hooks
// (AgentInitializer/AgentCleaner) so lifecycle tests can verify the manager
// invokes them at the right phase and handles their failures.
type lifecycleStubAgent struct {
	stubAgent
	initErr       error
	cleanupErr    error
	initCalled    int
	cleanupCalled int
}

func (a *lifecycleStubAgent) Init(ctx context.Context) error {
	a.initCalled++
	return a.initErr
}

func (a *lifecycleStubAgent) Cleanup(ctx context.Context) error {
	a.cleanupCalled++
	return a.cleanupErr
}

func newLifecycleAgent(id string) *lifecycleStubAgent {
	return &lifecycleStubAgent{stubAgent: stubAgent{metadata: AgentMetadata{ID: id, Name: id}}}
}

func TestCanTransitionAgent_ValidTransitionsSucceed(t *testing.T) {
	cases := []struct{ from, to types.AgentStatus }{
		{types.AgentStatusRegistered, types.AgentStatusInitializing},
		{types.AgentStatusInitializing, types.AgentStatusReady},
		{types.AgentStatusReady, types.AgentStatusRunning},
		{types.AgentStatusReady, types.AgentStatusStopping},
		{types.AgentStatusRunning, types.AgentStatusStopping},
		{types.AgentStatusStopping, types.AgentStatusStopped},
		{types.AgentStatusRegistered, types.AgentStatusFailed},
		{types.AgentStatusRunning, types.AgentStatusFailed},
	}
	for _, c := range cases {
		if !CanTransitionAgent(c.from, c.to) {
			t.Errorf("CanTransitionAgent(%q, %q) = false, want true", c.from, c.to)
		}
	}
}

func TestCanTransitionAgent_InvalidTransitionsFail(t *testing.T) {
	cases := []struct{ from, to types.AgentStatus }{
		{types.AgentStatusRegistered, types.AgentStatusRunning},    // skips initializing/ready
		{types.AgentStatusStopped, types.AgentStatusRunning},       // terminal
		{types.AgentStatusFailed, types.AgentStatusRegistered},     // terminal
		{types.AgentStatusRunning, types.AgentStatusReady},         // backward
		{types.AgentStatusRegistered, types.AgentStatusRegistered}, // self-transition
	}
	for _, c := range cases {
		if CanTransitionAgent(c.from, c.to) {
			t.Errorf("CanTransitionAgent(%q, %q) = true, want false", c.from, c.to)
		}
	}
}

// TestLifecycleManager_AgentsTransitionCorrectly exercises SPEC-0021's first
// testing criterion end to end: an agent moves REGISTERED -> INITIALIZING ->
// READY -> RUNNING -> STOPPING -> STOPPED, with each step's history recorded.
func TestLifecycleManager_AgentsTransitionCorrectly(t *testing.T) {
	registry := NewRegistry()
	m := NewLifecycleManager(registry)
	agent := newLifecycleAgent("agent-1")

	if err := m.Register(agent); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if state, err := m.State("agent-1"); err != nil || state != types.AgentStatusRegistered {
		t.Fatalf("State after Register = (%q, %v), want (registered, nil)", state, err)
	}

	if err := m.Initialize(context.Background(), "agent-1"); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	if state, _ := m.State("agent-1"); state != types.AgentStatusReady {
		t.Errorf("State after Initialize = %q, want ready", state)
	}
	if agent.initCalled != 1 {
		t.Errorf("Init called %d times, want 1", agent.initCalled)
	}

	if err := m.Start("agent-1"); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if state, _ := m.State("agent-1"); state != types.AgentStatusRunning {
		t.Errorf("State after Start = %q, want running", state)
	}

	if err := m.Stop(context.Background(), "agent-1"); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	if state, _ := m.State("agent-1"); state != types.AgentStatusStopped {
		t.Errorf("State after Stop = %q, want stopped", state)
	}

	wantStates := []types.AgentStatus{
		types.AgentStatusInitializing,
		types.AgentStatusReady,
		types.AgentStatusRunning,
		types.AgentStatusStopping,
		types.AgentStatusStopped,
	}
	history := m.History("agent-1")
	if len(history) != len(wantStates) {
		t.Fatalf("len(History) = %d, want %d: %+v", len(history), len(wantStates), history)
	}
	for i, record := range history {
		if record.To != wantStates[i] {
			t.Errorf("history[%d].To = %q, want %q", i, record.To, wantStates[i])
		}
		if record.AgentID != "agent-1" {
			t.Errorf("history[%d].AgentID = %q, want agent-1", i, record.AgentID)
		}
	}
}

// TestLifecycleManager_ShutdownCleansResources verifies SPEC-0021's second
// testing criterion: Stop invokes an Agent's Cleanup hook before marking it
// STOPPED.
func TestLifecycleManager_ShutdownCleansResources(t *testing.T) {
	registry := NewRegistry()
	m := NewLifecycleManager(registry)
	agent := newLifecycleAgent("agent-1")

	mustReachRunning(t, m, agent)

	if err := m.Stop(context.Background(), "agent-1"); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	if agent.cleanupCalled != 1 {
		t.Errorf("Cleanup called %d times, want 1", agent.cleanupCalled)
	}
	if state, _ := m.State("agent-1"); state != types.AgentStatusStopped {
		t.Errorf("State after Stop = %q, want stopped", state)
	}
}

// TestLifecycleManager_InitFailureIsHandled verifies SPEC-0021's third
// testing criterion: an Init hook failure fails the agent rather than
// leaving it stuck INITIALIZING or silently marked READY.
func TestLifecycleManager_InitFailureIsHandled(t *testing.T) {
	registry := NewRegistry()
	m := NewLifecycleManager(registry)
	agent := newLifecycleAgent("agent-1")
	agent.initErr = errors.New("boom")

	if err := m.Register(agent); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	err := m.Initialize(context.Background(), "agent-1")
	if !pkgerrors.HasCode(err, "AGENT_INIT_FAILED") {
		t.Errorf("Initialize error = %v, want code AGENT_INIT_FAILED", err)
	}
	if state, _ := m.State("agent-1"); state != types.AgentStatusFailed {
		t.Errorf("State after failed Initialize = %q, want failed", state)
	}
}

// TestLifecycleManager_CleanupFailureIsHandled verifies a Cleanup hook
// failure fails the agent rather than marking it STOPPED anyway.
func TestLifecycleManager_CleanupFailureIsHandled(t *testing.T) {
	registry := NewRegistry()
	m := NewLifecycleManager(registry)
	agent := newLifecycleAgent("agent-1")
	agent.cleanupErr = errors.New("disk full")

	mustReachRunning(t, m, agent)

	err := m.Stop(context.Background(), "agent-1")
	if !pkgerrors.HasCode(err, "AGENT_CLEANUP_FAILED") {
		t.Errorf("Stop error = %v, want code AGENT_CLEANUP_FAILED", err)
	}
	if state, _ := m.State("agent-1"); state != types.AgentStatusFailed {
		t.Errorf("State after failed Stop = %q, want failed", state)
	}
}

// TestLifecycleManager_FailRecordsReason verifies Fail moves an agent to
// FAILED from any non-terminal state and records the cause on the
// transition.
func TestLifecycleManager_FailRecordsReason(t *testing.T) {
	registry := NewRegistry()
	m := NewLifecycleManager(registry)
	agent := newLifecycleAgent("agent-1")
	if err := m.Register(agent); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	if err := m.Fail("agent-1", errors.New("unexpected crash")); err != nil {
		t.Fatalf("Fail returned error: %v", err)
	}
	if state, _ := m.State("agent-1"); state != types.AgentStatusFailed {
		t.Errorf("State after Fail = %q, want failed", state)
	}

	history := m.History("agent-1")
	if len(history) != 1 || history[0].Reason != "unexpected crash" {
		t.Errorf("History = %+v, want one record with Reason=unexpected crash", history)
	}
}

// TestLifecycleManager_TerminalStateRejectsTransition verifies a terminal
// agent (STOPPED or FAILED) cannot be transitioned again.
func TestLifecycleManager_TerminalStateRejectsTransition(t *testing.T) {
	registry := NewRegistry()
	m := NewLifecycleManager(registry)
	agent := newLifecycleAgent("agent-1")
	mustReachRunning(t, m, agent)

	if err := m.Stop(context.Background(), "agent-1"); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}

	err := m.Start("agent-1")
	if !pkgerrors.HasCode(err, "AGENT_LIFECYCLE_INVALID_TRANSITION") {
		t.Errorf("Start after Stop error = %v, want code AGENT_LIFECYCLE_INVALID_TRANSITION", err)
	}
	if !pkgerrors.Is(err, pkgerrors.TypeInvalidInput) {
		t.Errorf("Start after Stop error type = %v, want TypeInvalidInput", err)
	}
}

// TestLifecycleManager_UnregisteredAgentIsNotFound verifies every lifecycle
// method reports TypeNotFound for an ID that was never Registered.
func TestLifecycleManager_UnregisteredAgentIsNotFound(t *testing.T) {
	registry := NewRegistry()
	m := NewLifecycleManager(registry)

	if _, err := m.State("missing"); !pkgerrors.HasCode(err, "AGENT_LIFECYCLE_NOT_REGISTERED") {
		t.Errorf("State error = %v, want code AGENT_LIFECYCLE_NOT_REGISTERED", err)
	}
	if err := m.Start("missing"); !pkgerrors.HasCode(err, "AGENT_LIFECYCLE_NOT_REGISTERED") {
		t.Errorf("Start error = %v, want code AGENT_LIFECYCLE_NOT_REGISTERED", err)
	}
	if err := m.Initialize(context.Background(), "missing"); !pkgerrors.HasCode(err, "AGENT_REGISTRY_AGENT_NOT_FOUND") {
		t.Errorf("Initialize error = %v, want code AGENT_REGISTRY_AGENT_NOT_FOUND", err)
	}
}

// TestLifecycleManager_RegisterRejectsDuplicate verifies Register delegates
// to the underlying AgentRegistry and records no lifecycle state when
// registration itself is rejected.
func TestLifecycleManager_RegisterRejectsDuplicate(t *testing.T) {
	registry := NewRegistry()
	m := NewLifecycleManager(registry)
	first := newLifecycleAgent("agent-1")
	second := newLifecycleAgent("agent-1")

	if err := m.Register(first); err != nil {
		t.Fatalf("Register first returned error: %v", err)
	}
	if err := m.Register(second); !pkgerrors.HasCode(err, "AGENT_REGISTRY_DUPLICATE_AGENT") {
		t.Errorf("Register second error = %v, want code AGENT_REGISTRY_DUPLICATE_AGENT", err)
	}
}

// TestLifecycleManager_HistoryReturnsCopyNotInternalSlice mirrors
// StateMachine's equivalent guarantee: a caller mutating a returned History
// slice must not affect the manager's own records.
func TestLifecycleManager_HistoryReturnsCopyNotInternalSlice(t *testing.T) {
	registry := NewRegistry()
	m := NewLifecycleManager(registry)
	agent := newLifecycleAgent("agent-1")
	if err := m.Register(agent); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if err := m.Fail("agent-1", nil); err != nil {
		t.Fatalf("Fail returned error: %v", err)
	}

	history := m.History("agent-1")
	history[0].To = types.AgentStatusRunning

	fresh := m.History("agent-1")
	if fresh[0].To != types.AgentStatusFailed {
		t.Errorf("mutating a returned History slice affected internal state: %+v", fresh)
	}
}

// mustReachRunning registers agent and drives it through Initialize and
// Start, failing the test immediately if any step errors.
func mustReachRunning(t *testing.T, m *LifecycleManager, agent Agent) {
	t.Helper()
	id := agent.Metadata().ID
	if err := m.Register(agent); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if err := m.Initialize(context.Background(), id); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	if err := m.Start(id); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
}
