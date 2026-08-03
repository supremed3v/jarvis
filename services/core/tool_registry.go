// tool_registry.go implements SPEC-0045: the Tool Registry. ToolRegistry
// stores the SPEC-0043 Tools that SPEC-0044's NewToolFromManifest (or any
// other Tool constructor) produces, and lets agents discover permitted
// capabilities by ID (SPEC-0045's Usage: "Agents use the registry to
// discover permitted capabilities"). This mirrors SPEC-0020's AgentRegistry
// (agent_registry.go) exactly - same Register/Lookup/Remove/List shape for
// Tool as that file already provides for Agent - plus an IsAvailable check
// for SPEC-0045's "Tool availability checks" requirement, which AgentRegistry
// has no equivalent of. This is the concrete contract that fills the
// Container.ToolRegistry slot reserved by SPEC-0008.
package core

import (
	"fmt"
	"sort"
	"sync"

	"jarvis-pa/packages/errors"
)

// ToolRegistry is the SPEC-0045 contract for storing and discovering
// registered Tools.
type ToolRegistry interface {
	// Register adds tool to the registry, keyed by tool.Metadata().ID.
	// Register rejects a tool whose Metadata fails ToolMetadata.Validate,
	// and a tool whose ID is already registered.
	Register(tool Tool) error

	// Lookup returns the registered Tool with the given ID. It returns a
	// packages/errors error typed TypeNotFound if no tool has that ID.
	Lookup(id string) (Tool, error)

	// Remove unregisters the tool with the given ID. It returns a
	// packages/errors error typed TypeNotFound if no tool has that ID.
	Remove(id string) error

	// List returns every registered Tool, ordered by ID.
	List() []Tool

	// IsAvailable reports whether a tool with the given ID is currently
	// registered - SPEC-0045's "Tool availability checks" requirement, for
	// a caller that only needs a yes/no answer without handling Lookup's
	// not-found error.
	IsAvailable(id string) bool
}

// ToolRegistryStore is an in-memory ToolRegistry. ToolRegistryStore is safe
// for concurrent use.
type ToolRegistryStore struct {
	mu    sync.Mutex
	tools map[string]Tool
}

// NewToolRegistry creates a ready-to-use, empty ToolRegistryStore.
func NewToolRegistry() *ToolRegistryStore {
	return &ToolRegistryStore{tools: make(map[string]Tool)}
}

func (r *ToolRegistryStore) Register(tool Tool) error {
	metadata := tool.Metadata()
	if err := metadata.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[metadata.ID]; exists {
		return errors.New(errors.TypeAlreadyExists, "TOOL_REGISTRY_DUPLICATE_TOOL", "core.toolregistry",
			fmt.Sprintf("tool %q is already registered", metadata.ID)).With("toolId", metadata.ID)
	}

	r.tools[metadata.ID] = tool
	return nil
}

func (r *ToolRegistryStore) Lookup(id string) (Tool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	tool, ok := r.tools[id]
	if !ok {
		return nil, errors.New(errors.TypeNotFound, "TOOL_REGISTRY_TOOL_NOT_FOUND", "core.toolregistry",
			fmt.Sprintf("no registered tool with id %q", id)).With("toolId", id)
	}
	return tool, nil
}

func (r *ToolRegistryStore) Remove(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.tools[id]; !ok {
		return errors.New(errors.TypeNotFound, "TOOL_REGISTRY_TOOL_NOT_FOUND", "core.toolregistry",
			fmt.Sprintf("no registered tool with id %q", id)).With("toolId", id)
	}
	delete(r.tools, id)
	return nil
}

func (r *ToolRegistryStore) List() []Tool {
	r.mu.Lock()
	defer r.mu.Unlock()

	ids := make([]string, 0, len(r.tools))
	for id := range r.tools {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]Tool, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.tools[id])
	}
	return out
}

func (r *ToolRegistryStore) IsAvailable(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, ok := r.tools[id]
	return ok
}
