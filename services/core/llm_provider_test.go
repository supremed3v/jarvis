package core

import (
	"context"
	"testing"
	"time"

	"jarvis-pa/packages/errors"
)

// stubProvider is a minimal Provider implementation used to verify the
// SPEC-0026 contract can be implemented and driven by a caller. err, when
// set, is returned by every method that can fail, so a single stub covers
// both the success and failure paths.
type stubProvider struct {
	name   string
	models []ModelInfo
	health HealthStatus
	err    error
	chunks []StreamChunk
}

func (p *stubProvider) Name() string { return p.name }

func (p *stubProvider) Configure(cfg ProviderConfig) error { return p.err }

func (p *stubProvider) Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error) {
	if p.err != nil {
		return GenerateResponse{}, p.err
	}
	return GenerateResponse{Model: req.Model, Text: "generated: " + req.Prompt}, nil
}

func (p *stubProvider) Stream(ctx context.Context, req GenerateRequest, onChunk func(StreamChunk) error) error {
	if p.err != nil {
		return p.err
	}
	for _, chunk := range p.chunks {
		if err := onChunk(chunk); err != nil {
			return err
		}
	}
	return nil
}

func (p *stubProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.models, nil
}

func (p *stubProvider) HealthCheck(ctx context.Context) (HealthStatus, error) {
	if p.err != nil {
		return HealthStatus{}, p.err
	}
	return p.health, nil
}

// TestProvider_InterfaceCanBeImplemented verifies a concrete type can
// satisfy the Provider interface (SPEC-0026 testing criterion 1).
func TestProvider_InterfaceCanBeImplemented(t *testing.T) {
	var provider Provider = &stubProvider{name: "stub"}

	if got := provider.Name(); got != "stub" {
		t.Errorf("Name() = %q, want %q", got, "stub")
	}
	if err := provider.Configure(ProviderConfig{BaseURL: "http://localhost", Timeout: time.Second}); err != nil {
		t.Errorf("Configure() returned error: %v", err)
	}
}

// TestProvider_ResponsesFollowContract verifies Generate, Stream, and
// ListModels return results matching the request and the declared shapes
// (SPEC-0026 testing criterion 2).
func TestProvider_ResponsesFollowContract(t *testing.T) {
	provider := &stubProvider{
		name:   "stub",
		models: []ModelInfo{{Name: "llama3", ContextSize: 8192}},
		health: HealthStatus{Healthy: true, Message: "ok"},
		chunks: []StreamChunk{
			{Model: "llama3", Text: "hello "},
			{Model: "llama3", Text: "world", Done: true},
		},
	}

	resp, err := provider.Generate(context.Background(), GenerateRequest{Model: "llama3", Prompt: "hi"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if resp.Model != "llama3" || resp.Text != "generated: hi" {
		t.Errorf("Generate() = %+v, want Model=llama3 Text=%q", resp, "generated: hi")
	}

	var streamed string
	var lastDone bool
	err = provider.Stream(context.Background(), GenerateRequest{Model: "llama3", Prompt: "hi"}, func(c StreamChunk) error {
		streamed += c.Text
		lastDone = c.Done
		return nil
	})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	if streamed != "hello world" {
		t.Errorf("streamed text = %q, want %q", streamed, "hello world")
	}
	if !lastDone {
		t.Error("last chunk Done = false, want true")
	}

	models, err := provider.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels returned error: %v", err)
	}
	if len(models) != 1 || models[0].Name != "llama3" || models[0].ContextSize != 8192 {
		t.Errorf("ListModels() = %+v, want one model llama3 with ContextSize=8192", models)
	}

	health, err := provider.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck returned error: %v", err)
	}
	if !health.Healthy || health.Message != "ok" {
		t.Errorf("HealthCheck() = %+v, want Healthy=true Message=%q", health, "ok")
	}
}

// TestProvider_FailuresAreHandledCorrectly verifies that a failing Provider
// surfaces its error from every method rather than the call succeeding
// silently, and that Stream stops at the first onChunk error instead of
// continuing to deliver chunks (SPEC-0026 testing criterion 3).
func TestProvider_FailuresAreHandledCorrectly(t *testing.T) {
	wantErr := errors.New(errors.TypeUnavailable, "LLM_PROVIDER_UNREACHABLE", "core.llmprovider", "provider unreachable")
	provider := &stubProvider{name: "stub", err: wantErr}

	if _, err := provider.Generate(context.Background(), GenerateRequest{Model: "llama3", Prompt: "hi"}); err != wantErr {
		t.Errorf("Generate() error = %v, want %v", err, wantErr)
	}
	if _, err := provider.ListModels(context.Background()); err != wantErr {
		t.Errorf("ListModels() error = %v, want %v", err, wantErr)
	}
	if _, err := provider.HealthCheck(context.Background()); err != wantErr {
		t.Errorf("HealthCheck() error = %v, want %v", err, wantErr)
	}

	stopErr := errors.New(errors.TypeInternal, "LLM_STREAM_CONSUMER_FAILED", "core.llmprovider", "consumer failed")
	streaming := &stubProvider{
		name: "stub",
		chunks: []StreamChunk{
			{Model: "llama3", Text: "hello "},
			{Model: "llama3", Text: "world", Done: true},
		},
	}
	callCount := 0
	err := streaming.Stream(context.Background(), GenerateRequest{Model: "llama3", Prompt: "hi"}, func(c StreamChunk) error {
		callCount++
		return stopErr
	})
	if err != stopErr {
		t.Errorf("Stream() error = %v, want %v", err, stopErr)
	}
	if callCount != 1 {
		t.Errorf("onChunk called %d times, want 1 (stream must stop at first error)", callCount)
	}
}

// TestGenerateRequest_Validate verifies GenerateRequest's required fields
// are enforced and reported individually.
func TestGenerateRequest_Validate(t *testing.T) {
	tests := []struct {
		name     string
		req      GenerateRequest
		wantCode string
	}{
		{
			name: "valid",
			req:  GenerateRequest{Model: "llama3", Prompt: "hi"},
		},
		{
			name:     "missing model",
			req:      GenerateRequest{Prompt: "hi"},
			wantCode: "LLM_GENERATE_REQUEST_MISSING_MODEL",
		},
		{
			name:     "missing prompt",
			req:      GenerateRequest{Model: "llama3"},
			wantCode: "LLM_GENERATE_REQUEST_MISSING_PROMPT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantCode == "" {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error with code %s", tt.wantCode)
			}
			if !errors.HasCode(err, tt.wantCode) {
				t.Errorf("missing code %s: %v", tt.wantCode, err)
			}
			if !errors.Is(err, errors.TypeInvalidInput) {
				t.Errorf("error type = %v, want TypeInvalidInput", err)
			}
		})
	}
}
