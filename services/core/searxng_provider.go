package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"jarvis-pa/packages/errors"
)

const defaultSearXNGTimeout = 15 * time.Second

// searxngResponse is the top-level JSON returned by SearXNG's /search endpoint
// when format=json.
type searxngResponse struct {
	Query              string           `json:"query"`
	NumberOfResults    float64          `json:"number_of_results"`
	Results            []searxngResult  `json:"results"`
	InfoBoxes          []any            `json:"infoboxes"`
	Suggestions        []string         `json:"suggestions"`
	UnresponsiveEngines []any           `json:"unresponsive_engines"`
}

// searxngResult is a single result entry from SearXNG's JSON response.
type searxngResult struct {
	URL           string   `json:"url"`
	Title         string   `json:"title"`
	Content       string   `json:"content"`
	Engine        string   `json:"engine"`
	Engines       []string `json:"engines"`
	Score         float64  `json:"score"`
	Category      string   `json:"category"`
	PublishedDate string   `json:"publishedDate"`
}

// SearXNGProvider is a concrete SearchProvider that queries a self-hosted
// SearXNG instance via its JSON API, per ADR-0005.
type SearXNGProvider struct {
	mu         sync.RWMutex
	baseURL    string
	timeout    time.Duration
	httpClient *http.Client
	options    map[string]any
}

// NewSearXNGProvider creates a SearXNGProvider with default settings.
// Call Configure to set the instance URL before use.
func NewSearXNGProvider() *SearXNGProvider {
	return &SearXNGProvider{
		timeout:    defaultSearXNGTimeout,
		httpClient: &http.Client{Timeout: defaultSearXNGTimeout},
	}
}

func (p *SearXNGProvider) Name() string { return "searxng" }

func (p *SearXNGProvider) Configure(cfg SearchProviderConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	u, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return errors.Wrap(err, errors.TypeInvalidInput, "SEARXNG_CONFIGURE_INVALID_URL", "core.searxng",
			"failed to parse SearXNG base URL")
	}
	if u.Scheme == "" {
		u.Scheme = "http"
	}
	p.baseURL = strings.TrimRight(u.String(), "/")

	if cfg.Timeout > 0 {
		p.timeout = cfg.Timeout
	}
	p.httpClient = &http.Client{Timeout: p.timeout}
	p.options = cfg.Options

	return nil
}

func (p *SearXNGProvider) Search(ctx context.Context, query SearchQuery) (SearchResponse, error) {
	if err := query.Validate(); err != nil {
		return SearchResponse{}, err
	}

	p.mu.RLock()
	base := p.baseURL
	client := p.httpClient
	opts := p.options
	p.mu.RUnlock()

	if base == "" {
		return SearchResponse{}, errors.New(errors.TypeInvalidInput, "SEARXNG_NOT_CONFIGURED", "core.searxng",
			"SearXNG provider has not been configured with a base URL")
	}

	params := url.Values{}
	params.Set("q", query.Query)
	params.Set("format", "json")

	if query.Language != "" {
		params.Set("language", query.Language)
	}
	if query.SafeSearch {
		params.Set("safesearch", "2")
	} else {
		params.Set("safesearch", "0")
	}
	if query.TimeRange != "" {
		params.Set("time_range", query.TimeRange)
	}
	if len(query.Categories) > 0 {
		params.Set("categories", strings.Join(query.Categories, ","))
	}
	if query.MaxResults > 0 {
		params.Set("pageno", "1")
	}

	if engines, ok := opts["engines"]; ok {
		if s, ok := engines.(string); ok && s != "" {
			params.Set("engines", s)
		}
	}

	reqURL := base + "/search?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return SearchResponse{}, errors.Wrap(err, errors.TypeInternal, "SEARXNG_REQUEST_FAILED", "core.searxng",
			"failed to create HTTP request")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return SearchResponse{}, p.mapConnectionError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return SearchResponse{}, p.readErrorResponse(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return SearchResponse{}, errors.Wrap(err, errors.TypeInternal, "SEARXNG_READ_FAILED", "core.searxng",
			"failed to read SearXNG response body")
	}

	var sxResp searxngResponse
	if err := json.Unmarshal(body, &sxResp); err != nil {
		return SearchResponse{}, errors.Wrap(err, errors.TypeInternal, "SEARXNG_DECODE_FAILED", "core.searxng",
			"failed to decode SearXNG JSON response")
	}

	results := make([]SearchResult, 0, len(sxResp.Results))
	for _, r := range sxResp.Results {
		source := r.Engine
		if len(r.Engines) > 0 {
			source = strings.Join(r.Engines, ",")
		}

		sr := SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
			Source:  source,
			Metadata: SearchResultMetadata{
				PublishedDate: r.PublishedDate,
				Score:         r.Score,
				Extra: map[string]any{
					"category": r.Category,
				},
			},
		}
		results = append(results, sr)
	}

	if query.MaxResults > 0 && len(results) > query.MaxResults {
		results = results[:query.MaxResults]
	}

	totalResults := int(sxResp.NumberOfResults)
	if totalResults == 0 {
		totalResults = len(results)
	}

	return SearchResponse{
		Query:        query.Query,
		Results:      results,
		TotalResults: totalResults,
	}, nil
}

func (p *SearXNGProvider) HealthCheck(ctx context.Context) (SearchHealthStatus, error) {
	p.mu.RLock()
	base := p.baseURL
	client := p.httpClient
	p.mu.RUnlock()

	if base == "" {
		return SearchHealthStatus{Healthy: false, Message: "not configured"}, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/search?q=test&format=json", nil)
	if err != nil {
		return SearchHealthStatus{Healthy: false, Message: err.Error()}, nil
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return SearchHealthStatus{Healthy: false, Message: err.Error()}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return SearchHealthStatus{
			Healthy: false,
			Message: fmt.Sprintf("SearXNG returned HTTP %d", resp.StatusCode),
		}, nil
	}

	return SearchHealthStatus{Healthy: true, Message: "SearXNG is reachable"}, nil
}

func (p *SearXNGProvider) mapConnectionError(err error) error {
	if ue, ok := err.(*url.Error); ok && ue.Timeout() {
		return errors.Wrap(err, errors.TypeTimeout, "SEARXNG_REQUEST_TIMEOUT", "core.searxng",
			"SearXNG request timed out")
	}
	return errors.Wrap(err, errors.TypeUnavailable, "SEARXNG_CONNECTION_FAILED", "core.searxng",
		"failed to connect to SearXNG instance")
}

func (p *SearXNGProvider) readErrorResponse(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.New(errors.TypeUnavailable, "SEARXNG_HTTP_ERROR", "core.searxng",
			fmt.Sprintf("SearXNG returned HTTP %d", resp.StatusCode))
	}
	return errors.New(errors.TypeUnavailable, "SEARXNG_HTTP_ERROR", "core.searxng",
		fmt.Sprintf("SearXNG returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
}
