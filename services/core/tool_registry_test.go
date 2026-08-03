package core

import (
	"fmt"
	"sync"
	"testing"

	"jarvis-pa/packages/errors"
)

// TestToolRegistry_ToolsRegisterSuccessfully verifies SPEC-0045's first
// testing criterion: a valid Tool registers without error and can
// immediately be found by ID.
func TestToolRegistry_ToolsRegisterSuccessfully(t *testing.T) {
	r := NewToolRegistry()
	tool := &stubTool{metadata: ToolMetadata{ID: "tool-1", Name: "Tool One"}}

	if err := r.Register(tool); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	got, err := r.Lookup("tool-1")
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if got != Tool(tool) {
		t.Errorf("Lookup returned %+v, want the registered tool", got)
	}
}

// TestToolRegistry_RejectsInvalidMetadata verifies Register refuses a tool
// whose Metadata fails ToolMetadata.Validate rather than registering it
// under an empty ID.
func TestToolRegistry_RejectsInvalidMetadata(t *testing.T) {
	r := NewToolRegistry()
	tool := &stubTool{metadata: ToolMetadata{Name: "No ID"}}

	err := r.Register(tool)
	if !errors.HasCode(err, "TOOL_METADATA_MISSING_ID") {
		t.Errorf("Register error = %v, want code TOOL_METADATA_MISSING_ID", err)
	}
}

// TestToolRegistry_ToolsCanBeDiscovered verifies SPEC-0045's second testing
// criterion: registered tools are found by Lookup, appear in List, and are
// reported available by IsAvailable.
func TestToolRegistry_ToolsCanBeDiscovered(t *testing.T) {
	r := NewToolRegistry()
	t1 := &stubTool{metadata: ToolMetadata{ID: "tool-1", Name: "Tool One"}}
	t2 := &stubTool{metadata: ToolMetadata{ID: "tool-2", Name: "Tool Two"}}

	if err := r.Register(t1); err != nil {
		t.Fatalf("Register t1 returned error: %v", err)
	}
	if err := r.Register(t2); err != nil {
		t.Fatalf("Register t2 returned error: %v", err)
	}

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("List() returned %d tools, want 2", len(list))
	}
	if list[0].Metadata().ID != "tool-1" || list[1].Metadata().ID != "tool-2" {
		t.Errorf("List() = %+v, want ordered [tool-1 tool-2]", list)
	}

	if !r.IsAvailable("tool-1") {
		t.Error("IsAvailable(tool-1) = false, want true")
	}
	if r.IsAvailable("missing") {
		t.Error("IsAvailable(missing) = true, want false")
	}
}

// TestToolRegistry_LookupNotFound verifies looking up an unregistered ID is
// reported as a distinct not-found error.
func TestToolRegistry_LookupNotFound(t *testing.T) {
	r := NewToolRegistry()

	_, err := r.Lookup("missing")
	if !errors.HasCode(err, "TOOL_REGISTRY_TOOL_NOT_FOUND") {
		t.Errorf("Lookup error = %v, want code TOOL_REGISTRY_TOOL_NOT_FOUND", err)
	}
	if !errors.Is(err, errors.TypeNotFound) {
		t.Errorf("Lookup error type = %v, want TypeNotFound", err)
	}
}

// TestToolRegistry_DuplicateRegistrationsAreHandled verifies SPEC-0045's
// third testing criterion: registering a second tool under an ID already in
// use is rejected rather than silently overwriting the first registration.
func TestToolRegistry_DuplicateRegistrationsAreHandled(t *testing.T) {
	r := NewToolRegistry()
	first := &stubTool{metadata: ToolMetadata{ID: "tool-1", Name: "First"}}
	second := &stubTool{metadata: ToolMetadata{ID: "tool-1", Name: "Second"}}

	if err := r.Register(first); err != nil {
		t.Fatalf("Register first returned error: %v", err)
	}

	err := r.Register(second)
	if !errors.HasCode(err, "TOOL_REGISTRY_DUPLICATE_TOOL") {
		t.Errorf("Register second error = %v, want code TOOL_REGISTRY_DUPLICATE_TOOL", err)
	}
	if !errors.Is(err, errors.TypeAlreadyExists) {
		t.Errorf("Register second error type = %v, want TypeAlreadyExists", err)
	}

	got, lookupErr := r.Lookup("tool-1")
	if lookupErr != nil {
		t.Fatalf("Lookup returned error: %v", lookupErr)
	}
	if got.Metadata().Name != "First" {
		t.Errorf("Lookup().Metadata().Name = %q, want First (rejected duplicate must not overwrite)", got.Metadata().Name)
	}
}

// TestToolRegistry_Remove verifies a removed tool is no longer discoverable
// or available, and that removing an unregistered ID is a distinct
// not-found error.
func TestToolRegistry_Remove(t *testing.T) {
	r := NewToolRegistry()
	tool := &stubTool{metadata: ToolMetadata{ID: "tool-1", Name: "Tool One"}}
	if err := r.Register(tool); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	if err := r.Remove("tool-1"); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if _, err := r.Lookup("tool-1"); !errors.HasCode(err, "TOOL_REGISTRY_TOOL_NOT_FOUND") {
		t.Errorf("Lookup after Remove = %v, want code TOOL_REGISTRY_TOOL_NOT_FOUND", err)
	}
	if r.IsAvailable("tool-1") {
		t.Error("IsAvailable after Remove = true, want false")
	}

	err := r.Remove("tool-1")
	if !errors.HasCode(err, "TOOL_REGISTRY_TOOL_NOT_FOUND") {
		t.Errorf("Remove already-removed tool error = %v, want code TOOL_REGISTRY_TOOL_NOT_FOUND", err)
	}
}

// TestToolRegistry_ListEmpty verifies List returns no tools for a fresh
// ToolRegistryStore.
func TestToolRegistry_ListEmpty(t *testing.T) {
	r := NewToolRegistry()
	if got := r.List(); len(got) != 0 {
		t.Errorf("List() = %+v, want empty", got)
	}
}

// TestToolRegistry_ConcurrentRegistration verifies ToolRegistryStore is safe
// for concurrent use: registering distinct IDs from multiple goroutines
// never loses or duplicates an entry.
func TestToolRegistry_ConcurrentRegistration(t *testing.T) {
	r := NewToolRegistry()
	const n = 50

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("tool-%d", i)
			_ = r.Register(&stubTool{metadata: ToolMetadata{ID: id, Name: "Tool"}})
		}(i)
	}
	wg.Wait()

	if got := len(r.List()); got != n {
		t.Errorf("List() returned %d tools, want %d", got, n)
	}
}
