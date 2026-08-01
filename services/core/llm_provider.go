// llm_provider.go implements SPEC-0026: the LLM Provider Interface. Provider
// is the contract Core Runtime (and, in turn, SPEC-0018 Agents) use to talk
// to a language model without depending on any single model implementation -
// ADR-0004 locks Ollama as the required initial provider, but Provider
// itself names no vendor. SPEC-0027 (Ollama Integration) supplies the first
// concrete Provider; this spec defines only the contract and its supporting
// types.
package core

import (
	"context"
	"time"

	"jarvis-pa/packages/errors"
)

// GenerateRequest is the input to Provider.Generate and Provider.Stream: the
// model to run and the prompt to run it on, plus provider-specific tuning
// (e.g. temperature) in Options.
type GenerateRequest struct {
	Model   string
	Prompt  string
	Options map[string]any
}

// Validate reports whether r has the minimum fields a Provider needs to run
// a request: a non-empty Model and Prompt. It returns a packages/errors
// error typed TypeInvalidInput naming the first missing field, or nil if r
// is valid.
func (r GenerateRequest) Validate() error {
	if r.Model == "" {
		return errors.New(errors.TypeInvalidInput, "LLM_GENERATE_REQUEST_MISSING_MODEL", "core.llmprovider",
			"generate request is missing a Model")
	}
	if r.Prompt == "" {
		return errors.New(errors.TypeInvalidInput, "LLM_GENERATE_REQUEST_MISSING_PROMPT", "core.llmprovider",
			"generate request is missing a Prompt").With("model", r.Model)
	}
	return nil
}

// GenerateResponse is a Provider's completed, non-streamed result for a
// GenerateRequest.
type GenerateResponse struct {
	Model    string
	Text     string
	Metadata map[string]any
}

// StreamChunk is one incremental piece of a streamed GenerateRequest result,
// delivered to the callback passed to Provider.Stream. Done marks the final
// chunk of the stream; a provider may send a final chunk with empty Text if
// it has nothing left to flush.
type StreamChunk struct {
	Model string
	Text  string
	Done  bool
}

// ModelInfo describes one model a Provider can serve (the "Model
// information" requirement).
type ModelInfo struct {
	Name        string
	Description string
	ContextSize int
}

// HealthStatus is the result of a Provider.HealthCheck call (the "Health
// checks" requirement).
type HealthStatus struct {
	Healthy bool
	Message string
}

// ProviderConfig configures a Provider (the "Provider configuration"
// requirement). BaseURL and Timeout are the settings every provider needs at
// minimum to reach its backend; Options carries provider-specific settings
// that don't belong in the shared contract.
type ProviderConfig struct {
	BaseURL string
	Timeout time.Duration
	Options map[string]any
}

// Provider is the SPEC-0026 contract Core Runtime uses to talk to a language
// model without depending on which one is actually running. Generate and
// Stream both accept a GenerateRequest so a caller can choose either mode
// without constructing two different request shapes.
type Provider interface {
	// Name returns the provider's identifier (e.g. "ollama"), matching
	// packages/config's ModelConfig.Provider / ADR-0004's provider choice.
	Name() string

	// Configure applies cfg to the provider. Configure must be safe to call
	// again to reconfigure an already-running provider.
	Configure(cfg ProviderConfig) error

	// Generate runs req to completion and returns the full response.
	// Generate must respect ctx cancellation.
	Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error)

	// Stream runs req and invokes onChunk for each incremental piece of the
	// response as it's produced, including the final chunk
	// (StreamChunk.Done == true). Stream returns onChunk's error immediately
	// if onChunk returns one, and must respect ctx cancellation.
	Stream(ctx context.Context, req GenerateRequest, onChunk func(StreamChunk) error) error

	// ListModels returns the models this provider currently has available.
	ListModels(ctx context.Context) ([]ModelInfo, error)

	// HealthCheck reports whether the provider is currently reachable and
	// able to serve requests.
	HealthCheck(ctx context.Context) (HealthStatus, error)
}
