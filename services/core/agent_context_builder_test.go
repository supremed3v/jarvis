package core

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"jarvis-pa/packages/logger"
	types "jarvis-pa/packages/shared-types"
)

// TestContextBuilder_Build_EmptyInputProducesEmptyContext verifies "avoid
// unnecessary context": a ContextInput with nothing set yields no Items and
// no Truncated sections, not empty placeholders.
func TestContextBuilder_Build_EmptyInputProducesEmptyContext(t *testing.T) {
	got := NewContextBuilder().Build(ContextInput{})
	if len(got.Items) != 0 {
		t.Errorf("Build(empty) Items = %v, want none", got.Items)
	}
	if got.TotalSize != 0 {
		t.Errorf("Build(empty) TotalSize = %d, want 0", got.TotalSize)
	}
	if len(got.Truncated) != 0 {
		t.Errorf("Build(empty) Truncated = %v, want none", got.Truncated)
	}
}

// TestContextBuilder_Build_IncludesAllSectionsInOrder verifies "Required
// information is included" and "maintain ordering": every one of SPEC-0023's
// six listed inputs appears, in contextSectionOrder, regardless of the field
// order they were set in on ContextInput.
func TestContextBuilder_Build_IncludesAllSectionsInOrder(t *testing.T) {
	task := &types.Task{ID: "t1", Title: "Do the thing", Type: "demo", Status: types.TaskStatusExecuting}
	input := ContextInput{
		// Deliberately populated out of section order.
		AvailableTools:      []string{"filesystem"},
		PreviousResults:     []HistoryRecord{{TaskID: "t1", EventType: "TASK_STARTED", Timestamp: time.Now()}},
		Memories:            []string{"user prefers concise answers"},
		UserMessage:         "What's on my calendar today?",
		Task:                task,
		ConversationHistory: []string{"user: hi", "assistant: hello"},
	}

	got := NewContextBuilder().Build(input)

	wantSections := []ContextSection{
		ContextSectionUserMessage,
		ContextSectionConversationHistory,
		ContextSectionConversationHistory,
		ContextSectionMemories,
		ContextSectionTask,
		ContextSectionAvailableTools,
		ContextSectionPreviousResults,
	}
	if len(got.Items) != len(wantSections) {
		t.Fatalf("Build Items count = %d, want %d (got %+v)", len(got.Items), len(wantSections), got.Items)
	}
	for i, want := range wantSections {
		if got.Items[i].Section != want {
			t.Errorf("Items[%d].Section = %q, want %q", i, got.Items[i].Section, want)
		}
	}
	if got.Items[0].Content != input.UserMessage {
		t.Errorf("Items[0].Content = %q, want %q", got.Items[0].Content, input.UserMessage)
	}
}

// TestContextBuilder_Build_SkipsBlankEntries verifies blank strings and a nil
// Task contribute no items, alongside real content in the same input.
func TestContextBuilder_Build_SkipsBlankEntries(t *testing.T) {
	input := ContextInput{
		UserMessage:         "  ",
		ConversationHistory: []string{"", "  ", "assistant: hello"},
		Memories:            nil,
		Task:                nil,
		AvailableTools:      []string{""},
		PreviousResults:     nil,
	}

	got := NewContextBuilder().Build(input)
	if len(got.Items) != 1 {
		t.Fatalf("Build Items = %+v, want exactly 1 (the non-blank history entry)", got.Items)
	}
	if got.Items[0].Content != "assistant: hello" {
		t.Errorf("Items[0].Content = %q, want %q", got.Items[0].Content, "assistant: hello")
	}
}

// TestContextBuilder_Build_TaskItemCarriesData verifies the Task section's
// Data field round-trips the original *types.Task for a caller that needs
// more than the rendered Content string.
func TestContextBuilder_Build_TaskItemCarriesData(t *testing.T) {
	task := &types.Task{ID: "t1", Title: "Do the thing", Type: "demo", Status: types.TaskStatusExecuting}
	got := NewContextBuilder().Build(ContextInput{Task: task})

	if len(got.Items) != 1 {
		t.Fatalf("Build Items = %+v, want exactly 1", got.Items)
	}
	if got.Items[0].Data != task {
		t.Errorf("Items[0].Data = %v, want the same *types.Task pointer", got.Items[0].Data)
	}
}

