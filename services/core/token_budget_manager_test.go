package core

import (
	"bytes"
	"context"
	"strings"
	"testing"

	cfgpkg "jarvis-pa/packages/config"
	"jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
)

// testModelConfig is defined in model_router_test.go and reused here: a
// ModelConfig with "general"/"coding"/"fast" Models, DefaultModel "general",
// and an AgentModels override of "developer-agent" -> "coding".

// --- Testing criterion 1: token usage is tracked ---

func TestBudgetManager_Record_TracksCumulativeUsage(t *testing.T) {
	b := NewBudgetManager(testModelConfig())
	ctx := context.Background()

	report, err := b.Record(ctx, "assistant", TokenUsage{InputTokens: 100, OutputTokens: 50})
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if report.Usage.InputTokens != 100 || report.Usage.OutputTokens != 50 {
		t.Fatalf("Record() first call usage = %+v, want {100 50}", report.Usage)
	}

	report, err = b.Record(ctx, "assistant", TokenUsage{InputTokens: 30, OutputTokens: 20})
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if report.Usage.InputTokens != 130 || report.Usage.OutputTokens != 70 {
		t.Fatalf("Record() second call cumulative usage = %+v, want {130 70}", report.Usage)
	}
	if report.Usage.Total() != 200 {
		t.Fatalf("Usage.Total() = %d, want 200", report.Usage.Total())
	}

	if got := b.Report("assistant"); got != report {
		t.Fatalf("Report(%q) = %+v, want %+v", "assistant", got, report)
	}
}

func TestBudgetManager_Record_TracksSeparatelyPerAgent(t *testing.T) {
	b := NewBudgetManager(testModelConfig())
	ctx := context.Background()

	if _, err := b.Record(ctx, "a", TokenUsage{InputTokens: 10}); err != nil {
		t.Fatalf("Record(a) returned error: %v", err)
	}
	if _, err := b.Record(ctx, "b", TokenUsage{InputTokens: 999}); err != nil {
		t.Fatalf("Record(b) returned error: %v", err)
	}

	if got := b.Report("a").Usage.InputTokens; got != 10 {
		t.Fatalf("agent a InputTokens = %d, want 10 (should not see agent b's usage)", got)
	}
}

func TestBudgetManager_Reset_ClearsUsage(t *testing.T) {
	b := NewBudgetManager(testModelConfig())
	ctx := context.Background()

	if _, err := b.Record(ctx, "assistant", TokenUsage{InputTokens: 500}); err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	b.Reset("assistant")

	if got := b.Report("assistant"); got != (BudgetReport{}) {
		t.Fatalf("Report() after Reset = %+v, want zero value", got)
	}

	report, err := b.Record(ctx, "assistant", TokenUsage{InputTokens: 1})
	if err != nil {
		t.Fatalf("Record after Reset returned error: %v", err)
	}
	if report.Usage.InputTokens != 1 {
		t.Fatalf("Record after Reset usage = %+v, want {InputTokens:1}", report.Usage)
	}
}

func TestBudgetManager_Report_UnknownAgentReturnsZeroValue(t *testing.T) {
	b := NewBudgetManager(testModelConfig())
	if got := b.Report("never-recorded"); got != (BudgetReport{}) {
		t.Fatalf("Report() for unrecorded agent = %+v, want zero value", got)
	}
}

// --- Testing criterion 2: limits trigger correctly ---

func TestBudgetManager_Record_StatusTransitionsAcrossThresholds(t *testing.T) {
	provider := &stubProvider{models: []ModelInfo{{Name: "qwen", ContextSize: 100}}}
	b := NewBudgetManager(testModelConfig(), WithBudgetProvider(provider))
	ctx := context.Background()

	report, err := b.Record(ctx, "assistant", TokenUsage{InputTokens: 10})
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if report.Status != BudgetOK {
		t.Fatalf("Status after 10/100 tokens = %q, want %q", report.Status, BudgetOK)
	}
	if report.Limit != 100 {
		t.Fatalf("Limit = %d, want 100 (from Provider.ListModels ContextSize)", report.Limit)
	}

	report, err = b.Record(ctx, "assistant", TokenUsage{InputTokens: 75}) // total 85/100 = 0.85 >= 0.8
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if report.Status != BudgetWarning {
		t.Fatalf("Status after 85/100 tokens = %q, want %q", report.Status, BudgetWarning)
	}

	report, err = b.Record(ctx, "assistant", TokenUsage{InputTokens: 20}) // total 105/100
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if report.Status != BudgetExceeded {
		t.Fatalf("Status after 105/100 tokens = %q, want %q", report.Status, BudgetExceeded)
	}
}

