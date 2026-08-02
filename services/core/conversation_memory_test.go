package core

import (
	"context"
	"testing"

	"jarvis-pa/packages/errors"
)

// TestConversationMemory_MessagesPersist verifies AddMessage stores a
// message durably (retrievable by ID through the underlying Memory) with
// its role, content, timestamp, metadata, and related tasks intact
// (SPEC-0036 testing criterion 1).
func TestConversationMemory_MessagesPersist(t *testing.T) {
	mem := NewStorageMemory(NewLocalStore())
	cm := NewConversationMemory(mem)
	ctx := context.Background()

	stored, err := cm.AddMessage(ctx, ConversationMessage{
		ConversationID: "conv-1",
		Role:           RoleUser,
		Content:        "hello there",
		RelatedTaskIDs: []string{"task-1"},
		Metadata:       map[string]any{"client": "desktop"},
	})
	if err != nil {
		t.Fatalf("AddMessage() error = %v", err)
	}
	if stored.ID == "" {
		t.Fatal("AddMessage() returned empty ID")
	}
	if stored.CreatedAt.IsZero() {
		t.Error("AddMessage() returned zero CreatedAt")
	}

	rec, err := mem.Retrieve(ctx, stored.ID)
	if err != nil {
		t.Fatalf("Memory.Retrieve() error = %v", err)
	}
	got := recordToMessage(rec)
	if got.ConversationID != "conv-1" || got.Role != RoleUser || got.Content != "hello there" {
		t.Errorf("persisted message = %+v, want ConversationID=conv-1 Role=user Content=%q", got, "hello there")
	}
	if len(got.RelatedTaskIDs) != 1 || got.RelatedTaskIDs[0] != "task-1" {
		t.Errorf("persisted RelatedTaskIDs = %v, want [task-1]", got.RelatedTaskIDs)
	}
	if got.Metadata["client"] != "desktop" {
		t.Errorf("persisted Metadata = %v, want client=desktop", got.Metadata)
	}
}

// TestConversationMemory_AddMessageValidatesInput verifies AddMessage
// rejects messages missing required fields rather than storing them.
func TestConversationMemory_AddMessageValidatesInput(t *testing.T) {
	cm := NewConversationMemory(NewStorageMemory(NewLocalStore()))
	ctx := context.Background()

	tests := []struct {
		name     string
		msg      ConversationMessage
		wantCode string
	}{
		{
			name:     "missing conversation id",
			msg:      ConversationMessage{Role: RoleUser, Content: "hi"},
			wantCode: "CONVERSATION_MESSAGE_MISSING_CONVERSATION_ID",
		},
		{
			name:     "missing role",
			msg:      ConversationMessage{ConversationID: "c1", Content: "hi"},
			wantCode: "CONVERSATION_MESSAGE_MISSING_ROLE",
		},
		{
			name:     "invalid role",
			msg:      ConversationMessage{ConversationID: "c1", Role: "bogus", Content: "hi"},
			wantCode: "CONVERSATION_MESSAGE_INVALID_ROLE",
		},
		{
			name:     "missing content",
			msg:      ConversationMessage{ConversationID: "c1", Role: RoleUser},
			wantCode: "CONVERSATION_MESSAGE_MISSING_CONTENT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := cm.AddMessage(ctx, tt.msg)
			if err == nil {
				t.Fatalf("AddMessage() = nil error, want code %s", tt.wantCode)
			}
			if !errors.HasCode(err, tt.wantCode) {
				t.Errorf("AddMessage() error = %v, want code %s", err, tt.wantCode)
			}
			if !errors.Is(err, errors.TypeInvalidInput) {
				t.Errorf("AddMessage() error type = %v, want TypeInvalidInput", err)
			}
		})
	}
}

// TestConversationMemory_ConversationsCanBeRetrieved verifies messages
// added to a conversation can be loaded back in order, isolated from other
// conversations (SPEC-0036 testing criterion 2).
func TestConversationMemory_ConversationsCanBeRetrieved(t *testing.T) {
	cm := NewConversationMemory(NewStorageMemory(NewLocalStore()))
	ctx := context.Background()

	mustAdd(t, cm, ctx, "conv-1", RoleUser, "first")
	mustAdd(t, cm, ctx, "conv-1", RoleAssistant, "second")
	mustAdd(t, cm, ctx, "conv-2", RoleUser, "other conversation")

	got, err := cm.Conversation(ctx, "conv-1")
	if err != nil {
		t.Fatalf("Conversation() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Conversation() returned %d messages, want 2", len(got))
	}
	if got[0].Content != "first" || got[1].Content != "second" {
		t.Errorf("Conversation() order = [%q, %q], want [first, second]", got[0].Content, got[1].Content)
	}
	for _, m := range got {
		if m.ConversationID != "conv-1" {
			t.Errorf("Conversation(%q) returned message from %q", "conv-1", m.ConversationID)
		}
	}

	other, err := cm.Conversation(ctx, "conv-2")
	if err != nil {
		t.Fatalf("Conversation() error = %v", err)
	}
	if len(other) != 1 || other[0].Content != "other conversation" {
		t.Errorf("Conversation(conv-2) = %+v, want one message %q", other, "other conversation")
	}
}

