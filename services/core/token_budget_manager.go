// token_budget_manager.go implements SPEC-0033: the Token Budget Manager -
// the seventh spec of Phase 4 Intelligence's LLM branch. Where WindowManager
// (context_window_manager.go, SPEC-0032) fits an already-built Context to a
// caller-supplied budget int but deliberately does not decide what that
// budget should be, BudgetManager owns that resolution: given an agent name,
// it resolves the agent's configured Model (packages/config's
// ModelConfig.ModelFor, SPEC-0028) and looks up that model's real context
// window size via Provider.ListModels' ModelInfo.ContextSize (SPEC-0026/27),
// falling back to a conservative default when no Provider is configured or
// the model reports no size - covering the "Model limits" tracking
// requirement.
//
// BudgetManager also accumulates "Input tokens" / "Output tokens" usage per
// agent across calls (the "Track ... Context usage" requirement - a
// session's running total against its resolved limit, distinct from
// WindowManager.Usage's single-Fit-call accounting), classifies that total
// against the resolved limit into a BudgetStatus (the "Budget warnings"
// requirement, logged via an optional packages/logger.Logger exactly as
// WindowManager.Fit logs trimming), and exposes ReduceContext, a thin
// wrapper composing Limit with WindowManager.Fit (the "Context reduction
// strategies" requirement) so a caller doesn't have to resolve a budget
// itself before trimming.
//
// TokenEstimator (the "Token estimation" requirement) is SPEC-0032's own
// TokenEstimator type, reused rather than redefined, for the same reason
// BudgetManager reuses WindowManager for reduction: this spec resolves and
// tracks budgets, it does not reinvent WindowManager's own concerns.
package core

import (
	"context"
	"sync"

	cfgpkg "jarvis-pa/packages/config"
	"jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
)

// defaultContextLimit is the context window size, in tokens, BudgetManager
// assumes for a model when no Provider is configured or the configured
// Provider's ListModels reports no ContextSize for it - a conservative
// floor common to small local models, used only as a last resort.
const defaultContextLimit = 4096

// defaultWarnThreshold is the fraction of a resolved Limit at which Record
// reports BudgetWarning instead of BudgetOK, absent a WithWarnThreshold
// override.
const defaultWarnThreshold = 0.8

// TokenUsage records an input/output token count for one accounting
// operation, e.g. one Provider.Generate/Stream call.
type TokenUsage struct {
	InputTokens  int
	OutputTokens int
}

// Total returns the combined input and output token count.
func (u TokenUsage) Total() int {
	return u.InputTokens + u.OutputTokens
}

// BudgetStatus classifies a BudgetReport's cumulative Usage against its
// Limit.
type BudgetStatus string

const (
	// BudgetOK means cumulative usage is below the warn threshold, or the
	// resolved Limit is unlimited (<= 0, mirroring WindowManager.Fit's
	// budget<=0 convention).
	BudgetOK BudgetStatus = "ok"
	// BudgetWarning means cumulative usage has reached the warn threshold
	// but not yet the Limit itself.
	BudgetWarning BudgetStatus = "warning"
	// BudgetExceeded means cumulative usage has reached or passed the
	// Limit.
	BudgetExceeded BudgetStatus = "exceeded"
)

// BudgetReport is BudgetManager's accounting snapshot for one agent: its
// resolved model name, cumulative Usage recorded via Record, the resolved
// Limit (the model's context window size), and Status comparing the two.
type BudgetReport struct {
	Model  string
	Usage  TokenUsage
	Limit  int
	Status BudgetStatus
}

// BudgetManager resolves per-agent token budgets from packages/config's
// ModelConfig and an optional Provider, tracks cumulative usage against
// those budgets, and reduces an over-budget Context via a WindowManager.
// Create one with NewBudgetManager.
type BudgetManager struct {
	cfg      cfgpkg.ModelConfig
	provider Provider
	window   *WindowManager
	estimate TokenEstimator
	log      *logger.Logger

	defaultLimit int
	warnAt       float64

	mu      sync.Mutex
	usage   map[string]TokenUsage
	reports map[string]BudgetReport
}

