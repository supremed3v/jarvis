package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"jarvis-pa/packages/errors"
)

func TestSearXNGProvider_Name(t *testing.T) {
	p := NewSearXNGProvider()
	if p.Name() != "searxng" {
		t.Errorf("Name() = %q, want %q", p.Name(), "searxng")
	}
}

func TestSearXNGProvider_InterfaceCompliance(t *testing.T) {
	var _ SearchProvider = &SearXNGProvider{}
}

func TestSearXNGProvider_Configure_Valid(t *testing.T) {
	p := NewSearXNGProvider()
	err := p.Configure(SearchProviderConfig{
		BaseURL: "http://localhost:8888",
		Timeout: 5 * time.Second,
		Options: map[string]any{"engines": "google,duckduckgo"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearXNGProvider_Configure_EmptyURL(t *testing.T) {
	p := NewSearXNGProvider()
	err := p.Configure(SearchProviderConfig{})
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

func TestSearXNGProvider_Configure_InvalidURL(t *testing.T) {
	p := NewSearXNGProvider()
	err := p.Configure(SearchProviderConfig{BaseURL: "://bad"})
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
	je, ok := err.(*errors.Error)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T", err)
	}
	if je.Code != "SEARXNG_CONFIGURE_INVALID_URL" {
		t.Errorf("code = %q, want SEARXNG_CONFIGURE_INVALID_URL", je.Code)
	}
}

func TestSearXNGProvider_Search_NotConfigured(t *testing.T) {
	p := NewSearXNGProvider()
	_, err := p.Search(context.Background(), SearchQuery{Query: "test"})
	if err == nil {
		t.Fatal("expected error when not configured")
	}
	je, ok := err.(*errors.Error)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T", err)
	}
	if je.Code != "SEARXNG_NOT_CONFIGURED" {
		t.Errorf("code = %q, want SEARXNG_NOT_CONFIGURED", je.Code)
	}
}

func TestSearXNGProvider_Search_EmptyQuery(t *testing.T) {
	p := NewSearXNGProvider()
	_ = p.Configure(SearchProviderConfig{BaseURL: "http://localhost:8888"})
	_, err := p.Search(context.Background(), SearchQuery{})
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
}

func fakeSearXNGServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func TestSearXNGProvider_Search_ReturnsResults(t *testing.T) {
	srv := fakeSearXNGServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		q := r.URL.Query().Get("q")
		if q != "golang" {
			t.Errorf("query = %q, want %q", q, "golang")
		}
		if r.URL.Query().Get("format") != "json" {
			t.Error("format != json")
		}

		resp := searxngResponse{
			Query:           q,
			NumberOfResults: 2,
			Results: []searxngResult{
				{
					URL:     "https://go.dev",
					Title:   "The Go Programming Language",
					Content: "Build simple, secure, scalable systems with Go",
					Engine:  "google",
					Engines: []string{"google", "duckduckgo"},
					Score:   9.5,
					Category: "it",
				},
				{
					URL:           "https://go.dev/doc",
					Title:         "Documentation - The Go Programming Language",
					Content:       "Get started with Go",
					Engine:        "duckduckgo",
					Score:         7.2,
					Category:      "it",
					PublishedDate: "2026-01-10",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	p := NewSearXNGProvider()
	_ = p.Configure(SearchProviderConfig{BaseURL: srv.URL})

	result, err := p.Search(context.Background(), SearchQuery{Query: "golang"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Query != "golang" {
		t.Errorf("query = %q, want %q", result.Query, "golang")
	}
	if len(result.Results) != 2 {
		t.Fatalf("results count = %d, want 2", len(result.Results))
	}
	if result.Results[0].Title != "The Go Programming Language" {
		t.Errorf("title = %q, want %q", result.Results[0].Title, "The Go Programming Language")
	}
	if result.Results[0].URL != "https://go.dev" {
		t.Errorf("url = %q, want %q", result.Results[0].URL, "https://go.dev")
	}
	if result.Results[0].Source != "google,duckduckgo" {
		t.Errorf("source = %q, want %q", result.Results[0].Source, "google,duckduckgo")
	}
	if result.Results[0].Metadata.Score != 9.5 {
		t.Errorf("score = %f, want 9.5", result.Results[0].Metadata.Score)
	}
	if result.Results[1].Metadata.PublishedDate != "2026-01-10" {
		t.Errorf("published date = %q, want %q", result.Results[1].Metadata.PublishedDate, "2026-01-10")
	}
	cat, ok := result.Results[0].Metadata.Extra["category"]
	if !ok || cat != "it" {
		t.Errorf("category = %v, want %q", cat, "it")
	}
}

func TestSearXNGProvider_Search_WithCategories(t *testing.T) {
	srv := fakeSearXNGServer(t, func(w http.ResponseWriter, r *http.Request) {
		cats := r.URL.Query().Get("categories")
		if cats != "it,science" {
			t.Errorf("categories = %q, want %q", cats, "it,science")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(searxngResponse{Query: "test", Results: []searxngResult{}})
	})
	defer srv.Close()

	p := NewSearXNGProvider()
	_ = p.Configure(SearchProviderConfig{BaseURL: srv.URL})

	_, err := p.Search(context.Background(), SearchQuery{
		Query:      "test",
		Categories: []string{"it", "science"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearXNGProvider_Search_WithLanguage(t *testing.T) {
	srv := fakeSearXNGServer(t, func(w http.ResponseWriter, r *http.Request) {
		lang := r.URL.Query().Get("language")
		if lang != "de" {
			t.Errorf("language = %q, want %q", lang, "de")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(searxngResponse{Query: "test", Results: []searxngResult{}})
	})
	defer srv.Close()

	p := NewSearXNGProvider()
	_ = p.Configure(SearchProviderConfig{BaseURL: srv.URL})

	_, err := p.Search(context.Background(), SearchQuery{Query: "test", Language: "de"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearXNGProvider_Search_SafeSearchEnabled(t *testing.T) {
	srv := fakeSearXNGServer(t, func(w http.ResponseWriter, r *http.Request) {
		ss := r.URL.Query().Get("safesearch")
		if ss != "2" {
			t.Errorf("safesearch = %q, want %q", ss, "2")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(searxngResponse{Query: "test", Results: []searxngResult{}})
	})
	defer srv.Close()

	p := NewSearXNGProvider()
	_ = p.Configure(SearchProviderConfig{BaseURL: srv.URL})

	_, err := p.Search(context.Background(), SearchQuery{Query: "test", SafeSearch: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearXNGProvider_Search_SafeSearchDisabled(t *testing.T) {
	srv := fakeSearXNGServer(t, func(w http.ResponseWriter, r *http.Request) {
		ss := r.URL.Query().Get("safesearch")
		if ss != "0" {
			t.Errorf("safesearch = %q, want %q", ss, "0")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(searxngResponse{Query: "test", Results: []searxngResult{}})
	})
	defer srv.Close()

	p := NewSearXNGProvider()
	_ = p.Configure(SearchProviderConfig{BaseURL: srv.URL})

	_, err := p.Search(context.Background(), SearchQuery{Query: "test", SafeSearch: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearXNGProvider_Search_MaxResultsTruncates(t *testing.T) {
	srv := fakeSearXNGServer(t, func(w http.ResponseWriter, r *http.Request) {
		resp := searxngResponse{
			Query:           "test",
			NumberOfResults: 3,
			Results: []searxngResult{
				{URL: "https://a.com", Title: "A", Content: "a"},
				{URL: "https://b.com", Title: "B", Content: "b"},
				{URL: "https://c.com", Title: "C", Content: "c"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	p := NewSearXNGProvider()
	_ = p.Configure(SearchProviderConfig{BaseURL: srv.URL})

	result, err := p.Search(context.Background(), SearchQuery{Query: "test", MaxResults: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Results) != 2 {
		t.Errorf("results count = %d, want 2", len(result.Results))
	}
}

func TestSearXNGProvider_Search_WithEnginesOption(t *testing.T) {
	srv := fakeSearXNGServer(t, func(w http.ResponseWriter, r *http.Request) {
		engines := r.URL.Query().Get("engines")
		if engines != "google,brave" {
			t.Errorf("engines = %q, want %q", engines, "google,brave")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(searxngResponse{Query: "test", Results: []searxngResult{}})
	})
	defer srv.Close()

	p := NewSearXNGProvider()
	_ = p.Configure(SearchProviderConfig{
		BaseURL: srv.URL,
		Options: map[string]any{"engines": "google,brave"},
	})

	_, err := p.Search(context.Background(), SearchQuery{Query: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearXNGProvider_Search_HTTPError(t *testing.T) {
	srv := fakeSearXNGServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	})
	defer srv.Close()

	p := NewSearXNGProvider()
	_ = p.Configure(SearchProviderConfig{BaseURL: srv.URL})

	_, err := p.Search(context.Background(), SearchQuery{Query: "test"})
	if err == nil {
		t.Fatal("expected error for HTTP 429")
	}
	je, ok := err.(*errors.Error)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T", err)
	}
	if je.Code != "SEARXNG_HTTP_ERROR" {
		t.Errorf("code = %q, want SEARXNG_HTTP_ERROR", je.Code)
	}
	if !strings.Contains(je.Message, "429") {
		t.Errorf("message should contain status code 429, got %q", je.Message)
	}
}

func TestSearXNGProvider_Search_InvalidJSON(t *testing.T) {
	srv := fakeSearXNGServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	})
	defer srv.Close()

	p := NewSearXNGProvider()
	_ = p.Configure(SearchProviderConfig{BaseURL: srv.URL})

	_, err := p.Search(context.Background(), SearchQuery{Query: "test"})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	je, ok := err.(*errors.Error)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T", err)
	}
	if je.Code != "SEARXNG_DECODE_FAILED" {
		t.Errorf("code = %q, want SEARXNG_DECODE_FAILED", je.Code)
	}
}

func TestSearXNGProvider_Search_ConnectionRefused(t *testing.T) {
	p := NewSearXNGProvider()
	_ = p.Configure(SearchProviderConfig{
		BaseURL: "http://127.0.0.1:1",
		Timeout: 1 * time.Second,
	})

	_, err := p.Search(context.Background(), SearchQuery{Query: "test"})
	if err == nil {
		t.Fatal("expected connection error")
	}
	je, ok := err.(*errors.Error)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T", err)
	}
	if je.Code != "SEARXNG_CONNECTION_FAILED" && je.Code != "SEARXNG_REQUEST_TIMEOUT" {
		t.Errorf("code = %q, want SEARXNG_CONNECTION_FAILED or SEARXNG_REQUEST_TIMEOUT", je.Code)
	}
}

func TestSearXNGProvider_Search_ContextCancelled(t *testing.T) {
	srv := fakeSearXNGServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	})
	defer srv.Close()

	p := NewSearXNGProvider()
	_ = p.Configure(SearchProviderConfig{BaseURL: srv.URL})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.Search(ctx, SearchQuery{Query: "test"})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestSearXNGProvider_Search_EmptyResults(t *testing.T) {
	srv := fakeSearXNGServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(searxngResponse{Query: "obscure", Results: []searxngResult{}})
	})
	defer srv.Close()

	p := NewSearXNGProvider()
	_ = p.Configure(SearchProviderConfig{BaseURL: srv.URL})

	result, err := p.Search(context.Background(), SearchQuery{Query: "obscure"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Results) != 0 {
		t.Errorf("results count = %d, want 0", len(result.Results))
	}
	if result.Query != "obscure" {
		t.Errorf("query = %q, want %q", result.Query, "obscure")
	}
}

func TestSearXNGProvider_Search_WithTimeRange(t *testing.T) {
	srv := fakeSearXNGServer(t, func(w http.ResponseWriter, r *http.Request) {
		tr := r.URL.Query().Get("time_range")
		if tr != "month" {
			t.Errorf("time_range = %q, want %q", tr, "month")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(searxngResponse{Query: "test", Results: []searxngResult{}})
	})
	defer srv.Close()

	p := NewSearXNGProvider()
	_ = p.Configure(SearchProviderConfig{BaseURL: srv.URL})

	_, err := p.Search(context.Background(), SearchQuery{Query: "test", TimeRange: "month"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearXNGProvider_HealthCheck_Healthy(t *testing.T) {
	srv := fakeSearXNGServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(searxngResponse{Query: "test", Results: []searxngResult{}})
	})
	defer srv.Close()

	p := NewSearXNGProvider()
	_ = p.Configure(SearchProviderConfig{BaseURL: srv.URL})

	status, err := p.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Healthy {
		t.Error("expected healthy = true")
	}
}

func TestSearXNGProvider_HealthCheck_Unhealthy_HTTPError(t *testing.T) {
	srv := fakeSearXNGServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	})
	defer srv.Close()

	p := NewSearXNGProvider()
	_ = p.Configure(SearchProviderConfig{BaseURL: srv.URL})

	status, err := p.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Healthy {
		t.Error("expected healthy = false")
	}
	if !strings.Contains(status.Message, "500") {
		t.Errorf("message should contain 500, got %q", status.Message)
	}
}

func TestSearXNGProvider_HealthCheck_Unhealthy_ConnectionRefused(t *testing.T) {
	p := NewSearXNGProvider()
	_ = p.Configure(SearchProviderConfig{
		BaseURL: "http://127.0.0.1:1",
		Timeout: 1 * time.Second,
	})

	status, err := p.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Healthy {
		t.Error("expected healthy = false")
	}
}

func TestSearXNGProvider_HealthCheck_NotConfigured(t *testing.T) {
	p := NewSearXNGProvider()

	status, err := p.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Healthy {
		t.Error("expected healthy = false when not configured")
	}
	if status.Message != "not configured" {
		t.Errorf("message = %q, want %q", status.Message, "not configured")
	}
}

func TestSearXNGProvider_Search_SingleEngineSource(t *testing.T) {
	srv := fakeSearXNGServer(t, func(w http.ResponseWriter, r *http.Request) {
		resp := searxngResponse{
			Query: "test",
			Results: []searxngResult{
				{URL: "https://a.com", Title: "A", Content: "a", Engine: "brave", Engines: nil},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	p := NewSearXNGProvider()
	_ = p.Configure(SearchProviderConfig{BaseURL: srv.URL})

	result, err := p.Search(context.Background(), SearchQuery{Query: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Results[0].Source != "brave" {
		t.Errorf("source = %q, want %q", result.Results[0].Source, "brave")
	}
}
