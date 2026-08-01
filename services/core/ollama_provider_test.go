// ollama_provider_test.go implements SPEC-0027 tests for OllamaProvider.
package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"jarvis-pa/packages/errors"
)

func TestOllamaProvider_Name(t *testing.T) {
	p := NewOllamaProvider()
	if p.Name() != "ollama" {
		t.Errorf("Name() = %q, want %q", p.Name(), "ollama")
	}
}

func TestOllamaProvider_Configure(t *testing.T) {
	p := NewOllamaProvider()

	// Test default values
	if p.baseURL != "http://127.0.0.1:11434" {
		t.Errorf("default baseURL = %q, want %q", p.baseURL, "http://127.0.0.1:11434")
	}
	if p.timeout != defaultOllamaTimeout {
		t.Errorf("default timeout = %v, want %v", p.timeout, defaultOllamaTimeout)
	}

	// Test custom configuration
	err := p.Configure(ProviderConfig{
		BaseURL: "http://custom-host:8080",
		Timeout: 10 * time.Second,
		Options: map[string]any{"model": "llama3"},
	})
	if err != nil {
		t.Fatalf("Configure returned error: %v", err)
	}

	if p.baseURL != "http://custom-host:8080" {
		t.Errorf("configured baseURL = %q, want %q", p.baseURL, "http://custom-host:8080")
	}
	if p.timeout != 10*time.Second {
		t.Errorf("configured timeout = %v, want %v", p.timeout, 10*time.Second)
	}
	if p.defaultModel != "llama3" {
		t.Errorf("configured defaultModel = %q, want %q", p.defaultModel, "llama3")
	}
}

func TestOllamaProvider_Configure_InvalidURL(t *testing.T) {
	p := NewOllamaProvider()
	// Use an invalid URL that url.Parse will actually reject
	err := p.Configure(ProviderConfig{BaseURL: "://invalid"})
	if err == nil {
		t.Fatal("Configure should return error for invalid URL")
	}
	if !errors.HasCode(err, "OLLAMA_CONFIGURE_INVALID_URL") {
		t.Errorf("Configure error code = %v, want OLLAMA_CONFIGURE_INVALID_URL", err)
	}
}

func TestOllamaProvider_Generate_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		resp := ollamaGenerateResponse{
			Model:    "llama3",
			Response: "Hello, world!",
			Done:     true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOllamaProvider()
	if err := p.Configure(ProviderConfig{BaseURL: server.URL}); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	ctx := context.Background()
	resp, err := p.Generate(ctx, GenerateRequest{Model: "llama3", Prompt: "Hi"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if resp.Model != "llama3" {
		t.Errorf("resp.Model = %q, want %q", resp.Model, "llama3")
	}
	if resp.Text != "Hello, world!" {
		t.Errorf("resp.Text = %q, want %q", resp.Text, "Hello, world!")
	}
	if resp.Metadata["done"] != true {
		t.Errorf("resp.Metadata[done] = %v, want true", resp.Metadata["done"])
	}
}

func TestOllamaProvider_Generate_MissingModel(t *testing.T) {
	p := NewOllamaProvider()
	_, err := p.Generate(context.Background(), GenerateRequest{Prompt: "Hi"})
	if err == nil {
		t.Fatal("Generate should return error when no model specified")
	}
	if !errors.HasCode(err, "OLLAMA_MISSING_MODEL") {
		t.Errorf("Generate error code = %v, want OLLAMA_MISSING_MODEL", err)
	}
}

func TestOllamaProvider_Generate_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ollamaErrorResponse{Error: "model not found"})
	}))
	defer server.Close()

	p := NewOllamaProvider()
	if err := p.Configure(ProviderConfig{BaseURL: server.URL}); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	_, err := p.Generate(context.Background(), GenerateRequest{Model: "llama3", Prompt: "Hi"})
	if err == nil {
		t.Fatal("Generate should return error for server error")
	}
	if !errors.HasCode(err, "OLLAMA_SERVER_ERROR") {
		t.Errorf("Generate error code = %v, want OLLAMA_SERVER_ERROR", err)
	}
}

func TestOllamaProvider_Stream_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		chunks := []ollamaStreamChunk{
			{Model: "llama3", Response: "Hello ", Done: false},
			{Model: "llama3", Response: "world", Done: false},
			{Model: "llama3", Response: "!", Done: true},
		}
		enc := json.NewEncoder(w)
		for _, c := range chunks {
			enc.Encode(c)
		}
	}))
	defer server.Close()

	p := NewOllamaProvider()
	if err := p.Configure(ProviderConfig{BaseURL: server.URL}); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	var streamed string
	var lastDone bool
	err := p.Stream(context.Background(), GenerateRequest{Model: "llama3", Prompt: "Hi"}, func(c StreamChunk) error {
		streamed += c.Text
		lastDone = c.Done
		return nil
	})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	if streamed != "Hello world!" {
		t.Errorf("streamed = %q, want %q", streamed, "Hello world!")
	}
	if !lastDone {
		t.Error("last chunk Done = false, want true")
	}
}

