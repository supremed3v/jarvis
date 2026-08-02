package core

import (
	"context"
	"testing"

	"jarvis-pa/packages/errors"
)

func TestLocalStore_PutGetRoundTrip(t *testing.T) {
	s := NewLocalStore()
	ctx := context.Background()

	rec := MemoryRecord{Type: MemoryTypeConversation, Content: "hello world", Metadata: map[string]any{"k": "v"}}
	localID, err := s.Put(ctx, rec)
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if localID == "" {
		t.Fatalf("Put() returned empty localID")
	}

	got, err := s.Get(ctx, localID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Content != rec.Content || got.Type != rec.Type {
		t.Errorf("Get() = %+v, want Content/Type from %+v", got, rec)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Errorf("Get() timestamps not set: %+v", got)
	}
}

func TestLocalStore_GetMissingIsNotFound(t *testing.T) {
	s := NewLocalStore()
	if _, err := s.Get(context.Background(), "missing"); !errors.Is(err, errors.TypeNotFound) {
		t.Errorf("Get() of missing ID error = %v, want TypeNotFound", err)
	}
}

func TestLocalStore_QuerySubstringMatchAndLimit(t *testing.T) {
	s := NewLocalStore()
	ctx := context.Background()

	mustPut(t, s, MemoryRecord{Type: MemoryTypeConversation, Content: "the quick brown fox"})
	mustPut(t, s, MemoryRecord{Type: MemoryTypeConversation, Content: "quick start guide"})
	mustPut(t, s, MemoryRecord{Type: MemoryTypeKnowledge, Content: "unrelated entry"})

	matches, err := s.Query(ctx, MemoryQuery{Query: "quick", Limit: 1})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("Query() returned %d matches, want 1 (Limit)", len(matches))
	}

	all, err := s.Query(ctx, MemoryQuery{Query: "quick"})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("Query() returned %d matches, want 2", len(all))
	}
}

func TestLocalStore_QueryFiltersByType(t *testing.T) {
	s := NewLocalStore()
	ctx := context.Background()

	mustPut(t, s, MemoryRecord{Type: MemoryTypeConversation, Content: "shared word apple"})
	mustPut(t, s, MemoryRecord{Type: MemoryTypeKnowledge, Content: "shared word apple"})

	matches, err := s.Query(ctx, MemoryQuery{Query: "apple", Type: MemoryTypeKnowledge})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(matches) != 1 || matches[0].Type != MemoryTypeKnowledge {
		t.Errorf("Query() = %+v, want a single MemoryTypeKnowledge match", matches)
	}
}

func TestLocalStore_ReplaceUpdatesFieldsAndPreservesCreatedAt(t *testing.T) {
	s := NewLocalStore()
	ctx := context.Background()

	localID := mustPut(t, s, MemoryRecord{Type: MemoryTypeConversation, Content: "original"})
	original, err := s.Get(ctx, localID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if err := s.Replace(ctx, localID, MemoryRecord{Type: MemoryTypeConversation, Content: "updated"}); err != nil {
		t.Fatalf("Replace() error = %v", err)
	}

	got, err := s.Get(ctx, localID)
	if err != nil {
		t.Fatalf("Get() after Replace error = %v", err)
	}
	if got.Content != "updated" {
		t.Errorf("Get() after Replace Content = %q, want %q", got.Content, "updated")
	}
	if !got.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("Replace() changed CreatedAt: got %v, want %v", got.CreatedAt, original.CreatedAt)
	}
}

func TestLocalStore_ReplaceMissingIsNotFound(t *testing.T) {
	s := NewLocalStore()
	err := s.Replace(context.Background(), "missing", MemoryRecord{Type: MemoryTypeConversation, Content: "x"})
	if !errors.Is(err, errors.TypeNotFound) {
		t.Errorf("Replace() of missing ID error = %v, want TypeNotFound", err)
	}
}

func TestLocalStore_RemoveThenGetIsNotFound(t *testing.T) {
	s := NewLocalStore()
	ctx := context.Background()
	localID := mustPut(t, s, MemoryRecord{Type: MemoryTypeConversation, Content: "x"})

	if err := s.Remove(ctx, localID); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := s.Get(ctx, localID); !errors.Is(err, errors.TypeNotFound) {
		t.Errorf("Get() after Remove error = %v, want TypeNotFound", err)
	}
}

func TestLocalStore_RemoveMissingIsNotFound(t *testing.T) {
	s := NewLocalStore()
	if err := s.Remove(context.Background(), "missing"); !errors.Is(err, errors.TypeNotFound) {
		t.Errorf("Remove() of missing ID error = %v, want TypeNotFound", err)
	}
}

func TestLocalStore_Name(t *testing.T) {
	if got := NewLocalStore().Name(); got != "local" {
		t.Errorf("Name() = %q, want %q", got, "local")
	}
}

// mustPut is a test helper that Puts rec into provider and fails the test on
// error, returning the assigned local ID.
func mustPut(t *testing.T, provider MemoryStorageProvider, rec MemoryRecord) string {
	t.Helper()
	localID, err := provider.Put(context.Background(), rec)
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	return localID
}
