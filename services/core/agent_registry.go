// agent_registry.go implements SPEC-0020: the Agent Registry. Registry
// stores registered SPEC-0018 Agents and lets Core Runtime discover them by
// ID, inspect their capabilities (via the returned Agent's Metadata), and
// route Tasks to them (via the returned Agent's Execute). This is the
// concrete contract that fills the Container.AgentRegistry slot reserved by
// SPEC-0008.
package core

import (
	"fmt"
	"sort"
	"sync"

	"jarvis-pa/packages/errors"
)

// AgentRegistry is the SPEC-0020 contract for storing and discovering
// registered Agents.
type AgentRegistry interface {
	// Register adds agent to the registry, keyed by agent.Metadata().ID.
	// Register rejects an agent whose Metadata fails AgentMetadata.Validate,
	// and an agent whose ID is already registered - "check capabilities" and
	// "route tasks" (SPEC-0020's Registry Usage) both go through Lookup
	// rather than a separate API, since Agent's own Metadata/Execute already
	// expose what's needed once an Agent is found.
	Register(agent Agent) error

	// Lookup returns the registered Agent with the given ID. It returns a
	// packages/errors error typed TypeNotFound if no agent has that ID.
	Lookup(id string) (Agent, error)

	// Remove unregisters the agent with the given ID. It returns a
	// packages/errors error typed TypeNotFound if no agent has that ID.
	Remove(id string) error

	// List returns every registered Agent, ordered by ID.
	List() []Agent
}

// Registry is an in-memory AgentRegistry. Registry is safe for concurrent
// use.
type Registry struct {
	mu     sync.Mutex
	agents map[string]Agent
}

// NewRegistry creates a ready-to-use, empty Registry.
func NewRegistry() *Registry {
	return &Registry{agents: make(map[string]Agent)}
}

func (r *Registry) Register(agent Agent) error {
	metadata := agent.Metadata()
	if err := metadata.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.agents[metadata.ID]; exists {
		return errors.New(errors.TypeAlreadyExists, "AGENT_REGISTRY_DUPLICATE_AGENT", "core.agentregistry",
			fmt.Sprintf("agent %q is already registered", metadata.ID)).With("agentId", metadata.ID)
	}

	r.agents[metadata.ID] = agent
	return nil
}

func (r *Registry) Lookup(id string) (Agent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	agent, ok := r.agents[id]
	if !ok {
		return nil, errors.New(errors.TypeNotFound, "AGENT_REGISTRY_AGENT_NOT_FOUND", "core.agentregistry",
			fmt.Sprintf("no registered agent with id %q", id)).With("agentId", id)
	}
	return agent, nil
}

func (r *Registry) Remove(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.agents[id]; !ok {
		return errors.New(errors.TypeNotFound, "AGENT_REGISTRY_AGENT_NOT_FOUND", "core.agentregistry",
			fmt.Sprintf("no registered agent with id %q", id)).With("agentId", id)
	}
	delete(r.agents, id)
	return nil
}

func (r *Registry) List() []Agent {
	r.mu.Lock()
	defer r.mu.Unlock()

	ids := make([]string, 0, len(r.agents))
	for id := range r.agents {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]Agent, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.agents[id])
	}
	return out
}
