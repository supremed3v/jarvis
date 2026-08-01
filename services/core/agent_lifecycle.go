// agent_lifecycle.go implements SPEC-0021: the Agent Lifecycle Manager.
// LifecycleManager tracks each registered Agent's runtime state through
// initialization, readiness, running, and shutdown, registering new Agents
// on the AgentRegistry (SPEC-0020) and validating every state transition -
// mirroring the Task State Machine's precedent (SPEC-0012,
// task_state_machine.go) for how JARVIS models a lifecycle: a closed
// transition table plus a recorded history, rather than a free-form status
// field.
package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	"jarvis-pa/packages/errors"
	types "jarvis-pa/packages/shared-types"
)

// AgentInitializer is an optional Agent capability (SPEC-0021). An Agent
// implementing it has Init called during the INITIALIZING phase, before it
// is marked READY; an Agent that doesn't implement it moves straight to
// READY.
type AgentInitializer interface {
	Init(ctx context.Context) error
}

// AgentCleaner is an optional Agent capability (SPEC-0021). An Agent
// implementing it has Cleanup called during the STOPPING phase, before it is
// marked STOPPED, so it can release any resources it holds; an Agent that
// doesn't implement it moves straight to STOPPED.
type AgentCleaner interface {
	Cleanup(ctx context.Context) error
}

// agentValidTransitions is the closed set of allowed AgentStatus
// transitions. Stopped and Failed are terminal: once reached, an Agent
// cannot leave that state (mirrors task_state_machine.go's
// validTransitions).
var agentValidTransitions = map[types.AgentStatus][]types.AgentStatus{
	types.AgentStatusRegistered:   {types.AgentStatusInitializing, types.AgentStatusFailed},
	types.AgentStatusInitializing: {types.AgentStatusReady, types.AgentStatusFailed},
	types.AgentStatusReady:        {types.AgentStatusRunning, types.AgentStatusStopping, types.AgentStatusFailed},
	types.AgentStatusRunning:      {types.AgentStatusStopping, types.AgentStatusFailed},
	types.AgentStatusStopping:     {types.AgentStatusStopped, types.AgentStatusFailed},
	types.AgentStatusStopped:      {},
	types.AgentStatusFailed:       {},
}

