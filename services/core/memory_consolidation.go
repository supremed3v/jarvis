// memory_consolidation.go implements SPEC-0042: the Memory Consolidation
// Engine, the process that decides what stored memory becomes (or remains)
// long-term. Consolidate takes a caller-supplied batch of MemoryRecords
// (typically ones a scheduler pulled back out of a Memory) and, per SPEC-0042's
// four requirements: scores each record's importance (ImportanceScorer),
// groups and merges same-Type near-duplicate content (embedding cosine
// similarity, reusing memory_embedding.go's Embedder/cosineSimilarity),
// persists the resulting importance back to Memory via Update - populating
// the MemoryRecord.Metadata["importance"] convention MemoryRetriever
// (SPEC-0041) reads but never itself writes - and expires (deletes) stale,
// low-importance records while leaving stale-but-important ones untouched.
//
// Memory (SPEC-0034) has no "list all records" operation - MemoryQuery.Query
// must be non-empty - so Consolidate cannot fetch its own working set; it
// operates purely on the records its caller passes in, which is why every
// input record must already carry the ID Memory.Store/Search assigned it.
package core

import (
	"context"
	"sort"
	"strings"
	"time"

	"jarvis-pa/packages/errors"
)

const (
	// defaultDuplicateSimilarityThreshold is the cosine similarity, at or
	// above which two same-Type records are considered duplicates of one
	// another.
	defaultDuplicateSimilarityThreshold = 0.85

	// defaultMinImportance is the importance below which a record is
	// low-value: ActionIgnored (if not yet expirable) or ActionExpired (if
	// past ExpireAfter).
	defaultMinImportance = 1.0

	// defaultExpireAfter is how old a low-importance record must be before
	// Consolidate deletes it.
	defaultExpireAfter = 90 * 24 * time.Hour

	// baseImportance, perWordImportance, and maxScoredImportance shape
	// DefaultImportanceScorer's length-based score: baseImportance at zero
	// words, growing by perWordImportance per word, capped at
	// maxScoredImportance.
	baseImportance      = 0.5
	perWordImportance   = 0.1
	maxScoredImportance = 5.0

	// pinnedImportance is the fixed score DefaultImportanceScorer gives a
	// record explicitly flagged Metadata[pinnedMetadataKey] == true,
	// overriding the length-based score entirely.
	pinnedImportance = 5.0

	// reinforcementPerDuplicate is the importance bonus (capped at
	// maxScoredImportance) Consolidate adds to a merge survivor's score per
	// duplicate merged into it - seeing the same thing repeated is itself a
	// signal the memory matters.
	reinforcementPerDuplicate = 0.25

	// pinnedMetadataKey is the MemoryRecord.Metadata key
	// DefaultImportanceScorer treats as an explicit "always important"
	// override.
	pinnedMetadataKey = "pinned"

	// consolidatedCountMetaKey is the MemoryRecord.Metadata key Consolidate
	// uses to track how many records have been merged into a survivor over
	// its lifetime (starting at 1, for itself).
	consolidatedCountMetaKey = "consolidatedCount"
)

// ImportanceScorer assigns an importance score to a MemoryRecord - the
// signal Consolidate uses to decide whether a record is preserved,
// reinforced, or eventually expired. Higher means more important.
type ImportanceScorer interface {
	Score(rec MemoryRecord) float64
}

// DefaultImportanceScorer is a dependency-free ImportanceScorer: a record
// explicitly flagged Metadata[pinnedMetadataKey] == true always scores
// pinnedImportance; otherwise importance grows with content length, since
// more substantive content is more likely worth keeping, from
// baseImportance at zero words up to maxScoredImportance.
type DefaultImportanceScorer struct{}

// Score implements ImportanceScorer.
func (DefaultImportanceScorer) Score(rec MemoryRecord) float64 {
	if pinned, ok := rec.Metadata[pinnedMetadataKey].(bool); ok && pinned {
		return pinnedImportance
	}
	words := len(strings.Fields(rec.Content))
	score := baseImportance + perWordImportance*float64(words)
	if score > maxScoredImportance {
		score = maxScoredImportance
	}
	return score
}

// ConsolidationAction reports what Consolidate did with one input record.
type ConsolidationAction string

