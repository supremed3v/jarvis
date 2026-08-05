package core

import (
	"testing"
)

func TestNewQueryPlanner(t *testing.T) {
	p := NewQueryPlanner()
	if p == nil {
		t.Fatal("NewQueryPlanner returned nil")
	}
}

func TestPlanEmptyRequest(t *testing.T) {
	p := NewQueryPlanner()
	_, err := p.Plan("")
	if err == nil {
		t.Fatal("expected error for empty request")
	}
}

func TestPlanWhitespaceOnlyRequest(t *testing.T) {
	p := NewQueryPlanner()
	_, err := p.Plan("   ")
	if err == nil {
		t.Fatal("expected error for whitespace-only request")
	}
}

func TestPlanPreservesOriginalRequest(t *testing.T) {
	p := NewQueryPlanner()
	plan, err := p.Plan("find the best local AI coding model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.OriginalRequest != "find the best local AI coding model" {
		t.Errorf("OriginalRequest = %q, want %q", plan.OriginalRequest, "find the best local AI coding model")
	}
}

func TestPlanComparisonIntent(t *testing.T) {
	p := NewQueryPlanner()
	plan, err := p.Plan("compare Python vs Go for web development")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Intent != IntentComparison {
		t.Errorf("Intent = %q, want %q", plan.Intent, IntentComparison)
	}
	if len(plan.Queries) < 2 {
		t.Errorf("expected at least 2 queries for comparison, got %d", len(plan.Queries))
	}
}

func TestPlanHowToIntent(t *testing.T) {
	p := NewQueryPlanner()
	plan, err := p.Plan("how to install Docker on Ubuntu")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Intent != IntentHowTo {
		t.Errorf("Intent = %q, want %q", plan.Intent, IntentHowTo)
	}
	if len(plan.Queries) < 2 {
		t.Errorf("expected at least 2 queries for how-to, got %d", len(plan.Queries))
	}
}

func TestPlanNewsIntent(t *testing.T) {
	p := NewQueryPlanner()
	plan, err := p.Plan("latest updates on Rust programming language")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Intent != IntentNews {
		t.Errorf("Intent = %q, want %q", plan.Intent, IntentNews)
	}
	for _, q := range plan.Queries {
		if q.TimeRange == "" {
			t.Error("news query should have TimeRange set")
		}
	}
}

func TestPlanReviewIntent(t *testing.T) {
	p := NewQueryPlanner()
	plan, err := p.Plan("review of the Framework laptop experience")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Intent != IntentReview {
		t.Errorf("Intent = %q, want %q", plan.Intent, IntentReview)
	}
	if len(plan.Queries) < 2 {
		t.Errorf("expected at least 2 queries for review, got %d", len(plan.Queries))
	}
}

func TestPlanFactualIntent(t *testing.T) {
	p := NewQueryPlanner()
	plan, err := p.Plan("what is the speed of light")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Intent != IntentFactual {
		t.Errorf("Intent = %q, want %q", plan.Intent, IntentFactual)
	}
}

func TestPlanExploratoryIntent(t *testing.T) {
	p := NewQueryPlanner()
	plan, err := p.Plan("interesting things about quantum computing applications")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Intent != IntentExploratory {
		t.Errorf("Intent = %q, want %q", plan.Intent, IntentExploratory)
	}
}

func TestPlanComplexRequestProducesMultipleQueries(t *testing.T) {
	p := NewQueryPlanner()

	requests := []string{
		"find the best local AI coding model",
		"compare React vs Vue for large enterprise apps",
		"how to set up Kubernetes cluster on bare metal",
	}

	for _, req := range requests {
		plan, err := p.Plan(req)
		if err != nil {
			t.Fatalf("Plan(%q): unexpected error: %v", req, err)
		}
		if len(plan.Queries) < 2 {
			t.Errorf("Plan(%q): expected multiple queries, got %d", req, len(plan.Queries))
		}
	}
}

func TestPlanSearchIntentPreserved(t *testing.T) {
	p := NewQueryPlanner()
	plan, err := p.Plan("compare Python vs Go for web development")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, q := range plan.Queries {
		if q.Query == "" {
			t.Error("decomposed query has empty Query field")
		}
	}
}

func TestPlanSingleWordQuery(t *testing.T) {
	p := NewQueryPlanner()
	plan, err := p.Plan("kubernetes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Queries) == 0 {
		t.Fatal("expected at least one query")
	}
	if plan.Queries[0].Query != "kubernetes" {
		t.Errorf("expected query to contain the original word, got %q", plan.Queries[0].Query)
	}
}

func TestPlanComparisonSubjectExtraction(t *testing.T) {
	p := NewQueryPlanner()
	plan, err := p.Plan("Python vs Go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, q := range plan.Queries {
		if q.Query == "python features specifications" || q.Query == "go features specifications" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected per-subject queries for comparison")
	}
}

func TestPlanRefinementHints(t *testing.T) {
	p := NewQueryPlanner()
	plan, err := p.Plan("compare Vim vs Emacs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.RefinementHints) == 0 {
		t.Error("comparison plan should have refinement hints")
	}
}

func TestRefineDropsEmptyResults(t *testing.T) {
	p := NewQueryPlanner()
	plan := SearchPlan{
		OriginalRequest: "test query planner",
		Intent:          IntentExploratory,
		Queries: []SearchQuery{
			{Query: "test query planner"},
			{Query: "query planner details"},
		},
	}

	results := []SearchResponse{
		{Query: "test query planner", Results: []SearchResult{
			{Title: "Result 1", Snippet: "snippet"},
		}},
		{Query: "query planner details", Results: nil},
	}

	refined := p.Refine(plan, results)
	if refined.OriginalRequest != plan.OriginalRequest {
		t.Error("Refine should preserve OriginalRequest")
	}
	if refined.Intent != plan.Intent {
		t.Error("Refine should preserve Intent")
	}
	if len(refined.Queries) == 0 {
		t.Fatal("Refine should produce at least one query")
	}
}

