// ollama_provider.go implements SPEC-0027: Ollama Integration. OllamaProvider
// is the first concrete Provider for the SPEC-0026 Provider interface,
// connecting to a local Ollama server per ADR-0004.
package core

import (
	"bufio"
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

const defaultOllamaTimeout = 30 * time.Second

// ollamaGenerateRequest is the JSON body sent to POST /api/generate.
type ollamaGenerateRequest struct {
	Model   string         `json:"model"`
	Prompt  string         `json:"prompt"`
	Stream  bool           `json:"stream,omitempty"`
	Options map[string]any `json:"options,omitempty"`
}

// ollamaGenerateResponse is the JSON body returned by POST /api/generate for
// a single (non-streaming) completion.
type ollamaGenerateResponse struct {
	Model    string `json:"model"`
	Response string `json:"response"`
	Done     bool   `json:"done"`
	Error    string `json:"error,omitempty"`
}

// ollamaStreamChunk is one NDJSON line from a streaming /api/generate call.
type ollamaStreamChunk struct {
	Model    string `json:"model"`
	Response string `json:"response"`
	Done     bool   `json:"done"`
	Error    string `json:"error,omitempty"`
}

// ollamaTagsResponse is the JSON body returned by GET /api/tags.
type ollamaTagsResponse struct {
	Models []ollamaModelEntry `json:"models"`
}

// ollamaModelEntry is one entry in the /api/tags response.
type ollamaModelEntry struct {
	Name    string              `json:"name"`
	Size    int64               `json:"size"`
	Details *ollamaModelDetails `json:"details,omitempty"`
}

// ollamaModelDetails carries optional model metadata from /api/tags.
type ollamaModelDetails struct {
	Format        string   `json:"format"`
	Families      []string `json:"families"`
	ParameterSize string   `json:"parameter_size"`
	ContextLength int      `json:"context_length,omitempty"`
}

// ollamaErrorResponse represents an error response body from Ollama.
type ollamaErrorResponse struct {
	Error string `json:"error"`
}

// OllamaProvider is a concrete Provider that talks to a local Ollama server
// via its REST API. Create one with NewOllamaProvider, then Configure to
// set the base URL and timeout before calling Generate/Stream/ListModels.
type OllamaProvider struct {
	mu           sync.RWMutex
	baseURL      string
	timeout      time.Duration
	defaultModel string
	httpClient   *http.Client
}

// NewOllamaProvider creates an OllamaProvider with default settings:
// baseURL derived from the typical localhost:11434, a 30-second timeout,
// and no default model. Call Configure to override these.
func NewOllamaProvider() *OllamaProvider {
	return &OllamaProvider{
		baseURL:    "http://127.0.0.1:11434",
		timeout:    defaultOllamaTimeout,
		httpClient: &http.Client{Timeout: defaultOllamaTimeout},
	}
}

// Name returns "ollama", matching ADR-0004's provider choice.
func (p *OllamaProvider) Name() string { return "ollama" }

// Configure applies cfg to the provider. BaseURL overrides the default
// Ollama server address; Timeout overrides the HTTP client timeout.
// Options["model"] can set the default model for requests that leave
// Model blank.
func (p *OllamaProvider) Configure(cfg ProviderConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if cfg.BaseURL != "" {
		u, err := url.Parse(cfg.BaseURL)
		if err != nil {
			return errors.Wrap(err, errors.TypeInvalidInput, "OLLAMA_CONFIGURE_INVALID_URL", "core.ollama",
				"failed to parse Ollama base URL")
		}
		if u.Scheme == "" {
			u.Scheme = "http"
		}
		p.baseURL = strings.TrimRight(u.String(), "/")
	}

	if cfg.Timeout > 0 {
		p.timeout = cfg.Timeout
	}
	p.httpClient = &http.Client{Timeout: p.timeout}

	if model, ok := cfg.Options["model"]; ok {
		if s, ok := model.(string); ok {
			p.defaultModel = s
		}
	}

	return nil
}

// resolveURL returns the provider's current base URL under a read lock.
func (p *OllamaProvider) resolveURL() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.baseURL
}

