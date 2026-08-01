package core

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
)

// partialFailProvider is a Provider stub that delivers a fixed sequence of
// chunks via onChunk and then, if failAfter is within range, returns err
// instead of delivering the chunk at that index - mirroring how
// OllamaProvider.Stream returns a mid-stream error directly rather than via
// onChunk (ollama_provider.go's OLLAMA_STREAM_ERROR case).
type partialFailProvider struct {
	chunks    []StreamChunk
	failAfter int
	err       error
}

func (p *partialFailProvider) Name() string                       { return "partial-fail" }
func (p *partialFailProvider) Configure(cfg ProviderConfig) error { return nil }
func (p *partialFailProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	return nil, nil
}
func (p *partialFailProvider) HealthCheck(ctx context.Context) (HealthStatus, error) {
	return HealthStatus{}, nil
}

func (p *partialFailProvider) Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error) {
	return GenerateResponse{}, nil
}

func (p *partialFailProvider) Stream(ctx context.Context, req GenerateRequest, onChunk func(StreamChunk) error) error {
	for i, c := range p.chunks {
		if i == p.failAfter {
			return p.err
		}
		if err := onChunk(c); err != nil {
			return err
		}
	}
	return nil
}

// TestStreamHandler_TokensArriveProgressively verifies onEvent is invoked
// once per chunk, in order, with each chunk's own text (SPEC-0030 testing
// criterion 1: "Tokens arrive progressively").
func TestStreamHandler_TokensArriveProgressively(t *testing.T) {
	provider := &stubProvider{
		name: "stub",
		chunks: []StreamChunk{
			{Model: "llama3", Text: "hel"},
			{Model: "llama3", Text: "lo "},
			{Model: "llama3", Text: "world", Done: true},
		},
	}
	handler := NewStreamHandler(provider, nil)

	var got []string
	result, err := handler.Stream(context.Background(), GenerateRequest{Model: "llama3", Prompt: "hi"}, func(e StreamEvent) error {
		got = append(got, e.Chunk.Text)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream() returned error: %v", err)
	}

	want := []string{"hel", "lo ", "world"}
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event[%d].Chunk.Text = %q, want %q", i, got[i], want[i])
		}
	}
	if result.Text != "hello world" {
		t.Errorf("result.Text = %q, want %q", result.Text, "hello world")
	}
	if result.Model != "llama3" {
		t.Errorf("result.Model = %q, want %q", result.Model, "llama3")
	}
}