const (
	// ActionKept means the record (a duplicate-free record, or a merge
	// survivor) met MinImportance and had its scored importance persisted.
	ActionKept ConsolidationAction = "kept"
	// ActionMerged means the record was a same-Type near-duplicate of
	// another, higher-priority record in the same batch, and was deleted;
	// ConsolidationResult.MergedInto names the surviving record.
	ActionMerged ConsolidationAction = "merged"
	// ActionExpired means the record scored below MinImportance and was
	// older than ExpireAfter, and was deleted.
	ActionExpired ConsolidationAction = "expired"
	// ActionIgnored means the record scored below MinImportance but was not
	// yet old enough to expire, and was left untouched.
	ActionIgnored ConsolidationAction = "ignored"
)

// ConsolidationResult reports what Consolidate did with one input record.
type ConsolidationResult struct {
	RecordID   string
	Action     ConsolidationAction
	Importance float64
	// MergedInto is the surviving record's ID. Set only when Action is
	// ActionMerged.
	MergedInto string
}

// ConsolidationEngine implements SPEC-0042 on top of a Memory: importance
// scoring, duplicate detection and merging, persisting memory updates, and
// expiration.
type ConsolidationEngine struct {
	memory        Memory
	embedder      Embedder
	scorer        ImportanceScorer
	dupThreshold  float64
	minImportance float64
	expireAfter   time.Duration
	now           func() time.Time
}

// ConsolidationEngineOption configures a ConsolidationEngine created by
// NewConsolidationEngine.
type ConsolidationEngineOption func(*ConsolidationEngine)

// WithConsolidationEmbedder overrides the Embedder used for duplicate
// detection. The default is a HashEmbedder. Named distinctly from
// VectorStore's and EmbeddingPipeline's embedder options (different option
// types) to avoid a same-package naming collision.
func WithConsolidationEmbedder(e Embedder) ConsolidationEngineOption {
	return func(c *ConsolidationEngine) { c.embedder = e }
}

// WithImportanceScorer overrides the ImportanceScorer used to score records.
// The default is DefaultImportanceScorer.
func WithImportanceScorer(s ImportanceScorer) ConsolidationEngineOption {
	return func(c *ConsolidationEngine) { c.scorer = s }
}

// WithDuplicateThreshold overrides the cosine similarity at or above which
// two same-Type records are treated as duplicates. The default is
// defaultDuplicateSimilarityThreshold.
func WithDuplicateThreshold(threshold float64) ConsolidationEngineOption {
	return func(c *ConsolidationEngine) { c.dupThreshold = threshold }
}

// WithMinImportance overrides the importance below which a record is
// low-value (ActionIgnored or ActionExpired). The default is
// defaultMinImportance.
func WithMinImportance(min float64) ConsolidationEngineOption {
	return func(c *ConsolidationEngine) { c.minImportance = min }
}

// WithExpireAfter overrides how old a low-importance record must be before
// Consolidate deletes it. The default is defaultExpireAfter.
func WithExpireAfter(d time.Duration) ConsolidationEngineOption {
	return func(c *ConsolidationEngine) { c.expireAfter = d }
}

// WithConsolidationClock overrides the clock Consolidate uses to evaluate a
// record's age against ExpireAfter. The default is time.Now; tests use this
// to simulate the passage of time without sleeping.
func WithConsolidationClock(now func() time.Time) ConsolidationEngineOption {
	return func(c *ConsolidationEngine) { c.now = now }
}

