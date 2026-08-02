package core

import (
	"context"
	"testing"

	"jarvis-pa/packages/errors"
)

// Compile-time assertion that StorageMemory satisfies SPEC-0034's Memory.
var _ Memory = (*StorageMemory)(nil)

// TestStorageMemory_ProvidersCanBeSwapped is SPEC-0035 testing criterion 1:
// the same Memory contract, driven the same way, works regardless of which
// concrete MemoryStorageProvider backs it.
func TestStorageMemory_ProvidersCanBeSwapped(t *testing.T) {
	ctx := context.Background()
	rec := MemoryRecord{Type: MemoryTypeConversation, Content: "swap me"}

	for _, fallback := range []MemoryStorageProvider{NewLocalStore(), NewVectorStore()} {
		t.Run(fallback.Name(), func(t *testing.T) {
			mem := NewStorageMemory(fallback)

			id, err := mem.Store(ctx, rec)
			if err != nil {
				t.Fatalf("Store() error = %v", err)
			}
			got, err := mem.Retrieve(ctx, id)
			if err != nil {
				t.Fatalf("Retrieve() error = %v", err)
			}
			if got.Content != rec.Content {
				t.Errorf("Retrieve() Content = %q, want %q", got.Content, rec.Content)
			}

			if err := mem.Delete(ctx, id); err != nil {
				t.Fatalf("Delete() error = %v", err)
			}
			if _, err := mem.Retrieve(ctx, id); !errors.Is(err, errors.TypeNotFound) {
				t.Errorf("Retrieve() after Delete error = %v, want TypeNotFound", err)
			}
		})
	}
}

// TestStorageMemory_RoutesByMemoryType verifies WithProviderFor sends a
// MemoryType's records to its configured provider, and a different type
// falls back to the default, without a caller of Memory choosing either.
func TestStorageMemory_RoutesByMemoryType(t *testing.T) {
	ctx := context.Background()
	local := NewLocalStore()
	vector := NewVectorStore()
	mem := NewStorageMemory(local, WithProviderFor(MemoryTypeKnowledge, vector))

	convID, err := mem.Store(ctx, MemoryRecord{Type: MemoryTypeConversation, Content: "conversation entry"})
	if err != nil {
		t.Fatalf("Store() conversation error = %v", err)
	}
	knowledgeID, err := mem.Store(ctx, MemoryRecord{Type: MemoryTypeKnowledge, Content: "knowledge entry"})
	if err != nil {
		t.Fatalf("Store() knowledge error = %v", err)
	}

	if _, err := local.Get(ctx, "1"); err != nil {
		t.Errorf("expected conversation record in LocalStore under local ID 1: %v", err)
	}
	if _, err := vector.Get(ctx, "1"); err != nil {
		t.Errorf("expected knowledge record in VectorStore under local ID 1: %v", err)
	}

	if convID == knowledgeID {
		t.Errorf("expected distinct StorageMemory IDs across providers, got %q for both", convID)
	}
}

// TestStorageMemory_DataContractsRemainConsistent is SPEC-0035 testing
// criterion 2: a MemoryRecord round-trips through Store/Retrieve with the
// same Type, Content, and Metadata, regardless of which provider handled it.
func TestStorageMemory_DataContractsRemainConsistent(t *testing.T) {
	ctx := context.Background()
	mem := NewStorageMemory(NewLocalStore(), WithProviderFor(MemoryTypeKnowledge, NewVectorStore()))

	for _, rec := range []MemoryRecord{
		{Type: MemoryTypeConversation, Content: "routed to local", Metadata: map[string]any{"n": 1}},
		{Type: MemoryTypeKnowledge, Content: "routed to vector", Metadata: map[string]any{"n": 2}},
	} {
		id, err := mem.Store(ctx, rec)
		if err != nil {
			t.Fatalf("Store() error = %v", err)
		}
		got, err := mem.Retrieve(ctx, id)
		if err != nil {
			t.Fatalf("Retrieve() error = %v", err)
		}
		if got.Type != rec.Type || got.Content != rec.Content {
			t.Errorf("Retrieve() = %+v, want Type/Content from %+v", got, rec)
		}
		if got.Metadata["n"] != rec.Metadata["n"] {
			t.Errorf("Retrieve() Metadata = %+v, want %+v", got.Metadata, rec.Metadata)
		}
	}
}

