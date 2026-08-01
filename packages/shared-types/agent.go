package types

// AgentStatus is the current lifecycle state of an Agent (SPEC-0021 Agent
// Lifecycle Manager).
type AgentStatus string

const (
	AgentStatusIdle    AgentStatus = "idle"
	AgentStatusRunning AgentStatus = "running"
	AgentStatusStopped AgentStatus = "stopped"
	AgentStatusError   AgentStatus = "error"
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
