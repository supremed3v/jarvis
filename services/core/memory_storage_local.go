// memory_storage_local.go implements LocalStore, the SPEC-0035 "Local
// database storage" MemoryStorageProvider: an in-process, mutex-protected
// map, mirroring the map+mutex approach SPEC-0017's HistoryStore already
// established elsewhere in this package rather than introducing a real
// database dependency this phase doesn't call for. Query does an exact
// substring match against Content, the kind of lookup a relational store's
// LIKE query would give.
package core

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"jarvis-pa/packages/errors"
)

// LocalStore is a MemoryStorageProvider backed by an in-memory map,
// representing SPEC-0035's local/relational storage backend. LocalStore is
// safe for concurrent use.
type LocalStore struct {
	mu      sync.Mutex
	records map[string]MemoryRecord
	nextID  int
}

// NewLocalStore creates an empty LocalStore.
func NewLocalStore() *LocalStore {
	return &LocalStore{records: make(map[string]MemoryRecord)}
}

// Name implements MemoryStorageProvider.
func (s *LocalStore) Name() string { return "local" }

// Put implements MemoryStorageProvider.
func (s *LocalStore) Put(_ context.Context, rec MemoryRecord) (string, error) {
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
func (s *LocalStore) Get(_ context.Context, localID string) (MemoryRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.records[localID]
	if !ok {
		return MemoryRecord{}, notFoundErr("MEMORY_LOCAL_STORE_NOT_FOUND", localID)
	}
	return rec, nil
}

// Query implements MemoryStorageProvider: a case-insensitive substring match
// of q.Query against each candidate record's Content, ordered by CreatedAt
// ascending and capped at q.Limit (0 meaning no cap).
func (s *LocalStore) Query(_ context.Context, q MemoryQuery) ([]MemoryRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	needle := strings.ToLower(q.Query)
	var matches []MemoryRecord
	for _, rec := range s.records {
		if q.Type != "" && rec.Type != q.Type {
			continue
		}
		if !strings.Contains(strings.ToLower(rec.Content), needle) {
			continue
		}
		matches = append(matches, rec)
	}

	sort.Slice(matches, func(i, j int) bool { return matches[i].CreatedAt.Before(matches[j].CreatedAt) })
	if q.Limit > 0 && len(matches) > q.Limit {
		matches = matches[:q.Limit]
	}
	return matches, nil
}

// Replace implements MemoryStorageProvider.
func (s *LocalStore) Replace(_ context.Context, localID string, rec MemoryRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.records[localID]
	if !ok {
		return notFoundErr("MEMORY_LOCAL_STORE_NOT_FOUND", localID)
	}
	rec.ID = localID
	rec.CreatedAt = existing.CreatedAt
	rec.UpdatedAt = time.Now()
	s.records[localID] = rec
	return nil
}

// Remove implements MemoryStorageProvider.
func (s *LocalStore) Remove(_ context.Context, localID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.records[localID]; !ok {
		return notFoundErr("MEMORY_LOCAL_STORE_NOT_FOUND", localID)
	}
	delete(s.records, localID)
	return nil
}

// notFoundErr builds the TypeNotFound error a MemoryStorageProvider returns
// when localID doesn't exist.
func notFoundErr(code, localID string) error {
	return errors.New(errors.TypeNotFound, code, "core.memorystorage",
		"no memory record exists with this ID").With("id", localID)
}