// TestContextBuilder_Build_MaxSizeTruncatesLaterSections verifies "Support
// token limits" / "Large contexts are handled": a tight WithMaxSize budget
// stops adding items once the next one would exceed it, and reports which
// sections lost items.
func TestContextBuilder_Build_MaxSizeTruncatesLaterSections(t *testing.T) {
	input := ContextInput{
		UserMessage:    "hello there",                                          // 2 words
		Memories:       []string{"remembered fact one", "remembered fact two"}, // 3 words each
		AvailableTools: []string{"filesystem browser terminal"},                // 3 words
	}

	b := NewContextBuilder(WithMaxSize(5)) // fits "hello there" (2) + one memory (3) = 5
	got := b.Build(input)

	if got.TotalSize != 5 {
		t.Errorf("TotalSize = %d, want 5", got.TotalSize)
	}
	if len(got.Items) != 2 {
		t.Fatalf("Items = %+v, want exactly 2", got.Items)
	}
	if got.Items[0].Section != ContextSectionUserMessage {
		t.Errorf("Items[0].Section = %q, want %q", got.Items[0].Section, ContextSectionUserMessage)
	}
	if got.Items[1].Section != ContextSectionMemories {
		t.Errorf("Items[1].Section = %q, want %q", got.Items[1].Section, ContextSectionMemories)
	}

	foundMemories, foundTools := false, false
	for _, s := range got.Truncated {
		switch s {
		case ContextSectionMemories:
			foundMemories = true
		case ContextSectionAvailableTools:
			foundTools = true
		}
	}
	if !foundMemories {
		t.Errorf("Truncated = %v, want it to include %q (second memory dropped)", got.Truncated, ContextSectionMemories)
	}
	if !foundTools {
		t.Errorf("Truncated = %v, want it to include %q (tools section dropped entirely)", got.Truncated, ContextSectionAvailableTools)
	}
}

// TestContextBuilder_Build_NoMaxSizeIsUnlimited verifies the default
// (WithMaxSize unset) never truncates.
func TestContextBuilder_Build_NoMaxSizeIsUnlimited(t *testing.T) {
	input := ContextInput{
		Memories: []string{"a fairly long remembered fact that would exceed any tiny budget"},
	}
	got := NewContextBuilder().Build(input)
	if len(got.Truncated) != 0 {
		t.Errorf("Truncated = %v, want none with no size limit configured", got.Truncated)
	}
	if len(got.Items) != 1 {
		t.Errorf("Items = %+v, want exactly 1", got.Items)
	}
}

// TestContextBuilder_Build_CustomSizeEstimator verifies WithSizeEstimator
// overrides the default word-count estimator.
func TestContextBuilder_Build_CustomSizeEstimator(t *testing.T) {
	calls := 0
	estimator := func(item ContextItem) int {
		calls++
		return len(item.Content) // character count instead of word count
	}

	input := ContextInput{UserMessage: "hi"}
	got := NewContextBuilder(WithSizeEstimator(estimator)).Build(input)

	if calls != 1 {
		t.Errorf("custom estimator called %d times, want 1", calls)
	}
	if got.TotalSize != 2 {
		t.Errorf("TotalSize = %d, want 2 (character count of %q)", got.TotalSize, "hi")
	}
}

// TestContextBuilder_Build_LogsTruncation verifies WithContextBuilderLogger
// reports truncation, mirroring ExecutionLoop's failure-logging precedent.
func TestContextBuilder_Build_LogsTruncation(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New("test", logger.WithOutput(&buf))

	b := NewContextBuilder(WithMaxSize(1), WithContextBuilderLogger(log))
	b.Build(ContextInput{UserMessage: "one two three"})

	if !strings.Contains(buf.String(), "context truncated") {
		t.Errorf("log output = %q, want it to mention truncation", buf.String())
	}
}

// TestContextBuilder_Build_NilSizeEstimatorFallsBackToDefault verifies a nil
// SizeEstimator (whether from WithSizeEstimator(nil) or a zero-value
// ContextBuilder{} built without NewContextBuilder) falls back to
// defaultSizeEstimator instead of panicking on the first Build call.
func TestContextBuilder_Build_NilSizeEstimatorFallsBackToDefault(t *testing.T) {
	viaOption := NewContextBuilder(WithSizeEstimator(nil)).Build(ContextInput{UserMessage: "one two three"})
	if viaOption.TotalSize != 3 {
		t.Errorf("WithSizeEstimator(nil): TotalSize = %d, want 3 (default word count)", viaOption.TotalSize)
	}

	viaZeroValue := (&ContextBuilder{}).Build(ContextInput{UserMessage: "one two three"})
	if viaZeroValue.TotalSize != 3 {
		t.Errorf("zero-value ContextBuilder{}: TotalSize = %d, want 3 (default word count)", viaZeroValue.TotalSize)
	}
}

// TestDefaultSizeEstimator_CountsWords is a focused unit check on the
// default estimator's behavior, independent of ContextBuilder.
func TestDefaultSizeEstimator_CountsWords(t *testing.T) {
	got := defaultSizeEstimator(ContextItem{Content: "one two three"})
	if got != 3 {
		t.Errorf("defaultSizeEstimator = %d, want 3", got)
	}
}
