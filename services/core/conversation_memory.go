// conversation_memory.go implements SPEC-0036: Conversation Memory.
// ConversationMemory is a façade over SPEC-0034's Memory interface,
// specialized for storing and retrieving conversation messages (each one a
// MemoryRecord of MemoryTypeConversation) without requiring callers to
// hand-encode conversation structure into MemoryRecord.Metadata themselves.
//
// Memory.Search requires a non-empty query and has no notion of "distinct
// conversations", so ConversationMemory keeps its own in-process index
// (conversationID -> ordered message IDs, and a recency-ordered list of
// conversation IDs) alongside the underlying Memory - the same map+mutex
// approach memory_storage_local.go's LocalStore already uses to satisfy its
// own interface. Free-text search still delegates to Memory.Search.
package core

import (
	"context"
	"sync"
	"time"

	"jarvis-pa/packages/errors"
)

// MessageRole identifies who authored a ConversationMessage.
type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleSystem    MessageRole = "system"
)

// IsValid reports whether r is one of the roles ConversationMessage
// supports.
func (r MessageRole) IsValid() bool {
	switch r {
	case RoleUser, RoleAssistant, RoleSystem:
		return true
	default:
		return false
	}
}

// ConversationMessage is one message within a conversation: its role,
// content, optional links to related tasks, free-form metadata, and the
// timestamp the underlying Memory assigned it.
type ConversationMessage struct {
	ID             string
	ConversationID string
	Role           MessageRole
	Content        string
	RelatedTaskIDs []string
	Metadata       map[string]any
	CreatedAt      time.Time
}

// Validate reports whether m has the minimum fields ConversationMemory
// needs to store it: a ConversationID, a known Role, and non-empty Content.
// It returns a packages/errors error typed TypeInvalidInput naming the
// first missing or invalid field, or nil if m is valid.
func (m ConversationMessage) Validate() error {
	if m.ConversationID == "" {
		return errors.New(errors.TypeInvalidInput, "CONVERSATION_MESSAGE_MISSING_CONVERSATION_ID",
			"core.conversationmemory", "conversation message is missing a ConversationID")
	}
	if m.Role == "" {
		return errors.New(errors.TypeInvalidInput, "CONVERSATION_MESSAGE_MISSING_ROLE",
			"core.conversationmemory", "conversation message is missing a Role").With("conversationId", m.ConversationID)
	}
	if !m.Role.IsValid() {
		return errors.New(errors.TypeInvalidInput, "CONVERSATION_MESSAGE_INVALID_ROLE",
			"core.conversationmemory", "conversation message has an unknown Role").With("role", string(m.Role))
	}
	if m.Content == "" {
		return errors.New(errors.TypeInvalidInput, "CONVERSATION_MESSAGE_MISSING_CONTENT",
			"core.conversationmemory", "conversation message is missing Content").With("conversationId", m.ConversationID)
	}
	return nil
}

// ConversationSummary describes one conversation for RecentConversations:
// its ID, how many messages it holds, and when the most recent one was
// added.
type ConversationSummary struct {
	ConversationID string
	MessageCount   int
	LastMessageAt  time.Time
}

// metadata keys ConversationMemory encodes onto / decodes from a
// MemoryRecord.Metadata map. Only ConversationMemory reads or writes these
// keys; they are not part of the Memory or MemoryStorageProvider contracts.
const (
	metaConversationID = "conversationId"
	metaRole           = "role"
	metaRelatedTaskIDs = "relatedTaskIds"
)

// ConversationMemory implements SPEC-0036: storing and retrieving
// conversation messages on top of a SPEC-0034 Memory. It is safe for
// concurrent use.
type ConversationMemory struct {
	memory Memory

	mu             sync.Mutex
	byConversation map[string][]string  // conversationID -> message record IDs, append order (chronological)
	lastMessageAt  map[string]time.Time // conversationID -> CreatedAt of its most recent message
	recent         []string             // conversationIDs, most-recently-active first, deduplicated
}

// NewConversationMemory creates a ConversationMemory backed by memory.
// memory must not be nil.
func NewConversationMemory(memory Memory) *ConversationMemory {
	return &ConversationMemory{
		memory:         memory,
		byConversation: make(map[string][]string),
		lastMessageAt:  make(map[string]time.Time),
	}
}

