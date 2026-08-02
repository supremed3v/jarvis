package core

import (
	"context"
	"testing"

	"jarvis-pa/packages/errors"
)

// stubMemory is a minimal Memory implementation used to verify the
// SPEC-0034 contract can be implemented and driven by a caller. err, when
// set, is returned by every method that can fail, so a single stub covers
// both the success and failure paths.
type stubMemory struct {
	records map[string]MemoryRecord
	nextID  int
	err     error
}

func newStubMemory() *stubMemory {
	return &stubMemory{records: make(map[string]MemoryRecord)}
}

func (m *stubMemory) Store(ctx context.Context, rec MemoryRecord) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if rec.ID == "" {
		m.nextID++
		rec.ID = string(rune('a' + m.nextID - 1))
	}
	m.records[rec.ID] = rec
	return rec.ID, nil
}

func (m *stubMemory) Retrieve(ctx context.Context, id string) (MemoryRecord, error) {
	if m.err != nil {
		return MemoryRecord{}, m.err
	}
	rec, ok := m.records[id]
	if !ok {
		return MemoryRecord{}, errors.New(errors.TypeNotFound, "MEMORY_RECORD_NOT_FOUND", "core.memory",
			"memory record not found").With("id", id)
	}
	return rec, nil
}

func (m *stubMemory) Search(ctx context.Context, q MemoryQuery) ([]MemoryRecord, error) {
	if m.err != nil {
		return nil, m.err
	}
	var results []MemoryRecord
	for _, rec := range m.records {
		if q.Type != "" && rec.Type != q.Type {
			continue
		}
		results = append(results, rec)
	}
	return results, nil
}

func (m *stubMemory) Update(ctx context.Context, rec MemoryRecord) error {
	if m.err != nil {
		return m.err
	}
	if _, ok := m.records[rec.ID]; !ok {
		return errors.New(errors.TypeNotFound, "MEMORY_RECORD_NOT_FOUND", "core.memory",
			"memory record not found").With("id", rec.ID)
	}
	m.records[rec.ID] = rec
	return nil
}

func (m *stubMemory) Delete(ctx context.Context, id string) error {
	if m.err != nil {
		return m.err
	}
	if _, ok := m.records[id]; !ok {
		return errors.New(errors.TypeNotFound, "MEMORY_RECORD_NOT_FOUND", "core.memory",
			"memory record not found").With("id", id)
	}
	delete(m.records, id)
	return nil
}

// TestMemory_InterfaceCanBeImplemented verifies a concrete type can satisfy
// the Memory interface (SPEC-0034 testing criterion 1).
func TestMemory_InterfaceCanBeImplemented(t *testing.T) {
	var mem Memory = newStubMemory()

	id, err := mem.Store(context.Background(), MemoryRecord{Type: MemoryTypeConversation, Content: "hello"})
	if err != nil {
		t.Fatalf("Store() returned error: %v", err)
	}
	if id == "" {
		t.Error("Store() returned empty ID")
	}
}

// TestMemory_OperationsFollowContract verifies Store, Retrieve, Search,
// Update, and Delete behave as documented for all four memory types
// (SPEC-0034 testing criterion 2).
func TestMemory_OperationsFollowContract(t *testing.T) {
	mem := newStubMemory()
	ctx := context.Background()

	id, err := mem.Store(ctx, MemoryRecord{Type: MemoryTypeUserProfile, Content: "likes dark mode"})
	if err != nil {
		t.Fatalf("Store() returned error: %v", err)
	}

	got, err := mem.Retrieve(ctx, id)
	if err != nil {
		t.Fatalf("Retrieve() returned error: %v", err)
	}
	if got.Type != MemoryTypeUserProfile || got.Content != "likes dark mode" {
		t.Errorf("Retrieve() = %+v, want Type=%q Content=%q", got, MemoryTypeUserProfile, "likes dark mode")
	}

	if _, err := mem.Store(ctx, MemoryRecord{Type: MemoryTypeKnowledge, Content: "go is a language"}); err != nil {
		t.Fatalf("Store() returned error: %v", err)
	}
	if _, err := mem.Store(ctx, MemoryRecord{Type: MemoryTypeExperience, Content: "deployed a fix on Friday"}); err != nil {
		t.Fatalf("Store() returned error: %v", err)
	}

	results, err := mem.Search(ctx, MemoryQuery{Type: MemoryTypeUserProfile, Query: "dark mode"})
	if err != nil {
		t.Fatalf("Search() returned error: %v", err)
	}
	if len(results) != 1 || results[0].ID != id {
		t.Errorf("Search() = %+v, want one result with ID %q", results, id)
	}

	got.Content = "likes light mode now"
	if err := mem.Update(ctx, got); err != nil {
		t.Fatalf("Update() returned error: %v", err)
	}
	updated, err := mem.Retrieve(ctx, id)
	if err != nil {
		t.Fatalf("Retrieve() after Update returned error: %v", err)
	}
	if updated.Content != "likes light mode now" {
		t.Errorf("Retrieve() after Update = %+v, want Content=%q", updated, "likes light mode now")
	}

	if err := mem.Delete(ctx, id); err != nil {
		t.Fatalf("Delete() returned error: %v", err)
	}
	if _, err := mem.Retrieve(ctx, id); !errors.Is(err, errors.TypeNotFound) {
		t.Errorf("Retrieve() after Delete error = %v, want TypeNotFound", err)
	}
}

