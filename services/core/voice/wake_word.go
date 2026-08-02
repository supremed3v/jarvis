// wake_word.go implements SPEC-0055: Wake Word Detection.
// Uses openWakeWord via a Python subprocess for local wake word detection,
// avoiding a CGO dependency and working reliably on Windows.
package voice

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"jarvis-pa/services/core"
)

//go:embed scripts/wake_word_detector.py
var wakeWordScript []byte

// WakeWordDetectorImpl implements core.WakeWordDetector using the
// openWakeWord Python library, launched as a subprocess. Stop owns its own
// cancellation independent of the ctx/audioCh Start was given, so it can
// always terminate the detector's goroutines even if a caller never closes
// audioCh or cancels its own ctx.
type WakeWordDetectorImpl struct {
	modelPath  string
	pythonPath string
	scriptPath string

	mu       sync.Mutex
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	running  bool
	onDetect func()
	wg       sync.WaitGroup
}

// NewWakeWordDetector creates a WakeWordDetectorImpl that runs pythonPath
// against the embedded wake_word_detector.py script (extracted to a temp
// file so it can be exec'd regardless of the process's working directory),
// using the openWakeWord model at modelPath.
func NewWakeWordDetector(modelPath, pythonPath string) (*WakeWordDetectorImpl, error) {
	scriptPath, err := extractScript("wake_word_detector.py", wakeWordScript)
	if err != nil {
		return nil, err
	}
	return &WakeWordDetectorImpl{
		modelPath:  modelPath,
		pythonPath: pythonPath,
		scriptPath: scriptPath,
	}, nil
}

// Start implements core.WakeWordDetector: it launches the Python subprocess
// and starts goroutines forwarding audioCh chunks to its stdin as
// length-prefixed frames, reading "DETECTED" lines from its stdout to
// invoke onDetect, and draining its stderr.
func (w *WakeWordDetectorImpl) Start(ctx context.Context, audioCh <-chan []byte, onDetect func()) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return nil
	}
	w.mu.Unlock()

	runCtx, cancel := context.WithCancel(ctx)

	cmd := exec.CommandContext(runCtx, w.pythonPath, w.scriptPath, w.modelPath)

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("voice: create stdin pipe: %w", err)
	}
	stdin := bufio.NewWriter(stdinPipe)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("voice: create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("voice: create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("voice: start wake word detector: %w", err)
	}

	w.mu.Lock()
	w.cmd = cmd
	w.cancel = cancel
	w.onDetect = onDetect
	w.running = true
	w.mu.Unlock()

	w.wg.Add(3)
	go func() { defer w.wg.Done(); drainReader(stderr) }()
	go func() { defer w.wg.Done(); w.writeAudioLoop(runCtx, stdin, audioCh) }()
	go func() { defer w.wg.Done(); w.readDetectionLoop(stdout) }()

	return nil
}

// writeAudioLoop forwards audioCh chunks to stdin as length-prefixed frames
// until audioCh closes, ctx is done (Stop was called), or a write fails
// (e.g. the subprocess has exited).
func (w *WakeWordDetectorImpl) writeAudioLoop(ctx context.Context, stdin *bufio.Writer, audioCh <-chan []byte) {
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

// readDetectionLoop reads "DETECTED" lines from stdout and invokes onDetect
// for each one, until stdout closes (the subprocess exited).
func (w *WakeWordDetectorImpl) readDetectionLoop(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		if scanner.Text() != "DETECTED" {
			continue
		}
		w.mu.Lock()
		onDetect := w.onDetect
		w.mu.Unlock()
		if onDetect != nil {
			onDetect()
		}
	}
}

// drainReader discards r's content line by line until it hits EOF (the
// subprocess exited), so its pipe doesn't fill up and block the writer on
// the other end.
func drainReader(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
	}
}

// Stop stops wake word detection. It cancels Start's internal context
// (guaranteeing writeAudioLoop exits even if the caller never closes
// audioCh), kills the subprocess, and waits for every goroutine Start
// spawned to exit before returning - the order that makes this safe to call
// concurrently with an active detection session with no shared-state races.
func (w *WakeWordDetectorImpl) Stop() error {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return nil
	}
	cmd := w.cmd
	cancel := w.cancel
	w.running = false
	w.mu.Unlock()

	cancel()
	if cmd != nil && cmd.Process != nil {
		cmd.Process.Kill()
		cmd.Wait()
	}
	w.wg.Wait()

	w.mu.Lock()
	w.cmd = nil
	w.cancel = nil
	w.mu.Unlock()

	return nil
}

// Ensure WakeWordDetectorImpl implements core.WakeWordDetector.
var _ core.WakeWordDetector = (*WakeWordDetectorImpl)(nil)
