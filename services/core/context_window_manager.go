// context_window_manager.go implements SPEC-0032: the Context Window
// Manager. Where ContextBuilder (agent_context_builder.go, SPEC-0023)
// assembles a Context and only offers a word-count SizeEstimator plus
// "cut later sections first" truncation as a placeholder, WindowManager is
// the real token-accounting and prioritization layer agent_context_builder.go's
// own package doc comment explicitly deferred to this spec: Fit takes an
// already-built Context and a token budget, and returns the
// highest-ContextPriority subset of it that fits, plus a Usage report of
// what that cost.
//
// WindowManager deliberately does not decide what the token budget itself
// should be for a given model or agent - packages/config's Model.MaxTokens
// (SPEC-0028) caps generated *output* length, not input context size, and
// SPEC-0028's own build history notes resolving a model's actual context
// window size is left to SPEC-0032/SPEC-0033. Owning that resolution (model
// lookup, defaults, per-agent overrides) is SPEC-0033 Token Budget
// Manager's job, still Planned; WindowManager only needs a caller-supplied
// budget int, so it has nothing to gain from reaching into config itself.
//
// Likewise, TokenEstimator is a character-count heuristic (~4 chars/token,
// a standard rough approximation for English text), not a real
// model-specific tokenizer - Provider (SPEC-0026) has no CountTokens/tokenize
// method yet, and ADR-0004's local-only Ollama runtime has no
// tokenize-without-generating endpoint this layer can call. Callers with a
// real tokenizer can supply one via WithTokenEstimator; this default is a
// stand-in, exactly as agent_context_builder.go's own word-count
// defaultSizeEstimator was for SPEC-0023.
package core

import (
	"sort"
	"strings"

	"jarvis-pa/packages/logger"
)

// ContextPriority ranks how important a ContextItem is to keep when a
// Context must be trimmed to fit a token budget. Higher survives longer.
// Named ContextPriority rather than Priority to avoid colliding with
// task_queue.go's existing TaskPriority constants (SPEC-0013), which share
// the same Low/Normal/High/Critical vocabulary for an unrelated concept.
type ContextPriority int

const (
	ContextPriorityLow ContextPriority = iota
	ContextPriorityNormal
	ContextPriorityHigh
	ContextPriorityCritical
)

// TokenEstimator estimates how many LLM tokens s would consume. Defaults to
// defaultTokenEstimator (see package doc comment).
type TokenEstimator func(s string) int

// defaultTokenEstimator approximates token count as one token per four
// characters of trimmed content, rounded up - a common rough estimate for
// English text absent a model-specific tokenizer. Non-blank content always
// counts as at least one token.
func defaultTokenEstimator(s string) int {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return 0
	}
	n := (len(trimmed) + 3) / 4
	if n < 1 {
		n = 1
	}
	return n
}

// PriorityFunc assigns a ContextPriority to a ContextItem. Defaults to
// defaultPriority, which ranks by ContextSection.
type PriorityFunc func(ContextItem) ContextPriority

// defaultPriority ranks a ContextItem by its Section: the user's current
// message and the task being worked both must survive trimming
// (ContextPriorityCritical); memories (e.g. user preferences) materially
// change correctness and rank next (ContextPriorityHigh); conversation
// history and available tools are useful but individually droppable
// (ContextPriorityNormal); previous execution results are the most
// reconstructable from other state and rank lowest (ContextPriorityLow).
func defaultPriority(item ContextItem) ContextPriority {
	switch item.Section {
	case ContextSectionUserMessage, ContextSectionTask:
		return ContextPriorityCritical
	case ContextSectionMemories:
		return ContextPriorityHigh
	case ContextSectionConversationHistory, ContextSectionAvailableTools:
		return ContextPriorityNormal
	case ContextSectionPreviousResults:
		return ContextPriorityLow
	default:
		return ContextPriorityNormal
	}
}

// Usage reports WindowManager.Fit's token accounting for one call: Budget is
// the limit it was asked to fit within (0 meaning unlimited, mirroring
// ContextBuilder.WithMaxSize's convention), Used is the total token count of
// the items kept, and BySection breaks Used down per ContextSection for
// callers that want visibility into what is costing the most.
type Usage struct {
	Budget    int
	Used      int
	BySection map[ContextSection]int
}

// WindowManager fits a Context to a token budget, prioritizing important
// information over unnecessary context (SPEC-0032's "Must" requirements)
// rather than ContextBuilder's simpler "keep whatever came first" behavior.
// WindowManager holds no per-call state, so it is safe for concurrent use,
// like ContextBuilder.
type WindowManager struct {
	estimate TokenEstimator
	priority PriorityFunc
	log      *logger.Logger
}

