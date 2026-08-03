package core

import (
	"context"
	"testing"

	"jarvis-pa/packages/errors"
)

// stubSTTProvider is a minimal STTProvider implementation used to verify the
// SPEC-0056 contract can be implemented and driven by a caller. err, when
// set, is returned by Transcribe and StreamTranscribe, so a single stub
// covers both the success and failure paths.
type stubSTTProvider struct {
	result TranscriptionResult
	chunks []TranscriptionChunk
	err    error
}

func (p *stubSTTProvider) Transcribe(ctx context.Context, audio []byte) (TranscriptionResult, error) {
	if p.err != nil {
		return TranscriptionResult{}, p.err
	}
	return p.result, nil
}

func (p *stubSTTProvider) StreamTranscribe(ctx context.Context, audioCh <-chan []byte, resultCh chan<- TranscriptionChunk) error {
	if p.err != nil {
		return p.err
	}
	for _, chunk := range p.chunks {
		select {
		case resultCh <- chunk:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

var _ STTProvider = (*stubSTTProvider)(nil)

// TestSTTProvider_InterfaceCanBeImplemented verifies a concrete type can
// satisfy the STTProvider interface and that Transcribe reports both text
// and a confidence score (SPEC-0056 testing criterion: audio converts to
// text).
func TestSTTProvider_InterfaceCanBeImplemented(t *testing.T) {
	var provider STTProvider = &stubSTTProvider{
		result: TranscriptionResult{Text: "hello world", Confidence: 0.92},
	}

	got, err := provider.Transcribe(context.Background(), []byte{0x01, 0x02})
	if err != nil {
		t.Fatalf("Transcribe() returned error: %v", err)
	}
	if got.Text != "hello world" || got.Confidence != 0.92 {
		t.Errorf("Transcribe() = %+v, want Text=%q Confidence=%v", got, "hello world", 0.92)
	}
}

// TestSTTProvider_StreamingDeliversChunksInOrder verifies StreamTranscribe
// delivers incremental results, including confidence, in order and marks
// the final chunk Done (SPEC-0056 testing criterion: streaming
// transcription works).
func TestSTTProvider_StreamingDeliversChunksInOrder(t *testing.T) {
	provider := &stubSTTProvider{
		chunks: []TranscriptionChunk{
			{Text: "hello", Confidence: 0.8},
			{Text: "hello world", Confidence: 0.95, Done: true},
		},
	}

	audioCh := make(chan []byte)
	close(audioCh)
	resultCh := make(chan TranscriptionChunk, 2)

	if err := provider.StreamTranscribe(context.Background(), audioCh, resultCh); err != nil {
		t.Fatalf("StreamTranscribe() returned error: %v", err)
	}
	close(resultCh)

	var got []TranscriptionChunk
	for chunk := range resultCh {
		got = append(got, chunk)
	}

	if len(got) != 2 {
		t.Fatalf("got %d chunks, want 2: %+v", len(got), got)
	}
	if got[0].Text != "hello" || got[0].Done {
		t.Errorf("first chunk = %+v, want Text=hello Done=false", got[0])
	}
	if got[1].Text != "hello world" || !got[1].Done || got[1].Confidence != 0.95 {
		t.Errorf("last chunk = %+v, want Text=%q Done=true Confidence=0.95", got[1], "hello world")
	}
}

// TestSTTProvider_FailuresAreHandledCorrectly verifies that a failing
// provider surfaces its error from both Transcribe and StreamTranscribe
// rather than the call succeeding silently (SPEC-0056 testing criterion:
// errors are handled).
func TestSTTProvider_FailuresAreHandledCorrectly(t *testing.T) {
	wantErr := errors.New(errors.TypeUnavailable, "STT_PROVIDER_UNAVAILABLE", "core.sttprovider", "provider unavailable")
	provider := &stubSTTProvider{err: wantErr}

	if _, err := provider.Transcribe(context.Background(), []byte{0x01}); err != wantErr {
		t.Errorf("Transcribe() error = %v, want %v", err, wantErr)
	}

	audioCh := make(chan []byte)
	close(audioCh)
	if err := provider.StreamTranscribe(context.Background(), audioCh, make(chan TranscriptionChunk, 1)); err != wantErr {
		t.Errorf("StreamTranscribe() error = %v, want %v", err, wantErr)
	}
}

// TestSTTProvider_StreamTranscribeRespectsContextCancellation verifies a
// provider blocked delivering a chunk to a full resultCh stops promptly once
// ctx is cancelled, instead of blocking forever.
func TestSTTProvider_StreamTranscribeRespectsContextCancellation(t *testing.T) {
	provider := &stubSTTProvider{
		chunks: []TranscriptionChunk{{Text: "a"}, {Text: "b"}},
	}

	audioCh := make(chan []byte)
	close(audioCh)
	resultCh := make(chan TranscriptionChunk) // unbuffered and never drained

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := provider.StreamTranscribe(ctx, audioCh, resultCh); err != context.Canceled {
		t.Errorf("StreamTranscribe() error = %v, want %v", err, context.Canceled)
	}
}
