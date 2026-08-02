// user_profile_memory.go implements SPEC-0037: User Profile Memory.
// UserProfileMemory is a façade over SPEC-0034's Memory interface,
// specialized for storing and retrieving durable facts about the user -
// preferences, personal information, working style, projects, and other
// important facts - each one a MemoryRecord of MemoryTypeUserProfile.
//
// Unlike ConversationMemory (SPEC-0036), which only ever appends, a
// profile fact carries a caller-assigned Key identifying what it's about
// (e.g. "preference:language"); Remember uses that Key to find and replace
// any record already stored for it via Memory.Update, rather than
// accumulating duplicates, satisfying SPEC-0037's "updates replace
// outdated information" requirement. The Key -> record ID mapping is kept
// as an in-process index, the same map+mutex approach ConversationMemory
// and memory_storage_local.go's LocalStore already use to satisfy their
// own interfaces.
package core

import (
	"context"
	"sort"
	"sync"
	"time"

	"jarvis-pa/packages/errors"
)

// ProfileCategory classifies a ProfileFact into one of the kinds SPEC-0037
// requires support for.
type ProfileCategory string

const (
	ProfileCategoryPreference   ProfileCategory = "preference"
	ProfileCategoryPersonalInfo ProfileCategory = "personal_info"
	ProfileCategoryWorkingStyle ProfileCategory = "working_style"
	ProfileCategoryProject      ProfileCategory = "project"
	ProfileCategoryFact         ProfileCategory = "fact"
)

// IsValid reports whether c is one of the categories ProfileFact supports.
func (c ProfileCategory) IsValid() bool {
	switch c {
	case ProfileCategoryPreference, ProfileCategoryPersonalInfo, ProfileCategoryWorkingStyle, ProfileCategoryProject, ProfileCategoryFact:
		return true
	default:
		return false
	}
}

