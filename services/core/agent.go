// agent.go implements SPEC-0018: the Agent Interface. Agent is the base
// contract every JARVIS agent (agents/core-agent, agents/developer-agent,
// agents/research-agent, ...) implements, so Core Runtime can discover,
// execute, and manage any of them consistently regardless of what the
// agent actually does. This is the concrete contract SPEC-0020's Agent
// Registry will hold instances of, filling the Container.AgentRegistry
// slot reserved by SPEC-0008.
package core

import (
	"context"

	"jarvis-pa/packages/errors"
	types "jarvis-pa/packages/shared-types"
)

// AgentMetadata is an Agent's static, declarative identity and declared
// requirements - the SPEC-0018 contract fields the runtime can inspect
// without ever invoking the agent (e.g. to check permissions or discover
// capabilities before a Task is handed to it). Tools, MemoryAccess, and
// Permissions are plain identifiers here; resolving them to concrete
// capabilities is owned by their respective specs (Tool Registry
// SPEC-0043..0045, the memory system, and a future permission system),
// none of which exist yet - mirroring Task's precedent of TaskSource/
// TaskPriority being bare strings that other components own the meaning
// of (packages/shared-types/task.go).
type AgentMetadata struct {
	ID           string
	Name         string
	Description  string
	Instructions string
	Tools        []string
	MemoryAccess []string
	Permissions  []string
}

// Validate reports whether m has the minimum identity a runtime needs to
// discover and manage the agent: a non-empty ID and Name. It returns a
// packages/errors error typed TypeInvalidInput naming the first missing
// field, or nil if m is valid.
func (m AgentMetadata) Validate() error {
	if m.ID == "" {
		return errors.New(errors.TypeInvalidInput, "AGENT_METADATA_MISSING_ID", "core.agent",
			"agent metadata is missing an ID")
	}
	if m.Name == "" {
		return errors.New(errors.TypeInvalidInput, "AGENT_METADATA_MISSING_NAME", "core.agent",
			"agent metadata is missing a Name").With("agentId", m.ID)
	}
	return nil
}

// Agent is the SPEC-0018 base contract every JARVIS agent implements.
// Metadata lets the runtime discover and validate the agent without
// running it; Execute carries out an assigned Task and returns a
// structured result (Agent Responsibilities: understanding the task,
// planning, using approved tools, returning structured results - the
// planning and tool use are internal to the Agent's own Execute logic,
// not separately exposed by this contract).
//
// Execute's signature intentionally matches the Executor type
// (task_worker.go, SPEC-0014): an Agent's Execute method value can be
// passed directly to NewWorker without an adapter, so a Worker can drive
// any Agent the same way it drives a plain Executor func.
type Agent interface {
	// Metadata returns the Agent's static identity and declared
	// requirements.
	Metadata() AgentMetadata

	// Execute performs task and returns its result payload on success, or
	// an error if execution failed. Execute must respect ctx cancellation.
	Execute(ctx context.Context, task *types.Task) (map[string]any, error)
}
