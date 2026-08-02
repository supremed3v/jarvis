package core

import (
	"context"
	"testing"
	"time"

	"jarvis-pa/packages/errors"
)

// spyMemory is a Memory stub that serves Retrieve/Update/Delete out of an
// in-memory map and records every Update/Delete call, so tests can assert on
// exactly what ConsolidationEngine persisted or removed without depending on
// a real storage backend's own logic.
type spyMemory struct {
	records map[string]MemoryRecord
	updated []MemoryRecord
	deleted []string
}

func newSpyMemory(records ...MemoryRecord) *spyMemory {
	m := &spyMemory{records: make(map[string]MemoryRecord)}
	for _, rec := range records {
		m.records[rec.ID] = rec
	}
	return m
}

func (m *spyMemory) Store(context.Context, MemoryRecord) (string, error) { return "", nil }

func (m *spyMemory) Retrieve(_ context.Context, id string) (MemoryRecord, error) {
	rec, ok := m.records[id]
	if !ok {
		return MemoryRecord{}, errors.New(errors.TypeNotFound, "SPY_MEMORY_NOT_FOUND", "core.test", "no record").With("id", id)
	}
	return rec, nil
}

func (m *spyMemory) Search(context.Context, MemoryQuery) ([]MemoryRecord, error) { return nil, nil }

func (m *spyMemory) Update(_ context.Context, rec MemoryRecord) error {
	if _, ok := m.records[rec.ID]; !ok {
		return errors.New(errors.TypeNotFound, "SPY_MEMORY_NOT_FOUND", "core.test", "no record").With("id", rec.ID)
	}
	m.records[rec.ID] = rec
	m.updated = append(m.updated, rec)
	return nil
}

func (m *spyMemory) Delete(_ context.Context, id string) error {
	if _, ok := m.records[id]; !ok {
		return errors.New(errors.TypeNotFound, "SPY_MEMORY_NOT_FOUND", "core.test", "no record").With("id", id)
	}
	delete(m.records, id)
	m.deleted = append(m.deleted, id)
	return nil
}

func resultFor(results []ConsolidationResult, id string) (ConsolidationResult, bool) {
	for _, r := range results {
		if r.RecordID == id {
			return r, true
		}
	}
	return ConsolidationResult{}, false
}

func TestConsolidationEngine_RejectsRecordsMissingID(t *testing.T) {
	mem := newSpyMemory()
	c := NewConsolidationEngine(mem)

	_, err := c.Consolidate(context.Background(), []MemoryRecord{{Type: MemoryTypeKnowledge, Content: "no id"}})
	if !errors.Is(err, errors.TypeInvalidInput) {
		t.Errorf("Consolidate() error = %v, want TypeInvalidInput", err)
	}
}

// Testing requirement 1: important memories are preserved.
func TestConsolidationEngine_ImportantMemoriesArePreserved(t *testing.T) {
	old := time.Now().Add(-365 * 24 * time.Hour)
	rec := MemoryRecord{
		ID: "pinned", Type: MemoryTypeUserProfile, Content: "short",
		Metadata: map[string]any{pinnedMetadataKey: true}, CreatedAt: old,
	}
	mem := newSpyMemory(rec)
	c := NewConsolidationEngine(mem)

	results, err := c.Consolidate(context.Background(), []MemoryRecord{rec})
	if err != nil {
		t.Fatalf("Consolidate() error = %v", err)
	}

	res, ok := resultFor(results, "pinned")
	if !ok || res.Action != ActionKept {
		t.Fatalf("Consolidate() result for pinned = %+v, want Action=%q", res, ActionKept)
	}
	if res.Importance != pinnedImportance {
		t.Errorf("Importance = %v, want %v (pinned override)", res.Importance, pinnedImportance)
	}
	if len(mem.deleted) != 0 {
		t.Errorf("Delete() called for %v, want none - pinned record must survive despite its age", mem.deleted)
	}
	stored := mem.records["pinned"]
	if stored.Metadata[importanceMetadataKey] != pinnedImportance {
		t.Errorf("stored importance = %v, want %v persisted via Update", stored.Metadata[importanceMetadataKey], pinnedImportance)
	}
}

