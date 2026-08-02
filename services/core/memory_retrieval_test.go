package core

import (
	"context"
	"testing"

	"jarvis-pa/packages/errors"
)

// fakeMemory is a Memory stub whose Search always returns a fixed slice,
// ignoring the composed query text. It isolates MemoryRetriever's own
// reranking, filtering, and limit logic from VectorStore's embedding-based
// scoring, which the end-to-end tests below already exercise.
type fakeMemory struct {
	searchResult []MemoryRecord
	lastQuery    MemoryQuery
}

func (f *fakeMemory) Store(context.Context, MemoryRecord) (string, error) { return "", nil }
func (f *fakeMemory) Retrieve(context.Context, string) (MemoryRecord, error) {
	return MemoryRecord{}, nil
}
func (f *fakeMemory) Search(_ context.Context, q MemoryQuery) ([]MemoryRecord, error) {
	f.lastQuery = q
	return f.searchResult, nil
}
func (f *fakeMemory) Update(context.Context, MemoryRecord) error { return nil }
func (f *fakeMemory) Delete(context.Context, string) error       { return nil }

func TestMemoryRetriever_RetrieveRejectsEmptyQuery(t *testing.T) {
	r := NewMemoryRetriever(&fakeMemory{})
	_, err := r.Retrieve(context.Background(), RetrievalRequest{})
	if !errors.Is(err, errors.TypeInvalidInput) {
		t.Errorf("Retrieve() error = %v, want TypeInvalidInput", err)
	}
}

func TestMemoryRetriever_RetrieveRejectsInvalidType(t *testing.T) {
	r := NewMemoryRetriever(&fakeMemory{})
	_, err := r.Retrieve(context.Background(), RetrievalRequest{Query: "q", Type: "bogus"})
	if !errors.Is(err, errors.TypeInvalidInput) {
		t.Errorf("Retrieve() error = %v, want TypeInvalidInput", err)
	}
}

func TestMemoryRetriever_ComposesSearchTextFromQueryTaskContextAndAgentRole(t *testing.T) {
	fm := &fakeMemory{}
	r := NewMemoryRetriever(fm)

	_, err := r.Retrieve(context.Background(), RetrievalRequest{
		Query:       "wake word status",
		TaskContext: "voice pipeline debugging",
		AgentRole:   "core-agent",
		Type:        MemoryTypeKnowledge,
		Filters:     map[string]any{"path": "voice.go"},
	})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}

	const want = "wake word status voice pipeline debugging core-agent"
	if fm.lastQuery.Query != want {
		t.Errorf("composed search text = %q, want %q", fm.lastQuery.Query, want)
	}
	if fm.lastQuery.Type != MemoryTypeKnowledge {
		t.Errorf("Search() Type = %q, want %q", fm.lastQuery.Type, MemoryTypeKnowledge)
	}
	if fm.lastQuery.Filters["path"] != "voice.go" {
		t.Errorf("Search() Filters = %v, want path=voice.go", fm.lastQuery.Filters)
	}
	if fm.lastQuery.Limit != 0 {
		t.Errorf("Search() Limit = %d, want 0 (Retrieve fetches uncapped, then ranks and cuts itself)", fm.lastQuery.Limit)
	}
}

func TestMemoryRetriever_RanksByImportanceAboveRelevanceOrderAlone(t *testing.T) {
	fm := &fakeMemory{searchResult: []MemoryRecord{
		{ID: "a", Type: MemoryTypeKnowledge, Content: "x"},
		{ID: "b", Type: MemoryTypeKnowledge, Content: "y", Metadata: map[string]any{"importance": 5.0}},
	}}
	r := NewMemoryRetriever(fm)

	got, err := r.Retrieve(context.Background(), RetrievalRequest{Query: "q"})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != "b" || got[1].ID != "a" {
		t.Errorf("Retrieve() order = %v, want [b a] (b's higher importance should outrank a's better relevance rank)", ids(got))
	}
}

func TestMemoryRetriever_MinImportanceFiltersLowImportanceMemories(t *testing.T) {
	fm := &fakeMemory{searchResult: []MemoryRecord{
		{ID: "a", Type: MemoryTypeKnowledge, Content: "x"},
		{ID: "b", Type: MemoryTypeKnowledge, Content: "y", Metadata: map[string]any{"importance": 0.2}},
	}}
	r := NewMemoryRetriever(fm)

	got, err := r.Retrieve(context.Background(), RetrievalRequest{Query: "q", MinImportance: 0.5})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "a" {
		t.Errorf("Retrieve() = %v, want only [a] (b's importance 0.2 is below MinImportance 0.5)", ids(got))
	}
}