// resolveModel returns req.Model if non-empty, otherwise the provider's
// default model if set, otherwise an error.
func (p *OllamaProvider) resolveModel(req GenerateRequest) (string, error) {
	if req.Model != "" {
		return req.Model, nil
	}
	p.mu.RLock()
	dm := p.defaultModel
	p.mu.RUnlock()
	if dm != "" {
		return dm, nil
	}
	return "", errors.New(errors.TypeInvalidInput, "OLLAMA_MISSING_MODEL", "core.ollama",
		"no model specified in request and no default model configured")
}

// doPost sends a POST request to the given endpoint with the given body and
// returns the full response body. It respects ctx cancellation via the HTTP
// client's Do method with a derived context.
func (p *OllamaProvider) doPost(ctx context.Context, endpoint string, body any) (*http.Response, error) {
	base := p.resolveURL()
	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, errors.Wrap(err, errors.TypeInternal, "OLLAMA_MARSHAL_FAILED", "core.ollama",
			"failed to marshal request body")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+endpoint, strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, errors.Wrap(err, errors.TypeInternal, "OLLAMA_NEW_REQUEST_FAILED", "core.ollama",
			"failed to create HTTP request")
	}
	req.Header.Set("Content-Type", "application/json")

	p.mu.RLock()
	client := p.httpClient
	p.mu.RUnlock()

	resp, err := client.Do(req)
	if err != nil {
		return nil, p.mapConnectionError(err)
	}
	return resp, nil
}

// doGet sends a GET request to the given endpoint.
func (p *OllamaProvider) doGet(ctx context.Context, endpoint string) (*http.Response, error) {
	base := p.resolveURL()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+endpoint, nil)
	if err != nil {
		return nil, errors.Wrap(err, errors.TypeInternal, "OLLAMA_NEW_REQUEST_FAILED", "core.ollama",
			"failed to create HTTP request")
	}

	p.mu.RLock()
	client := p.httpClient
	p.mu.RUnlock()

	resp, err := client.Do(req)
	if err != nil {
		return nil, p.mapConnectionError(err)
	}
	return resp, nil
}

// mapConnectionError wraps common HTTP/network errors into packages/errors
// types so callers can classify failures programmatically.
func (p *OllamaProvider) mapConnectionError(err error) error {
	if ue, ok := err.(*url.Error); ok && ue.Timeout() {
		return errors.Wrap(err, errors.TypeTimeout, "OLLAMA_REQUEST_TIMEOUT", "core.ollama",
			"Ollama request timed out")
	}
	return errors.Wrap(err, errors.TypeUnavailable, "OLLAMA_CONNECTION_FAILED", "core.ollama",
		"failed to connect to Ollama server")
}