func TestBudgetManager_Record_LogsWarningOnlyWhenNotOK(t *testing.T) {
	provider := &stubProvider{models: []ModelInfo{{Name: "qwen", ContextSize: 100}}}
	var buf bytes.Buffer
	log := logger.New("test", logger.WithOutput(&buf))
	b := NewBudgetManager(testModelConfig(), WithBudgetProvider(provider), WithBudgetLogger(log))
	ctx := context.Background()

	if _, err := b.Record(ctx, "assistant", TokenUsage{InputTokens: 10}); err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("Record() at BudgetOK logged output %q, want nothing logged", buf.String())
	}

	if _, err := b.Record(ctx, "assistant", TokenUsage{InputTokens: 200}); err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if !strings.Contains(buf.String(), "token budget") {
		t.Fatalf("Record() at BudgetExceeded log output = %q, want it to mention the token budget", buf.String())
	}
}

func TestBudgetManager_Record_NoLoggerRunsSilently(t *testing.T) {
	provider := &stubProvider{models: []ModelInfo{{Name: "qwen", ContextSize: 100}}}
	b := NewBudgetManager(testModelConfig(), WithBudgetProvider(provider))

	if _, err := b.Record(context.Background(), "assistant", TokenUsage{InputTokens: 200}); err != nil {
		t.Fatalf("Record with no logger configured returned error: %v", err)
	}
}

func TestBudgetManager_Record_CustomWarnThreshold(t *testing.T) {
	provider := &stubProvider{models: []ModelInfo{{Name: "qwen", ContextSize: 100}}}
	b := NewBudgetManager(testModelConfig(), WithBudgetProvider(provider), WithWarnThreshold(0.5))

	report, err := b.Record(context.Background(), "assistant", TokenUsage{InputTokens: 60})
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if report.Status != BudgetWarning {
		t.Fatalf("Status with 0.5 threshold at 60/100 tokens = %q, want %q", report.Status, BudgetWarning)
	}
}

func TestBudgetManager_Limit_FallsBackToDefaultWithoutProvider(t *testing.T) {
	b := NewBudgetManager(testModelConfig())
	limit, err := b.Limit(context.Background(), "assistant")
	if err != nil {
		t.Fatalf("Limit returned error: %v", err)
	}
	if limit != defaultContextLimit {
		t.Fatalf("Limit() = %d, want defaultContextLimit (%d)", limit, defaultContextLimit)
	}
}

func TestBudgetManager_Limit_CustomDefaultLimit(t *testing.T) {
	b := NewBudgetManager(testModelConfig(), WithDefaultLimit(2048))
	limit, err := b.Limit(context.Background(), "assistant")
	if err != nil {
		t.Fatalf("Limit returned error: %v", err)
	}
	if limit != 2048 {
		t.Fatalf("Limit() = %d, want 2048", limit)
	}
}

func TestBudgetManager_Limit_UsesProviderContextSizeForResolvedModel(t *testing.T) {
	provider := &stubProvider{models: []ModelInfo{
		{Name: "qwen", ContextSize: 8192},
		{Name: "qwen-coder", ContextSize: 16384},
	}}
	b := NewBudgetManager(testModelConfig(), WithBudgetProvider(provider))

	limit, err := b.Limit(context.Background(), "developer-agent") // AgentModels override -> "coding" -> qwen-coder
	if err != nil {
		t.Fatalf("Limit returned error: %v", err)
	}
	if limit != 16384 {
		t.Fatalf("Limit(developer-agent) = %d, want 16384 (qwen-coder's ContextSize)", limit)
	}

	limit, err = b.Limit(context.Background(), "assistant") // no override -> DefaultModel "general" -> qwen
	if err != nil {
		t.Fatalf("Limit returned error: %v", err)
	}
	if limit != 8192 {
		t.Fatalf("Limit(assistant) = %d, want 8192 (qwen's ContextSize)", limit)
	}
}

func TestBudgetManager_Limit_FallsBackWhenProviderReportsNoMatch(t *testing.T) {
	provider := &stubProvider{models: []ModelInfo{{Name: "some-other-model", ContextSize: 8192}}}
	b := NewBudgetManager(testModelConfig(), WithBudgetProvider(provider))

	limit, err := b.Limit(context.Background(), "assistant")
	if err != nil {
		t.Fatalf("Limit returned error: %v", err)
	}
	if limit != defaultContextLimit {
		t.Fatalf("Limit() = %d, want defaultContextLimit (%d) when provider has no matching model", limit, defaultContextLimit)
	}
}

