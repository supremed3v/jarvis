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

func TestVectorStore_QueryRanksBySimilarity(t *testing.T) {
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
		t.Fatalf("Query() returned %d matches, want 2 (zero-similarity excluded)", len(matches))
	}
	if matches[0].ID != highID {
		t.Errorf("Query()[0].ID = %q, want %q (highest similarity first)", matches[0].ID, highID)
	}
	if matches[1].ID != lowID {
		t.Errorf("Query()[1].ID = %q, want %q", matches[1].ID, lowID)
	}
}

// TestVectorStore_QuerySimilarityDistinguishesExactFromNoisyMatch verifies
// ranking is real cosine similarity, not a raw shared-word count: a record
// that exactly matches the query outranks one that shares the same words
// plus several unrelated ones (both would tie under a raw-overlap-count
// scorer, since both share exactly 2 words with the query).
func TestVectorStore_QuerySimilarityDistinguishesExactFromNoisyMatch(t *testing.T) {
	s := NewVectorStore()
	ctx := context.Background()

	exactID := mustPut(t, s, MemoryRecord{Type: MemoryTypeKnowledge, Content: "alpha beta"})
	noisyID := mustPut(t, s, MemoryRecord{Type: MemoryTypeKnowledge, Content: "alpha beta gamma delta epsilon zeta"})

	matches, err := s.Query(ctx, MemoryQuery{Query: "alpha beta"})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("Query() returned %d matches, want 2", len(matches))
	}
	if matches[0].ID != exactID {
		t.Errorf("Query()[0].ID = %q, want %q (exact match ranks above noisy match)", matches[0].ID, exactID)
	}
	if matches[1].ID != noisyID {
		t.Errorf("Query()[1].ID = %q, want %q", matches[1].ID, noisyID)
	}
}

// TestVectorStore_QueryMetadataFiltering is SPEC-0038's "Filtering works"
// testing criterion: Query only returns candidates whose Metadata matches
// every key/value pair in q.Filters.
func TestVectorStore_QueryMetadataFiltering(t *testing.T) {
	s := NewVectorStore()
	ctx := context.Background()

	workID := mustPut(t, s, MemoryRecord{
		Type: MemoryTypeKnowledge, Content: "project status update",
		Metadata: map[string]any{"category": "work"},
	})
	mustPut(t, s, MemoryRecord{
		Type: MemoryTypeKnowledge, Content: "project status update",
		Metadata: map[string]any{"category": "personal"},
	})

	matches, err := s.Query(ctx, MemoryQuery{
		Query:   "project status update",
		Filters: map[string]any{"category": "work"},
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(matches) != 1 || matches[0].ID != workID {
		t.Errorf("Query() with Filters = %+v, want single match %q", matches, workID)
	}

	none, err := s.Query(ctx, MemoryQuery{
		Query:   "project status update",
		Filters: map[string]any{"category": "nonexistent"},
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(none) != 0 {
		t.Errorf("Query() with non-matching Filters returned %d matches, want 0", len(none))
	}
}

// TestVectorStore_ReplaceRecomputesEmbedding verifies Replace re-embeds the
// new Content rather than leaving the old embedding in place.
func TestVectorStore_ReplaceRecomputesEmbedding(t *testing.T) {
	s := NewVectorStore()
	ctx := context.Background()

	localID := mustPut(t, s, MemoryRecord{Type: MemoryTypeKnowledge, Content: "zzz yyy xxx"})

	if err := s.Replace(ctx, localID, MemoryRecord{Type: MemoryTypeKnowledge, Content: "alpha beta"}); err != nil {
		t.Fatalf("Replace() error = %v", err)
	}

	matches, err := s.Query(ctx, MemoryQuery{Query: "alpha beta"})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(matches) != 1 || matches[0].ID != localID {
		t.Errorf("Query() after Replace = %+v, want single match %q with updated embedding", matches, localID)
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

func TestVectorStore_RemoveMissingIsNotFound(t *testing.T) {
	err := NewVectorStore().Remove(context.Background(), "missing")
	if !errors.Is(err, errors.TypeNotFound) {
		t.Errorf("Remove() of missing ID error = %v, want TypeNotFound", err)
	}
}

// TestVectorStore_QueryFiltersExcludeRecordsWithNilMetadata is an edge case
// for metadata filtering: a record with no Metadata at all never matches a
// non-empty Filters, rather than panicking on a nil map read.
func TestVectorStore_QueryFiltersExcludeRecordsWithNilMetadata(t *testing.T) {
	s := NewVectorStore()
	ctx := context.Background()
	mustPut(t, s, MemoryRecord{Type: MemoryTypeKnowledge, Content: "alpha beta"})

	matches, err := s.Query(ctx, MemoryQuery{Query: "alpha beta", Filters: map[string]any{"category": "work"}})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("Query() with Filters against nil Metadata = %+v, want 0 matches", matches)
	}
}

func TestVectorStore_Name(t *testing.T) {
	if got := NewVectorStore().Name(); got != "vector" {
		t.Errorf("Name() = %q, want %q", got, "vector")
	}
}
