// ollama_embedder_test.go implements SPEC-0039 tests for OllamaEmbedder.
package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOllamaEmbedder_Embed_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		var req ollamaEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if req.Model != "nomic-embed-text" {
			t.Errorf("request Model = %q, want %q", req.Model, "nomic-embed-text")
		}
		if req.Prompt != "hello world" {
			t.Errorf("request Prompt = %q, want %q", req.Prompt, "hello world")
		}

		json.NewEncoder(w).Encode(ollamaEmbedResponse{Embedding: []float64{0.1, 0.2, 0.3}})
	}))
	defer server.Close()

	e := NewOllamaEmbedder(server.URL, "nomic-embed-text")
	got := e.Embed("hello world")

	want := []float64{0.1, 0.2, 0.3}
	if len(got) != len(want) {
		t.Fatalf("Embed() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Embed() = %v, want %v", got, want)
		}
	}
}

func TestOllamaEmbedder_Embed_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	e := NewOllamaEmbedder(server.URL, "nomic-embed-text")
	if got := e.Embed("hello"); got != nil {
		t.Fatalf("Embed() = %v, want nil on HTTP error", got)
	}
}

func TestOllamaEmbedder_Embed_ServerReportedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(ollamaEmbedResponse{Error: "model not found"})
	}))
	defer server.Close()

	e := NewOllamaEmbedder(server.URL, "nomic-embed-text")
	if got := e.Embed("hello"); got != nil {
		t.Fatalf("Embed() = %v, want nil when the server reports an error", got)
	}
}

func TestOllamaEmbedder_Embed_MalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	e := NewOllamaEmbedder(server.URL, "nomic-embed-text")
	if got := e.Embed("hello"); got != nil {
		t.Fatalf("Embed() = %v, want nil on malformed response body", got)
	}
}

func TestOllamaEmbedder_Embed_ConnectionFailure(t *testing.T) {
	e := NewOllamaEmbedder("http://127.0.0.1:1", "nomic-embed-text")
	if got := e.Embed("hello"); got != nil {
		t.Fatalf("Embed() = %v, want nil when the server is unreachable", got)
	}
}

func TestOllamaEmbedder_Embed_NoModelConfigured(t *testing.T) {
	e := NewOllamaEmbedder("http://127.0.0.1:11434", "")
	if got := e.Embed("hello"); got != nil {
		t.Fatalf("Embed() = %v, want nil when no model is configured", got)
	}
}

func TestNewOllamaEmbedder_DefaultsBaseURL(t *testing.T) {
	e := NewOllamaEmbedder("", "nomic-embed-text")
	if e.baseURL != "http://127.0.0.1:11434" {
		t.Errorf("default baseURL = %q, want %q", e.baseURL, "http://127.0.0.1:11434")
	}
}

func TestNewOllamaEmbedder_TrimsTrailingSlash(t *testing.T) {
	e := NewOllamaEmbedder("http://example.com:11434/", "nomic-embed-text")
	if e.baseURL != "http://example.com:11434" {
		t.Errorf("baseURL = %q, want trailing slash trimmed", e.baseURL)
	}
}
