// agent_context_builder.go implements SPEC-0023: the Agent Context Builder -
// the system that assembles the context an Agent's Execute logic (SPEC-0018)
// needs before running, most naturally as an ExecutionLoop's ContextAnalyzer
// (agent_execution_loop.go, SPEC-0022's Analyze Context stage).
//
// Two of SPEC-0023's six listed context inputs - Memories and Available
// tools - have no owning system yet (Memory layer SPEC-0034 onward and Tool
// Registry SPEC-0043..0045 are both still Planned), so ContextInput accepts
// them as caller-resolved []string identifiers rather than fetching them
// itself, mirroring AgentMetadata.Tools/MemoryAccess's existing precedent
// (agent.go, SPEC-0018) of leaving what those identifiers resolve to as a
// later system's concern. Likewise, "Support token limits" here is a simple
// size budget with a pluggable estimator standing in for a real tokenizer -
// tracking real token usage and prioritizing what to keep under a model's
// actual limit is SPEC-0032 Context Window Manager and SPEC-0033 Token
// Budget Manager's job, both also still Planned; this builder only needs to
// not silently produce an unbounded context.
package core

import (
	"fmt"
	"strings"

	"jarvis-pa/packages/logger"
	types "jarvis-pa/packages/shared-types"
)

// ContextSection identifies which of SPEC-0023's six listed context inputs a
// ContextItem belongs to.
type ContextSection string

const (
	ContextSectionUserMessage         ContextSection = "user_message"
	ContextSectionConversationHistory ContextSection = "conversation_history"
	ContextSectionMemories            ContextSection = "memories"
	ContextSectionTask                ContextSection = "task"
	ContextSectionAvailableTools      ContextSection = "available_tools"
	ContextSectionPreviousResults     ContextSection = "previous_results"
)

// contextSectionOrder is the fixed order ContextBuilder assembles sections
// in - the same order SPEC-0023's Requirements list them - so "maintain
// ordering" has one canonical answer independent of the order a caller
// populates ContextInput's fields in.
var contextSectionOrder = []ContextSection{
	ContextSectionUserMessage,
	ContextSectionConversationHistory,
	ContextSectionMemories,
	ContextSectionTask,
	ContextSectionAvailableTools,
	ContextSectionPreviousResults,
}

// ContextItem is one piece of assembled context. Content is the item's
// rendered text, used for both display and size estimation; Data optionally
// carries the original structured value (e.g. the *types.Task for a
// ContextSectionTask item) for a caller that needs more than text.
type ContextItem struct {
	Section ContextSection
	Content string
	Data    any
}

// Context is the ordered, size-bounded result ContextBuilder.Build produces.
// Items are in contextSectionOrder, and within a Section in the order they
// were supplied. Truncated lists every Section that lost at least one item
// (or was omitted entirely) to a configured size limit.
type Context struct {
	Items     []ContextItem
	TotalSize int
	Truncated []ContextSection
}

// ContextInput carries the raw, already-resolved material a ContextBuilder
// assembles into a Context. Memories and AvailableTools are bare identifiers
// (see package doc comment); ConversationHistory is pre-rendered turns for
// the same reason - no chat-turn type exists yet (Conversation Memory,
// SPEC-0036, is still Planned). PreviousResults reuses HistoryRecord
// (task_history.go, SPEC-0017), which already is a Task's recorded
// execution timeline.
type ContextInput struct {
	UserMessage         string
	ConversationHistory []string
	Memories            []string
	Task                *types.Task
	AvailableTools      []string
	PreviousResults     []HistoryRecord
}

// SizeEstimator estimates a ContextItem's size for the token-limit
// requirement. Defaults to defaultSizeEstimator (word count), a stand-in for
// a real tokenizer until the LLM layer (SPEC-0026 onward) provides one.
type SizeEstimator func(ContextItem) int

// defaultSizeEstimator counts words in item.Content as a rough,
// dependency-free proxy for token count.
func defaultSizeEstimator(item ContextItem) int {
	return len(strings.Fields(item.Content))
}

// ContextBuilder assembles a Context from a ContextInput. ContextBuilder is
// safe for concurrent use - it holds no per-build state.
type ContextBuilder struct {
	estimate SizeEstimator
	maxSize  int
	log      *logger.Logger
}

// ContextBuilderOption configures a ContextBuilder created by
// NewContextBuilder.
type ContextBuilderOption func(*ContextBuilder)

