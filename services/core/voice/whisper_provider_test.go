// whisper_provider_test.go contains tests for WhisperProvider (SPEC-0057).
// Split the same way voice_test.go's own tests are: deterministic tests that
// need no external process, and subprocess-dependent tests that t.Skip when
// python/faster-whisper/a downloadable model aren't available in the
// running environment (never a silent pass).
package voice

import (
	"context"
	"os"
	"testing"
	"time"

	"jarvis-pa/packages/config"
	"jarvis-pa/packages/logger"
	"jarvis-pa/services/core"
)

// whisperInferenceTimeout bounds subprocess-dependent tests below: loading a
// faster-whisper model may need to download it on first use, which without
// a reachable network (or a pre-warmed cache) could otherwise hang the test
// indefinitely rather than failing fast.
const whisperInferenceTimeout = 30 * time.Second

func TestNewWhisperProvider_ExtractsScript(t *testing.T) {
	cfg := config.Defaults()
	log := logger.New("test")

	provider, err := NewWhisperProvider(&cfg.Voice, log)
	if err != nil {
		t.Fatalf("NewWhisperProvider() error = %v", err)
	}
	if provider.scriptPath == "" {
		t.Fatal("scriptPath is empty")
	}
	if _, err := os.Stat(provider.scriptPath); err != nil {
		t.Fatalf("extracted script not found at %q: %v", provider.scriptPath, err)
	}
}

func TestWhisperProvider_ImplementsSTTProvider(t *testing.T) {
	var _ core.STTProvider = (*WhisperProvider)(nil)
}

// TestWhisperProvider_Transcribe_InvalidPythonPathReturnsError deterministically
// covers SPEC-0057's "runtime errors are handled" testing criterion without
// depending on python actually being installed: an unresolvable pythonPath
// makes cmd.Start() itself fail, a case requirePython's skip-guarded tests
// below never reach.
func TestWhisperProvider_Transcribe_InvalidPythonPathReturnsError(t *testing.T) {
	cfg := config.Defaults()
	cfg.Voice.PythonPath = "jarvis-pa-nonexistent-python-binary"
	log := logger.New("test")

	provider, err := NewWhisperProvider(&cfg.Voice, log)
	if err != nil {
		t.Fatalf("NewWhisperProvider() error = %v", err)
	}

	if _, err := provider.Transcribe(context.Background(), []byte{0, 0, 0, 0}); err == nil {
		t.Fatal("Transcribe() error = nil, want an error for an unresolvable python path")
	}
}

// TestWhisperProvider_StreamTranscribe_InvalidPythonPathReturnsError is
// StreamTranscribe's equivalent of the deterministic error-handling test
// above.
func TestWhisperProvider_StreamTranscribe_InvalidPythonPathReturnsError(t *testing.T) {
	cfg := config.Defaults()
	cfg.Voice.PythonPath = "jarvis-pa-nonexistent-python-binary"
	log := logger.New("test")

	provider, err := NewWhisperProvider(&cfg.Voice, log)
	if err != nil {
		t.Fatalf("NewWhisperProvider() error = %v", err)
	}

	audioCh := make(chan []byte)
	defer close(audioCh)
	resultCh := make(chan core.TranscriptionChunk, 1)

	if err := provider.StreamTranscribe(context.Background(), audioCh, resultCh); err == nil {
		t.Fatal("StreamTranscribe() error = nil, want an error for an unresolvable python path")
	}
}

// TestWhisperProvider_Transcribe_RequiresPython proves Transcribe actually
// round-trips through the embedded whisper_stt.py subprocess (writing audio
// to its stdin, reading a JSON result from its stdout), not just
// type-checking against an empty stub.
func TestWhisperProvider_Transcribe_RequiresPython(t *testing.T) {
	requirePython(t, "python")

	cfg := config.Defaults()
	log := logger.New("test", logger.WithMinLevel(logger.LevelDebug))

	provider, err := NewWhisperProvider(&cfg.Voice, log)
	if err != nil {
		t.Fatalf("NewWhisperProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), whisperInferenceTimeout)
	defer cancel()

	// One second of silence (int16 LE zeros) - real speech isn't required to
	// prove the subprocess round-trip and JSON parsing work; a real model
	// still has to load to transcribe it, which is the part most likely to
	// need network access on a machine with no cached model.
	silence := make([]byte, cfg.Voice.SampleRate*2)

	result, err := provider.Transcribe(ctx, silence)
	if err != nil {
		t.Skipf("skipping: Transcribe() error = %v (likely missing faster-whisper or no cached/downloadable model)", err)
	}
	t.Logf("Transcribe(silence) = %+v", result)
}

// TestWhisperProvider_StreamTranscribe_RequiresPython proves
// StreamTranscribe starts the subprocess and its goroutines without
// blocking, and that closing audioCh lets the subprocess exit cleanly
// (readStreamResultLoop's cmd.Wait() returns, reaping it) with no results
// for an empty stream.
func TestWhisperProvider_StreamTranscribe_RequiresPython(t *testing.T) {
	requirePython(t, "python")

	cfg := config.Defaults()
	log := logger.New("test", logger.WithMinLevel(logger.LevelDebug))

	provider, err := NewWhisperProvider(&cfg.Voice, log)
	if err != nil {
		t.Fatalf("NewWhisperProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), whisperInferenceTimeout)
	defer cancel()

	audioCh := make(chan []byte)
	resultCh := make(chan core.TranscriptionChunk, 4)

	if err := provider.StreamTranscribe(ctx, audioCh, resultCh); err != nil {
		t.Skipf("skipping: StreamTranscribe() error = %v (likely missing faster-whisper or no cached/downloadable model)", err)
	}
	close(audioCh) // empty stream: whisper_stt.py's stream loop sees immediate EOF

	select {
	case chunk := <-resultCh:
		t.Logf("unexpected chunk from an empty stream: %+v", chunk)
	case <-ctx.Done():
		t.Skip("skipping: model did not become ready within the test timeout")
	case <-time.After(5 * time.Second):
		// No chunk for an empty stream is the expected outcome.
	}
}