// BudgetManagerOption configures a BudgetManager created by
// NewBudgetManager.
type BudgetManagerOption func(*BudgetManager)

// WithBudgetProvider sets the Provider consulted for a model's real
// ContextSize via ListModels. A nil Provider (the default) means Limit
// always falls back to the configured default limit.
func WithBudgetProvider(p Provider) BudgetManagerOption {
	return func(b *BudgetManager) { b.provider = p }
}

// WithBudgetWindowManager sets the WindowManager ReduceContext delegates
// to. Defaults to a WindowManager created with NewWindowManager().
func WithBudgetWindowManager(w *WindowManager) BudgetManagerOption {
	return func(b *BudgetManager) { b.window = w }
}

// WithBudgetTokenEstimator overrides the TokenEstimator used by
// EstimateTokens. Defaults to defaultTokenEstimator, the same default
// WindowManager uses.
func WithBudgetTokenEstimator(e TokenEstimator) BudgetManagerOption {
	return func(b *BudgetManager) { b.estimate = e }
}

// WithBudgetLogger attaches a Logger used to report budget warnings.
// Optional; a BudgetManager with no logger runs silently.
func WithBudgetLogger(log *logger.Logger) BudgetManagerOption {
	return func(b *BudgetManager) { b.log = log }
}

// WithDefaultLimit overrides defaultContextLimit, the fallback Limit used
// when no Provider is configured or the model's ContextSize is unknown.
func WithDefaultLimit(n int) BudgetManagerOption {
	return func(b *BudgetManager) { b.defaultLimit = n }
}

// WithWarnThreshold overrides defaultWarnThreshold, the fraction of a
// resolved Limit at which Record reports BudgetWarning.
func WithWarnThreshold(f float64) BudgetManagerOption {
	return func(b *BudgetManager) { b.warnAt = f }
}

