// memory_interface.go implements SPEC-0034: the Memory Interface. Memory is
// the contract Core Runtime (and, in turn, agents and the LLM layer) use to
// store and retrieve persistent context without depending on a specific
// storage engine - ADR-0007 locks a relational + vector storage split, but
// Memory itself names no backend. SPEC-0035 (Memory Storage Abstraction)
// supplies the first concrete storage layer; this spec defines only the
// contract and its supporting types.
package core

import (
	"context"
	"time"

	"jarvis-pa/packages/errors"
)

// MemoryType classifies a MemoryRecord into one of the memory types SPEC-0034
// requires support for.
type MemoryType string

const (
	MemoryTypeConversation MemoryType = "conversation"
	MemoryTypeUserProfile  MemoryType = "user_profile"
	MemoryTypeKnowledge    MemoryType = "knowledge"
	MemoryTypeExperience   MemoryType = "experience"
)

// IsValid reports whether t is one of the memory types SPEC-0034 defines.
func (t MemoryType) IsValid() bool {
	switch t {
	case MemoryTypeConversation, MemoryTypeUserProfile, MemoryTypeKnowledge, MemoryTypeExperience:
		return true
	default:
		return false
	}
}

// MemoryRecord is one stored unit of memory: its type, content, and
// free-form metadata, plus the timestamps a Memory provider fills in.
type MemoryRecord struct {
	ID        string
	Type      MemoryType
	Content   string
	Metadata  map[string]any
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Validate reports whether r has the minimum fields a Memory provider needs
// to store it: a known Type and non-empty Content. It returns a
// packages/errors error typed TypeInvalidInput naming the first missing or
// invalid field, or nil if r is valid.
func (r MemoryRecord) Validate() error {
	if r.Type == "" {
		return errors.New(errors.TypeInvalidInput, "MEMORY_RECORD_MISSING_TYPE", "core.memory",
			"memory record is missing a Type")
	}
	if !r.Type.IsValid() {
		return errors.New(errors.TypeInvalidInput, "MEMORY_RECORD_INVALID_TYPE", "core.memory",
			"memory record has an unknown Type").With("type", string(r.Type))
	}
	if r.Content == "" {
		return errors.New(errors.TypeInvalidInput, "MEMORY_RECORD_MISSING_CONTENT", "core.memory",
			"memory record is missing Content").With("type", string(r.Type))
	}
	return nil
}

// MemoryQuery is the input to Memory.Search: what to look for, optionally
// scoped to one MemoryType, with a result Limit and provider-specific
// Filters.
type MemoryQuery struct {
	Type    MemoryType
	Query   string
	Limit   int
	Filters map[string]any
}

// Validate reports whether q has the minimum fields a Memory provider needs
// to run a search: a non-empty Query, and a Type that is either unset (all
// types) or one of the known MemoryType values. It returns a
// packages/errors error typed TypeInvalidInput, or nil if q is valid.
func (q MemoryQuery) Validate() error {
	if q.Query == "" {
		return errors.New(errors.TypeInvalidInput, "MEMORY_QUERY_MISSING_QUERY", "core.memory",
			"memory query is missing a Query")
	}
	if q.Type != "" && !q.Type.IsValid() {
		return errors.New(errors.TypeInvalidInput, "MEMORY_QUERY_INVALID_TYPE", "core.memory",
			"memory query has an unknown Type").With("type", string(q.Type))
	}
	return nil
}

// Memory is the SPEC-0034 contract Core Runtime uses to store and retrieve
// memory without depending on which storage engine is actually backing it.
type Memory interface {
	// Store persists rec and returns its assigned ID. If rec.ID is already
	// set, implementations may treat this as a request to store under that
	// ID rather than generating a new one.
	Store(ctx context.Context, rec MemoryRecord) (string, error)

	// Retrieve returns the MemoryRecord previously stored under id, or a
	// TypeNotFound error if no such record exists.
	Retrieve(ctx context.Context, id string) (MemoryRecord, error)

	// Search returns the records matching q, ordered and scored at the
	// provider's discretion, up to q.Limit results (0 meaning provider
	// default).
	Search(ctx context.Context, q MemoryQuery) ([]MemoryRecord, error)

	// Update replaces the record stored under rec.ID with rec, or returns a
	// TypeNotFound error if no such record exists.
	Update(ctx context.Context, rec MemoryRecord) error

	// Delete removes the record stored under id, or returns a TypeNotFound
	// error if no such record exists.
	Delete(ctx context.Context, id string) error
}