// recordToMessage converts a MemoryRecord ConversationMemory previously
// stored (or a matching Search result) back into a ConversationMessage,
// decoding the metadata keys AddMessage encoded.
func recordToMessage(rec MemoryRecord) ConversationMessage {
	msg := ConversationMessage{
		ID:        rec.ID,
		Content:   rec.Content,
		CreatedAt: rec.CreatedAt,
	}
	if rec.Metadata == nil {
		return msg
	}
	if v, ok := rec.Metadata[metaConversationID].(string); ok {
		msg.ConversationID = v
	}
	if v, ok := rec.Metadata[metaRole].(string); ok {
		msg.Role = MessageRole(v)
	}
	if v, ok := rec.Metadata[metaRelatedTaskIDs].([]string); ok {
		msg.RelatedTaskIDs = v
	}
	if len(rec.Metadata) > 0 {
		userMeta := make(map[string]any, len(rec.Metadata))
		for k, v := range rec.Metadata {
			if k == metaConversationID || k == metaRole || k == metaRelatedTaskIDs {
				continue
			}
			userMeta[k] = v
		}
		if len(userMeta) > 0 {
			msg.Metadata = userMeta
		}
	}
	return msg
}

// touchIndex records id (created at) as the newest message in
// conversationID's history and moves conversationID to the front of the
// recency-ordered list.
func (c *ConversationMemory) touchIndex(conversationID, id string, at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.byConversation[conversationID] = append(c.byConversation[conversationID], id)
	c.lastMessageAt[conversationID] = at

	for i, cid := range c.recent {
		if cid == conversationID {
			c.recent = append(c.recent[:i], c.recent[i+1:]...)
			break
		}
	}
	c.recent = append([]string{conversationID}, c.recent...)
}

// AddMessage validates and stores msg, returning the stored message with
// its assigned ID and provider-set CreatedAt filled in.
func (c *ConversationMemory) AddMessage(ctx context.Context, msg ConversationMessage) (ConversationMessage, error) {
	if err := msg.Validate(); err != nil {
		return ConversationMessage{}, err
	}

	metadata := make(map[string]any, len(msg.Metadata)+3)
	for k, v := range msg.Metadata {
		metadata[k] = v
	}
	metadata[metaConversationID] = msg.ConversationID
	metadata[metaRole] = string(msg.Role)
	if msg.RelatedTaskIDs != nil {
		metadata[metaRelatedTaskIDs] = msg.RelatedTaskIDs
	}

	id, err := c.memory.Store(ctx, MemoryRecord{
		Type:     MemoryTypeConversation,
		Content:  msg.Content,
		Metadata: metadata,
	})
	if err != nil {
		return ConversationMessage{}, err
	}

	rec, err := c.memory.Retrieve(ctx, id)
	if err != nil {
		return ConversationMessage{}, err
	}

	c.touchIndex(msg.ConversationID, id, rec.CreatedAt)
	return recordToMessage(rec), nil
}

// Conversation returns every message stored for conversationID, in
// chronological order.
func (c *ConversationMemory) Conversation(ctx context.Context, conversationID string) ([]ConversationMessage, error) {
	c.mu.Lock()
	ids := append([]string(nil), c.byConversation[conversationID]...)
	c.mu.Unlock()

	messages := make([]ConversationMessage, 0, len(ids))
	for _, id := range ids {
		rec, err := c.memory.Retrieve(ctx, id)
		if err != nil {
			return nil, err
		}
		messages = append(messages, recordToMessage(rec))
	}
	return messages, nil
}

// RecentConversations returns up to limit conversations ordered by most
// recent activity (limit <= 0 means no cap).
func (c *ConversationMemory) RecentConversations(limit int) []ConversationSummary {
	c.mu.Lock()
	defer c.mu.Unlock()

	ids := c.recent
	if limit > 0 && len(ids) > limit {
		ids = ids[:limit]
	}

	summaries := make([]ConversationSummary, 0, len(ids))
	for _, cid := range ids {
		msgIDs := c.byConversation[cid]
		summaries = append(summaries, ConversationSummary{
			ConversationID: cid,
			MessageCount:   len(msgIDs),
			LastMessageAt:  c.lastMessageAt[cid],
		})
	}
	return summaries
}

// Search performs a free-text search over conversation messages, delegating
// to the underlying Memory's Search scoped to MemoryTypeConversation.
func (c *ConversationMemory) Search(ctx context.Context, query string, limit int) ([]ConversationMessage, error) {
	results, err := c.memory.Search(ctx, MemoryQuery{
		Type:  MemoryTypeConversation,
		Query: query,
		Limit: limit,
	})
	if err != nil {
		return nil, err
	}

	messages := make([]ConversationMessage, 0, len(results))
	for _, rec := range results {
		messages = append(messages, recordToMessage(rec))
	}
	return messages, nil
}

// PrepareContext returns the trailing maxMessages messages of conversationID
// in chronological order, ready to feed into an agent's context (maxMessages
// <= 0 means no cap - the full conversation is returned).
func (c *ConversationMemory) PrepareContext(ctx context.Context, conversationID string, maxMessages int) ([]ConversationMessage, error) {
	messages, err := c.Conversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if maxMessages > 0 && len(messages) > maxMessages {
		messages = messages[len(messages)-maxMessages:]
	}
	return messages, nil
}
