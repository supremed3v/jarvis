package core

import (
	"fmt"
	"sync"
	"testing"

	"jarvis-pa/packages/errors"
)

// TestRegistry_AgentsRegisterSuccessfully verifies SPEC-0020's first testing
// criterion: a valid Agent registers without error and can immediately be
// found by ID.
func TestRegistry_AgentsRegisterSuccessfully(t *testing.T) {
	r := NewRegistry()
	agent := &stubAgent{metadata: AgentMetadata{ID: "agent-1", Name: "Agent One"}}

	if err := r.Register(agent); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	got, err := r.Lookup("agent-1")
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if got != Agent(agent) {
		t.Errorf("Lookup returned %+v, want the registered agent", got)
	}
}

// TestRegistry_RejectsInvalidMetadata verifies Register refuses an agent
// whose Metadata fails AgentMetadata.Validate rather than registering it
// under an empty ID.
func TestRegistry_RejectsInvalidMetadata(t *testing.T) {
	r := NewRegistry()
	agent := &stubAgent{metadata: AgentMetadata{Name: "No ID"}}

	err := r.Register(agent)
	if !errors.HasCode(err, "AGENT_METADATA_MISSING_ID") {
		t.Errorf("Register error = %v, want code AGENT_METADATA_MISSING_ID", err)
	}
}

// TestRegistry_AgentsCanBeDiscovered verifies SPEC-0020's second testing
// criterion: registered agents are found by Lookup and appear in List.
func TestRegistry_AgentsCanBeDiscovered(t *testing.T) {
	r := NewRegistry()
	a1 := &stubAgent{metadata: AgentMetadata{ID: "agent-1", Name: "Agent One"}}
	a2 := &stubAgent{metadata: AgentMetadata{ID: "agent-2", Name: "Agent Two"}}

	if err := r.Register(a1); err != nil {
		t.Fatalf("Register a1 returned error: %v", err)
	}
	if err := r.Register(a2); err != nil {
		t.Fatalf("Register a2 returned error: %v", err)
	}

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("List() returned %d agents, want 2", len(list))
	}
	if list[0].Metadata().ID != "agent-1" || list[1].Metadata().ID != "agent-2" {
		t.Errorf("List() = %+v, want ordered [agent-1 agent-2]", list)
	}
}

// TestRegistry_LookupNotFound verifies looking up an unregistered ID is
// reported as a distinct not-found error.
func TestRegistry_LookupNotFound(t *testing.T) {
	r := NewRegistry()

	_, err := r.Lookup("missing")
	if !errors.HasCode(err, "AGENT_REGISTRY_AGENT_NOT_FOUND") {
		t.Errorf("Lookup error = %v, want code AGENT_REGISTRY_AGENT_NOT_FOUND", err)
	}
	if !errors.Is(err, errors.TypeNotFound) {
		t.Errorf("Lookup error type = %v, want TypeNotFound", err)
	}
}

// TestRegistry_DuplicateRegistrationsAreHandled verifies SPEC-0020's third
// testing criterion: registering a second agent under an ID already in use
// is rejected rather than silently overwriting the first registration.
func TestRegistry_DuplicateRegistrationsAreHandled(t *testing.T) {
	r := NewRegistry()
	first := &stubAgent{metadata: AgentMetadata{ID: "agent-1", Name: "First"}}
	second := &stubAgent{metadata: AgentMetadata{ID: "agent-1", Name: "Second"}}

	if err := r.Register(first); err != nil {
		t.Fatalf("Register first returned error: %v", err)
	}

	err := r.Register(second)
	if !errors.HasCode(err, "AGENT_REGISTRY_DUPLICATE_AGENT") {
		t.Errorf("Register second error = %v, want code AGENT_REGISTRY_DUPLICATE_AGENT", err)
	}
	if !errors.Is(err, errors.TypeAlreadyExists) {
		t.Errorf("Register second error type = %v, want TypeAlreadyExists", err)
	}

	got, lookupErr := r.Lookup("agent-1")
	if lookupErr != nil {
		t.Fatalf("Lookup returned error: %v", lookupErr)
	}
	if got.Metadata().Name != "First" {
		t.Errorf("Lookup().Metadata().Name = %q, want First (rejected duplicate must not overwrite)", got.Metadata().Name)
	}
}

// TestRegistry_Remove verifies a removed agent is no longer discoverable,
// and that removing an unregistered ID is a distinct not-found error.
func TestRegistry_Remove(t *testing.T) {
	r := NewRegistry()
	agent := &stubAgent{metadata: AgentMetadata{ID: "agent-1", Name: "Agent One"}}
	if err := r.Register(agent); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	if err := r.Remove("agent-1"); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if _, err := r.Lookup("agent-1"); !errors.HasCode(err, "AGENT_REGISTRY_AGENT_NOT_FOUND") {
		t.Errorf("Lookup after Remove = %v, want code AGENT_REGISTRY_AGENT_NOT_FOUND", err)
	}

	err := r.Remove("agent-1")
	if !errors.HasCode(err, "AGENT_REGISTRY_AGENT_NOT_FOUND") {
		t.Errorf("Remove already-removed agent error = %v, want code AGENT_REGISTRY_AGENT_NOT_FOUND", err)
	}
}

// TestRegistry_ListEmpty verifies List returns no agents for a fresh
// Registry.
func TestRegistry_ListEmpty(t *testing.T) {
	r := NewRegistry()
	if got := r.List(); len(got) != 0 {
		t.Errorf("List() = %+v, want empty", got)
	}
}

// TestRegistry_ConcurrentRegistration verifies Registry is safe for
// concurrent use: registering distinct IDs from multiple goroutines never
// loses or duplicates an entry.
func TestRegistry_ConcurrentRegistration(t *testing.T) {
	r := NewRegistry()
	const n = 50

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("agent-%d", i)
			_ = r.Register(&stubAgent{metadata: AgentMetadata{ID: id, Name: "Agent"}})
		}(i)
	}
	wg.Wait()

	if got := len(r.List()); got != n {
		t.Errorf("List() returned %d agents, want %d", got, n)
	}
}