// CanTransitionAgent reports whether an Agent may move from "from" to "to"
// directly.
func CanTransitionAgent(from, to types.AgentStatus) bool {
	for _, allowed := range agentValidTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// LifecycleManager manages the SPEC-0021 lifecycle of Agents registered on
// an AgentRegistry: it validates every state transition, invokes an Agent's
// optional Init/Cleanup hooks at the right phase, and records each accepted
// transition so an Agent's full history can be inspected. LifecycleManager
// is safe for concurrent use.
type LifecycleManager struct {
	registry AgentRegistry

	mu      sync.Mutex
	states  map[string]types.AgentStatus
	history map[string][]types.AgentTransition
}

// NewLifecycleManager creates a ready-to-use LifecycleManager whose Register
// delegates to registry.
func NewLifecycleManager(registry AgentRegistry) *LifecycleManager {
	return &LifecycleManager{
		registry: registry,
		states:   make(map[string]types.AgentStatus),
		history:  make(map[string][]types.AgentTransition),
	}
}

// Register adds agent to the underlying AgentRegistry and marks it
// REGISTERED. It returns whatever error the AgentRegistry's Register
// returns (e.g. invalid metadata, duplicate ID) without recording any
// lifecycle state.
func (m *LifecycleManager) Register(agent Agent) error {
	if err := m.registry.Register(agent); err != nil {
		return err
	}

	id := agent.Metadata().ID
	m.mu.Lock()
	m.states[id] = types.AgentStatusRegistered
	m.mu.Unlock()
	return nil
}

// Initialize moves agentID from REGISTERED to READY, calling the Agent's
// Init method first if it implements AgentInitializer. If Init returns an
// error, the Agent is transitioned to FAILED instead and the error is
// returned.
func (m *LifecycleManager) Initialize(ctx context.Context, agentID string) error {
	agent, err := m.registry.Lookup(agentID)
	if err != nil {
		return err
	}

	if _, err := m.transition(agentID, types.AgentStatusInitializing, ""); err != nil {
		return err
	}

	if initializer, ok := agent.(AgentInitializer); ok {
		if err := initializer.Init(ctx); err != nil {
			wrapped := errors.Wrap(err, errors.TypeInternal, "AGENT_INIT_FAILED", "core.agentlifecycle",
				fmt.Sprintf("agent %q failed to initialize", agentID)).With("agentId", agentID)
			m.transition(agentID, types.AgentStatusFailed, wrapped.Error())
			return wrapped
		}
	}

	_, err = m.transition(agentID, types.AgentStatusReady, "")
	return err
}

// Start moves agentID from READY to RUNNING.
func (m *LifecycleManager) Start(agentID string) error {
	_, err := m.transition(agentID, types.AgentStatusRunning, "")
	return err
}

// Stop moves agentID to STOPPING then STOPPED, calling the Agent's Cleanup
// method first if it implements AgentCleaner. If Cleanup returns an error,
// the Agent is transitioned to FAILED instead and the error is returned.
func (m *LifecycleManager) Stop(ctx context.Context, agentID string) error {
	agent, err := m.registry.Lookup(agentID)
	if err != nil {
		return err
	}

	if _, err := m.transition(agentID, types.AgentStatusStopping, ""); err != nil {
		return err
	}

	if cleaner, ok := agent.(AgentCleaner); ok {
		if err := cleaner.Cleanup(ctx); err != nil {
			wrapped := errors.Wrap(err, errors.TypeInternal, "AGENT_CLEANUP_FAILED", "core.agentlifecycle",
				fmt.Sprintf("agent %q failed to clean up", agentID)).With("agentId", agentID)
			m.transition(agentID, types.AgentStatusFailed, wrapped.Error())
			return wrapped
		}
	}

	_, err = m.transition(agentID, types.AgentStatusStopped, "")
	return err
}

// Fail transitions agentID to FAILED from its current state, recording
// cause's message as the transition's Reason. It returns a packages/errors
// error typed TypeNotFound if agentID has no recorded lifecycle state, or
// TypeInvalidInput if agentID is already in a terminal state (STOPPED or
// FAILED).
func (m *LifecycleManager) Fail(agentID string, cause error) error {
	reason := ""
	if cause != nil {
		reason = cause.Error()
	}
	_, err := m.transition(agentID, types.AgentStatusFailed, reason)
	return err
}

// State reports agentID's current lifecycle state. It returns a
// packages/errors error typed TypeNotFound if agentID has no recorded
// lifecycle state.
func (m *LifecycleManager) State(agentID string) (types.AgentStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.states[agentID]
	if !ok {
		return "", errors.New(errors.TypeNotFound, "AGENT_LIFECYCLE_NOT_REGISTERED", "core.agentlifecycle",
			fmt.Sprintf("agent %q has no recorded lifecycle state", agentID)).With("agentId", agentID)
	}
	return state, nil
}

// History returns the recorded transitions for agentID in the order they
// were applied. It returns nil if agentID has no recorded transitions.
func (m *LifecycleManager) History(agentID string) []types.AgentTransition {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]types.AgentTransition(nil), m.history[agentID]...)
}

// transition validates and records agentID's move to "to", stamping the
// recorded AgentTransition with reason.
func (m *LifecycleManager) transition(agentID string, to types.AgentStatus, reason string) (types.AgentTransition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	from, ok := m.states[agentID]
	if !ok {
		return types.AgentTransition{}, errors.New(errors.TypeNotFound, "AGENT_LIFECYCLE_NOT_REGISTERED", "core.agentlifecycle",
			fmt.Sprintf("agent %q has no recorded lifecycle state", agentID)).With("agentId", agentID)
	}
	if !CanTransitionAgent(from, to) {
		return types.AgentTransition{}, errors.New(errors.TypeInvalidInput, "AGENT_LIFECYCLE_INVALID_TRANSITION", "core.agentlifecycle",
			fmt.Sprintf("invalid agent transition from %q to %q", from, to)).With("agentId", agentID).With("from", from).With("to", to)
	}

	now := time.Now().UTC()
	record := types.AgentTransition{AgentID: agentID, From: from, To: to, Reason: reason, Timestamp: now}
	m.states[agentID] = to
	m.history[agentID] = append(m.history[agentID], record)
	return record, nil
}
