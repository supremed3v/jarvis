// memory_storage_vector.go implements VectorStore, the SPEC-0035 "Vector
// storage" MemoryStorageProvider, upgraded per SPEC-0038 (Vector Memory
// Engine) to real embedding-based similarity: every record is embedded via
// an Embedder (memory_embedding.go) on write, and Query ranks candidates by
// cosine similarity between the query's embedding and each candidate's,
// with metadata filtering applied first. Real model-backed embedding
// generation is SPEC-0039 (Embedding Pipeline)'s job - VectorStore accepts
// any Embedder, defaulting to the dependency-free HashEmbedder.
package core

import (
	"context"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"time"
)

// VectorStore is a MemoryStorageProvider backed by an in-memory map,
// representing SPEC-0035's vector storage backend with SPEC-0038's
// embedding-based similarity search. VectorStore is safe for concurrent use.
type VectorStore struct {
	mu         sync.Mutex
	records    map[string]MemoryRecord
	embeddings map[string][]float64
	nextID     int
	embedder   Embedder
}

// VectorStoreOption configures a VectorStore created by NewVectorStore.
type VectorStoreOption func(*VectorStore)

// WithEmbedder overrides the Embedder a VectorStore uses to embed record
// content and queries. The default is a HashEmbedder.
func WithEmbedder(e Embedder) VectorStoreOption {
	return func(s *VectorStore) { s.embedder = e }
}

// NewVectorStore creates an empty VectorStore, defaulting to a HashEmbedder
// unless overridden via WithEmbedder.
func NewVectorStore(opts ...VectorStoreOption) *VectorStore {
	s := &VectorStore{
		records:    make(map[string]MemoryRecord),
		embeddings: make(map[string][]float64),
		embedder:   NewHashEmbedder(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Name implements MemoryStorageProvider.
func (s *VectorStore) Name() string { return "vector" }

// Put implements MemoryStorageProvider.
func (s *VectorStore) Put(_ context.Context, rec MemoryRecord) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	localID := strconv.Itoa(s.nextID)

	now := time.Now()
	rec.ID = localID
	rec.CreatedAt = now
	rec.UpdatedAt = now
	s.records[localID] = rec
	s.embeddings[localID] = s.embedder.Embed(rec.Content)
	return localID, nil
}

// Get implements MemoryStorageProvider.
func (s *VectorStore) Get(_ context.Context, localID string) (MemoryRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.records[localID]
	if !ok {
		return MemoryRecord{}, notFoundErr("MEMORY_VECTOR_STORE_NOT_FOUND", localID)
	}
	return rec, nil
}

// Query implements MemoryStorageProvider: candidates are narrowed by q.Type
// and q.Filters (each Filters key must equal-match rec.Metadata), scored by
// cosine similarity between q.Query's embedding and the candidate's stored
// embedding, zero-or-negative-score candidates are excluded, and the rest
// are ordered by score descending (ties broken by CreatedAt ascending),
// capped at q.Limit (0 meaning no cap).
func (s *VectorStore) Query(_ context.Context, q MemoryQuery) ([]MemoryRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	queryVec := s.embedder.Embed(q.Query)

	type scored struct {
		rec   MemoryRecord
		score float64
	}
	var candidates []scored
	for id, rec := range s.records {
		if q.Type != "" && rec.Type != q.Type {
			continue
		}
		if !matchesFilters(rec, q.Filters) {
			continue
		}
		score := cosineSimilarity(queryVec, s.embeddings[id])
		if score <= 0 {
			continue
		}
		candidates = append(candidates, scored{rec: rec, score: score})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].rec.CreatedAt.Before(candidates[j].rec.CreatedAt)
	})

	if q.Limit > 0 && len(candidates) > q.Limit {
		candidates = candidates[:q.Limit]
	}
	matches := make([]MemoryRecord, len(candidates))
	for i, c := range candidates {
		matches[i] = c.rec
	}
	return matches, nil
}

// matchesFilters reports whether rec.Metadata equal-matches every key/value
// pair in filters. An empty or nil filters matches everything.
func matchesFilters(rec MemoryRecord, filters map[string]any) bool {
	for k, want := range filters {
		got, ok := rec.Metadata[k]
		if !ok || !reflect.DeepEqual(got, want) {
			return false
		}
	}
	return true
}

// Replace implements MemoryStorageProvider.
func (s *VectorStore) Replace(_ context.Context, localID string, rec MemoryRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.records[localID]
	if !ok {
		return notFoundErr("MEMORY_VECTOR_STORE_NOT_FOUND", localID)
	}
	rec.ID = localID
	rec.CreatedAt = existing.CreatedAt
	rec.UpdatedAt = time.Now()
	s.records[localID] = rec
	s.embeddings[localID] = s.embedder.Embed(rec.Content)
	return nil
}

// Remove implements MemoryStorageProvider.
func (s *VectorStore) Remove(_ context.Context, localID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.records[localID]; !ok {
		return notFoundErr("MEMORY_VECTOR_STORE_NOT_FOUND", localID)
	}
	delete(s.records, localID)
	delete(s.embeddings, localID)
	return nil
}