// WithSizeEstimator overrides the SizeEstimator used to size items and
// enforce a configured WithMaxSize budget. Defaults to defaultSizeEstimator.
func WithSizeEstimator(e SizeEstimator) ContextBuilderOption {
	return func(b *ContextBuilder) { b.estimate = e }
}

// WithMaxSize caps a built Context's TotalSize (in SizeEstimator units).
// Items are added in contextSectionOrder until the next item would exceed
// max, at which point building stops - later sections (Available tools,
// Previous execution results) are the first to be cut, since they sort
// last. Zero (the default) means unlimited.
func WithMaxSize(max int) ContextBuilderOption {
	return func(b *ContextBuilder) { b.maxSize = max }
}

// WithContextBuilderLogger attaches a Logger used to report truncation.
// Optional; a builder with no logger runs silently.
func WithContextBuilderLogger(log *logger.Logger) ContextBuilderOption {
	return func(b *ContextBuilder) { b.log = log }
}

// NewContextBuilder creates a ready-to-use ContextBuilder.
func NewContextBuilder(opts ...ContextBuilderOption) *ContextBuilder {
	b := &ContextBuilder{estimate: defaultSizeEstimator}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Build assembles input into an ordered Context. Empty or nil fields
// contribute no items (SPEC-0023's "avoid unnecessary context"
// requirement) - Build never errors, since a Context with fewer items than
// requested (whether from empty input or a size limit) is still a valid
// result.
func (b *ContextBuilder) Build(input ContextInput) Context {
	estimate := b.estimate
	if estimate == nil {
		estimate = defaultSizeEstimator
	}

	sections := map[ContextSection][]ContextItem{
		ContextSectionUserMessage:         userMessageItems(input.UserMessage),
		ContextSectionConversationHistory: stringItems(ContextSectionConversationHistory, input.ConversationHistory),
		ContextSectionMemories:            stringItems(ContextSectionMemories, input.Memories),
		ContextSectionTask:                taskItems(input.Task),
		ContextSectionAvailableTools:      stringItems(ContextSectionAvailableTools, input.AvailableTools),
		ContextSectionPreviousResults:     previousResultItems(input.PreviousResults),
	}

	result := Context{}
	for _, section := range contextSectionOrder {
		items := sections[section]
		kept := 0
		for _, item := range items {
			size := estimate(item)
			if b.maxSize > 0 && result.TotalSize+size > b.maxSize {
				break
			}
			result.Items = append(result.Items, item)
			result.TotalSize += size
			kept++
		}
		if kept < len(items) {
			result.Truncated = append(result.Truncated, section)
		}
	}

	if b.log != nil && len(result.Truncated) > 0 {
		sections := make([]string, len(result.Truncated))
		for i, s := range result.Truncated {
			sections[i] = string(s)
		}
		b.log.Warn("context truncated to fit size limit", map[string]any{
			"maxSize":   b.maxSize,
			"totalSize": result.TotalSize,
			"sections":  sections,
		})
	}

	return result
}

// userMessageItems renders msg as a single ContextItem, or none if msg is
// blank.
func userMessageItems(msg string) []ContextItem {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return nil
	}
	return []ContextItem{{Section: ContextSectionUserMessage, Content: msg}}
}

// stringItems renders each non-blank entry of values as its own ContextItem
// tagged section.
func stringItems(section ContextSection, values []string) []ContextItem {
	items := make([]ContextItem, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		items = append(items, ContextItem{Section: section, Content: v})
	}
	return items
}

// taskItems renders task as a single ContextItem, or none if task is nil.
func taskItems(task *types.Task) []ContextItem {
	if task == nil {
		return nil
	}
	content := fmt.Sprintf("task %q (%s): %s [status=%s]", task.ID, task.Type, task.Title, task.Status)
	if task.Description != "" {
		content += " - " + task.Description
	}
	return []ContextItem{{Section: ContextSectionTask, Content: content, Data: task}}
}

// previousResultItems renders each HistoryRecord as its own ContextItem.
func previousResultItems(records []HistoryRecord) []ContextItem {
	items := make([]ContextItem, 0, len(records))
	for _, r := range records {
		content := fmt.Sprintf("[%s] task %q: %s", r.Timestamp.Format("15:04:05"), r.TaskID, r.EventType)
		items = append(items, ContextItem{Section: ContextSectionPreviousResults, Content: content, Data: r})
	}
	return items
}