// Testing requirement 2: duplicates are merged.
func TestConsolidationEngine_DuplicatesAreMerged(t *testing.T) {
	shared := "the wake word detector currently listens for the word jarvis on the primary microphone input"
	older := MemoryRecord{ID: "older", Type: MemoryTypeKnowledge, Content: shared, CreatedAt: time.Now().Add(-time.Hour)}
	newer := MemoryRecord{ID: "newer", Type: MemoryTypeKnowledge, Content: shared, CreatedAt: time.Now()}
	mem := newSpyMemory(older, newer)
	c := NewConsolidationEngine(mem)

	results, err := c.Consolidate(context.Background(), []MemoryRecord{older, newer})
	if err != nil {
		t.Fatalf("Consolidate() error = %v", err)
	}

	// Equal importance (identical content) ties break to the earlier
	// CreatedAt, so "older" survives and "newer" is merged into it.
	survivorRes, ok := resultFor(results, "older")
	if !ok || survivorRes.Action != ActionKept {
		t.Fatalf("result for older = %+v, want Action=%q", survivorRes, ActionKept)
	}
	dupRes, ok := resultFor(results, "newer")
	if !ok || dupRes.Action != ActionMerged || dupRes.MergedInto != "older" {
		t.Fatalf("result for newer = %+v, want Action=%q MergedInto=older", dupRes, ActionMerged)
	}

	if len(mem.deleted) != 1 || mem.deleted[0] != "newer" {
		t.Errorf("deleted = %v, want [newer]", mem.deleted)
	}
	if _, stillThere := mem.records["newer"]; stillThere {
		t.Error("newer record still present after merge, want deleted")
	}
	survivor, ok := mem.records["older"]
	if !ok {
		t.Fatal("older record missing after merge, want it to survive")
	}
	if count, _ := survivor.Metadata[consolidatedCountMetaKey].(int); count != 2 {
		t.Errorf("survivor consolidatedCount = %v, want 2 (itself + the merged duplicate)", survivor.Metadata[consolidatedCountMetaKey])
	}
}

func TestConsolidationEngine_DuplicateDetectionIsScopedToSameMemoryType(t *testing.T) {
	shared := "the wake word detector currently listens for the word jarvis on the primary microphone input"
	a := MemoryRecord{ID: "a", Type: MemoryTypeKnowledge, Content: shared, CreatedAt: time.Now()}
	b := MemoryRecord{ID: "b", Type: MemoryTypeExperience, Content: shared, CreatedAt: time.Now()}
	mem := newSpyMemory(a, b)
	c := NewConsolidationEngine(mem)

	results, err := c.Consolidate(context.Background(), []MemoryRecord{a, b})
	if err != nil {
		t.Fatalf("Consolidate() error = %v", err)
	}

	for _, id := range []string{"a", "b"} {
		res, ok := resultFor(results, id)
		if !ok || res.Action != ActionKept {
			t.Errorf("result for %s = %+v, want Action=%q (different MemoryTypes must not be merged)", id, res, ActionKept)
		}
	}
	if len(mem.deleted) != 0 {
		t.Errorf("deleted = %v, want none", mem.deleted)
	}
}

// Testing requirement 3: low-value memories are ignored.
func TestConsolidationEngine_LowValueMemoriesAreIgnored(t *testing.T) {
	rec := MemoryRecord{ID: "trivial", Type: MemoryTypeConversation, Content: "ok", CreatedAt: time.Now()}
	mem := newSpyMemory(rec)
	c := NewConsolidationEngine(mem)

	results, err := c.Consolidate(context.Background(), []MemoryRecord{rec})
	if err != nil {
		t.Fatalf("Consolidate() error = %v", err)
	}

	res, ok := resultFor(results, "trivial")
	if !ok || res.Action != ActionIgnored {
		t.Fatalf("result for trivial = %+v, want Action=%q", res, ActionIgnored)
	}
	if len(mem.updated) != 0 {
		t.Errorf("Update() called for %v, want none - a low-value, not-yet-expirable record should be left untouched", mem.updated)
	}
	if len(mem.deleted) != 0 {
		t.Errorf("Delete() called for %v, want none - it is not old enough to expire yet", mem.deleted)
	}
}

func TestConsolidationEngine_LowValueRecordsExpireOnceStale(t *testing.T) {
	old := MemoryRecord{
		ID: "stale", Type: MemoryTypeConversation, Content: "ok",
		CreatedAt: time.Now().Add(-100 * 24 * time.Hour),
	}
	mem := newSpyMemory(old)
	c := NewConsolidationEngine(mem, WithExpireAfter(90*24*time.Hour))

	results, err := c.Consolidate(context.Background(), []MemoryRecord{old})
	if err != nil {
		t.Fatalf("Consolidate() error = %v", err)
	}

	res, ok := resultFor(results, "stale")
	if !ok || res.Action != ActionExpired {
		t.Fatalf("result for stale = %+v, want Action=%q", res, ActionExpired)
	}
	if len(mem.deleted) != 1 || mem.deleted[0] != "stale" {
		t.Errorf("deleted = %v, want [stale]", mem.deleted)
	}
}