func TestRefineNarrowsLowRelevance(t *testing.T) {
	p := NewQueryPlanner()
	plan := SearchPlan{
		OriginalRequest: "specific database optimization techniques",
		Intent:          IntentExploratory,
		Queries: []SearchQuery{
			{Query: "database"},
		},
	}

	results := []SearchResponse{
		{
			Query: "database",
			Results: []SearchResult{
				{Title: "Unrelated 1", Metadata: SearchResultMetadata{Score: 0.1}},
				{Title: "Unrelated 2", Metadata: SearchResultMetadata{Score: 0.2}},
				{Title: "Unrelated 3", Metadata: SearchResultMetadata{Score: 0.15}},
			},
		},
	}

	refined := p.Refine(plan, results)
	if len(refined.RefinementHints) == 0 {
		t.Error("expected refinement hints for low-relevance results")
	}

	foundNarrowed := false
	for _, hint := range refined.RefinementHints {
		if len(hint) > 0 {
			foundNarrowed = true
		}
	}
	if !foundNarrowed {
		t.Error("expected narrowing hint")
	}
}

func TestRefinePreservesGoodResults(t *testing.T) {
	p := NewQueryPlanner()
	original := SearchQuery{Query: "Go concurrency patterns"}
	plan := SearchPlan{
		OriginalRequest: "Go concurrency patterns",
		Intent:          IntentExploratory,
		Queries:         []SearchQuery{original},
	}

	results := []SearchResponse{
		{
			Query: "Go concurrency patterns",
			Results: []SearchResult{
				{Title: "Go Concurrency", Metadata: SearchResultMetadata{Score: 0.9}},
				{Title: "Goroutine Patterns", Metadata: SearchResultMetadata{Score: 0.8}},
			},
		},
	}

	refined := p.Refine(plan, results)
	if len(refined.Queries) == 0 {
		t.Fatal("Refine should keep queries with good results")
	}
	if refined.Queries[0].Query != original.Query {
		t.Errorf("expected preserved query %q, got %q", original.Query, refined.Queries[0].Query)
	}
}

func TestRefineWithEmptyPlan(t *testing.T) {
	p := NewQueryPlanner()
	plan := SearchPlan{
		OriginalRequest: "test",
		Intent:          IntentExploratory,
	}

	refined := p.Refine(plan, nil)
	if refined.OriginalRequest != "test" {
		t.Error("should preserve original request")
	}
}

func TestRefineWithMoreResultsThanQueries(t *testing.T) {
	p := NewQueryPlanner()
	plan := SearchPlan{
		OriginalRequest: "test",
		Intent:          IntentExploratory,
		Queries:         []SearchQuery{{Query: "test"}},
	}

	results := []SearchResponse{
		{Query: "test", Results: []SearchResult{{Title: "R1"}}},
		{Query: "extra", Results: []SearchResult{{Title: "R2"}}},
	}

	refined := p.Refine(plan, results)
	if len(refined.Queries) == 0 {
		t.Fatal("should produce queries")
	}
}

func TestSignificantWords(t *testing.T) {
	words := significantWords("find the best local AI coding model")
	if len(words) == 0 {
		t.Fatal("expected significant words")
	}
	for _, w := range words {
		if stopWords[w] {
			t.Errorf("significant word %q is a stop word", w)
		}
	}
}

func TestExtractComparisonSubjects(t *testing.T) {
	tests := []struct {
		input    string
		wantLen  int
	}{
		{"python vs go", 2},
		{"react versus vue", 2},
		{"emacs or vim", 2},
		{"just a regular query", 0},
	}

	for _, tt := range tests {
		subjects := extractComparisonSubjects(tt.input)
		if len(subjects) != tt.wantLen {
			t.Errorf("extractComparisonSubjects(%q) = %d subjects, want %d", tt.input, len(subjects), tt.wantLen)
		}
	}
}

func TestDetectIntentMultipleSignals(t *testing.T) {
	intent := detectIntent("how to set up and install Docker")
	if intent != IntentHowTo {
		t.Errorf("expected IntentHowTo for multiple how-to signals, got %q", intent)
	}
}

func TestBroaden(t *testing.T) {
	q := SearchQuery{Query: "advanced kubernetes cluster management"}
	b := broaden(q)
	words := significantWords(b.Query)
	origWords := significantWords(q.Query)
	if len(words) >= len(origWords) {
		t.Error("broadened query should have fewer words")
	}
}

func TestBroadenShortQuery(t *testing.T) {
	q := SearchQuery{Query: "go rust"}
	b := broaden(q)
	if b.Query != q.Query {
		t.Errorf("short query should not be broadened, got %q", b.Query)
	}
}

func TestExtractKeyTerm(t *testing.T) {
	term := extractKeyTerm("find the best programming language")
	if term == "" {
		t.Fatal("expected a key term")
	}
	if stopWords[term] {
		t.Errorf("key term %q should not be a stop word", term)
	}
}

func TestExtractKeyTermEmpty(t *testing.T) {
	term := extractKeyTerm("the a an")
	if term != "" {
		t.Errorf("expected empty key term for all stop words, got %q", term)
	}
}
