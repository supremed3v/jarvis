package core

import (
	"context"
	"strings"
	"testing"

	"jarvis-pa/packages/errors"
)

// newViewerFixture builds the SPEC-0071 wiring: a StorageMemory over a
// LocalStore serving Search/Update/Delete, plus the SPEC-0036/0037 façades
// List enumerates. Returning the components lets tests seed and inspect them.
func newViewerFixture(t *testing.T) (*CoreMemoryViewer, *ConversationMemory, *UserProfileMemory) {
	t.Helper()
	mem := NewStorageMemory(NewLocalStore())
	conv := NewConversationMemory(mem)
	profile := NewUserProfileMemory(mem)
	viewer := NewCoreMemoryViewer(mem,
		WithViewerConversations(conv),
		WithViewerProfile(profile),
	)
	return viewer, conv, profile
}

// storeRecord stores rec directly and returns the compound ID StorageMemory
// assigned it.
func storeRecord(t *testing.T, mem Memory, rec MemoryRecord) string {
	t.Helper()
	id, err := mem.Store(context.Background(), rec)
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	return id
}

func TestCoreMemoryViewer_ListEmptyWithoutFaçades(t *testing.T) {
	viewer := NewCoreMemoryViewer(NewStorageMemory(NewLocalStore()))

	got, err := viewer.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("List() = %v, want empty", got)
	}
}

func TestCoreMemoryViewer_ListGroupsByTypeInDisplayOrder(t *testing.T) {
	viewer, conv, profile := newViewerFixture(t)

	if _, err := profile.Remember(context.Background(), ProfileFact{Key: "pref", Category: ProfileCategoryPreference, Content: "prefers Go"}); err != nil {
		t.Fatalf("Remember() error = %v", err)
	}

	if _, err := conv.AddMessage(context.Background(), ConversationMessage{
		ConversationID: "conv-1", Role: RoleUser, Content: "hello",
	}); err != nil {
		t.Fatalf("AddMessage() error = %v", err)
	}
	if _, err := conv.AddMessage(context.Background(), ConversationMessage{
		ConversationID: "conv-1", Role: RoleAssistant, Content: "hi there",
	}); err != nil {
		t.Fatalf("AddMessage() error = %v", err)
	}

	got, err := viewer.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	// user_profile first, then conversation (knowledge/experience are empty
	// without a registered lister).
	if len(got) != 3 {
		t.Fatalf("List() = %d records, want 3", len(got))
	}
	if got[0].Type != MemoryTypeUserProfile || got[0].Content != "prefers Go" {
		t.Fatalf("got[0] = %+v, want the user-profile fact", got[0])
	}
	if got[1].Type != MemoryTypeConversation || got[1].Content != "hello" || got[1].Metadata[metaConversationID] != "conv-1" || got[1].Metadata[metaRole] != "user" {
		t.Fatalf("got[1] = %+v, want the user message with conversation metadata re-encoded", got[1])
	}
	if got[2].Type != MemoryTypeConversation || got[2].Content != "hi there" || got[2].Metadata[metaRole] != "assistant" {
		t.Fatalf("got[2] = %+v, want the assistant message", got[2])
	}
}

func TestCoreMemoryViewer_ListFiltersByType(t *testing.T) {
	viewer, conv, profile := newViewerFixture(t)
	if _, err := profile.Remember(context.Background(), ProfileFact{Key: "pref", Category: ProfileCategoryPreference, Content: "prefers Go"}); err != nil {
		t.Fatalf("Remember() error = %v", err)
	}
	if _, err := conv.AddMessage(context.Background(), ConversationMessage{
		ConversationID: "conv-1", Role: RoleUser, Content: "hello",
	}); err != nil {
		t.Fatalf("AddMessage() error = %v", err)
	}

	got, err := viewer.List(context.Background(), MemoryTypeConversation)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 || got[0].Type != MemoryTypeConversation {
		t.Fatalf("List(conversation) = %v, want only the message", got)
	}
}

func TestCoreMemoryViewer_ListRejectsUnknownType(t *testing.T) {
	viewer, _, _ := newViewerFixture(t)
	_, err := viewer.List(context.Background(), "telepathy")
	if !errors.Is(err, errors.TypeInvalidInput) {
		t.Fatalf("List(unknown) error = %v, want TypeInvalidInput", err)
	}
}

