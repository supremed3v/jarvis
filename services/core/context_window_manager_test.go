package core

import (
	"bytes"
	"strings"
	"testing"

	"jarvis-pa/packages/logger"
)

// TestWindowManager_Fit_ZeroBudgetIsUnlimited verifies budget <= 0 returns c
// unchanged (aside from TotalSize now being in token units), mirroring
// ContextBuilder.WithMaxSize's "0 means unlimited" convention.
func TestWindowManager_Fit_ZeroBudgetIsUnlimited(t *testing.T) {
	c := Context{Items: []ContextItem{
		{Section: ContextSectionUserMessage, Content: "hello there"},
		{Section: ContextSectionMemories, Content: "a remembered fact"},
	}}

	got, usage := NewWindowManager().Fit(c, 0)

	if len(got.Items) != 2 {
		t.Fatalf("Fit(0) Items = %+v, want both items kept", got.Items)
	}
	if len(got.Truncated) != 0 {
		t.Errorf("Fit(0) Truncated = %v, want none", got.Truncated)
	}
	if usage.Budget != 0 {
		t.Errorf("usage.Budget = %d, want 0", usage.Budget)
	}
	if usage.Used != got.TotalSize {
		t.Errorf("usage.Used = %d, want it to match got.TotalSize = %d", usage.Used, got.TotalSize)
	}
}

// TestWindowManager_Fit_UnderBudgetKeepsEverything verifies a Context that
// already fits within budget is returned whole, not trimmed unnecessarily.
func TestWindowManager_Fit_UnderBudgetKeepsEverything(t *testing.T) {
	c := Context{Items: []ContextItem{
		{Section: ContextSectionUserMessage, Content: "hi"},
	}}

	got, usage := NewWindowManager().Fit(c, 1000)

	if len(got.Items) != 1 {
		t.Fatalf("Fit Items = %+v, want the one item kept", got.Items)
	}
	if len(got.Truncated) != 0 {
		t.Errorf("Fit Truncated = %v, want none", got.Truncated)
	}
	if usage.Used == 0 {
		t.Error("usage.Used = 0, want a nonzero token count for non-blank content")
	}
}

// TestWindowManager_Fit_StaysWithinBudget verifies "Context stays within
// limits": however items are prioritized, the returned Context's summed
// token cost never exceeds the requested budget.
func TestWindowManager_Fit_StaysWithinBudget(t *testing.T) {
	c := Context{Items: []ContextItem{
		{Section: ContextSectionUserMessage, Content: strings.Repeat("critical content that is fairly long ", 5)},
		{Section: ContextSectionMemories, Content: strings.Repeat("remembered fact ", 5)},
		{Section: ContextSectionConversationHistory, Content: strings.Repeat("chat turn ", 5)},
		{Section: ContextSectionAvailableTools, Content: strings.Repeat("tool definition ", 5)},
		{Section: ContextSectionPreviousResults, Content: strings.Repeat("previous result ", 5)},
	}}

	budget := 10
	got, usage := NewWindowManager().Fit(c, budget)

	if got.TotalSize > budget {
		t.Errorf("got.TotalSize = %d, want <= budget %d", got.TotalSize, budget)
	}
	if usage.Used > budget {
		t.Errorf("usage.Used = %d, want <= budget %d", usage.Used, budget)
	}
}

// TestWindowManager_Fit_ImportantInformationRemains verifies "Important
// information remains": ContextPriorityCritical items (the user's message
// and the task) survive even when a much larger, lower-priority section
// would otherwise consume the whole budget.
func TestWindowManager_Fit_ImportantInformationRemains(t *testing.T) {
	userMsg := "what's on my calendar"
	c := Context{Items: []ContextItem{
		{Section: ContextSectionUserMessage, Content: userMsg},
		{Section: ContextSectionPreviousResults, Content: strings.Repeat("noise ", 200)},
	}}

	// Budget only large enough for the critical item, not the huge low-priority one.
	got, _ := NewWindowManager().Fit(c, defaultTokenEstimator(userMsg))

	if len(got.Items) != 1 || got.Items[0].Section != ContextSectionUserMessage {
		t.Fatalf("Fit Items = %+v, want only the ContextPriorityCritical user message kept", got.Items)
	}
	found := false
	for _, s := range got.Truncated {
		if s == ContextSectionPreviousResults {
			found = true
		}
	}
	if !found {
		t.Errorf("Truncated = %v, want it to include %q", got.Truncated, ContextSectionPreviousResults)
	}
}

