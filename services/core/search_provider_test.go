package core

import (
	"context"
	"testing"
	"time"

	"jarvis-pa/packages/errors"
)

func TestSearchQuery_Validate_EmptyQuery(t *testing.T) {
	q := SearchQuery{}
	err := q.Validate()
	if err == nil {
		t.Fatal("expected error for empty query")
	}
	je, ok := err.(*errors.Error)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T", err)
	}
	if je.Code != "SEARCH_QUERY_MISSING" {
		t.Errorf("code = %q, want SEARCH_QUERY_MISSING", je.Code)
	}
	if je.Type != errors.TypeInvalidInput {
		t.Errorf("type = %q, want %q", je.Type, errors.TypeInvalidInput)
	}
}

func TestSearchQuery_Validate_ValidQuery(t *testing.T) {
	q := SearchQuery{Query: "golang testing"}
	if err := q.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearchQuery_Validate_WithOptions(t *testing.T) {
	q := SearchQuery{
		Query:      "rust async",
		MaxResults: 5,
		SafeSearch: true,
		Language:   "en",
		TimeRange:  "month",
		Categories: []string{"it", "science"},
	}
	if err := q.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearchProviderConfig_Validate_EmptyURL(t *testing.T) {
	c := SearchProviderConfig{}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for empty BaseURL")
	}
	je, ok := err.(*errors.Error)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T", err)
	}
	if je.Code != "SEARCH_CONFIG_MISSING_URL" {
		t.Errorf("code = %q, want SEARCH_CONFIG_MISSING_URL", je.Code)
	}
}

func TestSearchProviderConfig_Validate_Valid(t *testing.T) {
	c := SearchProviderConfig{
		BaseURL: "http://localhost:8888",
		Timeout: 10 * time.Second,
		Options: map[string]any{"apiKey": "test"},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// fakeSearchProvider is a minimal SearchProvider for interface compliance.
type fakeSearchProvider struct {
	name      string
	cfg       SearchProviderConfig
	healthy   bool
	results   []SearchResult
	searchErr error
}

func (f *fakeSearchProvider) Name() string { return f.name }

func (f *fakeSearchProvider) Configure(cfg SearchProviderConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	f.cfg = cfg
	return nil
}

func (f *fakeSearchProvider) Search(_ context.Context, q SearchQuery) (SearchResponse, error) {
	if err := q.Validate(); err != nil {
		return SearchResponse{}, err
	}
	if f.searchErr != nil {
		return SearchResponse{}, f.searchErr
	}
	return SearchResponse{
		Query:        q.Query,
		Results:      f.results,
		TotalResults: len(f.results),
	}, nil
}

func (f *fakeSearchProvider) HealthCheck(_ context.Context) (SearchHealthStatus, error) {
	return SearchHealthStatus{Healthy: f.healthy, Message: "fake"}, nil
}

func TestSearchProvider_InterfaceCompliance(t *testing.T) {
	var _ SearchProvider = &fakeSearchProvider{}
}

func TestSearchProvider_Search_ReturnsResults(t *testing.T) {
	p := &fakeSearchProvider{
		name:    "fake",
		healthy: true,
		results: []SearchResult{
			{Title: "Go docs", URL: "https://go.dev", Snippet: "The Go language", Source: "web"},
		},
	}
	_ = p.Configure(SearchProviderConfig{BaseURL: "http://localhost:8888"})

	resp, err := p.Search(context.Background(), SearchQuery{Query: "golang"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Query != "golang" {
		t.Errorf("query = %q, want %q", resp.Query, "golang")
	}
	if len(resp.Results) != 1 {
		t.Fatalf("results count = %d, want 1", len(resp.Results))
	}
	if resp.Results[0].Title != "Go docs" {
		t.Errorf("title = %q, want %q", resp.Results[0].Title, "Go docs")
	}
}

func TestSearchProvider_Search_InvalidQuery(t *testing.T) {
	p := &fakeSearchProvider{name: "fake"}
	_, err := p.Search(context.Background(), SearchQuery{})
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestSearchProvider_Search_ProviderError(t *testing.T) {
	p := &fakeSearchProvider{
		name:      "fake",
		searchErr: errors.New(errors.TypeInternal, "SEARCH_BACKEND_UNAVAILABLE", "core.searchprovider", "backend down"),
	}
	_ = p.Configure(SearchProviderConfig{BaseURL: "http://localhost:8888"})

	_, err := p.Search(context.Background(), SearchQuery{Query: "test"})
	if err == nil {
		t.Fatal("expected error from provider")
	}
	je, ok := err.(*errors.Error)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T", err)
	}
	if je.Code != "SEARCH_BACKEND_UNAVAILABLE" {
		t.Errorf("code = %q, want SEARCH_BACKEND_UNAVAILABLE", je.Code)
	}
}

func TestSearchProvider_HealthCheck(t *testing.T) {
	p := &fakeSearchProvider{name: "fake", healthy: true}
	status, err := p.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Healthy {
		t.Error("expected healthy = true")
	}
}

func TestSearchProvider_HealthCheck_Unhealthy(t *testing.T) {
	p := &fakeSearchProvider{name: "fake", healthy: false}
	status, err := p.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Healthy {
		t.Error("expected healthy = false")
	}
}

func TestSearchProvider_Configure_InvalidConfig(t *testing.T) {
	p := &fakeSearchProvider{name: "fake"}
	err := p.Configure(SearchProviderConfig{})
	if err == nil {
		t.Fatal("expected error for empty BaseURL")
	}
}

func TestSearchResult_Metadata(t *testing.T) {
	r := SearchResult{
		Title:   "Test",
		URL:     "https://example.com",
		Snippet: "A test result",
		Source:   "web",
		Metadata: SearchResultMetadata{
			PublishedDate: "2026-01-15",
			ContentType:   "text/html",
			Score:          0.95,
			Extra:          map[string]any{"engine": "google"},
		},
	}
	if r.Metadata.Score != 0.95 {
		t.Errorf("score = %f, want 0.95", r.Metadata.Score)
	}
	if r.Metadata.Extra["engine"] != "google" {
		t.Errorf("extra engine = %v, want google", r.Metadata.Extra["engine"])
	}
}