func TestConsolidationEngine_UsesInjectedClockForExpiration(t *testing.T) {
	rec := MemoryRecord{ID: "trivial", Type: MemoryTypeConversation, Content: "ok", CreatedAt: time.Now()}
	mem := newSpyMemory(rec)
	future := time.Now().Add(200 * 24 * time.Hour)
	c := NewConsolidationEngine(mem,
		WithExpireAfter(90*24*time.Hour),
		WithConsolidationClock(func() time.Time { return future }),
	)

	results, err := c.Consolidate(context.Background(), []MemoryRecord{rec})
	if err != nil {
		t.Fatalf("Consolidate() error = %v", err)
	}

	res, ok := resultFor(results, "trivial")
	if !ok || res.Action != ActionExpired {
		t.Fatalf("result for trivial = %+v, want Action=%q (clock advanced past ExpireAfter)", res, ActionExpired)
	}
}

func TestConsolidationEngine_SubstantiveContentIsKeptWithoutPinning(t *testing.T) {
	rec := MemoryRecord{
		ID: "rich", Type: MemoryTypeKnowledge, CreatedAt: time.Now(),
		Content: "the jarvis event bus dispatches typed events to every subscribed handler in registration order",
	}
	mem := newSpyMemory(rec)
	c := NewConsolidationEngine(mem)

	results, err := c.Consolidate(context.Background(), []MemoryRecord{rec})
	if err != nil {
		t.Fatalf("Consolidate() error = %v", err)
	}

	res, ok := resultFor(results, "rich")
	if !ok || res.Action != ActionKept {
		t.Fatalf("result for rich = %+v, want Action=%q", res, ActionKept)
	}
	if len(mem.updated) != 1 {
		t.Errorf("Update() called %d times, want 1", len(mem.updated))
	}
}

// stubScorer is an ImportanceScorer that returns a fixed score regardless of
// content, isolating WithImportanceScorer's wiring from
// DefaultImportanceScorer's own scoring logic (already covered above).
type stubScorer struct{ score float64 }

func (s stubScorer) Score(MemoryRecord) float64 { return s.score }

func TestConsolidationEngine_WithImportanceScorerOverridesDefault(t *testing.T) {
	rec := MemoryRecord{ID: "a", Type: MemoryTypeKnowledge, Content: "irrelevant to a stub scorer", CreatedAt: time.Now()}
	mem := newSpyMemory(rec)
	c := NewConsolidationEngine(mem, WithImportanceScorer(stubScorer{score: 3.5}))

	results, err := c.Consolidate(context.Background(), []MemoryRecord{rec})
	if err != nil {
		t.Fatalf("Consolidate() error = %v", err)
	}

	res, ok := resultFor(results, "a")
	if !ok || res.Importance != 3.5 {
		t.Fatalf("result = %+v, want Importance=3.5 from the injected ImportanceScorer", res)
	}
}

func TestConsolidationEngine_WithMinImportanceRaisesTheIgnoreBar(t *testing.T) {
	// 6 words scores 0.5+0.6=1.1 under DefaultImportanceScorer - above the
	// package default MinImportance (1.0) but, with a raised MinImportance,
	// below it.
	rec := MemoryRecord{ID: "a", Type: MemoryTypeKnowledge, Content: "six distinct words right here now", CreatedAt: time.Now()}
	mem := newSpyMemory(rec)
	c := NewConsolidationEngine(mem, WithMinImportance(2.0))

	results, err := c.Consolidate(context.Background(), []MemoryRecord{rec})
	if err != nil {
		t.Fatalf("Consolidate() error = %v", err)
	}

	res, ok := resultFor(results, "a")
	if !ok || res.Action != ActionIgnored {
		t.Fatalf("result = %+v, want Action=%q under a raised MinImportance", res, ActionIgnored)
	}
}

func TestConsolidationEngine_WithDuplicateThresholdControlsMerging(t *testing.T) {
	a := MemoryRecord{ID: "a", Type: MemoryTypeKnowledge, Content: "jarvis wake word detector notes", CreatedAt: time.Now()}
	b := MemoryRecord{ID: "b", Type: MemoryTypeKnowledge, Content: "jarvis wake word detector configuration", CreatedAt: time.Now()}
	mem := newSpyMemory(a, b)

	// A threshold of 0 makes any shared vocabulary at all count as a
	// duplicate, so these partially-overlapping records must merge.
	c := NewConsolidationEngine(mem, WithDuplicateThreshold(0))

	results, err := c.Consolidate(context.Background(), []MemoryRecord{a, b})
	if err != nil {
		t.Fatalf("Consolidate() error = %v", err)
	}

	mergedCount := 0
	for _, r := range results {
		if r.Action == ActionMerged {
			mergedCount++
		}
	}
	if mergedCount != 1 {
		t.Fatalf("results = %+v, want exactly one ActionMerged with DuplicateThreshold=0", results)
	}
}

