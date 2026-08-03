package core

import (
	"bytes"
	"context"
	"testing"

	"jarvis-pa/packages/errors"
)

// stubTTSProvider is a minimal TTSProvider implementation used to verify the
// SPEC-0058 contract can be implemented and driven by a caller. err, when
// set, is returned by Synthesize and StreamSynthesize, so a single stub
// covers both the success and failure paths.
type stubTTSProvider struct {
	audio  []byte
	chunks [][]byte
	err    error
}

func (p *stubTTSProvider) Synthesize(ctx context.Context, text string, opts VoiceOptions) ([]byte, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.audio, nil
}

func (p *stubTTSProvider) StreamSynthesize(ctx context.Context, text string, opts VoiceOptions, audioCh chan<- []byte) error {
	if p.err != nil {
		return p.err
	}
	for _, chunk := range p.chunks {
		select {
		case audioCh <- chunk:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

var _ TTSProvider = (*stubTTSProvider)(nil)

// TestTTSProvider_InterfaceCanBeImplemented verifies a concrete type can
// satisfy the TTSProvider interface and that Synthesize produces audio from
// text (SPEC-0058 testing criterion: audio output is generated).
func TestTTSProvider_InterfaceCanBeImplemented(t *testing.T) {
	want := []byte{0x01, 0x02, 0x03}
	provider := &stubTTSProvider{audio: want}

	got, err := provider.Synthesize(context.Background(), "hello world", VoiceOptions{})
	if err != nil {
		t.Fatalf("Synthesize() returned error: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("Synthesize() = %v, want %v", got, want)
	}
}

// TestTTSProvider_StreamingDeliversChunksInOrder verifies StreamSynthesize
// delivers incremental audio chunks in order (SPEC-0058 testing criterion:
// audio output is generated, via the streaming path).
func TestTTSProvider_StreamingDeliversChunksInOrder(t *testing.T) {
	provider := &stubTTSProvider{
		chunks: [][]byte{{0x01}, {0x02, 0x03}},
	}

	audioCh := make(chan []byte, 2)
	if err := provider.StreamSynthesize(context.Background(), "hello", VoiceOptions{}, audioCh); err != nil {
		t.Fatalf("StreamSynthesize() returned error: %v", err)
	}
	close(audioCh)

	var got [][]byte
	for chunk := range audioCh {
		got = append(got, chunk)
	}

	if len(got) != 2 {
		t.Fatalf("got %d chunks, want 2: %+v", len(got), got)
	}
	if !bytes.Equal(got[0], []byte{0x01}) || !bytes.Equal(got[1], []byte{0x02, 0x03}) {
		t.Errorf("chunks = %v, want [[0x01] [0x02 0x03]]", got)
	}
}

// TestTTSProvider_FailuresAreHandledCorrectly verifies that a failing
// provider surfaces its error from both Synthesize and StreamSynthesize
// rather than the call succeeding silently (SPEC-0058 testing criterion:
// errors are handled).
func TestTTSProvider_FailuresAreHandledCorrectly(t *testing.T) {
	wantErr := errors.New(errors.TypeUnavailable, "TTS_PROVIDER_UNAVAILABLE", "core.ttsprovider", "provider unavailable")
	provider := &stubTTSProvider{err: wantErr}

	if _, err := provider.Synthesize(context.Background(), "hello", VoiceOptions{}); err != wantErr {
		t.Errorf("Synthesize() error = %v, want %v", err, wantErr)
	}

	audioCh := make(chan []byte, 1)
	if err := provider.StreamSynthesize(context.Background(), "hello", VoiceOptions{}, audioCh); err != wantErr {
		t.Errorf("StreamSynthesize() error = %v, want %v", err, wantErr)
	}
}

// TestTTSProvider_StreamSynthesizeRespectsContextCancellation verifies a
// provider blocked delivering a chunk to a full audioCh stops promptly once
// ctx is cancelled, instead of blocking forever.
func TestTTSProvider_StreamSynthesizeRespectsContextCancellation(t *testing.T) {
	provider := &stubTTSProvider{
		chunks: [][]byte{{0x01}, {0x02}},
	}

	audioCh := make(chan []byte) // unbuffered and never drained

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := provider.StreamSynthesize(ctx, "hello", VoiceOptions{}, audioCh); err != context.Canceled {
		t.Errorf("StreamSynthesize() error = %v, want %v", err, context.Canceled)
	}
}
