// audio_engine.go implements SPEC-0053: Audio Engine Interface.
// Uses a Python subprocess (sounddevice) for audio capture and playback,
// avoiding a CGO dependency and working reliably on Windows without
// requiring a C compiler.
package voice

import (
	"bufio"
	_ "embed"
	"fmt"
	"io"
	"os/exec"
	"sync"

	cfgpkg "jarvis-pa/packages/config"
	"jarvis-pa/packages/logger"
	"jarvis-pa/services/core"
)

//go:embed scripts/audio_engine.py
var audioEngineScript []byte

// AudioEngine implements core.VoiceEngine using a Python subprocess.
type AudioEngine struct {
	cfg        *cfgpkg.VoiceConfig
	log        *logger.Logger
	pythonPath string
	scriptPath string

	mu         sync.Mutex
	running    bool
	captureCmd *exec.Cmd
	captureCh  chan []byte
	captureWG  sync.WaitGroup
}

// NewAudioEngine creates an AudioEngine backed by the embedded
// audio_engine.py script (extracted to a temp file so it can be exec'd
// regardless of the process's working directory or install location).
func NewAudioEngine(cfg *cfgpkg.VoiceConfig, log *logger.Logger) (*AudioEngine, error) {
	scriptPath, err := extractScript("audio_engine.py", audioEngineScript)
	if err != nil {
		return nil, err
	}
	return &AudioEngine{
		cfg:        cfg,
		log:        log,
		pythonPath: cfg.PythonPath,
		scriptPath: scriptPath,
	}, nil
}

// Initialize implements core.VoiceEngine.
func (e *AudioEngine) Initialize(cfg *cfgpkg.VoiceConfig, log *logger.Logger) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.running {
		return nil
	}

	e.cfg = cfg
	e.log = log
	e.captureCh = make(chan []byte, 100)
	e.running = true

	e.log.Info("voice: audio engine initialized",
		map[string]any{"sample_rate": cfg.SampleRate, "device": cfg.AudioDevice},
	)
	return nil
}

// Capture implements core.VoiceEngine: it returns a channel receiving raw
// PCM audio chunks (per cfg.SampleRate, mono, int16 LE), starting a Python
// capture subprocess if one isn't already running.
func (e *AudioEngine) Capture() (<-chan []byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running {
		return nil, fmt.Errorf("voice: audio engine not initialized")
	}
	if e.captureCmd != nil {
		return e.captureCh, nil // already running
	}

	cmd := exec.Command(e.pythonPath, e.scriptPath, "capture",
		fmt.Sprintf("--sample-rate=%d", e.cfg.SampleRate),
		fmt.Sprintf("--device=%s", e.cfg.AudioDevice),
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("voice: create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("voice: create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("voice: start capture: %w", err)
	}

	e.captureCmd = cmd
	captureCh := e.captureCh

	e.captureWG.Add(2)
	go func() { defer e.captureWG.Done(); e.drainCaptureStderr(stderr) }()
	go func() { defer e.captureWG.Done(); e.readCaptureLoop(stdout, captureCh) }()

	e.log.Info("voice: capture started", nil)
	return e.captureCh, nil
}

// drainCaptureStderr logs the capture subprocess's stderr line by line
// until it hits EOF (the subprocess exited).
func (e *AudioEngine) drainCaptureStderr(stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		e.log.Debug("voice: capture stderr", map[string]any{"msg": scanner.Text()})
	}
}

// readCaptureLoop reads length-prefixed PCM frames from the capture
// subprocess's stdout (framing.go's protocol, not line-oriented - raw PCM
// audio routinely contains 0x0A bytes as ordinary sample data, which a
// bufio.Scanner would incorrectly treat as line breaks) and forwards each
// one to captureCh, dropping a chunk rather than blocking if the channel is
// full so a slow consumer can't stall capture.
func (e *AudioEngine) readCaptureLoop(stdout io.Reader, captureCh chan<- []byte) {
	r := bufio.NewReader(stdout)
	for {
		frame, err := readFrame(r)
		if err != nil {
			return
		}
		if len(frame) == 0 {
			continue
		}
		select {
		case captureCh <- frame:
		default:
			e.log.Debug("voice: capture buffer full, dropping audio chunk", nil)
		}
	}
}

// Playback implements core.VoiceEngine: it plays raw PCM audio (per
// cfg.SampleRate, mono, int16 LE) via a one-shot Python playback subprocess.
// It does not hold the engine's mutex across the subprocess's lifetime, so a
// slow or hung playback can't block Capture/Shutdown/a later Playback call.
func (e *AudioEngine) Playback(audio []byte) error {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return fmt.Errorf("voice: audio engine not initialized")
	}
	pythonPath := e.pythonPath
	scriptPath := e.scriptPath
	sampleRate := e.cfg.SampleRate
	e.mu.Unlock()

	if len(audio) == 0 {
		return nil
	}

	cmd := exec.Command(pythonPath, scriptPath, "playback",
		fmt.Sprintf("--sample-rate=%d", sampleRate),
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("voice: create stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("voice: start playback: %w", err)
	}

	if _, err := stdin.Write(audio); err != nil {
		stdin.Close()
		cmd.Wait()
		return err
	}
	stdin.Close()

	return cmd.Wait()
}

// Shutdown implements core.VoiceEngine: it stops any running capture
// subprocess, waits for its goroutines to exit, and only then closes
// captureCh - the ordering that makes this safe to call concurrently with
// an active capture (no send-on-closed-channel race in readCaptureLoop).
func (e *AudioEngine) Shutdown() error {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return nil
	}
	cmd := e.captureCmd
	captureCh := e.captureCh
	e.running = false
	e.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		cmd.Process.Kill()
		cmd.Wait()
	}
	e.captureWG.Wait()

	if captureCh != nil {
		close(captureCh)
	}

	e.mu.Lock()
	e.captureCmd = nil
	e.mu.Unlock()

	e.log.Info("voice: audio engine shutdown", nil)
	return nil
}

// Ensure AudioEngine implements core.VoiceEngine.
var _ core.VoiceEngine = (*AudioEngine)(nil)