// NewConsolidationEngine creates a ConsolidationEngine backed by memory,
// defaulting to a HashEmbedder, a DefaultImportanceScorer,
// defaultDuplicateSimilarityThreshold, defaultMinImportance,
// defaultExpireAfter, and time.Now, unless overridden. memory must not be
// nil.
func NewConsolidationEngine(memory Memory, opts ...ConsolidationEngineOption) *ConsolidationEngine {
	c := &ConsolidationEngine{
		memory:        memory,
		embedder:      NewHashEmbedder(),
		scorer:        DefaultImportanceScorer{},
		dupThreshold:  defaultDuplicateSimilarityThreshold,
		minImportance: defaultMinImportance,
		expireAfter:   defaultExpireAfter,
		now:           time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Consolidate decides what becomes of each of records. It first groups
// records into same-Type near-duplicate clusters (see groupDuplicates) and,
// for every cluster with more than one member, deletes every member but the
// most important survivor (ActionMerged). It then scores each survivor's
// (or duplicate-free record's) importance, boosted per duplicate merged into
// it: records at or above MinImportance have that importance persisted to
// Memory via Update (ActionKept); records below MinImportance are deleted if
// older than ExpireAfter (ActionExpired) or otherwise left untouched
// (ActionIgnored). records must each have a non-empty ID, as returned by a
// prior Memory.Store or Search call; Consolidate returns a TypeInvalidInput
// error otherwise.
func (c *ConsolidationEngine) Consolidate(ctx context.Context, records []MemoryRecord) ([]ConsolidationResult, error) {
	for _, rec := range records {
		if rec.ID == "" {
			return nil, errors.New(errors.TypeInvalidInput, "CONSOLIDATION_RECORD_MISSING_ID", "core.memoryconsolidation",
				"consolidation input record is missing an ID")
		}
	}

	groups := c.groupDuplicates(records)
	now := c.now()

	results := make([]ConsolidationResult, 0, len(records))
	for _, group := range groups {
		survivor, duplicates := group[0], group[1:]

		for _, dup := range duplicates {
			if err := c.memory.Delete(ctx, dup.ID); err != nil {
				return nil, err
			}
			results = append(results, ConsolidationResult{
				RecordID:   dup.ID,
				Action:     ActionMerged,
				MergedInto: survivor.ID,
			})
		}

		importance := c.scorer.Score(survivor)
		if len(duplicates) > 0 {
			importance += reinforcementPerDuplicate * float64(len(duplicates))
			if importance > maxScoredImportance {
				importance = maxScoredImportance
			}
		}

		if importance < c.minImportance {
			if now.Sub(survivor.CreatedAt) >= c.expireAfter {
				if err := c.memory.Delete(ctx, survivor.ID); err != nil {
					return nil, err
				}
				results = append(results, ConsolidationResult{RecordID: survivor.ID, Action: ActionExpired, Importance: importance})
				continue
			}
			results = append(results, ConsolidationResult{RecordID: survivor.ID, Action: ActionIgnored, Importance: importance})
			continue
		}

		survivor.Metadata = mergedMetadata(survivor.Metadata, importance, len(group))
		if err := c.memory.Update(ctx, survivor); err != nil {
			return nil, err
		}
		results = append(results, ConsolidationResult{RecordID: survivor.ID, Action: ActionKept, Importance: importance})
	}

	return results, nil
}

// mergedMetadata returns a copy of meta with importanceMetadataKey set to
// importance and consolidatedCountMetaKey incremented by groupSize-1 (its
// prior value, defaulting to 1, plus the duplicates merged in this round).
func mergedMetadata(meta map[string]any, importance float64, groupSize int) map[string]any {
	merged := make(map[string]any, len(meta)+2)
	for k, v := range meta {
		merged[k] = v
	}
	merged[importanceMetadataKey] = importance

	count, ok := merged[consolidatedCountMetaKey].(int)
	if !ok || count < 1 {
		count = 1
	}
	if groupSize > 1 {
		count += groupSize - 1
	}
	merged[consolidatedCountMetaKey] = count

	return merged
}

// groupDuplicates partitions records into duplicate clusters: starting from
// each not-yet-assigned record, every later, not-yet-assigned record of the
// same MemoryType whose content embedding has cosine similarity at or above
// dupThreshold joins its cluster. (Similarity is checked against the
// cluster's anchor only, not transitively against every member already
// assigned, which is a deliberate simplification - full transitive
// clustering is not needed at this scope.) Each returned group is ordered by
// descending importance (ties broken by earliest CreatedAt), so element 0 is
// always the merge survivor. Records with no duplicate come back as
// singleton groups.
func (c *ConsolidationEngine) groupDuplicates(records []MemoryRecord) [][]MemoryRecord {
	embeddings := make([][]float64, len(records))
	for i, rec := range records {
		embeddings[i] = c.embedder.Embed(rec.Content)
	}

	assigned := make([]bool, len(records))
	var groups [][]MemoryRecord
	for i := range records {
		if assigned[i] {
			continue
		}
		assigned[i] = true
		group := []MemoryRecord{records[i]}

		for j := i + 1; j < len(records); j++ {
			if assigned[j] || records[j].Type != records[i].Type {
				continue
			}
			if cosineSimilarity(embeddings[i], embeddings[j]) >= c.dupThreshold {
				assigned[j] = true
				group = append(group, records[j])
			}
		}

		sort.SliceStable(group, func(a, b int) bool {
			sa, sb := c.scorer.Score(group[a]), c.scorer.Score(group[b])
			if sa != sb {
				return sa > sb
			}
			return group[a].CreatedAt.Before(group[b].CreatedAt)
		})
		groups = append(groups, group)
	}
	return groups
}
