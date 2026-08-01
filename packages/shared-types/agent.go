package types

import "time"

// AgentStatus is the lifecycle state of an Agent (SPEC-0021 Agent Lifecycle
// Manager). Valid transitions between these states are enforced by
// services/core's LifecycleManager, not by this package (SPEC-0004: shapes
// only, no behavior).
type AgentStatus string

const (
	AgentStatusRegistered   AgentStatus = "registered"
	AgentStatusInitializing AgentStatus = "initializing"
	AgentStatusReady        AgentStatus = "ready"
	AgentStatusRunning      AgentStatus = "running"
	AgentStatusStopping     AgentStatus = "stopping"
	AgentStatusStopped      AgentStatus = "stopped"
	AgentStatusFailed       AgentStatus = "failed"
)

// Agent describes a registered JARVIS agent (SPEC-0018 Agent Interface,
// SPEC-0020 Agent Registry).
type Agent struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Type         string      `json:"type"`
	Status       AgentStatus `json:"status"`
	Capabilities []string    `json:"capabilities,omitempty"`
}

// AgentTransition is a single recorded lifecycle state change for an Agent
// (SPEC-0021 Agent Lifecycle Manager). Produced by services/core's
// LifecycleManager; this package only defines its shape. Reason is set when
// the transition is into AgentStatusFailed and a cause is known; it is
// empty for every other transition.
type AgentTransition struct {
	AgentID   string      `json:"agentId"`
	From      AgentStatus `json:"from"`
	To        AgentStatus `json:"to"`
	Reason    string      `json:"reason,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}
