package core

import (
	"context"
	"testing"

	"jarvis-pa/packages/errors"
)

// TestUserProfileMemory_FactsPersist verifies Remember stores a fact
// durably (retrievable by ID through the underlying Memory) with its key,
// category, content, timestamps, and metadata intact (SPEC-0037 testing
// criterion 1: "User facts can be stored").
func TestUserProfileMemory_FactsPersist(t *testing.T) {
	mem := NewStorageMemory(NewLocalStore())
	pm := NewUserProfileMemory(mem)
	ctx := context.Background()

	stored, err := pm.Remember(ctx, ProfileFact{
		Key:      "preference:language",
		Category: ProfileCategoryPreference,
		Content:  "User prefers Go over Python",
		Metadata: map[string]any{"source": "conversation"},
	})
	if err != nil {
		t.Fatalf("Remember() error = %v", err)
	}
	if stored.ID == "" {
		t.Fatal("Remember() returned empty ID")
	}
	if stored.CreatedAt.IsZero() || stored.UpdatedAt.IsZero() {
		t.Error("Remember() returned zero timestamps")
	}

	rec, err := mem.Retrieve(ctx, stored.ID)
	if err != nil {
		t.Fatalf("Memory.Retrieve() error = %v", err)
	}
	got := recordToFact(rec)
	if got.Key != "preference:language" || got.Category != ProfileCategoryPreference || got.Content != "User prefers Go over Python" {
		t.Errorf("persisted fact = %+v, want Key=preference:language Category=preference Content=%q",
			got, "User prefers Go over Python")
	}
	if got.Metadata["source"] != "conversation" {
		t.Errorf("persisted Metadata = %v, want source=conversation", got.Metadata)
	}
}

// TestUserProfileMemory_RememberValidatesInput verifies Remember rejects
// facts missing required fields rather than storing them.
func TestUserProfileMemory_RememberValidatesInput(t *testing.T) {
	pm := NewUserProfileMemory(NewStorageMemory(NewLocalStore()))
	ctx := context.Background()

	tests := []struct {
		name     string
		fact     ProfileFact
		wantCode string
	}{
		{
			name:     "missing key",
			fact:     ProfileFact{Category: ProfileCategoryFact, Content: "something"},
			wantCode: "PROFILE_FACT_MISSING_KEY",
		},
		{
			name:     "missing category",
			fact:     ProfileFact{Key: "k1", Content: "something"},
			wantCode: "PROFILE_FACT_MISSING_CATEGORY",
		},
		{
			name:     "invalid category",
			fact:     ProfileFact{Key: "k1", Category: "bogus", Content: "something"},
			wantCode: "PROFILE_FACT_INVALID_CATEGORY",
		},
		{
			name:     "missing content",
			fact:     ProfileFact{Key: "k1", Category: ProfileCategoryFact},
			wantCode: "PROFILE_FACT_MISSING_CONTENT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := pm.Remember(ctx, tt.fact)
			if err == nil {
				t.Fatalf("Remember() = nil error, want code %s", tt.wantCode)
			}
			if !errors.HasCode(err, tt.wantCode) {
				t.Errorf("Remember() error = %v, want code %s", err, tt.wantCode)
			}
			if !errors.Is(err, errors.TypeInvalidInput) {
				t.Errorf("Remember() error type = %v, want TypeInvalidInput", err)
			}
		})
	}
}

// TestUserProfileMemory_FactsCanBeRetrieved verifies facts remembered under
// distinct keys can be retrieved back by key, isolated from each other, and
// that an unknown key reports TypeNotFound (SPEC-0037 testing criterion 2).
func TestUserProfileMemory_FactsCanBeRetrieved(t *testing.T) {
	pm := NewUserProfileMemory(NewStorageMemory(NewLocalStore()))
	ctx := context.Background()

	mustRemember(t, pm, ctx, "preference:language", ProfileCategoryPreference, "User prefers Go over Python")
	mustRemember(t, pm, ctx, "project:current", ProfileCategoryProject, "User works on Invoke Solutions")

	got, err := pm.Fact(ctx, "preference:language")
	if err != nil {
		t.Fatalf("Fact() error = %v", err)
	}
	if got.Content != "User prefers Go over Python" {
		t.Errorf("Fact(preference:language) = %+v, want Content=%q", got, "User prefers Go over Python")
	}

	other, err := pm.Fact(ctx, "project:current")
	if err != nil {
		t.Fatalf("Fact() error = %v", err)
	}
	if other.Content != "User works on Invoke Solutions" {
		t.Errorf("Fact(project:current) = %+v, want Content=%q", other, "User works on Invoke Solutions")
	}

	_, err = pm.Fact(ctx, "unknown:key")
	if err == nil {
		t.Fatal("Fact(unknown:key) = nil error, want TypeNotFound")
	}
	if !errors.Is(err, errors.TypeNotFound) {
		t.Errorf("Fact(unknown:key) error type = %v, want TypeNotFound", err)
	}
}

