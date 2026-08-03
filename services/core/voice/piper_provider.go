// piper_provider.go implements SPEC-0059: Piper TTS Integration - the first
// concrete core.TTSProvider (SPEC-0058), wrapping the Piper binary for local
// speech synthesis. Unlike whisper_provider.go (SPEC-0057), Piper ships as a
// standalone native binary rather than a Python package, so this provider
// shells out to cfg.PiperBinary directly instead of an embedded Python
// script.
package voice

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"

	cfgpkg "jarvis-pa/packages/config"
	"jarvis-pa/packages/logger"
	"jarvis-pa/services/core"
)

// piperReadBufferSize bounds each read from Piper's stdout during streaming
// synthesis - just a chunk size for forwarding audio as it arrives, not a
// protocol limit (unlike framing.go's length-prefixed frames, Piper's
// --output-raw stream carries no delimiters, so any chunk boundary is fine).
const piperReadBufferSize = 32 * 1024

// defaultSpeechSpeed is PiperProvider's own fallback for opts.Speed when a
// caller leaves it at its zero value - VoiceOptions' documented convention
// for "use the provider's default" - chosen slower than Piper's natural 1x
// pace, which testing found came out clearer as a project-wide default.
const defaultSpeechSpeed = 0.8

// PiperProvider implements core.TTSProvider using the Piper binary
// (SPEC-0059's local voice generation requirement), run as a one-shot
// subprocess per call: text is written to its stdin, and raw PCM audio
// (mono, int16 LE, per Piper's --output-raw mode) is read from its stdout.
type PiperProvider struct {
	cfg *cfgpkg.VoiceConfig
	log *logger.Logger
	bin string
}

// NewPiperProvider creates a PiperProvider that invokes cfg.PiperBinary.
func NewPiperProvider(cfg *cfgpkg.VoiceConfig, log *logger.Logger) *PiperProvider {
	return &PiperProvider{cfg: cfg, log: log, bin: cfg.PiperBinary}
}

// buildArgs assembles the Piper CLI arguments common to Synthesize and
// StreamSynthesize: --model selects the voice (cfg.TTSModel, passed straight
// through the same way WhisperProvider passes cfg.STTModel), --output-raw
// requests headerless PCM on stdout instead of a WAV file, opts.Voice maps to
// Piper's --speaker (multi-speaker model selection, the "voice model
// selection" requirement's per-call form), and opts.Speed maps to Piper's
// --length_scale - Piper's own speed knob, where smaller values speak
// faster, the inverse of opts.Speed's larger-is-faster convention. A caller
// leaving opts.Speed at its zero value gets defaultSpeechSpeed rather than
// Piper's own 1x pace, per VoiceOptions' "zero value falls back to the
// provider's own default" convention.
func (p *PiperProvider) buildArgs(opts core.VoiceOptions) []string {
	args := []string{"--model", p.cfg.TTSModel, "--output-raw"}
	if opts.Voice != "" {
		args = append(args, "--speaker", opts.Voice)
	}
	speed := opts.Speed
	if speed <= 0 {
		speed = defaultSpeechSpeed
	}
	args = append(args, "--length_scale", strconv.FormatFloat(1/speed, 'f', -1, 64))
	return args
}

// Synthesize implements core.TTSProvider: it runs Piper once, writing text to
// its stdin and reading the complete raw PCM output from its stdout.
// cmd.Stderr is captured and folded into the returned error on failure
// (rather than discarded), matching WhisperProvider.Transcribe's precedent
// for surfacing an otherwise-opaque "exit status 1".
func (p *PiperProvider) Synthesize(ctx context.Context, text string, opts core.VoiceOptions) ([]byte, error) {
	cmd := exec.CommandContext(ctx, p.bin, p.buildArgs(opts)...)
	cmd.Stdin = strings.NewReader(text)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if s := strings.TrimSpace(stderr.String()); s != "" {
			return nil, fmt.Errorf("voice: piper synthesize: %w (stderr: %s)", err, s)
		}
		return nil, fmt.Errorf("voice: piper synthesize: %w", err)
	}
	return stdout.Bytes(), nil
}

// StreamSynthesize implements core.TTSProvider: it runs Piper once, forwarding
// each chunk read from its stdout to audioCh as it arrives, and returns once
// Piper exits (synthesis completes) or ctx is cancelled - the interface's
// "runs until synthesis completes or ctx is cancelled" contract, distinct
// from STTProvider.StreamTranscribe's return-immediately/keep-running-in-
// background shape, since a TTS call has a natural end (the input text is
// finite) rather than an open-ended audio session.
func (p *PiperProvider) StreamSynthesize(ctx context.Context, text string, opts core.VoiceOptions, audioCh chan<- []byte) error {
	cmd := exec.CommandContext(ctx, p.bin, p.buildArgs(opts)...)
	cmd.Stdin = strings.NewReader(text)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("voice: create stdout pipe: %w", err)
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("voice: start piper stream: %w", err)
	}

	if err := p.forwardAudio(ctx, stdout, audioCh); err != nil {
		cmd.Wait()
		return err
	}

	if err := cmd.Wait(); err != nil {
		if s := strings.TrimSpace(stderrBuf.String()); s != "" {
			return fmt.Errorf("voice: piper stream: %w (stderr: %s)", err, s)
		}
		return fmt.Errorf("voice: piper stream: %w", err)
	}
	return nil
}

// forwardAudio reads chunks from stdout and sends each to audioCh until
// stdout is exhausted (Piper finished or was killed by ctx cancellation) or
// ctx is cancelled while a send is blocked on a full audioCh - the latter
// case returns ctx.Err() promptly instead of blocking past ctx's lifetime.
func (p *PiperProvider) forwardAudio(ctx context.Context, stdout io.Reader, audioCh chan<- []byte) error {
	buf := make([]byte, piperReadBufferSize)
	for {
		n, readErr := stdout.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			select {
			case audioCh <- chunk:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if readErr != nil {
			return nil // io.EOF (or the pipe closing after ctx killed Piper)
		}
	}
}

// Ensure PiperProvider implements core.TTSProvider.
var _ core.TTSProvider = (*PiperProvider)(nil)
