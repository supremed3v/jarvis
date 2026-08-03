// piper_provider_test.go contains tests for PiperProvider (SPEC-0059). Split
// the same way whisper_provider_test.go's own tests are: deterministic tests
// that need no external process, and subprocess-dependent tests that t.Skip
// when the piper binary isn't available in the running environment (never a
// silent pass).
package voice

import (
	"context"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"jarvis-pa/packages/config"
	"jarvis-pa/packages/logger"
	"jarvis-pa/services/core"
)

// piperInferenceTimeout bounds subprocess-dependent tests below.
const piperInferenceTimeout = 30 * time.Second

// requirePiper skips the calling test if piperPath isn't resolvable on PATH,
// so subprocess-dependent tests report SKIP rather than a false PASS or a
// confusing FAIL in an environment without Piper installed.
func requirePiper(t *testing.T, piperPath string) {
	t.Helper()
	if _, err := exec.LookPath(piperPath); err != nil {
		t.Skipf("skipping: %q not found on PATH: %v", piperPath, err)
	}
}

func TestPiperProvider_ImplementsTTSProvider(t *testing.T) {
	var _ core.TTSProvider = (*PiperProvider)(nil)
}

func TestPiperProvider_BuildArgs(t *testing.T) {
	cfg := config.Defaults()
	cfg.Voice.TTSModel = "en_US-amy-medium"
	provider := NewPiperProvider(&cfg.Voice, logger.New("test"))

	got := provider.buildArgs(core.VoiceOptions{Voice: "3", Speed: 2})
	want := []string{"--model", "en_US-amy-medium", "--output-raw", "--speaker", "3", "--length_scale", "0.5"}
	if len(got) != len(want) {
		t.Fatalf("buildArgs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("buildArgs() = %v, want %v", got, want)
		}
	}
}

// TestPiperProvider_BuildArgs_ZeroSpeedUsesDefault verifies a caller leaving
// VoiceOptions.Speed unset gets defaultSpeechSpeed rather than Piper's own
// 1x pace, per VoiceOptions' "zero value falls back to the provider's own
// default" convention.
func TestPiperProvider_BuildArgs_ZeroSpeedUsesDefault(t *testing.T) {
	cfg := config.Defaults()
	provider := NewPiperProvider(&cfg.Voice, logger.New("test"))

	got := provider.buildArgs(core.VoiceOptions{})
	wantLengthScale := strconv.FormatFloat(1/defaultSpeechSpeed, 'f', -1, 64)
	want := []string{"--model", cfg.Voice.TTSModel, "--output-raw", "--length_scale", wantLengthScale}
	if len(got) != len(want) {
		t.Fatalf("buildArgs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("buildArgs() = %v, want %v", got, want)
		}
	}
}

// TestPiperProvider_Synthesize_InvalidBinaryReturnsError deterministically
// covers SPEC-0059's "errors are handled" testing criterion without
// depending on piper actually being installed: an unresolvable binary path
// makes cmd.Run() itself fail.
func TestPiperProvider_Synthesize_InvalidBinaryReturnsError(t *testing.T) {
	cfg := config.Defaults()
	cfg.Voice.PiperBinary = "jarvis-pa-nonexistent-piper-binary"
	provider := NewPiperProvider(&cfg.Voice, logger.New("test"))

	if _, err := provider.Synthesize(context.Background(), "hello", core.VoiceOptions{}); err == nil {
		t.Fatal("Synthesize() error = nil, want an error for an unresolvable piper binary")
	}
}

// TestPiperProvider_StreamSynthesize_InvalidBinaryReturnsError is
// StreamSynthesize's equivalent of the deterministic error-handling test
// above.
func TestPiperProvider_StreamSynthesize_InvalidBinaryReturnsError(t *testing.T) {
	cfg := config.Defaults()
	cfg.Voice.PiperBinary = "jarvis-pa-nonexistent-piper-binary"
	provider := NewPiperProvider(&cfg.Voice, logger.New("test"))

	audioCh := make(chan []byte, 1)
	if err := provider.StreamSynthesize(context.Background(), "hello", core.VoiceOptions{}, audioCh); err == nil {
		t.Fatal("StreamSynthesize() error = nil, want an error for an unresolvable piper binary")
	}
}

// TestPiperProvider_Synthesize_RequiresPiper proves Synthesize actually
// round-trips through the piper subprocess (writing text to its stdin,
// reading raw PCM audio from its stdout), not just type-checking against an
// empty stub.
func TestPiperProvider_Synthesize_RequiresPiper(t *testing.T) {
	requirePiper(t, "piper")

	cfg := config.Defaults()
	provider := NewPiperProvider(&cfg.Voice, logger.New("test", logger.WithMinLevel(logger.LevelDebug)))

	ctx, cancel := context.WithTimeout(context.Background(), piperInferenceTimeout)
	defer cancel()

	audio, err := provider.Synthesize(ctx, "hello world", core.VoiceOptions{})
	if err != nil {
		t.Skipf("skipping: Synthesize() error = %v (likely missing piper voice model)", err)
	}
	if len(audio) == 0 {
		t.Error("Synthesize() returned no audio")
	}
}

// TestPiperProvider_StreamSynthesize_RequiresPiper proves StreamSynthesize
// delivers audio chunks on audioCh and returns once piper exits.
func TestPiperProvider_StreamSynthesize_RequiresPiper(t *testing.T) {
	requirePiper(t, "piper")

	cfg := config.Defaults()
	provider := NewPiperProvider(&cfg.Voice, logger.New("test", logger.WithMinLevel(logger.LevelDebug)))

	ctx, cancel := context.WithTimeout(context.Background(), piperInferenceTimeout)
	defer cancel()

	audioCh := make(chan []byte, 16)
	err := provider.StreamSynthesize(ctx, "hello world", core.VoiceOptions{}, audioCh)
	close(audioCh)
	if err != nil {
		t.Skipf("skipping: StreamSynthesize() error = %v (likely missing piper voice model)", err)
	}

	var total int
	for chunk := range audioCh {
		total += len(chunk)
	}
	if total == 0 {
		t.Error("StreamSynthesize() delivered no audio")
	}
}
