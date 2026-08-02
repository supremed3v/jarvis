package core

import (
	"context"
	"testing"

	"jarvis-pa/packages/errors"
)

func TestVectorStore_PutGetRoundTrip(t *testing.T) {
	s := NewVectorStore()
	ctx := context.Background()

	rec := MemoryRecord{Type: MemoryTypeKnowledge, Content: "vector storage entry"}
	localID, err := s.Put(ctx, rec)
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	got, err := s.Get(ctx, localID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Content != rec.Content || got.Type != rec.Type {
		t.Errorf("Get() = %+v, want Content/Type from %+v", got, rec)
	}
}

func TestVectorStore_GetMissingIsNotFound(t *testing.T) {
	s := NewVectorStore()
	if _, err := s.Get(context.Background(), "missing"); !errors.Is(err, errors.TypeNotFound) {
		t.Errorf("Get() of missing ID error = %v, want TypeNotFound", err)
	}
}

func TestVectorStore_QueryRanksByWordOverlap(t *testing.T) {
	s := NewVectorStore()
	ctx := context.Background()

	lowID := mustPut(t, s, MemoryRecord{Type: MemoryTypeKnowledge, Content: "jarvis handles one shared topic"})
	highID := mustPut(t, s, MemoryRecord{Type: MemoryTypeKnowledge, Content: "jarvis memory storage shared topic overlap"})
	mustPut(t, s, MemoryRecord{Type: MemoryTypeKnowledge, Content: "completely unrelated content"})

	matches, err := s.Query(ctx, MemoryQuery{Query: "jarvis memory storage shared topic"})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("Query() returned %d matches, want 2 (zero-overlap excluded)", len(matches))
	}
	if matches[0].ID != highID {
		t.Errorf("Query()[0].ID = %q, want %q (highest overlap first)", matches[0].ID, highID)
	}
	if matches[1].ID != lowID {
		t.Errorf("Query()[1].ID = %q, want %q", matches[1].ID, lowID)
	}
}

func TestVectorStore_QueryExcludesZeroScoreAndRespectsLimit(t *testing.T) {
	s := NewVectorStore()
	ctx := context.Background()

	mustPut(t, s, MemoryRecord{Type: MemoryTypeKnowledge, Content: "alpha beta gamma"})
	mustPut(t, s, MemoryRecord{Type: MemoryTypeKnowledge, Content: "alpha delta epsilon"})
	mustPut(t, s, MemoryRecord{Type: MemoryTypeKnowledge, Content: "zzz yyy xxx"})

	matches, err := s.Query(ctx, MemoryQuery{Query: "alpha", Limit: 1})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("Query() returned %d matches, want 1 (Limit)", len(matches))
	}
}

func TestVectorStore_ReplaceMissingIsNotFound(t *testing.T) {
	s := NewVectorStore()
	err := s.Replace(context.Background(), "missing", MemoryRecord{Type: MemoryTypeKnowledge, Content: "x"})
	if !errors.Is(err, errors.TypeNotFound) {
		t.Errorf("Replace() of missing ID error = %v, want TypeNotFound", err)
	}
}

func TestVectorStore_RemoveThenGetIsNotFound(t *testing.T) {
	s := NewVectorStore()
	ctx := context.Background()
	localID := mustPut(t, s, MemoryRecord{Type: MemoryTypeKnowledge, Content: "x"})

	if err := s.Remove(ctx, localID); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := s.Get(ctx, localID); !errors.Is(err, errors.TypeNotFound) {
		t.Errorf("Get() after Remove error = %v, want TypeNotFound", err)
	}
}

func TestVectorStore_Name(t *testing.T) {
	if got := NewVectorStore().Name(); got != "vector" {
		t.Errorf("Name() = %q, want %q", got, "vector")
	}
}