// constantEmbedder is an Embedder that returns the same vector for any text,
// isolating WithConsolidationEmbedder's wiring from HashEmbedder's own
// hashing logic (covered by memory_embedding_test.go).
type constantEmbedder struct{ vec []float64 }

func (e constantEmbedder) Embed(string) []float64 { return e.vec }

func TestConsolidationEngine_WithConsolidationEmbedderOverridesDefault(t *testing.T) {
	// Two records whose Content shares no words at all - the default
	// HashEmbedder would score them as dissimilar - but a constant Embedder
	// makes every record identical in embedding space, forcing a merge.
	a := MemoryRecord{ID: "a", Type: MemoryTypeKnowledge, Content: "alpha", CreatedAt: time.Now()}
	b := MemoryRecord{ID: "b", Type: MemoryTypeKnowledge, Content: "omega", CreatedAt: time.Now()}
	mem := newSpyMemory(a, b)
	c := NewConsolidationEngine(mem, WithConsolidationEmbedder(constantEmbedder{vec: []float64{1, 0, 0}}))

	results, err := c.Consolidate(context.Background(), []MemoryRecord{a, b})
	if err != nil {
		t.Fatalf("Consolidate() error = %v", err)
	}

	mergedCount := 0
	for _, r := range results {
		if r.Action == ActionMerged {
			mergedCount++
		}
	}
	if mergedCount != 1 {
		t.Fatalf("results = %+v, want exactly one ActionMerged with a constant Embedder", results)
	}
}

func TestDefaultImportanceScorer_PinnedOverridesLength(t *testing.T) {
	s := DefaultImportanceScorer{}
	got := s.Score(MemoryRecord{Content: "irrelevant length here", Metadata: map[string]any{pinnedMetadataKey: true}})
	if got != pinnedImportance {
		t.Errorf("Score() = %v, want %v", got, pinnedImportance)
	}
}

func TestDefaultImportanceScorer_GrowsWithContentLengthUpToCap(t *testing.T) {
	s := DefaultImportanceScorer{}

	short := s.Score(MemoryRecord{Content: "hi"})
	long := s.Score(MemoryRecord{Content: "this sentence has quite a few more distinct words in it than the short one"})
	if !(long > short) {
		t.Errorf("long content score %v should exceed short content score %v", long, short)
	}

	huge := make([]byte, 0, 1000)
	for i := 0; i < 200; i++ {
		huge = append(huge, []byte("word ")...)
	}
	capped := s.Score(MemoryRecord{Content: string(huge)})
	if capped != maxScoredImportance {
		t.Errorf("Score() for very long content = %v, want capped at %v", capped, maxScoredImportance)
	}
}

// End-to-end: ConsolidationEngine against a real StorageMemory backed by a
// VectorStore, exercising the full duplicate-detection path (real embeddings
// and cosine similarity) rather than a hand-picked threshold comparison.
func TestConsolidationEngine_EndToEndAgainstStorageMemory(t *testing.T) {
	mem := NewStorageMemory(NewVectorStore())
	ctx := context.Background()

	shared := "jarvis wake word detector configuration and calibration notes for the microphone array"
	id1 := mustStore(t, mem, MemoryRecord{Type: MemoryTypeKnowledge, Content: shared})
	id2 := mustStore(t, mem, MemoryRecord{Type: MemoryTypeKnowledge, Content: shared})
	unrelatedID := mustStore(t, mem, MemoryRecord{
		Type: MemoryTypeKnowledge,
		Content: "a completely unrelated substantive memory about quarterly budget planning " +
			"and expense tracking across every department",
	})

	rec1, err := mem.Retrieve(ctx, id1)
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	rec2, err := mem.Retrieve(ctx, id2)
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	unrelated, err := mem.Retrieve(ctx, unrelatedID)
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}

	c := NewConsolidationEngine(mem)
	results, err := c.Consolidate(ctx, []MemoryRecord{rec1, rec2, unrelated})
	if err != nil {
		t.Fatalf("Consolidate() error = %v", err)
	}

	mergedCount, keptCount := 0, 0
	for _, r := range results {
		switch r.Action {
		case ActionMerged:
			mergedCount++
		case ActionKept:
			keptCount++
		}
	}
	if mergedCount != 1 || keptCount != 2 {
		t.Errorf("results = %+v, want exactly one Merged and two Kept", results)
	}

	if _, err := mem.Retrieve(ctx, unrelatedID); err != nil {
		t.Errorf("unrelated record should still exist, Retrieve() error = %v", err)
	}
}
