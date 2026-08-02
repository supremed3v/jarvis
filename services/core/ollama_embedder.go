// ollama_embedder.go implements part of SPEC-0039 (Embedding Pipeline):
// OllamaEmbedder is the model-backed Embedder (SPEC-0038's interface)
// SPEC-0038's own build note anticipated, mirroring the
// Provider-interface/OllamaProvider-concrete split SPEC-0026/0027 already
// established, but for embeddings rather than text generation. It is a
// drop-in replacement for the dependency-free HashEmbedder wherever an
// Embedder is accepted (e.g. VectorStore's WithEmbedder, EmbeddingPipeline's
// WithPipelineEmbedder).
package core

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

const defaultOllamaEmbedTimeout = 30 * time.Second

// ollamaEmbedRequest is the JSON body sent to POST /api/embeddings.
type ollamaEmbedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// ollamaEmbedResponse is the JSON body returned by POST /api/embeddings.
type ollamaEmbedResponse struct {
	Embedding []float64 `json:"embedding"`
	Error     string    `json:"error,omitempty"`
}

// OllamaEmbedder is a concrete Embedder that calls a local Ollama server's
// embedding endpoint. Create one with NewOllamaEmbedder.
//
// Embed's signature (fixed by SPEC-0038's Embedder interface) has no error
// return, so any failure to reach Ollama, a non-2xx response, or a
// malformed response body degrades to a zero-length vector rather than
// panicking or surfacing an error: cosineSimilarity already treats any
// zero-magnitude vector as similarity 0, so a failed embedding call simply
// never matches anything instead of corrupting results.
type OllamaEmbedder struct {
	mu         sync.RWMutex
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewOllamaEmbedder creates an OllamaEmbedder targeting baseURL (e.g.
// "http://127.0.0.1:11434") using model for every Embed call.
func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder {
	trimmed := strings.TrimRight(baseURL, "/")
	if trimmed == "" {
		trimmed = "http://127.0.0.1:11434"
	}
	return &OllamaEmbedder{
		baseURL:    trimmed,
		model:      model,
		httpClient: &http.Client{Timeout: defaultOllamaEmbedTimeout},
	}
}

// resolve returns the embedder's current base URL and model under a read
// lock.
func (e *OllamaEmbedder) resolve() (string, string) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.baseURL, e.model
}

// Embed implements Embedder by calling Ollama's POST /api/embeddings.
func (e *OllamaEmbedder) Embed(text string) []float64 {
	base, model := e.resolve()
	if model == "" {
		return nil
	}

	reqBody, err := json.Marshal(ollamaEmbedRequest{Model: model, Prompt: text})
	if err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultOllamaEmbedTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/embeddings", strings.NewReader(string(reqBody)))
	if err != nil {
		return nil
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(httpReq)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil
	}

	var embedResp ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
		return nil
	}
	if embedResp.Error != "" {
		return nil
	}

	return embedResp.Embedding
}