func TestCoreMemoryViewer_ListUsesRegisteredLister(t *testing.T) {
	mem := NewStorageMemory(NewLocalStore())
	viewer := NewCoreMemoryViewer(mem, WithViewerLister(MemoryTypeKnowledge, func(ctx context.Context) ([]MemoryRecord, error) {
		return []MemoryRecord{{ID: "know::1", Type: MemoryTypeKnowledge, Content: "architecture notes"}}, nil
	}))

	got, err := viewer.List(context.Background(), MemoryTypeKnowledge)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "know::1" || got[0].Content != "architecture notes" {
		t.Fatalf("List() = %v, want the lister's record", got)
	}
}

func TestCoreMemoryViewer_SearchDelegatesToMemory(t *testing.T) {
	viewer, _, _ := newViewerFixture(t)
	storeRecord(t, viewer.memory, MemoryRecord{Type: MemoryTypeKnowledge, Content: "Go context cancellation patterns"})

	got, err := viewer.Search(context.Background(), MemoryQuery{Query: "cancellation", Type: MemoryTypeKnowledge})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(got) != 1 || !strings.Contains(got[0].Content, "cancellation") {
		t.Fatalf("Search() = %v, want the matching record", got)
	}
}

func TestCoreMemoryViewer_UpdateReplacesOnlyContent(t *testing.T) {
	viewer, _, _ := newViewerFixture(t)
	id := storeRecord(t, viewer.memory, MemoryRecord{
		Type:     MemoryTypeUserProfile,
		Content:  "old fact",
		Metadata: map[string]any{"importance": 3.0},
	})

	if err := viewer.Update(context.Background(), MemoryRecord{ID: id, Content: "new fact"}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	stored, err := viewer.memory.Retrieve(context.Background(), id)
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if stored.Content != "new fact" {
		t.Fatalf("stored content = %q, want the edited content", stored.Content)
	}
	if stored.Type != MemoryTypeUserProfile {
		t.Fatalf("stored type = %q, want user_profile preserved", stored.Type)
	}
	if stored.Metadata["importance"] != 3.0 {
		t.Fatalf("stored metadata = %v, want importance preserved", stored.Metadata)
	}
}

func TestCoreMemoryViewer_UpdateValidatesPayload(t *testing.T) {
	viewer, _, _ := newViewerFixture(t)

	if err := viewer.Update(context.Background(), MemoryRecord{Content: "no id"}); err == nil || !errors.Is(err, errors.TypeInvalidInput) {
		t.Fatalf("Update(missing id) error = %v, want TypeInvalidInput", err)
	}
	if err := viewer.Update(context.Background(), MemoryRecord{ID: "local::1"}); err == nil || !errors.Is(err, errors.TypeInvalidInput) {
		t.Fatalf("Update(missing content) error = %v, want TypeInvalidInput", err)
	}
}

func TestCoreMemoryViewer_DeleteRemovesRecord(t *testing.T) {
	viewer, _, _ := newViewerFixture(t)
	id := storeRecord(t, viewer.memory, MemoryRecord{Type: MemoryTypeKnowledge, Content: "notes"})

	if err := viewer.Delete(context.Background(), id); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := viewer.memory.Retrieve(context.Background(), id); !errors.Is(err, errors.TypeNotFound) {
		t.Fatalf("Retrieve() after Delete error = %v, want TypeNotFound", err)
	}
}

func TestCoreMemoryViewer_DeleteRejectsEmptyID(t *testing.T) {
	viewer, _, _ := newViewerFixture(t)
	if err := viewer.Delete(context.Background(), ""); err == nil || !errors.Is(err, errors.TypeInvalidInput) {
		t.Fatalf("Delete(\"\") error = %v, want TypeInvalidInput", err)
	}
}

func TestCoreMemoryViewer_UpdateUnknownRecordSurfacesNotFound(t *testing.T) {
	viewer, _, _ := newViewerFixture(t)
	err := viewer.Update(context.Background(), MemoryRecord{ID: "ghost", Content: "x"})
	if !errors.Is(err, errors.TypeNotFound) {
		t.Fatalf("Update(ghost) error = %v, want TypeNotFound from the backing store", err)
	}
}