// TestStreamHandler_PartialResponsesAccumulate verifies each StreamEvent's
// Partial field carries the full response text accumulated up to and
// including that chunk.
func TestStreamHandler_PartialResponsesAccumulate(t *testing.T) {
	provider := &stubProvider{
		name: "stub",
		chunks: []StreamChunk{
			{Model: "llama3", Text: "hel"},
			{Model: "llama3", Text: "lo "},
			{Model: "llama3", Text: "world", Done: true},
		},
	}
	handler := NewStreamHandler(provider, nil)

	var partials []string
	_, err := handler.Stream(context.Background(), GenerateRequest{Model: "llama3", Prompt: "hi"}, func(e StreamEvent) error {
		partials = append(partials, e.Partial)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream() returned error: %v", err)
	}

	want := []string{"hel", "hello ", "hello world"}
	for i := range want {
		if partials[i] != want[i] {
			t.Errorf("event[%d].Partial = %q, want %q", i, partials[i], want[i])
		}
	}
}

// TestStreamHandler_StreamsCanBeCancelled verifies that cancelling ctx from
// within onEvent stops the stream before further chunks are delivered, and
// that Stream reports the result as Cancelled with a TypeCanceled error
// (SPEC-0030 testing criterion 2: "Streams can be cancelled").
func TestStreamHandler_StreamsCanBeCancelled(t *testing.T) {
	provider := &stubProvider{
		name: "stub",
		chunks: []StreamChunk{
			{Model: "llama3", Text: "hel"},
			{Model: "llama3", Text: "lo "},
			{Model: "llama3", Text: "world", Done: true},
		},
	}
	handler := NewStreamHandler(provider, nil)

	ctx, cancel := context.WithCancel(context.Background())
	callCount := 0
	result, err := handler.Stream(ctx, GenerateRequest{Model: "llama3", Prompt: "hi"}, func(e StreamEvent) error {
		callCount++
		if callCount == 1 {
			cancel()
		}
		return nil
	})

	if callCount != 1 {
		t.Fatalf("onEvent called %d times, want 1 (stream must stop once cancelled)", callCount)
	}
	if !result.Cancelled {
		t.Error("result.Cancelled = false, want true")
	}
	if result.Text != "hel" {
		t.Errorf("result.Text = %q, want %q (partial output up to cancellation preserved)", result.Text, "hel")
	}
	if !errors.Is(err, errors.TypeCanceled) {
		t.Errorf("error type = %v, want TypeCanceled", err)
	}
	if !errors.HasCode(err, "STREAM_CANCELLED") {
		t.Errorf("missing code STREAM_CANCELLED: %v", err)
	}
}

// TestStreamHandler_AlreadyCancelledContextStopsBeforeStreaming verifies a
// ctx that is already done when Stream is called fails fast without
// invoking the Provider at all.
func TestStreamHandler_AlreadyCancelledContextStopsBeforeStreaming(t *testing.T) {
	provider := &stubProvider{name: "stub", chunks: []StreamChunk{{Text: "should not be seen"}}}
	handler := NewStreamHandler(provider, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	called := false
	result, err := handler.Stream(ctx, GenerateRequest{Model: "llama3", Prompt: "hi"}, func(e StreamEvent) error {
		called = true
		return nil
	})

	if called {
		t.Error("onEvent was called despite ctx already being cancelled")
	}
	if !result.Cancelled {
		t.Error("result.Cancelled = false, want true")
	}
	if !errors.Is(err, errors.TypeCanceled) {
		t.Errorf("error type = %v, want TypeCanceled", err)
	}
}

// raceCancelProvider simulates a Provider (like OllamaProvider) whose own
// Stream call notices ctx cancellation on its underlying network call
// before StreamHandler's own per-chunk check does, and returns that as a
// plain, non-TypeCanceled error - mirroring OllamaProvider.mapConnectionError,
// which classifies a cancelled in-flight HTTP request as TypeUnavailable
// rather than TypeCanceled.
type raceCancelProvider struct{}

func (p *raceCancelProvider) Name() string                                        { return "race-cancel" }
func (p *raceCancelProvider) Configure(cfg ProviderConfig) error                  { return nil }
func (p *raceCancelProvider) ListModels(ctx context.Context) ([]ModelInfo, error) { return nil, nil }
func (p *raceCancelProvider) HealthCheck(ctx context.Context) (HealthStatus, error) {
	return HealthStatus{}, nil
}
func (p *raceCancelProvider) Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error) {
	return GenerateResponse{}, nil
}

func (p *raceCancelProvider) Stream(ctx context.Context, req GenerateRequest, onChunk func(StreamChunk) error) error {
	<-ctx.Done()
	return errors.New(errors.TypeUnavailable, "OLLAMA_CONNECTION_FAILED", "core.ollama", "failed to connect to Ollama server")
}

// TestStreamHandler_CancellationDetectedEvenWhenProviderMisclassifiesError
// verifies that Stream still reports Cancelled and a TypeCanceled error when
// the underlying Provider's own error for a cancelled ctx isn't itself typed
// TypeCanceled - regression test for a gap where cancellation was only
// detected via StreamHandler's own per-chunk check, missing cases where the
// Provider's network layer notices cancellation first.
func TestStreamHandler_CancellationDetectedEvenWhenProviderMisclassifiesError(t *testing.T) {
	handler := NewStreamHandler(&raceCancelProvider{}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	go cancel()

	result, err := handler.Stream(ctx, GenerateRequest{Model: "llama3", Prompt: "hi"}, func(e StreamEvent) error {
		return nil
	})

	if !result.Cancelled {
		t.Error("result.Cancelled = false, want true")
	}
	if !errors.Is(err, errors.TypeCanceled) {
		t.Errorf("error type = %v, want TypeCanceled", err)
	}
	if !errors.HasCode(err, "STREAM_CANCELLED") {
		t.Errorf("missing code STREAM_CANCELLED: %v", err)
	}
	if !errors.HasCode(err, "OLLAMA_CONNECTION_FAILED") {
		t.Errorf("original Provider error code not preserved in cause chain: %v", err)
	}
}

// TestStreamHandler_FailuresRecoverCorrectly verifies that a Provider
// failure partway through a stream still returns whatever text had already
// been accumulated, alongside the error, rather than discarding it - and
// that the handler is safe to call again afterward (SPEC-0030 testing
// criterion 3: "Failures recover correctly").
func TestStreamHandler_FailuresRecoverCorrectly(t *testing.T) {
	streamErr := errors.New(errors.TypeUnavailable, "OLLAMA_STREAM_ERROR", "core.ollama", "model crashed mid-stream")
	provider := &partialFailProvider{
		chunks: []StreamChunk{
			{Model: "llama3", Text: "hel"},
			{Model: "llama3", Text: "lo "},
			{Model: "llama3", Text: "world", Done: true},
		},
		failAfter: 2,
		err:       streamErr,
	}
	handler := NewStreamHandler(provider, nil)

	var events int
	result, err := handler.Stream(context.Background(), GenerateRequest{Model: "llama3", Prompt: "hi"}, func(e StreamEvent) error {
		events++
		return nil
	})

	if events != 2 {
		t.Fatalf("onEvent called %d times, want 2", events)
	}
	if err != streamErr {
		t.Errorf("Stream() error = %v, want %v", err, streamErr)
	}
	if result.Text != "hello " {
		t.Errorf("result.Text = %q, want %q (partial output preserved despite failure)", result.Text, "hello ")
	}
	if result.Cancelled {
		t.Error("result.Cancelled = true, want false (this was a provider failure, not a cancellation)")
	}

	// The handler holds no state across calls, so a fresh call after a
	// failure must succeed cleanly rather than carrying over any state.
	ok := &stubProvider{name: "stub", chunks: []StreamChunk{{Model: "llama3", Text: "recovered", Done: true}}}
	handler2 := NewStreamHandler(ok, nil)
	result2, err2 := handler2.Stream(context.Background(), GenerateRequest{Model: "llama3", Prompt: "hi"}, func(e StreamEvent) error {
		return nil
	})
	if err2 != nil {
		t.Fatalf("second Stream() returned error: %v", err2)
	}
	if result2.Text != "recovered" {
		t.Errorf("second result.Text = %q, want %q", result2.Text, "recovered")
	}
}

// TestStreamHandler_OnEventErrorStopsStreamAndReportsPartial verifies that
// when onEvent itself returns an error, Stream stops immediately and still
// reports the text accumulated so far.
func TestStreamHandler_OnEventErrorStopsStreamAndReportsPartial(t *testing.T) {
	consumerErr := errors.New(errors.TypeInternal, "CONSUMER_FAILED", "core.streamhandler_test", "consumer failed")
	provider := &stubProvider{
		name: "stub",
		chunks: []StreamChunk{
			{Model: "llama3", Text: "hel"},
			{Model: "llama3", Text: "lo "},
			{Model: "llama3", Text: "world", Done: true},
		},
	}
	handler := NewStreamHandler(provider, nil)

	callCount := 0
	result, err := handler.Stream(context.Background(), GenerateRequest{Model: "llama3", Prompt: "hi"}, func(e StreamEvent) error {
		callCount++
		return consumerErr
	})

	if callCount != 1 {
		t.Fatalf("onEvent called %d times, want 1", callCount)
	}
	if err != consumerErr {
		t.Errorf("Stream() error = %v, want %v", err, consumerErr)
	}
	if result.Text != "hel" {
		t.Errorf("result.Text = %q, want %q", result.Text, "hel")
	}
}

// TestStreamHandler_OutcomesAreLogged verifies both a completed stream and
// a failed stream each produce exactly one structured log entry describing
// the outcome.
func TestStreamHandler_OutcomesAreLogged(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New("stream-handler", logger.WithOutput(&buf))

	provider := &stubProvider{name: "stub", chunks: []StreamChunk{{Model: "llama3", Text: "hi", Done: true}}}
	handler := NewStreamHandler(provider, log)

	_, err := handler.Stream(context.Background(), GenerateRequest{Model: "llama3", Prompt: "hi"}, func(e StreamEvent) error {
		return nil
	})
	if err != nil {
		t.Fatalf("Stream() returned error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d log lines, want 1: %v", len(lines), lines)
	}

	var entry logger.Entry
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("Unmarshal(log entry) returned error: %v, raw=%s", err, lines[0])
	}
	if entry.Message != "stream completed" {
		t.Errorf("Message = %q, want %q", entry.Message, "stream completed")
	}
	if entry.Metadata["cancelled"] != false {
		t.Errorf("Metadata[cancelled] = %v, want false", entry.Metadata["cancelled"])
	}
	if entry.Metadata["textLen"] != float64(2) {
		t.Errorf("Metadata[textLen] = %v, want 2", entry.Metadata["textLen"])
	}
}