// TestConversationMemory_RecentConversationsOrderedByActivity verifies
// RecentConversations lists conversations most-recently-active first and
// respects limit (SPEC-0036 "loading recent conversations" capability).
func TestConversationMemory_RecentConversationsOrderedByActivity(t *testing.T) {
	cm := NewConversationMemory(NewStorageMemory(NewLocalStore()))
	ctx := context.Background()

	mustAdd(t, cm, ctx, "conv-1", RoleUser, "a")
	mustAdd(t, cm, ctx, "conv-2", RoleUser, "b")
	mustAdd(t, cm, ctx, "conv-1", RoleUser, "c") // conv-1 active again, should sort first

	summaries := cm.RecentConversations(0)
	if len(summaries) != 2 {
		t.Fatalf("RecentConversations() returned %d, want 2", len(summaries))
	}
	if summaries[0].ConversationID != "conv-1" || summaries[0].MessageCount != 2 {
		t.Errorf("RecentConversations()[0] = %+v, want ConversationID=conv-1 MessageCount=2", summaries[0])
	}
	if summaries[1].ConversationID != "conv-2" || summaries[1].MessageCount != 1 {
		t.Errorf("RecentConversations()[1] = %+v, want ConversationID=conv-2 MessageCount=1", summaries[1])
	}

	limited := cm.RecentConversations(1)
	if len(limited) != 1 || limited[0].ConversationID != "conv-1" {
		t.Errorf("RecentConversations(1) = %+v, want just conv-1", limited)
	}

	last := mustAdd(t, cm, ctx, "conv-1", RoleUser, "d")
	summaries = cm.RecentConversations(0)
	if !summaries[0].LastMessageAt.Equal(last.CreatedAt) {
		t.Errorf("RecentConversations()[0].LastMessageAt = %v, want %v", summaries[0].LastMessageAt, last.CreatedAt)
	}
}

// TestConversationMemory_Search verifies free-text search finds matching
// conversation messages and leaves other memory types untouched.
func TestConversationMemory_Search(t *testing.T) {
	mem := NewStorageMemory(NewLocalStore())
	cm := NewConversationMemory(mem)
	ctx := context.Background()

	mustAdd(t, cm, ctx, "conv-1", RoleUser, "let's talk about deployments")
	mustAdd(t, cm, ctx, "conv-2", RoleUser, "unrelated topic")
	if _, err := mem.Store(ctx, MemoryRecord{Type: MemoryTypeKnowledge, Content: "deployments are risky"}); err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	results, err := cm.Search(ctx, "deployments", 0)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 || results[0].ConversationID != "conv-1" {
		t.Errorf("Search() = %+v, want one message from conv-1", results)
	}
}

// TestConversationMemory_PrepareContext verifies context preparation
// returns the trailing window of a conversation in order, and the whole
// conversation when no cap is given (SPEC-0036 testing criterion 3).
func TestConversationMemory_PrepareContext(t *testing.T) {
	cm := NewConversationMemory(NewStorageMemory(NewLocalStore()))
	ctx := context.Background()

	mustAdd(t, cm, ctx, "conv-1", RoleUser, "one")
	mustAdd(t, cm, ctx, "conv-1", RoleAssistant, "two")
	mustAdd(t, cm, ctx, "conv-1", RoleUser, "three")

	all, err := cm.PrepareContext(ctx, "conv-1", 0)
	if err != nil {
		t.Fatalf("PrepareContext() error = %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("PrepareContext(0) returned %d messages, want 3", len(all))
	}

	windowed, err := cm.PrepareContext(ctx, "conv-1", 2)
	if err != nil {
		t.Fatalf("PrepareContext() error = %v", err)
	}
	if len(windowed) != 2 || windowed[0].Content != "two" || windowed[1].Content != "three" {
		t.Errorf("PrepareContext(2) = %+v, want [two, three]", windowed)
	}
}

// TestMessageRole_IsValid verifies IsValid recognizes exactly the roles
// ConversationMessage supports.
func TestMessageRole_IsValid(t *testing.T) {
	tests := []struct {
		role MessageRole
		want bool
	}{
		{RoleUser, true},
		{RoleAssistant, true},
		{RoleSystem, true},
		{"bogus", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := tt.role.IsValid(); got != tt.want {
			t.Errorf("MessageRole(%q).IsValid() = %v, want %v", tt.role, got, tt.want)
		}
	}
}

// mustAdd is a test helper that adds a message and fails the test on error.
func mustAdd(t *testing.T, cm *ConversationMemory, ctx context.Context, conversationID string, role MessageRole, content string) ConversationMessage {
	t.Helper()
	msg, err := cm.AddMessage(ctx, ConversationMessage{ConversationID: conversationID, Role: role, Content: content})
	if err != nil {
		t.Fatalf("AddMessage() error = %v", err)
	}
	return msg
}
