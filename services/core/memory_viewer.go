// memory_viewer.go implements the memory-viewer seam for SPEC-0071's Memory
// Viewer UI: a desktop-facing view over the Memory layer that can enumerate
// what the SPEC-0034 Memory contract cannot. Memory has no "list all"
// operation - MemoryQuery.Validate requires a non-empty Query, so a viewer
// cannot populate its initial listing through the interface alone (the same
// limitation SPEC-0042's build note documented). This seam sits beside the
// locked Memory contract rather than widening it: the Bridge (ws_bridge.go)
// holds a MemoryViewer and stays agnostic of how enumeration is produced, and
// the embedding process wires whatever implementation fits its components.
package core

import (
	"context"

	"jarvis-pa/packages/errors"
)

// MemoryViewer is the view over the runtime's Memory layer the Bridge exposes
// to the desktop (SPEC-0071's data source). It is deliberately separate from
// the SPEC-0034 Memory contract: List supplies the "show me what's in memory"
// operation Memory lacks, while Search/Update/Delete mirror the Memory
// operations the viewer's UI surfaces.
type MemoryViewer interface {
	// List returns every memory record of type t (all types when t is empty),
	// in a stable order suitable for display.
	List(ctx context.Context, t MemoryType) ([]MemoryRecord, error)

	// Search returns the records matching q, delegating to SPEC-0034
	// Memory.Search.
	Search(ctx context.Context, q MemoryQuery) ([]MemoryRecord, error)

	// Update replaces the record stored under rec.ID. The viewer decides what
	// "editing" means for a record: CoreMemoryViewer treats it as a content
	// edit, re-fetching the stored record and replacing only its Content, so
	// Type/Metadata/timestamps survive.
	Update(ctx context.Context, rec MemoryRecord) error

	// Delete removes the record stored under id.
	Delete(ctx context.Context, id string) error
}

// CoreMemoryViewerOption configures a CoreMemoryViewer created by
// NewCoreMemoryViewer.
type CoreMemoryViewerOption func(*CoreMemoryViewer)

// WithViewerConversations wires the SPEC-0036 ConversationMemory whose
// RecentConversations/Conversation index List uses to enumerate
// MemoryTypeConversation records. Optional; without it, conversation records
// list as empty (they remain searchable through the underlying Memory).
func WithViewerConversations(cm *ConversationMemory) CoreMemoryViewerOption {
	return func(v *CoreMemoryViewer) { v.conversations = cm }
}

// WithViewerProfile wires the SPEC-0037 UserProfileMemory whose Facts
// enumeration List uses for MemoryTypeUserProfile records. Optional; without
// it, user-profile records list as empty (they remain searchable).
func WithViewerProfile(upm *UserProfileMemory) CoreMemoryViewerOption {
	return func(v *CoreMemoryViewer) { v.profile = upm }
}

// WithViewerLister registers a per-type enumeration source List falls back to
// for memory types with no façade lister (knowledge, experience) - e.g. a
// closure over a provider that can enumerate its own records. Optional;
// without one, List returns an empty result for those types, since the
// SPEC-0034 Memory contract exposes no list-all operation and the viewer
// cannot invent one.
func WithViewerLister(t MemoryType, lister func(ctx context.Context) ([]MemoryRecord, error)) CoreMemoryViewerOption {
	return func(v *CoreMemoryViewer) { v.listers[t] = lister }
}

// CoreMemoryViewer is the concrete MemoryViewer built over the real Memory
// layer: it delegates Search/Update/Delete to a SPEC-0034 Memory and
// enumerates List from the wired façades (SPEC-0036/0037) plus any registered
// per-type listers. It is safe for concurrent use.
type CoreMemoryViewer struct {
	memory        Memory
	conversations *ConversationMemory
	profile       *UserProfileMemory
	listers       map[MemoryType]func(ctx context.Context) ([]MemoryRecord, error)
}