// TestMemory_FailuresAreHandledCorrectly verifies that a failing Memory
// provider surfaces its error from every method rather than the call
// succeeding silently, and that operating on an unknown ID reports
// TypeNotFound (SPEC-0034 testing criterion 3).
func TestMemory_FailuresAreHandledCorrectly(t *testing.T) {
	wantErr := errors.New(errors.TypeUnavailable, "MEMORY_BACKEND_UNREACHABLE", "core.memory", "backend unreachable")
	mem := &stubMemory{records: make(map[string]MemoryRecord), err: wantErr}
	ctx := context.Background()

	if _, err := mem.Store(ctx, MemoryRecord{Type: MemoryTypeConversation, Content: "hi"}); err != wantErr {
		t.Errorf("Store() error = %v, want %v", err, wantErr)
	}
	if _, err := mem.Retrieve(ctx, "any"); err != wantErr {
		t.Errorf("Retrieve() error = %v, want %v", err, wantErr)
	}
	if _, err := mem.Search(ctx, MemoryQuery{Query: "any"}); err != wantErr {
		t.Errorf("Search() error = %v, want %v", err, wantErr)
	}
	if err := mem.Update(ctx, MemoryRecord{ID: "any"}); err != wantErr {
		t.Errorf("Update() error = %v, want %v", err, wantErr)
	}
	if err := mem.Delete(ctx, "any"); err != wantErr {
		t.Errorf("Delete() error = %v, want %v", err, wantErr)
	}

	clean := newStubMemory()
	if _, err := clean.Retrieve(ctx, "missing"); !errors.Is(err, errors.TypeNotFound) {
		t.Errorf("Retrieve() of missing ID error = %v, want TypeNotFound", err)
	}
	if err := clean.Update(ctx, MemoryRecord{ID: "missing", Type: MemoryTypeKnowledge, Content: "x"}); !errors.Is(err, errors.TypeNotFound) {
		t.Errorf("Update() of missing ID error = %v, want TypeNotFound", err)
	}
	if err := clean.Delete(ctx, "missing"); !errors.Is(err, errors.TypeNotFound) {
		t.Errorf("Delete() of missing ID error = %v, want TypeNotFound", err)
	}
}

// TestMemoryRecord_Validate verifies MemoryRecord's required fields are
// enforced and reported individually.
func TestMemoryRecord_Validate(t *testing.T) {
	tests := []struct {
		name     string
		rec      MemoryRecord
		wantCode string
	}{
		{
			name: "valid",
			rec:  MemoryRecord{Type: MemoryTypeConversation, Content: "hi"},
		},
		{
			name:     "missing type",
			rec:      MemoryRecord{Content: "hi"},
			wantCode: "MEMORY_RECORD_MISSING_TYPE",
		},
		{
			name:     "invalid type",
			rec:      MemoryRecord{Type: "bogus", Content: "hi"},
			wantCode: "MEMORY_RECORD_INVALID_TYPE",
		},
		{
			name:     "missing content",
			rec:      MemoryRecord{Type: MemoryTypeKnowledge},
			wantCode: "MEMORY_RECORD_MISSING_CONTENT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rec.Validate()
			if tt.wantCode == "" {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error with code %s", tt.wantCode)
			}
			if !errors.HasCode(err, tt.wantCode) {
				t.Errorf("missing code %s: %v", tt.wantCode, err)
			}
			if !errors.Is(err, errors.TypeInvalidInput) {
				t.Errorf("error type = %v, want TypeInvalidInput", err)
			}
		})
	}
}

// TestMemoryQuery_Validate verifies MemoryQuery's required fields are
// enforced and reported individually.
func TestMemoryQuery_Validate(t *testing.T) {
	tests := []struct {
		name     string
		q        MemoryQuery
		wantCode string
	}{
		{
			name: "valid with type",
			q:    MemoryQuery{Type: MemoryTypeExperience, Query: "deploy"},
		},
		{
			name: "valid without type",
			q:    MemoryQuery{Query: "deploy"},
		},
		{
			name:     "missing query",
			q:        MemoryQuery{Type: MemoryTypeExperience},
			wantCode: "MEMORY_QUERY_MISSING_QUERY",
		},
		{
			name:     "invalid type",
			q:        MemoryQuery{Type: "bogus", Query: "deploy"},
			wantCode: "MEMORY_QUERY_INVALID_TYPE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.q.Validate()
			if tt.wantCode == "" {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error with code %s", tt.wantCode)
			}
			if !errors.HasCode(err, tt.wantCode) {
				t.Errorf("missing code %s: %v", tt.wantCode, err)
			}
			if !errors.Is(err, errors.TypeInvalidInput) {
				t.Errorf("error type = %v, want TypeInvalidInput", err)
			}
		})
	}
}