// WindowManagerOption configures a WindowManager created by
// NewWindowManager.
type WindowManagerOption func(*WindowManager)

// WithTokenEstimator overrides the TokenEstimator used to size items and
// enforce a Fit call's budget. Defaults to defaultTokenEstimator.
func WithTokenEstimator(e TokenEstimator) WindowManagerOption {
	return func(w *WindowManager) { w.estimate = e }
}

// WithPriorityFunc overrides the PriorityFunc used to rank items when
// trimming is required. Defaults to defaultPriority.
func WithPriorityFunc(p PriorityFunc) WindowManagerOption {
	return func(w *WindowManager) { w.priority = p }
}

// WithWindowManagerLogger attaches a Logger used to report trimming.
// Optional; a WindowManager with no logger runs silently.
func WithWindowManagerLogger(log *logger.Logger) WindowManagerOption {
	return func(w *WindowManager) { w.log = log }
}

// NewWindowManager creates a ready-to-use WindowManager.
func NewWindowManager(opts ...WindowManagerOption) *WindowManager {
	w := &WindowManager{estimate: defaultTokenEstimator, priority: defaultPriority}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// scoredItem pairs a Context's ContextItem with its position in c.Items
// (preserved so kept items can be re-emitted in their original,
// contextSectionOrder-derived order), token cost, and Priority.
type scoredItem struct {
	item     ContextItem
	index    int
	tokens   int
	priority ContextPriority
}

// Fit returns the subset of c that fits within budget tokens, keeping the
// highest-Priority items first and, within a Priority tier, the items that
// appear later in c.Items (the more recently-added ones, e.g. the newest
// conversation turns) ahead of earlier ones. budget <= 0 means unlimited,
// matching ContextBuilder.WithMaxSize's convention - Fit returns c unchanged
// in that case. The returned Context reuses SPEC-0023's own shape (Items in
// their original order, TotalSize now in token units, Truncated listing
// every section that lost at least one item - including, via c.Truncated,
// any section an upstream ContextBuilder already dropped entirely before c
// ever reached Fit) so existing callers such as VariablesFromContext
// (SPEC-0031) keep working unmodified against Fit's output. Fit never
// errors: a Context with fewer items than requested is still a valid
// result, exactly as ContextBuilder.Build documents.
func (w *WindowManager) Fit(c Context, budget int) (Context, Usage) {
	estimate := w.estimate
	if estimate == nil {
		estimate = defaultTokenEstimator
	}
	prioritize := w.priority
	if prioritize == nil {
		prioritize = defaultPriority
	}

	scored := make([]scoredItem, len(c.Items))
	bySectionAll := make(map[ContextSection]int)
	totalTokens := 0
	for i, item := range c.Items {
		tokens := estimate(item.Content)
		scored[i] = scoredItem{item: item, index: i, tokens: tokens, priority: prioritize(item)}
		bySectionAll[item.Section] += tokens
		totalTokens += tokens
	}

	if budget <= 0 || totalTokens <= budget {
		result := c
		result.TotalSize = totalTokens
		return result, Usage{Budget: budget, Used: totalTokens, BySection: bySectionAll}
	}

	order := make([]int, len(scored))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		ia, ib := scored[order[a]], scored[order[b]]
		if ia.priority != ib.priority {
			return ia.priority > ib.priority
		}
		return ia.index > ib.index
	})

	keep := make([]bool, len(scored))
	used := 0
	for _, i := range order {
		tokens := scored[i].tokens
		if used+tokens > budget {
			continue
		}
		keep[i] = true
		used += tokens
	}

	var items []ContextItem
	bySectionKept := make(map[ContextSection]int)
	droppedSections := make(map[ContextSection]bool)
	for _, section := range c.Truncated {
		droppedSections[section] = true
	}
	for i, s := range scored {
		if keep[i] {
			items = append(items, s.item)
			bySectionKept[s.item.Section] += s.tokens
		} else {
			droppedSections[s.item.Section] = true
		}
	}

	var truncated []ContextSection
	for _, section := range contextSectionOrder {
		if droppedSections[section] {
			truncated = append(truncated, section)
		}
	}

	if w.log != nil && len(truncated) > 0 {
		sections := make([]string, len(truncated))
		for i, s := range truncated {
			sections[i] = string(s)
		}
		w.log.Warn("context window trimmed to fit token budget", map[string]any{
			"budget":   budget,
			"used":     used,
			"sections": sections,
		})
	}

	return Context{Items: items, TotalSize: used, Truncated: truncated},
		Usage{Budget: budget, Used: used, BySection: bySectionKept}
}