// NewCoreMemoryViewer creates a CoreMemoryViewer backed by memory, which must
// not be nil (it serves Search/Update/Delete). The listers map is only
// populated by WithViewerLister.
func NewCoreMemoryViewer(memory Memory, opts ...CoreMemoryViewerOption) *CoreMemoryViewer {
	v := &CoreMemoryViewer{
		memory:  memory,
		listers: make(map[MemoryType]func(ctx context.Context) ([]MemoryRecord, error)),
	}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// listTypes is the order List groups memory types in, matching SPEC-0071's
// "user memories, conversations, knowledge entries" display order.
var listTypes = []MemoryType{MemoryTypeUserProfile, MemoryTypeConversation, MemoryTypeKnowledge, MemoryTypeExperience}

// List returns every record of type t (all types when t is empty), grouped by
// memory type in listTypes order. User-profile records come from the wired
// UserProfileMemory's Facts (key-ordered); conversation records come from the
// wired ConversationMemory's recency-ordered conversations with their messages
// in chronological order; knowledge/experience records come from a registered
// per-type lister, defaulting to an empty result when none is wired.
func (v *CoreMemoryViewer) List(ctx context.Context, t MemoryType) ([]MemoryRecord, error) {
	if t != "" && !t.IsValid() {
		return nil, errors.New(errors.TypeInvalidInput, "MEMORY_QUERY_INVALID_TYPE", "core.memoryviewer",
			"memory type is unknown").With("type", string(t))
	}
	types := listTypes
	if t != "" {
		types = []MemoryType{t}
	}

	var out []MemoryRecord
	for _, mt := range types {
		recs, err := v.listType(ctx, mt)
		if err != nil {
			return nil, err
		}
		out = append(out, recs...)
	}
	return out, nil
}

// listType enumerates one memory type from the wired façade, a registered
// lister, or an empty result when neither is available.
func (v *CoreMemoryViewer) listType(ctx context.Context, t MemoryType) ([]MemoryRecord, error) {
	switch t {
	case MemoryTypeUserProfile:
		if v.profile != nil {
			facts, err := v.profile.Facts(ctx)
			if err != nil {
				return nil, err
			}
			recs := make([]MemoryRecord, 0, len(facts))
			for _, f := range facts {
				recs = append(recs, MemoryRecord{
					ID:        f.ID,
					Type:      MemoryTypeUserProfile,
					Content:   f.Content,
					Metadata:  f.Metadata,
					CreatedAt: f.CreatedAt,
					UpdatedAt: f.UpdatedAt,
				})
			}
			return recs, nil
		}
	case MemoryTypeConversation:
		if v.conversations != nil {
			var recs []MemoryRecord
			for _, summary := range v.conversations.RecentConversations(0) {
				messages, err := v.conversations.Conversation(ctx, summary.ConversationID)
				if err != nil {
					return nil, err
				}
				for _, m := range messages {
					recs = append(recs, MemoryRecord{
						ID:        m.ID,
						Type:      MemoryTypeConversation,
						Content:   m.Content,
						Metadata:  messageViewerMetadata(m),
						CreatedAt: m.CreatedAt,
					})
				}
			}
			return recs, nil
		}
	}
	if lister := v.listers[t]; lister != nil {
		return lister(ctx)
	}
	return []MemoryRecord{}, nil
}

// messageViewerMetadata rebuilds the metadata a stored conversation message
// carries, so the viewer can show which conversation a message belongs to and
// who said it. ConversationMemory's own metadata keys are re-encoded because
// recordToMessage strips them into ConversationMessage fields.
func messageViewerMetadata(m ConversationMessage) map[string]any {
	meta := make(map[string]any, len(m.Metadata)+2)
	for k, v := range m.Metadata {
		meta[k] = v
	}
	meta[metaConversationID] = m.ConversationID
	meta[metaRole] = string(m.Role)
	return meta
}

// Search delegates to the SPEC-0034 Memory backing the viewer.
func (v *CoreMemoryViewer) Search(ctx context.Context, q MemoryQuery) ([]MemoryRecord, error) {
	return v.memory.Search(ctx, q)
}

// Update re-fetches the stored record and replaces only its Content, so the
// "editing where allowed" of SPEC-0071 never silently drops a record's Type,
// Metadata, or timestamps.
func (v *CoreMemoryViewer) Update(ctx context.Context, rec MemoryRecord) error {
	if rec.ID == "" {
		return errors.New(errors.TypeInvalidInput, "MEMORY_RECORD_MISSING_ID", "core.memoryviewer",
			"memory record is missing an ID")
	}
	if rec.Content == "" {
		return errors.New(errors.TypeInvalidInput, "MEMORY_RECORD_MISSING_CONTENT", "core.memoryviewer",
			"memory record is missing Content").With("id", rec.ID)
	}
	stored, err := v.memory.Retrieve(ctx, rec.ID)
	if err != nil {
		return err
	}
	stored.Content = rec.Content
	return v.memory.Update(ctx, stored)
}

// Delete removes the record stored under id.
func (v *CoreMemoryViewer) Delete(ctx context.Context, id string) error {
	if id == "" {
		return errors.New(errors.TypeInvalidInput, "MEMORY_RECORD_MISSING_ID", "core.memoryviewer",
			"memory record is missing an ID")
	}
	return v.memory.Delete(ctx, id)
}

// Ensure CoreMemoryViewer satisfies the MemoryViewer seam.
var _ MemoryViewer = (*CoreMemoryViewer)(nil)