// TestStorageMemory_StorageErrorsAreHandled is SPEC-0035 testing criterion
// 3: invalid input, unresolvable IDs, and unconfigured memory types all
// surface as typed packages/errors errors rather than panics or silent
// zero values.
func TestStorageMemory_StorageErrorsAreHandled(t *testing.T) {
	ctx := context.Background()
	mem := NewStorageMemory(NewLocalStore())

	t.Run("invalid record never reaches a provider", func(t *testing.T) {
		if _, err := mem.Store(ctx, MemoryRecord{Content: "no type"}); !errors.Is(err, errors.TypeInvalidInput) {
			t.Errorf("Store() error = %v, want TypeInvalidInput", err)
		}
	})

	t.Run("retrieve of malformed ID", func(t *testing.T) {
		if _, err := mem.Retrieve(ctx, "not-a-compound-id"); !errors.Is(err, errors.TypeNotFound) {
			t.Errorf("Retrieve() error = %v, want TypeNotFound", err)
		}
	})

	t.Run("retrieve of unknown provider", func(t *testing.T) {
		if _, err := mem.Retrieve(ctx, "vector::1"); !errors.Is(err, errors.TypeNotFound) {
			t.Errorf("Retrieve() error = %v, want TypeNotFound", err)
		}
	})

	t.Run("retrieve of unknown local ID", func(t *testing.T) {
		id, err := mem.Store(ctx, MemoryRecord{Type: MemoryTypeConversation, Content: "x"})
		if err != nil {
			t.Fatalf("Store() error = %v", err)
		}
		if err := mem.Delete(ctx, id); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}
		if _, err := mem.Retrieve(ctx, id); !errors.Is(err, errors.TypeNotFound) {
			t.Errorf("Retrieve() after Delete error = %v, want TypeNotFound", err)
		}
	})

	t.Run("update of unknown ID", func(t *testing.T) {
		err := mem.Update(ctx, MemoryRecord{ID: "local::missing", Type: MemoryTypeConversation, Content: "x"})
		if !errors.Is(err, errors.TypeNotFound) {
			t.Errorf("Update() error = %v, want TypeNotFound", err)
		}
	})

	t.Run("no provider configured for type", func(t *testing.T) {
		empty := &StorageMemory{byType: make(map[MemoryType]MemoryStorageProvider)}
		if _, err := empty.Store(ctx, MemoryRecord{Type: MemoryTypeConversation, Content: "x"}); !errors.Is(err, errors.TypeNotFound) {
			t.Errorf("Store() error = %v, want TypeNotFound", err)
		}
	})
}

// TestStorageMemory_SearchMergesAcrossProviders verifies Search queries every
// distinct configured provider when q.Type is unset, and scopes to a single
// provider when it is set.
func TestStorageMemory_SearchMergesAcrossProviders(t *testing.T) {
	ctx := context.Background()
	mem := NewStorageMemory(NewLocalStore(), WithProviderFor(MemoryTypeKnowledge, NewVectorStore()))

	if _, err := mem.Store(ctx, MemoryRecord{Type: MemoryTypeConversation, Content: "shared keyword alpha"}); err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if _, err := mem.Store(ctx, MemoryRecord{Type: MemoryTypeKnowledge, Content: "shared keyword alpha"}); err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	all, err := mem.Search(ctx, MemoryQuery{Query: "alpha"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("Search() with no Type returned %d results, want 2 (both providers)", len(all))
	}

	scoped, err := mem.Search(ctx, MemoryQuery{Query: "alpha", Type: MemoryTypeKnowledge})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(scoped) != 1 || scoped[0].Type != MemoryTypeKnowledge {
		t.Errorf("Search() with Type = %+v, want a single MemoryTypeKnowledge result", scoped)
	}

	if _, err := mem.Search(ctx, MemoryQuery{Query: ""}); !errors.Is(err, errors.TypeInvalidInput) {
		t.Errorf("Search() with empty Query error = %v, want TypeInvalidInput", err)
	}
}

// TestStorageMemory_SearchPropagatesFiltersToProvider verifies q.Filters
// (SPEC-0038 metadata filtering) reaches the underlying MemoryStorageProvider
// unchanged when driven through the Memory interface, not just when calling
// a provider's Query directly.
func TestStorageMemory_SearchPropagatesFiltersToProvider(t *testing.T) {
	ctx := context.Background()
	mem := NewStorageMemory(NewLocalStore(), WithProviderFor(MemoryTypeKnowledge, NewVectorStore()))

	if _, err := mem.Store(ctx, MemoryRecord{
		Type: MemoryTypeKnowledge, Content: "project status update",
		Metadata: map[string]any{"category": "work"},
	}); err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if _, err := mem.Store(ctx, MemoryRecord{
		Type: MemoryTypeKnowledge, Content: "project status update",
		Metadata: map[string]any{"category": "personal"},
	}); err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	matches, err := mem.Search(ctx, MemoryQuery{
		Query:   "project status update",
		Type:    MemoryTypeKnowledge,
		Filters: map[string]any{"category": "work"},
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(matches) != 1 || matches[0].Metadata["category"] != "work" {
		t.Errorf("Search() with Filters = %+v, want single match with category=work", matches)
	}
}