// checkResponseError reads the body of an error response and wraps it into a
// packages/errors error.
func (p *OllamaProvider) checkResponseError(resp *http.Response) error {
	if resp.StatusCode < 400 {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.New(errors.TypeUnavailable, "OLLAMA_HTTP_ERROR", "core.ollama",
			fmt.Sprintf("Ollama returned HTTP %d", resp.StatusCode))
	}

	var errResp ollamaErrorResponse
	if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
		return errors.New(errors.TypeUnavailable, "OLLAMA_SERVER_ERROR", "core.ollama", errResp.Error)
	}

	return errors.New(errors.TypeUnavailable, "OLLAMA_HTTP_ERROR", "core.ollama",
		fmt.Sprintf("Ollama returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
}

// Generate runs req to completion and returns the full response.
func (p *OllamaProvider) Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error) {
	model, err := p.resolveModel(req)
	if err != nil {
		return GenerateResponse{}, err
	}

	ollamaReq := ollamaGenerateRequest{
		Model:   model,
		Prompt:  req.Prompt,
		Options: req.Options,
	}

	resp, err := p.doPost(ctx, "/api/generate", ollamaReq)
	if err != nil {
		return GenerateResponse{}, err
	}
	defer resp.Body.Close()

	if err := p.checkResponseError(resp); err != nil {
		return GenerateResponse{}, err
	}

	var ollamaResp ollamaGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return GenerateResponse{}, errors.Wrap(err, errors.TypeInternal, "OLLAMA_DECODE_FAILED", "core.ollama",
			"failed to decode Ollama generate response")
	}

	if ollamaResp.Error != "" {
		return GenerateResponse{}, errors.New(errors.TypeUnavailable, "OLLAMA_GENERATE_ERROR", "core.ollama",
			ollamaResp.Error)
	}

	return GenerateResponse{
		Model: ollamaResp.Model,
		Text:  ollamaResp.Response,
		Metadata: map[string]any{
			"done": ollamaResp.Done,
		},
	}, nil
}

// Stream runs req and invokes onChunk for each incremental piece of the
// response as it's produced. It reads NDJSON lines from Ollama's streaming
// /api/generate endpoint and decodes each line into a StreamChunk.
func (p *OllamaProvider) Stream(ctx context.Context, req GenerateRequest, onChunk func(StreamChunk) error) error {
	model, err := p.resolveModel(req)
	if err != nil {
		return err
	}

	ollamaReq := ollamaGenerateRequest{
		Model:   model,
		Prompt:  req.Prompt,
		Stream:  true,
		Options: req.Options,
	}

	resp, err := p.doPost(ctx, "/api/generate", ollamaReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := p.checkResponseError(resp); err != nil {
		return err
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var chunk ollamaStreamChunk
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			return errors.Wrap(err, errors.TypeInternal, "OLLAMA_STREAM_DECODE_FAILED", "core.ollama",
				"failed to decode streaming chunk from Ollama")
		}

		if chunk.Error != "" {
			return errors.New(errors.TypeUnavailable, "OLLAMA_STREAM_ERROR", "core.ollama", chunk.Error)
		}

		if err := onChunk(StreamChunk{
			Model: chunk.Model,
			Text:  chunk.Response,
			Done:  chunk.Done,
		}); err != nil {
			return err
		}

		if chunk.Done {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return p.mapConnectionError(err)
	}

	return nil
}

// ListModels returns the models this Ollama server currently has available.
func (p *OllamaProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	resp, err := p.doGet(ctx, "/api/tags")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := p.checkResponseError(resp); err != nil {
		return nil, err
	}

	var tagsResp ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tagsResp); err != nil {
		return nil, errors.Wrap(err, errors.TypeInternal, "OLLAMA_LIST_DECODE_FAILED", "core.ollama",
			"failed to decode Ollama tags response")
	}

	models := make([]ModelInfo, 0, len(tagsResp.Models))
	for _, m := range tagsResp.Models {
		info := ModelInfo{
			Name: m.Name,
		}
		if m.Details != nil {
			info.Description = fmt.Sprintf("%s (%s)", m.Details.Format, strings.Join(m.Details.Families, ", "))
			info.ContextSize = m.Details.ContextLength
		}
		models = append(models, info)
	}

	return models, nil
}

// HealthCheck reports whether the Ollama server is currently reachable.
func (p *OllamaProvider) HealthCheck(ctx context.Context) (HealthStatus, error) {
	resp, err := p.doGet(ctx, "/api/tags")
	if err != nil {
		return HealthStatus{Healthy: false, Message: err.Error()}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return HealthStatus{Healthy: false, Message: fmt.Sprintf("Ollama returned HTTP %d", resp.StatusCode)}, nil
	}

	return HealthStatus{Healthy: true, Message: "Ollama server is reachable"}, nil
}