func TestOllamaProvider_Stream_StopsOnCallbackError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunks := []ollamaStreamChunk{
			{Model: "llama3", Response: "Hello ", Done: false},
			{Model: "llama3", Response: "world", Done: false},
			{Model: "llama3", Response: "!", Done: true},
		}
		enc := json.NewEncoder(w)
		for _, c := range chunks {
			enc.Encode(c)
		}
	}))
	defer server.Close()

	p := NewOllamaProvider()
	if err := p.Configure(ProviderConfig{BaseURL: server.URL}); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	callCount := 0
	stopErr := errors.New(errors.TypeInternal, "CONSUMER_STOPPED", "test", "consumer stopped")
	err := p.Stream(context.Background(), GenerateRequest{Model: "llama3", Prompt: "Hi"}, func(c StreamChunk) error {
		callCount++
		if callCount == 1 {
			return stopErr
		}
		return nil
	})
	if err != stopErr {
		t.Errorf("Stream error = %v, want %v", err, stopErr)
	}
	if callCount != 1 {
		t.Errorf("onChunk called %d times, want 1 (stream must stop at first error)", callCount)
	}
}

func TestOllamaProvider_Stream_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ollamaErrorResponse{Error: "stream error"})
	}))
	defer server.Close()

	p := NewOllamaProvider()
	if err := p.Configure(ProviderConfig{BaseURL: server.URL}); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	err := p.Stream(context.Background(), GenerateRequest{Model: "llama3", Prompt: "Hi"}, func(c StreamChunk) error {
		return nil
	})
	if err == nil {
		t.Fatal("Stream should return error for server error")
	}
	if !errors.HasCode(err, "OLLAMA_SERVER_ERROR") {
		t.Errorf("Stream error code = %v, want OLLAMA_SERVER_ERROR", err)
	}
}

func TestOllamaProvider_ListModels_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := ollamaTagsResponse{
			Models: []ollamaModelEntry{
				{Name: "llama3", Size: 4661224448, Details: &ollamaModelDetails{Format: "gguf", Families: []string{"llama"}, ContextLength: 8192}},
				{Name: "mistral", Size: 4109066240, Details: &ollamaModelDetails{Format: "gguf", Families: []string{"mistral"}, ContextLength: 32768}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOllamaProvider()
	if err := p.Configure(ProviderConfig{BaseURL: server.URL}); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels returned error: %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].Name != "llama3" {
		t.Errorf("models[0].Name = %q, want %q", models[0].Name, "llama3")
	}
	if models[0].ContextSize != 8192 {
		t.Errorf("models[0].ContextSize = %d, want 8192", models[0].ContextSize)
	}
	if models[1].Name != "mistral" {
		t.Errorf("models[1].Name = %q, want %q", models[1].Name, "mistral")
	}
	if models[1].ContextSize != 32768 {
		t.Errorf("models[1].ContextSize = %d, want 32768", models[1].ContextSize)
	}
}

func TestOllamaProvider_ListModels_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ollamaErrorResponse{Error: "list failed"})
	}))
	defer server.Close()

	p := NewOllamaProvider()
	if err := p.Configure(ProviderConfig{BaseURL: server.URL}); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	_, err := p.ListModels(context.Background())
	if err == nil {
		t.Fatal("ListModels should return error for server error")
	}
	if !errors.HasCode(err, "OLLAMA_SERVER_ERROR") {
		t.Errorf("ListModels error code = %v, want OLLAMA_SERVER_ERROR", err)
	}
}

func TestOllamaProvider_HealthCheck_Healthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(ollamaTagsResponse{Models: []ollamaModelEntry{}})
	}))
	defer server.Close()

	p := NewOllamaProvider()
	if err := p.Configure(ProviderConfig{BaseURL: server.URL}); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	health, err := p.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck returned error: %v", err)
	}
	if !health.Healthy {
		t.Errorf("HealthCheck Healthy = false, want true")
	}
	if health.Message != "Ollama server is reachable" {
		t.Errorf("HealthCheck Message = %q, want %q", health.Message, "Ollama server is reachable")
	}
}

func TestOllamaProvider_HealthCheck_Unhealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	p := NewOllamaProvider()
	if err := p.Configure(ProviderConfig{BaseURL: server.URL}); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	health, err := p.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck returned error: %v", err)
	}
	if health.Healthy {
		t.Errorf("HealthCheck Healthy = true, want false")
	}
}