// TestUserProfileMemory_UpdatesReplaceOutdatedInformation verifies
// Remember-ing a fact again under the same Key replaces the earlier
// content in place rather than accumulating a duplicate record (SPEC-0037
// testing criterion 3: "Updates replace outdated information").
func TestUserProfileMemory_UpdatesReplaceOutdatedInformation(t *testing.T) {
	pm := NewUserProfileMemory(NewStorageMemory(NewLocalStore()))
	ctx := context.Background()

	first := mustRemember(t, pm, ctx, "preference:language", ProfileCategoryPreference, "User prefers Python")
	second := mustRemember(t, pm, ctx, "preference:language", ProfileCategoryPreference, "User prefers Go over Python")

	if second.ID != first.ID {
		t.Errorf("Remember() on existing key returned new ID %q, want same ID %q (in-place replace)", second.ID, first.ID)
	}
	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("Remember() on existing key changed CreatedAt from %v to %v", first.CreatedAt, second.CreatedAt)
	}

	got, err := pm.Fact(ctx, "preference:language")
	if err != nil {
		t.Fatalf("Fact() error = %v", err)
	}
	if got.Content != "User prefers Go over Python" {
		t.Errorf("Fact() after update = %+v, want Content=%q", got, "User prefers Go over Python")
	}

	facts, err := pm.Facts(ctx)
	if err != nil {
		t.Fatalf("Facts() error = %v", err)
	}
	if len(facts) != 1 {
		t.Errorf("Facts() returned %d facts, want 1 (update should not accumulate duplicates)", len(facts))
	}
}

// TestUserProfileMemory_Facts_ListsAllOrderedByKey verifies Facts returns
// every remembered fact, ordered by Key for a deterministic result.
func TestUserProfileMemory_Facts_ListsAllOrderedByKey(t *testing.T) {
	pm := NewUserProfileMemory(NewStorageMemory(NewLocalStore()))
	ctx := context.Background()

	mustRemember(t, pm, ctx, "project:current", ProfileCategoryProject, "User works on Invoke Solutions")
	mustRemember(t, pm, ctx, "preference:language", ProfileCategoryPreference, "User prefers Go over Python")
	mustRemember(t, pm, ctx, "working_style:review", ProfileCategoryWorkingStyle, "User prefers small PRs")

	facts, err := pm.Facts(ctx)
	if err != nil {
		t.Fatalf("Facts() error = %v", err)
	}
	if len(facts) != 3 {
		t.Fatalf("Facts() returned %d facts, want 3", len(facts))
	}
	wantOrder := []string{"preference:language", "project:current", "working_style:review"}
	for i, want := range wantOrder {
		if facts[i].Key != want {
			t.Errorf("Facts()[%d].Key = %q, want %q", i, facts[i].Key, want)
		}
	}
}

// TestProfileCategory_IsValid verifies IsValid recognizes exactly the
// categories ProfileFact supports.
func TestProfileCategory_IsValid(t *testing.T) {
	tests := []struct {
		category ProfileCategory
		want     bool
	}{
		{ProfileCategoryPreference, true},
		{ProfileCategoryPersonalInfo, true},
		{ProfileCategoryWorkingStyle, true},
		{ProfileCategoryProject, true},
		{ProfileCategoryFact, true},
		{"bogus", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := tt.category.IsValid(); got != tt.want {
			t.Errorf("ProfileCategory(%q).IsValid() = %v, want %v", tt.category, got, tt.want)
		}
	}
}

// mustRemember is a test helper that remembers a fact and fails the test on
// error.
func mustRemember(t *testing.T, pm *UserProfileMemory, ctx context.Context, key string, category ProfileCategory, content string) ProfileFact {
	t.Helper()
	fact, err := pm.Remember(ctx, ProfileFact{Key: key, Category: category, Content: content})
	if err != nil {
		t.Fatalf("Remember() error = %v", err)
	}
	return fact
}
