// memory_storage.go implements SPEC-0035: the Memory Storage Abstraction.
// MemoryStorageProvider is the storage-engine-level contract a concrete
// backend (a local/relational store, a vector store, or a future provider)
// implements; it is never referenced outside this file and the providers
// that satisfy it, keeping SPEC-0034's Memory the only surface agents and
// Core Runtime ever depend on - ADR-0007 locks a relational + vector split,
// but StorageMemory itself names no backend beyond what its caller
// configures. StorageMemory implements Memory by routing each operation, by
// MemoryType, to the MemoryStorageProvider configured for it.
package core

import (
	"context"
	"sort"
	"strings"

	"jarvis-pa/packages/errors"
)

// idSeparator joins a MemoryStorageProvider's Name with the local ID it
// assigned, forming the compound ID StorageMemory hands back from Store.
// StorageMemory alone interprets this format; MemoryStorageProvider
// implementations never see or produce it themselves.
const idSeparator = "::"

// MemoryStorageProvider is the SPEC-0035 contract a concrete storage backend
// implements. Where Memory (SPEC-0034) is the interface agents and Core
// Runtime call, MemoryStorageProvider is what backs one specific engine
// behind it - a caller of Memory never sees this interface.
type MemoryStorageProvider interface {
	// Name identifies the backend (e.g. "local", "vector"), matching the
	// key StorageMemory's configuration maps a MemoryType to.
	Name() string

	// Put persists rec under a new provider-local ID (ignoring rec.ID) and
	// returns that local ID.
	Put(ctx context.Context, rec MemoryRecord) (string, error)

	// Get returns the record previously stored under localID, or a
	// TypeNotFound error if none exists.
	Get(ctx context.Context, localID string) (MemoryRecord, error)

	// Query returns the records matching q, up to q.Limit results (0
	// meaning provider default), ordered at the provider's discretion.
	Query(ctx context.Context, q MemoryQuery) ([]MemoryRecord, error)

	// Replace overwrites the record stored under localID with rec, or
	// returns a TypeNotFound error if localID doesn't exist.
	Replace(ctx context.Context, localID string, rec MemoryRecord) error

	// Remove deletes the record stored under localID, or returns a
	// TypeNotFound error if localID doesn't exist.
	Remove(ctx context.Context, localID string) error
}

// StorageMemory implements SPEC-0034's Memory by delegating each operation
// to the MemoryStorageProvider configured for the relevant MemoryType,
// falling back to a default provider for any type with no specific
// configuration. StorageMemory is the "storage providers can be swapped"
// requirement: callers depend only on Memory, never on which
// MemoryStorageProvider (or providers) actually back it.
type StorageMemory struct {
	byType   map[MemoryType]MemoryStorageProvider
	fallback MemoryStorageProvider
}

// StorageMemoryOption configures a StorageMemory created by NewStorageMemory.
type StorageMemoryOption func(*StorageMemory)

// WithProviderFor routes MemoryType t to provider, overriding the fallback
// provider for that type only.
func WithProviderFor(t MemoryType, provider MemoryStorageProvider) StorageMemoryOption {
	return func(m *StorageMemory) { m.byType[t] = provider }
}