func TestBudgetManager_Limit_FallsBackWhenListModelsFails(t *testing.T) {
	provider := &stubProvider{err: errors.New(errors.TypeUnavailable, "TEST_LIST_MODELS_FAILED", "core.test", "list models failed")}
	b := NewBudgetManager(testModelConfig(), WithBudgetProvider(provider))

	limit, err := b.Limit(context.Background(), "assistant")
	if err != nil {
		t.Fatalf("Limit returned error: %v", err)
	}
	if limit != defaultContextLimit {
		t.Fatalf("Limit() = %d, want defaultContextLimit (%d) when ListModels fails", limit, defaultContextLimit)
	}
}

func TestBudgetManager_Record_UnresolvableModelReturnsError(t *testing.T) {
	b := NewBudgetManager(cfgpkg.ModelConfig{})
	_, err := b.Record(context.Background(), "assistant", TokenUsage{InputTokens: 1})
	if err == nil {
		t.Fatal("Record with an unresolvable model should return an error")
	}
	if !errors.Is(err, errors.TypeNotFound) {
		t.Fatalf("Record() error = %v, want a packages/errors TypeNotFound error", err)
	}
}

// --- Testing criterion 3: reduction strategies execute ---

func TestBudgetManager_ReduceContext_TrimsToResolvedLimit(t *testing.T) {
	provider := &stubProvider{models: []ModelInfo{{Name: "qwen", ContextSize: 5}}}
	b := NewBudgetManager(testModelConfig(), WithBudgetProvider(provider),
		WithBudgetWindowManager(NewWindowManager(WithTokenEstimator(func(s string) int { return len(s) }))))

	c := Context{Items: []ContextItem{
		{Section: ContextSectionUserMessage, Content: "hi"},               // critical, 2 tokens
		{Section: ContextSectionConversationHistory, Content: "abcdefgh"}, // normal, 8 tokens - won't fit
	}}

	reduced, usage, err := b.ReduceContext(context.Background(), "assistant", c)
	if err != nil {
		t.Fatalf("ReduceContext returned error: %v", err)
	}
	if usage.Budget != 5 {
		t.Fatalf("Usage.Budget = %d, want 5 (resolved Limit)", usage.Budget)
	}
	if len(reduced.Items) != 1 || reduced.Items[0].Section != ContextSectionUserMessage {
		t.Fatalf("reduced.Items = %+v, want only the critical UserMessage item kept", reduced.Items)
	}
	if len(reduced.Truncated) == 0 {
		t.Fatal("reduced.Truncated should record the dropped ConversationHistory section")
	}
}

func TestBudgetManager_ReduceContext_DefaultsWindowManagerWhenNilOptionGiven(t *testing.T) {
	b := NewBudgetManager(testModelConfig(), WithBudgetWindowManager(nil))
	c := Context{Items: []ContextItem{{Section: ContextSectionUserMessage, Content: "hi"}}}

	if _, _, err := b.ReduceContext(context.Background(), "assistant", c); err != nil {
		t.Fatalf("ReduceContext with WithBudgetWindowManager(nil) returned error: %v", err)
	}
}

func TestBudgetManager_ReduceContext_UnresolvableModelReturnsError(t *testing.T) {
	b := NewBudgetManager(cfgpkg.ModelConfig{})
	if _, _, err := b.ReduceContext(context.Background(), "assistant", Context{}); err == nil {
		t.Fatal("ReduceContext with an unresolvable model should return an error")
	}
}

// --- Supporting behavior: token estimation, nil-option fallbacks ---

func TestBudgetManager_EstimateTokens_DefaultsToWordCountHeuristic(t *testing.T) {
	b := NewBudgetManager(testModelConfig())
	if got := b.EstimateTokens("abcd"); got != 1 {
		t.Fatalf("EstimateTokens(%q) = %d, want 1", "abcd", got)
	}
}

func TestBudgetManager_EstimateTokens_CustomEstimator(t *testing.T) {
	b := NewBudgetManager(testModelConfig(), WithBudgetTokenEstimator(func(s string) int { return len(s) }))
	if got := b.EstimateTokens("abcd"); got != 4 {
		t.Fatalf("EstimateTokens(%q) = %d, want 4", "abcd", got)
	}
}

func TestBudgetManager_EstimateTokens_NilOptionFallsBackToDefault(t *testing.T) {
	b := NewBudgetManager(testModelConfig(), WithBudgetTokenEstimator(nil))
	if got := b.EstimateTokens(""); got != 0 {
		t.Fatalf("EstimateTokens(\"\") = %d, want 0", got)
	}
}