// TestWindowManager_Fit_LargeConversationsKeepMostRecentTurns verifies
// "Large conversations are handled": when ConversationHistory must be
// trimmed, the most recently added turns (later in Context.Items) are kept
// over older ones, not an arbitrary or purely first-come subset.
func TestWindowManager_Fit_LargeConversationsKeepMostRecentTurns(t *testing.T) {
	c := Context{Items: []ContextItem{
		{Section: ContextSectionConversationHistory, Content: "turn AAA"},
		{Section: ContextSectionConversationHistory, Content: "turn BBB"},
		{Section: ContextSectionConversationHistory, Content: "turn CCC"},
	}}

	// Budget for exactly one turn's worth of tokens (same-length turns estimate equally).
	budget := defaultTokenEstimator("turn AAA")
	got, _ := NewWindowManager().Fit(c, budget)

	if len(got.Items) != 1 {
		t.Fatalf("Fit Items = %+v, want exactly 1 turn kept", got.Items)
	}
	if got.Items[0].Content != "turn CCC" {
		t.Errorf("Items[0].Content = %q, want the most recent turn %q", got.Items[0].Content, "turn CCC")
	}
}

// TestWindowManager_Fit_UsageTracksPerSectionTokens verifies "Track token
// usage": Usage.BySection sums to Usage.Used and reflects only kept items.
func TestWindowManager_Fit_UsageTracksPerSectionTokens(t *testing.T) {
	c := Context{Items: []ContextItem{
		{Section: ContextSectionUserMessage, Content: "hello"},
		{Section: ContextSectionMemories, Content: "a fact"},
	}}

	_, usage := NewWindowManager().Fit(c, 1000)

	sum := 0
	for _, tokens := range usage.BySection {
		sum += tokens
	}
	if sum != usage.Used {
		t.Errorf("sum of BySection = %d, want it to equal Used = %d", sum, usage.Used)
	}
	if usage.BySection[ContextSectionUserMessage] == 0 {
		t.Error("BySection[ContextSectionUserMessage] = 0, want a nonzero token count")
	}
	if usage.BySection[ContextSectionMemories] == 0 {
		t.Error("BySection[ContextSectionMemories] = 0, want a nonzero token count")
	}
}

// TestWindowManager_Fit_CustomTokenEstimator verifies WithTokenEstimator
// overrides the default heuristic.
func TestWindowManager_Fit_CustomTokenEstimator(t *testing.T) {
	calls := 0
	estimator := func(s string) int {
		calls++
		return len(strings.Fields(s))
	}

	c := Context{Items: []ContextItem{{Section: ContextSectionUserMessage, Content: "one two three"}}}
	_, usage := NewWindowManager(WithTokenEstimator(estimator)).Fit(c, 1000)

	if calls != 1 {
		t.Errorf("custom estimator called %d times, want 1", calls)
	}
	if usage.Used != 3 {
		t.Errorf("usage.Used = %d, want 3 (word count of the one item)", usage.Used)
	}
}

// TestWindowManager_Fit_CustomPriorityFunc verifies WithPriorityFunc
// overrides the default section-based ranking.
func TestWindowManager_Fit_CustomPriorityFunc(t *testing.T) {
	c := Context{Items: []ContextItem{
		{Section: ContextSectionUserMessage, Content: strings.Repeat("x", 40)},
		{Section: ContextSectionPreviousResults, Content: "keep me"},
	}}

	// Invert the default ranking: PreviousResults now outranks UserMessage.
	inverted := func(item ContextItem) ContextPriority {
		if item.Section == ContextSectionPreviousResults {
			return ContextPriorityCritical
		}
		return ContextPriorityLow
	}

	got, _ := NewWindowManager(WithPriorityFunc(inverted)).Fit(c, defaultTokenEstimator("keep me"))

	if len(got.Items) != 1 || got.Items[0].Section != ContextSectionPreviousResults {
		t.Fatalf("Fit Items = %+v, want only the custom-Critical PreviousResults item kept", got.Items)
	}
}