// NewStorageMemory creates a StorageMemory that routes every MemoryType to
// fallback, except for any type overridden via WithProviderFor. fallback
// must not be nil.
func NewStorageMemory(fallback MemoryStorageProvider, opts ...StorageMemoryOption) *StorageMemory {
	m := &StorageMemory{byType: make(map[MemoryType]MemoryStorageProvider), fallback: fallback}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// providerFor returns the MemoryStorageProvider configured for t.
func (m *StorageMemory) providerFor(t MemoryType) MemoryStorageProvider {
	if p, ok := m.byType[t]; ok {
		return p
	}
	return m.fallback
}

// providers returns the distinct set of providers this StorageMemory can
// route to, deduplicated by Name, in a deterministic (Name-sorted) order.
func (m *StorageMemory) providers() []MemoryStorageProvider {
	seen := make(map[string]MemoryStorageProvider)
	if m.fallback != nil {
		seen[m.fallback.Name()] = m.fallback
	}
	for _, p := range m.byType {
		if p != nil {
			seen[p.Name()] = p
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]MemoryStorageProvider, 0, len(names))
	for _, name := range names {
		result = append(result, seen[name])
	}
	return result
}

// splitID parses a compound ID Store previously returned into the provider
// Name and provider-local ID it encodes. It returns a TypeNotFound error if
// id isn't in that format or names no provider StorageMemory currently
// knows about.
func (m *StorageMemory) splitID(id string) (MemoryStorageProvider, string, error) {
	name, localID, ok := strings.Cut(id, idSeparator)
	if !ok || name == "" || localID == "" {
		return nil, "", errors.New(errors.TypeNotFound, "MEMORY_STORAGE_MALFORMED_ID", "core.memorystorage",
			"memory record ID is not a recognized StorageMemory ID").With("id", id)
	}
	for _, p := range m.providers() {
		if p.Name() == name {
			return p, localID, nil
		}
	}
	return nil, "", errors.New(errors.TypeNotFound, "MEMORY_STORAGE_UNKNOWN_PROVIDER", "core.memorystorage",
		"memory record ID names a provider StorageMemory has none configured for").With("id", id).With("provider", name)
}

// Store implements Memory.Store.
func (m *StorageMemory) Store(ctx context.Context, rec MemoryRecord) (string, error) {
	if err := rec.Validate(); err != nil {
		return "", err
	}
	provider := m.providerFor(rec.Type)
	if provider == nil {
		return "", errors.New(errors.TypeNotFound, "MEMORY_STORAGE_NO_PROVIDER", "core.memorystorage",
			"no storage provider is configured for this memory type").With("type", string(rec.Type))
	}
	localID, err := provider.Put(ctx, rec)
	if err != nil {
		return "", err
	}
	return provider.Name() + idSeparator + localID, nil
}

// Retrieve implements Memory.Retrieve.
func (m *StorageMemory) Retrieve(ctx context.Context, id string) (MemoryRecord, error) {
	provider, localID, err := m.splitID(id)
	if err != nil {
		return MemoryRecord{}, err
	}
	rec, err := provider.Get(ctx, localID)
	if err != nil {
		return MemoryRecord{}, err
	}
	rec.ID = id
	return rec, nil
}

// Search implements Memory.Search. If q.Type is set, only the provider
// configured for that type is queried. Otherwise every distinct configured
// provider is queried and their results merged, capped at q.Limit (0
// meaning no cap).
func (m *StorageMemory) Search(ctx context.Context, q MemoryQuery) ([]MemoryRecord, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}

	var providers []MemoryStorageProvider
	if q.Type != "" {
		provider := m.providerFor(q.Type)
		if provider == nil {
			return nil, errors.New(errors.TypeNotFound, "MEMORY_STORAGE_NO_PROVIDER", "core.memorystorage",
				"no storage provider is configured for this memory type").With("type", string(q.Type))
		}
		providers = []MemoryStorageProvider{provider}
	} else {
		providers = m.providers()
	}

	var results []MemoryRecord
	for _, provider := range providers {
		matches, err := provider.Query(ctx, q)
		if err != nil {
			return nil, err
		}
		for _, rec := range matches {
			rec.ID = provider.Name() + idSeparator + rec.ID
			results = append(results, rec)
		}
	}

	if q.Limit > 0 && len(results) > q.Limit {
		results = results[:q.Limit]
	}
	return results, nil
}

// Update implements Memory.Update.
func (m *StorageMemory) Update(ctx context.Context, rec MemoryRecord) error {
	provider, localID, err := m.splitID(rec.ID)
	if err != nil {
		return err
	}
	if err := rec.Validate(); err != nil {
		return err
	}
	return provider.Replace(ctx, localID, rec)
}

// Delete implements Memory.Delete.
func (m *StorageMemory) Delete(ctx context.Context, id string) error {
	provider, localID, err := m.splitID(id)
	if err != nil {
		return err
	}
	return provider.Remove(ctx, localID)
}
