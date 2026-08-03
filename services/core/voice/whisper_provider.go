// whisper_provider.go implements SPEC-0057: Whisper Integration - the first
// concrete core.STTProvider (SPEC-0056), wrapping faster-whisper via a
// Python subprocess for local inference, matching the same
// no-CGO/Windows-friendly subprocess approach audio_engine.go (SPEC-0053)
// and wake_word.go (SPEC-0055) already use.
package voice

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"

	cfgpkg "jarvis-pa/packages/config"
	"jarvis-pa/packages/logger"
	"jarvis-pa/services/core"
)

// streamSegmentSeconds is the fixed segment length StreamTranscribe asks the
// whisper_stt.py "stream" subcommand to buffer before transcribing (see that
// script's module docstring for why Whisper's lack of true incremental
// decoding makes fixed-length segments, not word-by-word partials, the
// standard way to get streaming-shaped output from it).
const streamSegmentSeconds = 3.0

//go:embed scripts/whisper_stt.py
var whisperSTTScript []byte

// whisperResult mirrors whisper_stt.py's JSON output shape for both
// "transcribe" (Done always absent/false, unused) and "stream" (Done always
// true) subcommands.
type whisperResult struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
	Done       bool    `json:"done"`
}

// WhisperProvider implements core.STTProvider using faster-whisper, run as a
// Python subprocess per SPEC-0057's local-inference requirement.
type WhisperProvider struct {
	cfg        *cfgpkg.VoiceConfig
	log        *logger.Logger
	pythonPath string
	scriptPath string
}

// NewWhisperProvider creates a WhisperProvider backed by the embedded
// whisper_stt.py script (extracted to a temp file so it can be exec'd
// regardless of the process's working directory or install location).
// cfg supplies the "Model size" (STTModel), "Language settings"
// (STTLanguage), and "Device selection" (STTDevice) configuration SPEC-0057
// requires.
func NewWhisperProvider(cfg *cfgpkg.VoiceConfig, log *logger.Logger) (*WhisperProvider, error) {
	scriptPath, err := extractScript("whisper_stt.py", whisperSTTScript)
	if err != nil {
		return nil, err
	}
	return &WhisperProvider{
		cfg:        cfg,
		log:        log,
		pythonPath: cfg.PythonPath,
		scriptPath: scriptPath,
	}, nil
}

