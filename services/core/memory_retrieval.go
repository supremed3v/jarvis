// memory_retrieval.go implements SPEC-0041: the Memory Retrieval System.
// MemoryRetriever sits on top of the SPEC-0034 Memory contract and selects
// the memories relevant to one agent execution: it composes a single search
// query out of the user's query, task context, and agent role (SPEC-0041's
// "Retrieve based on" list), reranks the matches Memory.Search returns by
// blending their relevance order with an importance signal, and caps the
// result to a caller-chosen count - SPEC-0041's ranking, filtering, and
// context-limit requirements.
//
// Memory (SPEC-0034) carries no importance field of its own, and SPEC-0042
// (Memory Consolidation Engine, the spec that would own importance scoring
// end to end) is still Planned. Rather than block on it or widen SPEC-0034's
// contract, MemoryRetriever reads importance from MemoryRecord.Metadata
// under the "importance" key - the same free-form-metadata convention
// knowledge_ingestion.go already uses for parser-derived fields - treating a
// missing or non-numeric value as neutral (1.0).
package core

import (
	"context"
	"sort"
	"strings"

	"jarvis-pa/packages/errors"
)

// importanceMetadataKey is the MemoryRecord.Metadata key MemoryRetriever
// reads a record's importance from. See package doc comment.
const importanceMetadataKey = "importance"

// neutralImportance is the importance MemoryRetriever assigns a record whose
// Metadata carries no valid importance value.
const neutralImportance = 1.0

// RetrievalRequest is the input to MemoryRetriever.Retrieve: what to search
// for (Query, required) and the surrounding context to broaden that search
// with (TaskContext, AgentRole), plus how to narrow and bound the result.
type RetrievalRequest struct {
	// Query is the user's query text. Required.
	Query string

	// Type restricts retrieval to one MemoryType. Empty means every type.
	Type MemoryType

	// TaskContext is free text describing the task the retrieving agent is
	// executing (e.g. a task's title and description). Blended into the
	// search text so retrieval favors memories relevant to the current
	// task, not just Query in isolation.
	TaskContext string

	// AgentRole is the retrieving agent's role or name, blended into the
	// search text so retrieval favors memories relevant to that role.
	AgentRole string

	// Filters are exact-match MemoryRecord.Metadata constraints, passed
	// through to the underlying MemoryQuery.Filters unchanged.
	Filters map[string]any

	// Limit caps the number of memories Retrieve returns. Zero means no
	// cap.
	Limit int

	// MinImportance excludes memories whose importance (see package doc
	// comment) is below this value. Zero (the default) excludes nothing.
	MinImportance float64
}

// Validate reports whether req has the minimum fields Retrieve needs: a
// non-empty Query, and a Type that is either unset or one of the known
// MemoryType values. It returns a packages/errors error typed
// TypeInvalidInput, or nil if req is valid.
func (req RetrievalRequest) Validate() error {
	if strings.TrimSpace(req.Query) == "" {
		return errors.New(errors.TypeInvalidInput, "MEMORY_RETRIEVAL_MISSING_QUERY", "core.memoryretrieval",
			"retrieval request is missing a Query")
	}
	if req.Type != "" && !req.Type.IsValid() {
		return errors.New(errors.TypeInvalidInput, "MEMORY_RETRIEVAL_INVALID_TYPE", "core.memoryretrieval",
			"retrieval request has an unknown Type").With("type", string(req.Type))
	}
	return nil
}

// searchText composes the text MemoryRetriever searches with: Query, then
// TaskContext, then AgentRole, joined by spaces, blank parts omitted. This
// is SPEC-0041's "retrieve based on user query, task context, agent role" -
// blending all three into one search broadens recall towards memories tied
// to the current task and role rather than the literal query alone.
func (req RetrievalRequest) searchText() string {
	parts := make([]string, 0, 3)
	for _, part := range []string{req.Query, req.TaskContext, req.AgentRole} {
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, " ")
}

// MemoryRetriever implements SPEC-0041 on top of a Memory. MemoryRetriever
// is safe for concurrent use - it holds no per-retrieval state.
type MemoryRetriever struct {
	memory Memory
}

// NewMemoryRetriever creates a MemoryRetriever that searches memory. memory
// must not be nil.
func NewMemoryRetriever(memory Memory) *MemoryRetriever {
	return &MemoryRetriever{memory: memory}
}

// Retrieve selects the memories relevant to req: it searches memory with
// req's composed search text (uncapped, so reranking sees every candidate),
// filters out any record below req.MinImportance, ranks the survivors by
// relevance combined with importance, and caps the result to req.Limit (0
// meaning no cap).
func (r *MemoryRetriever) Retrieve(ctx context.Context, req RetrievalRequest) ([]MemoryRecord, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	matches, err := r.memory.Search(ctx, MemoryQuery{
		Type:    req.Type,
		Query:   req.searchText(),
		Filters: req.Filters,
	})
	if err != nil {
		return nil, err
	}

	filtered := make([]MemoryRecord, 0, len(matches))
	for _, rec := range matches {
		if importanceOf(rec) < req.MinImportance {
			continue
		}
		filtered = append(filtered, rec)
	}

	ranked := rankByRelevanceAndImportance(filtered)

	if req.Limit > 0 && len(ranked) > req.Limit {
		ranked = ranked[:req.Limit]
	}
	return ranked, nil
}

// importanceOf returns rec's importance: the float64 value of
// rec.Metadata[importanceMetadataKey], or neutralImportance if that key is
// absent or not a float64.
func importanceOf(rec MemoryRecord) float64 {
	v, ok := rec.Metadata[importanceMetadataKey]
	if !ok {
		return neutralImportance
	}
	f, ok := v.(float64)
	if !ok {
		return neutralImportance
	}
	return f
}

// scoredRecord pairs a MemoryRecord with the composite score
// rankByRelevanceAndImportance ranks it by.
type scoredRecord struct {
	rec   MemoryRecord
	score float64
}

// rankByRelevanceAndImportance stable-sorts records by a composite score
// that blends each record's relevance - its position in records, the
// relevance order Memory.Search already encoded - with its importance
// (importanceOf): relevance rank i scores 1/(i+1), multiplied by
// importance, so an equally-relevant record with above-neutral importance
// outranks one with neutral or below, while a very low ranked record
// generally cannot outrank a much more relevant one. The sort is stable, so
// records with an equal composite score keep Memory.Search's original
// relative order.
func rankByRelevanceAndImportance(records []MemoryRecord) []MemoryRecord {
	scored := make([]scoredRecord, len(records))
	for i, rec := range records {
		relevance := 1.0 / float64(i+1)
		scored[i] = scoredRecord{rec: rec, score: relevance * importanceOf(rec)}
	}

	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	ranked := make([]MemoryRecord, len(scored))
	for i, s := range scored {
		ranked[i] = s.rec
	}
	return ranked
}