// ProfileFact is one durable fact about the user: what it's about (Key),
// what kind of fact it is (Category), its Content (e.g. "User prefers Go
// over Python"), free-form metadata, and the timestamps the underlying
// Memory assigned it.
type ProfileFact struct {
	ID        string
	Key       string
	Category  ProfileCategory
	Content   string
	Metadata  map[string]any
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Validate reports whether f has the minimum fields UserProfileMemory needs
// to store it: a Key, a known Category, and non-empty Content. It returns a
// packages/errors error typed TypeInvalidInput naming the first missing or
// invalid field, or nil if f is valid.
func (f ProfileFact) Validate() error {
	if f.Key == "" {
		return errors.New(errors.TypeInvalidInput, "PROFILE_FACT_MISSING_KEY",
			"core.userprofilememory", "profile fact is missing a Key")
	}
	if f.Category == "" {
		return errors.New(errors.TypeInvalidInput, "PROFILE_FACT_MISSING_CATEGORY",
			"core.userprofilememory", "profile fact is missing a Category").With("key", f.Key)
	}
	if !f.Category.IsValid() {
		return errors.New(errors.TypeInvalidInput, "PROFILE_FACT_INVALID_CATEGORY",
			"core.userprofilememory", "profile fact has an unknown Category").With("category", string(f.Category))
	}
	if f.Content == "" {
		return errors.New(errors.TypeInvalidInput, "PROFILE_FACT_MISSING_CONTENT",
			"core.userprofilememory", "profile fact is missing Content").With("key", f.Key)
	}
	return nil
}

// metadata keys UserProfileMemory encodes onto / decodes from a
// MemoryRecord.Metadata map. Only UserProfileMemory reads or writes these
// keys; they are not part of the Memory or MemoryStorageProvider contracts.
const (
	metaProfileKey      = "profileKey"
	metaProfileCategory = "profileCategory"
)

// UserProfileMemory implements SPEC-0037: storing and retrieving durable
// facts about the user on top of a SPEC-0034 Memory. It is safe for
// concurrent use.
type UserProfileMemory struct {
	memory Memory

	mu    sync.Mutex
	byKey map[string]string // Key -> record ID
}

// NewUserProfileMemory creates a UserProfileMemory backed by memory. memory
// must not be nil.
func NewUserProfileMemory(memory Memory) *UserProfileMemory {
	return &UserProfileMemory{
		memory: memory,
		byKey:  make(map[string]string),
	}
}

// recordToFact converts a MemoryRecord UserProfileMemory previously stored
// back into a ProfileFact, decoding the metadata keys Remember encoded.
func recordToFact(rec MemoryRecord) ProfileFact {
	fact := ProfileFact{
		ID:        rec.ID,
		Content:   rec.Content,
		CreatedAt: rec.CreatedAt,
		UpdatedAt: rec.UpdatedAt,
	}
	if rec.Metadata == nil {
		return fact
	}
	if v, ok := rec.Metadata[metaProfileKey].(string); ok {
		fact.Key = v
	}
	if v, ok := rec.Metadata[metaProfileCategory].(string); ok {
		fact.Category = ProfileCategory(v)
	}
	if len(rec.Metadata) > 0 {
		userMeta := make(map[string]any, len(rec.Metadata))
		for k, v := range rec.Metadata {
			if k == metaProfileKey || k == metaProfileCategory {
				continue
			}
			userMeta[k] = v
		}
		if len(userMeta) > 0 {
			fact.Metadata = userMeta
		}
	}
	return fact
}

// Remember validates and stores fact, keyed by fact.Key. If a fact is
// already remembered under that Key, it is replaced in place (via
// Memory.Update) rather than stored as a new, duplicate record - satisfying
// SPEC-0037's "updates replace outdated information" requirement. It
// returns the stored fact with its assigned ID and provider-set timestamps
// filled in.
func (p *UserProfileMemory) Remember(ctx context.Context, fact ProfileFact) (ProfileFact, error) {
	if err := fact.Validate(); err != nil {
		return ProfileFact{}, err
	}

	metadata := make(map[string]any, len(fact.Metadata)+2)
	for k, v := range fact.Metadata {
		metadata[k] = v
	}
	metadata[metaProfileKey] = fact.Key
	metadata[metaProfileCategory] = string(fact.Category)

	p.mu.Lock()
	existingID, exists := p.byKey[fact.Key]
	p.mu.Unlock()

	id := existingID
	if exists {
		if err := p.memory.Update(ctx, MemoryRecord{
			ID:       existingID,
			Type:     MemoryTypeUserProfile,
			Content:  fact.Content,
			Metadata: metadata,
		}); err != nil {
			return ProfileFact{}, err
		}
	} else {
		stored, err := p.memory.Store(ctx, MemoryRecord{
			Type:     MemoryTypeUserProfile,
			Content:  fact.Content,
			Metadata: metadata,
		})
		if err != nil {
			return ProfileFact{}, err
		}
		id = stored
	}

	rec, err := p.memory.Retrieve(ctx, id)
	if err != nil {
		return ProfileFact{}, err
	}

	p.mu.Lock()
	p.byKey[fact.Key] = id
	p.mu.Unlock()

	return recordToFact(rec), nil
}

// Fact returns the ProfileFact currently remembered under key, or a
// packages/errors error typed TypeNotFound if nothing is remembered under
// that key.
func (p *UserProfileMemory) Fact(ctx context.Context, key string) (ProfileFact, error) {
	p.mu.Lock()
	id, ok := p.byKey[key]
	p.mu.Unlock()
	if !ok {
		return ProfileFact{}, errors.New(errors.TypeNotFound, "PROFILE_FACT_NOT_FOUND", "core.userprofilememory",
			"no profile fact is remembered under this key").With("key", key)
	}

	rec, err := p.memory.Retrieve(ctx, id)
	if err != nil {
		return ProfileFact{}, err
	}
	return recordToFact(rec), nil
}

// Facts returns every fact currently remembered, ordered by Key for a
// deterministic result - e.g. to assemble a user profile summary for an
// agent's context.
func (p *UserProfileMemory) Facts(ctx context.Context) ([]ProfileFact, error) {
	p.mu.Lock()
	ids := make(map[string]string, len(p.byKey))
	for k, v := range p.byKey {
		ids[k] = v
	}
	p.mu.Unlock()

	keys := make([]string, 0, len(ids))
	for k := range ids {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	facts := make([]ProfileFact, 0, len(keys))
	for _, k := range keys {
		rec, err := p.memory.Retrieve(ctx, ids[k])
		if err != nil {
			return nil, err
		}
		facts = append(facts, recordToFact(rec))
	}
	return facts, nil
}