// TestWindowManager_Fit_NilOptionsFallBackToDefaults verifies a nil
// TokenEstimator/PriorityFunc (whether from WithTokenEstimator(nil)/
// WithPriorityFunc(nil) or a zero-value WindowManager{}) falls back to the
// package defaults instead of panicking, mirroring ContextBuilder's same
// nil-safety precedent.
func TestWindowManager_Fit_NilOptionsFallBackToDefaults(t *testing.T) {
	c := Context{Items: []ContextItem{{Section: ContextSectionUserMessage, Content: "one two three four"}}}
	want := defaultTokenEstimator("one two three four")

	viaOptions := NewWindowManager(WithTokenEstimator(nil), WithPriorityFunc(nil))
	_, usage := viaOptions.Fit(c, 1000)
	if usage.Used != want {
		t.Errorf("nil options: usage.Used = %d, want %d (default estimator)", usage.Used, want)
	}

	viaZeroValue, _ := (&WindowManager{}).Fit(c, 1000)
	if viaZeroValue.TotalSize != want {
		t.Errorf("zero-value WindowManager{}: TotalSize = %d, want %d (default estimator)", viaZeroValue.TotalSize, want)
	}
}

// TestWindowManager_Fit_LogsTrimming verifies WithWindowManagerLogger
// reports trimming, mirroring ContextBuilder's same logging precedent.
func TestWindowManager_Fit_LogsTrimming(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New("test", logger.WithOutput(&buf))

	c := Context{Items: []ContextItem{
		{Section: ContextSectionPreviousResults, Content: strings.Repeat("noise ", 20)},
	}}

	NewWindowManager(WithWindowManagerLogger(log)).Fit(c, 1)

	if !strings.Contains(buf.String(), "context window trimmed") {
		t.Errorf("log output = %q, want it to mention trimming", buf.String())
	}
}

// TestWindowManager_Fit_NoLoggerRunsSilently verifies a WindowManager with
// no logger configured does not panic when trimming occurs.
func TestWindowManager_Fit_NoLoggerRunsSilently(t *testing.T) {
	c := Context{Items: []ContextItem{
		{Section: ContextSectionPreviousResults, Content: strings.Repeat("noise ", 20)},
	}}

	got, _ := NewWindowManager().Fit(c, 1)
	if len(got.Truncated) == 0 {
		t.Fatal("expected trimming to occur for this test to be meaningful")
	}
}

// TestDefaultTokenEstimator verifies the ~4-chars-per-token heuristic,
// including its blank-content and always-at-least-one-token edge cases.
func TestDefaultTokenEstimator(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 0},
		{"whitespace only", "   ", 0},
		{"short", "hi", 1},
		{"eight chars", "12345678", 2},
		{"trims surrounding whitespace", "  1234  ", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := defaultTokenEstimator(tt.in); got != tt.want {
				t.Errorf("defaultTokenEstimator(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

// TestDefaultPriority verifies the default section-based ranking table.
func TestDefaultPriority(t *testing.T) {
	tests := []struct {
		section ContextSection
		want    ContextPriority
	}{
		{ContextSectionUserMessage, ContextPriorityCritical},
		{ContextSectionTask, ContextPriorityCritical},
		{ContextSectionMemories, ContextPriorityHigh},
		{ContextSectionConversationHistory, ContextPriorityNormal},
		{ContextSectionAvailableTools, ContextPriorityNormal},
		{ContextSectionPreviousResults, ContextPriorityLow},
	}
	for _, tt := range tests {
		t.Run(string(tt.section), func(t *testing.T) {
			got := defaultPriority(ContextItem{Section: tt.section})
			if got != tt.want {
				t.Errorf("defaultPriority(%q) = %v, want %v", tt.section, got, tt.want)
			}
		})
	}
}