// NewBudgetManager creates a ready-to-use BudgetManager over cfg's model
// definitions.
func NewBudgetManager(cfg cfgpkg.ModelConfig, opts ...BudgetManagerOption) *BudgetManager {
	b := &BudgetManager{
		cfg:          cfg,
		window:       NewWindowManager(),
		estimate:     defaultTokenEstimator,
		defaultLimit: defaultContextLimit,
		warnAt:       defaultWarnThreshold,
		usage:        make(map[string]TokenUsage),
		reports:      make(map[string]BudgetReport),
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// resolveModel resolves agentName's configured Model via ModelConfig.ModelFor
// (SPEC-0028), wrapping any failure into a packages/errors Error (ModelFor
// itself returns a plain fmt.Errorf, since packages/config has no
// packages/errors dependency) so callers get the same typed-error contract
// every other services/core component provides.
func (b *BudgetManager) resolveModel(agentName string) (cfgpkg.Model, error) {
	model, err := b.cfg.ModelFor(agentName)
	if err != nil {
		return cfgpkg.Model{}, errors.Wrap(err, errors.TypeNotFound, "BUDGET_MANAGER_MODEL_UNRESOLVED", "core.tokenbudgetmanager",
			"could not resolve a model to budget for").With("agent", agentName)
	}
	return model, nil
}

// resolveLimit resolves model's context window size: the Provider's
// reported ModelInfo.ContextSize if one is configured and reports a
// positive size for model.Name, otherwise the configured default limit.
// A ListModels failure is treated the same as no match, mirroring
// ModelRouter.isAvailable's "don't guess at an unreachable provider's
// state" precedent (SPEC-0029).
func (b *BudgetManager) resolveLimit(ctx context.Context, model cfgpkg.Model) int {
	if b.provider != nil {
		if models, err := b.provider.ListModels(ctx); err == nil {
			for _, m := range models {
				if m.Name == model.Name && m.ContextSize > 0 {
					return m.ContextSize
				}
			}
		}
	}
	defaultLimit := b.defaultLimit
	if defaultLimit <= 0 {
		defaultLimit = defaultContextLimit
	}
	return defaultLimit
}

// Limit resolves the context window size, in tokens, agentName's configured
// model should be budgeted against (the "Track ... Model limits"
// requirement). It returns an error only if agentName's model cannot be
// resolved at all (ModelConfig.ModelFor).
func (b *BudgetManager) Limit(ctx context.Context, agentName string) (int, error) {
	model, err := b.resolveModel(agentName)
	if err != nil {
		return 0, err
	}
	return b.resolveLimit(ctx, model), nil
}

// EstimateTokens estimates s's token count using the configured
// TokenEstimator (the "Token estimation" requirement).
func (b *BudgetManager) EstimateTokens(s string) int {
	estimate := b.estimate
	if estimate == nil {
		estimate = defaultTokenEstimator
	}
	return estimate(s)
}

// classifyBudget compares used against limit, returning BudgetOK for a
// non-positive (unlimited) limit.
func classifyBudget(used, limit int, warnAt float64) BudgetStatus {
	if limit <= 0 {
		return BudgetOK
	}
	if used >= limit {
		return BudgetExceeded
	}
	if float64(used) >= warnAt*float64(limit) {
		return BudgetWarning
	}
	return BudgetOK
}

// Record adds usage to agentName's cumulative token counters (the "Track
// Input tokens / Output tokens" requirement), resolves agentName's current
// Limit, classifies the resulting total against it, logs a warning if the
// resulting Status is not BudgetOK (the "Budget warnings" requirement), and
// returns the resulting BudgetReport. Record returns an error only if
// agentName's model cannot be resolved.
func (b *BudgetManager) Record(ctx context.Context, agentName string, usage TokenUsage) (BudgetReport, error) {
	model, err := b.resolveModel(agentName)
	if err != nil {
		return BudgetReport{}, err
	}
	limit := b.resolveLimit(ctx, model)
	warnAt := b.warnAt
	if warnAt <= 0 {
		warnAt = defaultWarnThreshold
	}

	b.mu.Lock()
	total := b.usage[agentName]
	total.InputTokens += usage.InputTokens
	total.OutputTokens += usage.OutputTokens
	b.usage[agentName] = total
	report := BudgetReport{
		Model:  model.Name,
		Usage:  total,
		Limit:  limit,
		Status: classifyBudget(total.Total(), limit, warnAt),
	}
	b.reports[agentName] = report
	b.mu.Unlock()

	if b.log != nil && report.Status != BudgetOK {
		b.log.Warn("token budget threshold reached", map[string]any{
			"agent":  agentName,
			"model":  report.Model,
			"used":   report.Usage.Total(),
			"limit":  report.Limit,
			"status": string(report.Status),
		})
	}
	return report, nil
}

// Report returns the most recently computed BudgetReport for agentName
// without recording new usage or re-resolving its Limit. The zero
// BudgetReport (Status "") is returned if Record has never been called for
// agentName.
func (b *BudgetManager) Report(agentName string) BudgetReport {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.reports[agentName]
}

// Reset clears agentName's recorded usage and last BudgetReport, e.g. when
// starting a new conversation.
func (b *BudgetManager) Reset(agentName string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.usage, agentName)
	delete(b.reports, agentName)
}

// ReduceContext resolves agentName's Limit and fits c to it using the
// configured WindowManager (the "Context reduction strategies" requirement),
// returning the trimmed Context and the WindowManager's Usage report.
// ReduceContext returns an error only if agentName's model cannot be
// resolved.
func (b *BudgetManager) ReduceContext(ctx context.Context, agentName string, c Context) (Context, Usage, error) {
	limit, err := b.Limit(ctx, agentName)
	if err != nil {
		return Context{}, Usage{}, err
	}
	window := b.window
	if window == nil {
		window = NewWindowManager()
	}
	reduced, usage := window.Fit(c, limit)
	return reduced, usage, nil
}