func TestMemoryRetriever_RetrieveRespectsLimit(t *testing.T) {
	fm := &fakeMemory{searchResult: []MemoryRecord{
		{ID: "a", Type: MemoryTypeKnowledge, Content: "x"},
		{ID: "b", Type: MemoryTypeKnowledge, Content: "y"},
		{ID: "c", Type: MemoryTypeKnowledge, Content: "z"},
	}}
	r := NewMemoryRetriever(fm)

	got, err := r.Retrieve(context.Background(), RetrievalRequest{Query: "q", Limit: 2})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Retrieve() returned %d records, want 2 (Limit)", len(got))
	}
	if got[0].ID != "a" || got[1].ID != "b" {
		t.Errorf("Retrieve() = %v, want top 2 by relevance order [a b]", ids(got))
	}
}

// ids collects the IDs of records, for readable test-failure messages.
func ids(records []MemoryRecord) []string {
	out := make([]string, len(records))
	for i, r := range records {
		out[i] = r.ID
	}
	return out
}

// The following tests exercise MemoryRetriever end to end against a real
// StorageMemory backed by a VectorStore, matching SPEC-0041's Testing
// section: relevant memories are returned, irrelevant memories are
// filtered, and retrieval respects limits.

func newTestRetriever() (*MemoryRetriever, Memory) {
	mem := NewStorageMemory(NewVectorStore())
	return NewMemoryRetriever(mem), mem
}

func TestMemoryRetriever_ReturnsRelevantMemoriesAndFiltersIrrelevantOnes(t *testing.T) {
	r, mem := newTestRetriever()
	ctx := context.Background()

	relevantID := mustStore(t, mem, MemoryRecord{
		Type: MemoryTypeKnowledge, Content: "the wake word detector listens for jarvis on the microphone",
	})
	mustStore(t, mem, MemoryRecord{
		Type: MemoryTypeKnowledge, Content: "quarterly finance spreadsheet totals for the budget report",
	})

	got, err := r.Retrieve(ctx, RetrievalRequest{Query: "wake word detector microphone"})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != relevantID {
		t.Errorf("Retrieve() = %v, want only the relevant record %q (irrelevant record must be filtered out)", ids(got), relevantID)
	}
}

func TestMemoryRetriever_BlendsTaskContextIntoRetrieval(t *testing.T) {
	r, mem := newTestRetriever()
	ctx := context.Background()

	recID := mustStore(t, mem, MemoryRecord{
		Type: MemoryTypeKnowledge, Content: "wake word detector currently offline pending firmware fix",
	})

	// Query alone shares no words with the stored record, so it is filtered
	// out at the VectorStore layer (zero similarity).
	withoutContext, err := r.Retrieve(ctx, RetrievalRequest{Query: "status update"})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(withoutContext) != 0 {
		t.Fatalf("Retrieve() without TaskContext = %v, want none (query shares no words with the record)", ids(withoutContext))
	}

	// Blending in TaskContext gives the search text words the record shares,
	// so it is now returned.
	withContext, err := r.Retrieve(ctx, RetrievalRequest{
		Query:       "status update",
		TaskContext: "wake word detector",
	})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(withContext) != 1 || withContext[0].ID != recID {
		t.Errorf("Retrieve() with TaskContext = %v, want only %q", ids(withContext), recID)
	}
}

func TestMemoryRetriever_RetrieveRespectsLimitEndToEnd(t *testing.T) {
	r, mem := newTestRetriever()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		mustStore(t, mem, MemoryRecord{Type: MemoryTypeKnowledge, Content: "jarvis feature note"})
	}

	got, err := r.Retrieve(ctx, RetrievalRequest{Query: "jarvis feature note", Limit: 2})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(got) != 2 {
		t.Errorf("Retrieve() returned %d records, want 2 (Limit)", len(got))
	}
}

// mustStore is a test helper that Stores rec via mem and fails the test on
// error, returning the assigned ID.
func mustStore(t *testing.T, mem Memory, rec MemoryRecord) string {
	t.Helper()
	id, err := mem.Store(context.Background(), rec)
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	return id
}