// Transcribe implements core.STTProvider: it runs the embedded script's
// "transcribe" subcommand, writing audio to its stdin and reading a single
// JSON result line from its stdout. Writing the full buffer before reading
// any output is safe here (no deadlock) because whisper_stt.py's
// cmd_transcribe reads stdin to EOF before producing any output, the same
// precedent audio_engine.go's Playback already relies on for writing a
// potentially-large buffer to a subprocess's stdin. cmd.Stderr is captured
// into a buffer (rather than discarded) and folded into the returned error
// on failure, since whisper_stt.py's one truly actionable failure - a
// missing faster-whisper install - is reported only via stderr before it
// exits 1; without this, that failure would reach the caller as an opaque
// "exit status 1".
func (p *WhisperProvider) Transcribe(ctx context.Context, audio []byte) (core.TranscriptionResult, error) {
	cmd := exec.CommandContext(ctx, p.pythonPath, p.scriptPath, "transcribe",
		fmt.Sprintf("--model=%s", p.cfg.STTModel),
		fmt.Sprintf("--language=%s", p.cfg.STTLanguage),
		fmt.Sprintf("--device=%s", p.cfg.STTDevice),
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return core.TranscriptionResult{}, fmt.Errorf("voice: create stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return core.TranscriptionResult{}, fmt.Errorf("voice: create stdout pipe: %w", err)
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return core.TranscriptionResult{}, fmt.Errorf("voice: start whisper transcribe: %w", err)
	}

	if _, err := stdin.Write(audio); err != nil {
		stdin.Close()
		cmd.Wait()
		return core.TranscriptionResult{}, fmt.Errorf("voice: write audio to whisper: %w", err)
	}
	stdin.Close()

	var result whisperResult
	scanner := bufio.NewScanner(stdout)
	if scanner.Scan() {
		if err := json.Unmarshal(scanner.Bytes(), &result); err != nil {
			cmd.Wait()
			return core.TranscriptionResult{}, fmt.Errorf("voice: parse whisper output: %w", err)
		}
	}

	if err := cmd.Wait(); err != nil {
		if stderrText := strings.TrimSpace(stderrBuf.String()); stderrText != "" {
			return core.TranscriptionResult{}, fmt.Errorf("voice: whisper transcribe: %w (stderr: %s)", err, stderrText)
		}
		return core.TranscriptionResult{}, fmt.Errorf("voice: whisper transcribe: %w", err)
	}
	return core.TranscriptionResult{Text: result.Text, Confidence: result.Confidence}, nil
}

// StreamTranscribe implements core.STTProvider: it launches the embedded
// script's "stream" subcommand and starts goroutines forwarding audioCh
// chunks to its stdin as length-prefixed frames (framing.go's protocol,
// matching wake_word.go's writeAudioLoop), parsing each JSON result line
// from its stdout into a TranscriptionChunk on resultCh, and draining its
// stderr. It returns once the subprocess has started, not once streaming
// finishes - the same "returns immediately, keeps running in background"
// shape WakeWordDetector.Start uses, since STTProvider has no separate Stop
// method: the stream instead ends when audioCh closes (stdin closes, giving
// whisper_stt.py a chance to flush any partial final segment and exit
// cleanly) or ctx is cancelled (CommandContext kills the subprocess
// immediately).
func (p *WhisperProvider) StreamTranscribe(ctx context.Context, audioCh <-chan []byte, resultCh chan<- core.TranscriptionChunk) error {
	cmd := exec.CommandContext(ctx, p.pythonPath, p.scriptPath, "stream",
		fmt.Sprintf("--model=%s", p.cfg.STTModel),
		fmt.Sprintf("--language=%s", p.cfg.STTLanguage),
		fmt.Sprintf("--device=%s", p.cfg.STTDevice),
		fmt.Sprintf("--sample-rate=%d", p.cfg.SampleRate),
		fmt.Sprintf("--segment-seconds=%g", streamSegmentSeconds),
	)

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("voice: create stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("voice: create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("voice: create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("voice: start whisper stream: %w", err)
	}

	go p.drainStderr(stderr)
	go writeStreamAudioLoop(ctx, stdinPipe, audioCh)
	go p.readStreamResultLoop(ctx, stdout, resultCh, cmd)

	return nil
}

// drainStderr logs each stderr line from the whisper subprocess at Debug
// level, matching audio_engine.go's drainCaptureStderr precedent, instead of
// silently discarding it the way wake_word.go's bare drainReader does -
// StreamTranscribe has no synchronous return path to fold a failure's
// stderr into (unlike Transcribe), so logging is the only way an actionable
// failure like a missing faster-whisper install stays visible.
func (p *WhisperProvider) drainStderr(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if p.log != nil {
			p.log.Debug("voice: whisper stderr", map[string]any{"msg": scanner.Text()})
		}
	}
}

// writeStreamAudioLoop forwards audioCh chunks to stdinPipe as
// length-prefixed frames until audioCh closes, ctx is done, or a write
// fails, then closes stdinPipe - signalling EOF to the subprocess's stdin
// read loop either way, so it always gets a chance to exit (with or without
// flushing a final partial segment, per its own EOF handling).
func writeStreamAudioLoop(ctx context.Context, stdinPipe io.WriteCloser, audioCh <-chan []byte) {
	defer stdinPipe.Close()
	stdin := bufio.NewWriter(stdinPipe)
	for {
		select {
		case pcm, ok := <-audioCh:
			if !ok {
				return
			}
			if err := writeFrame(stdin, pcm); err != nil {
				return
			}
			if err := stdin.Flush(); err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// readStreamResultLoop reads newline-delimited JSON result lines from
// stdout, forwarding each as a TranscriptionChunk on resultCh (dropping the
// send, not blocking past ctx's lifetime, if ctx is cancelled first), until
// stdout closes - then reaps the subprocess via cmd.Wait() so it never
// becomes a zombie process.
func (p *WhisperProvider) readStreamResultLoop(ctx context.Context, stdout io.Reader, resultCh chan<- core.TranscriptionChunk, cmd *exec.Cmd) {
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		var result whisperResult
		if err := json.Unmarshal(scanner.Bytes(), &result); err != nil {
			if p.log != nil {
				p.log.Error("voice: parse whisper stream output", map[string]any{"error": err.Error()})
			}
			continue
		}
		select {
		case resultCh <- core.TranscriptionChunk{Text: result.Text, Confidence: result.Confidence, Done: result.Done}:
		case <-ctx.Done():
			return
		}
	}
	cmd.Wait()
}

// Ensure WhisperProvider implements core.STTProvider.
var _ core.STTProvider = (*WhisperProvider)(nil)
