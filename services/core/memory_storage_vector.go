// memory_storage_vector.go implements VectorStore, the SPEC-0035 "Vector
// storage" MemoryStorageProvider. Real embedding-based similarity is
// SPEC-0038 (Vector Memory Engine) and SPEC-0039 (Embedding Pipeline)'s job;
// this spec only needs a backend whose retrieval behaves differently from
// LocalStore's exact substring match while satisfying the same contract, so
// Query instead ranks candidates by a naive word-overlap score - a
// placeholder relevance measure, not a real vector similarity metric.
package core

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// VectorStore is a MemoryStorageProvider backed by an in-memory map,
// representing SPEC-0035's vector storage backend. VectorStore is safe for
// concurrent use.
type VectorStore struct {
	mu      sync.Mutex
	records map[string]MemoryRecord
	nextID  int
}

// NewVectorStore creates an empty VectorStore.
func NewVectorStore() *VectorStore {
	return &VectorStore{records: make(map[string]MemoryRecord)}
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

// Query implements MemoryStorageProvider: candidates are scored by the
// number of words shared with q.Query (case-insensitive), zero-score
// candidates are excluded, and the rest are ordered by score descending
// (ties broken by CreatedAt ascending), capped at q.Limit (0 meaning no
// cap).
func (s *VectorStore) Query(_ context.Context, q MemoryQuery) ([]MemoryRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	queryWords := wordSet(q.Query)

	type scored struct {
		rec   MemoryRecord
		score int
	}
	var candidates []scored
	for _, rec := range s.records {
		if q.Type != "" && rec.Type != q.Type {
			continue
		}
		score := overlapScore(queryWords, wordSet(rec.Content))
		if score == 0 {
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
	return nil
}

// wordSet lowercases and splits s into a set of its distinct words.
func wordSet(s string) map[string]bool {
	words := strings.Fields(strings.ToLower(s))
	set := make(map[string]bool, len(words))
	for _, w := range words {
		set[w] = true
	}
	return set
}

// overlapScore counts how many words a and b have in common.
func overlapScore(a, b map[string]bool) int {
	score := 0
	for w := range a {
		if b[w] {
			score++
		}
	}
	return score
}
