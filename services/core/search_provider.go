package core

import (
	"context"
	"time"

	"jarvis-pa/packages/errors"
)

// SearchQuery is the input to SearchProvider.Search: the query string to
// search for, plus optional filters (MaxResults, SafeSearch, Language,
// TimeRange, Categories) that a provider may honour or ignore depending on
// its capabilities.
type SearchQuery struct {
	Query      string
	MaxResults int
	SafeSearch bool
	Language   string
	TimeRange  string
	Categories []string
}

// Validate reports whether q has the minimum fields a SearchProvider needs
// to run a search: a non-empty Query. It returns a packages/errors error
// typed TypeInvalidInput, or nil if q is valid.
func (q SearchQuery) Validate() error {
	if q.Query == "" {
		return errors.New(errors.TypeInvalidInput, "SEARCH_QUERY_MISSING", "core.searchprovider",
			"search query is empty")
	}
	return nil
}

// SearchResult is a single result returned by SearchProvider.Search.
type SearchResult struct {
	Title   string
	URL     string
	Snippet string
	Source   string
	Metadata SearchResultMetadata
}

// SearchResultMetadata carries optional, provider-dependent context about a
// single SearchResult — publish date, content type, relevance score, and a
// bag for anything else.
type SearchResultMetadata struct {
	PublishedDate string
	ContentType   string
	Score         float64
	Extra         map[string]any
}

// SearchResponse is the complete result of a SearchProvider.Search call:
// the results themselves, how many matched in total (which may exceed
// len(Results) when the provider caps or the caller limited via
// MaxResults), and the query that produced them.
type SearchResponse struct {
	Query        string
	Results      []SearchResult
	TotalResults int
}

// SearchProviderConfig configures a SearchProvider. BaseURL and Timeout are
// the settings every provider needs at minimum; Options carries
// provider-specific settings.
type SearchProviderConfig struct {
	BaseURL string
	Timeout time.Duration
	Options map[string]any
}

// Validate reports whether c has the minimum fields needed to configure a
// SearchProvider: a non-empty BaseURL. Returns a packages/errors error
// typed TypeInvalidInput, or nil if c is valid.
func (c SearchProviderConfig) Validate() error {
	if c.BaseURL == "" {
		return errors.New(errors.TypeInvalidInput, "SEARCH_CONFIG_MISSING_URL", "core.searchprovider",
			"search provider config is missing BaseURL")
	}
	return nil
}

// SearchHealthStatus is the result of a SearchProvider.HealthCheck call.
type SearchHealthStatus struct {
	Healthy bool
	Message string
}

// SearchProvider is the SPEC-0073 contract Core Runtime uses to perform web
// searches without depending on which search backend is running. ADR-0005
// locks SearXNG as the initial provider; this interface names no vendor.
type SearchProvider interface {
	Name() string
	Configure(cfg SearchProviderConfig) error
	Search(ctx context.Context, query SearchQuery) (SearchResponse, error)
	HealthCheck(ctx context.Context) (SearchHealthStatus, error)
}
