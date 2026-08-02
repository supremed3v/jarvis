// voice_test.go contains tests for the voice subsystem (SPEC-0053..0055).
//
// Tests split into two groups: deterministic tests that need no external
// process and must genuinely pass or fail, and subprocess-dependent tests
// that use t.Skip (never a silent pass) when python/the required Python
// packages/an audio device aren't available in the running environment.
package voice

import (
	"context"
	"os/exec"
	"sync"
	"testing"
	"time"

	"jarvis-pa/packages/config"
	"jarvis-pa/packages/logger"
	"jarvis-pa/services/core"
)

// requirePython skips the calling test if pythonPath isn't resolvable on
// PATH, so subprocess-dependent tests report SKIP rather than a false PASS
// or a confusing FAIL in an environment without Python installed.
func requirePython(t *testing.T, pythonPath string) {
	t.Helper()
	if _, err := exec.LookPath(pythonPath); err != nil {
		t.Skipf("skipping: %q not found on PATH: %v", pythonPath, err)
	}
}

// mockSTTProvider is a minimal core.STTProvider stub for tests that need a
// non-nil provider but don't exercise transcription itself.
type mockSTTProvider struct{}

func (m *mockSTTProvider) Transcribe(ctx context.Context, audio []byte) (string, error) {
	return "", nil
}

func (m *mockSTTProvider) StreamTranscribe(ctx context.Context, audioCh <-chan []byte, textCh chan<- string) error {
	go func() {
		for range audioCh {
			select {
			case textCh <- "":
			case <-ctx.Done():
				return
			}
		}
	}()
	return nil
}

var _ core.STTProvider = (*mockSTTProvider)(nil)

// fakeVoiceEngine is a core.VoiceEngine test double whose Capture() channel
// the test controls directly, so audio can be injected without a real
// subprocess.
type fakeVoiceEngine struct {
	captureCh chan []byte
}

func (f *fakeVoiceEngine) Initialize(cfg *config.VoiceConfig, log *logger.Logger) error {
	return nil
}

func (f *fakeVoiceEngine) Capture() (<-chan []byte, error) {
	return f.captureCh, nil
}

func (f *fakeVoiceEngine) Playback(audio []byte) error { return nil }

func (f *fakeVoiceEngine) Shutdown() error { return nil }

var _ core.VoiceEngine = (*fakeVoiceEngine)(nil)

// fakeWakeWordDetector is a core.WakeWordDetector test double that records
// the audioCh it was started with, so a test can prove a real Microphone
// actually wires captured audio through to it.
type fakeWakeWordDetector struct {
	mu        sync.Mutex
	startedCh <-chan []byte
}

func (f *fakeWakeWordDetector) Start(ctx context.Context, audioCh <-chan []byte, onDetect func()) error {
	f.mu.Lock()
	f.startedCh = audioCh
	f.mu.Unlock()
	return nil
}

func (f *fakeWakeWordDetector) audioChannel() <-chan []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.startedCh
}

func (f *fakeWakeWordDetector) Stop() error { return nil }

var _ core.WakeWordDetector = (*fakeWakeWordDetector)(nil)

// TestMicrophone_FansAudioToWakeWordDetector proves the wiring between
// Microphone's distribution loop and WakeWordDetector.Start's audioCh
// parameter actually delivers captured audio, without needing a real
// subprocess on either side.
func TestMicrophone_FansAudioToWakeWordDetector(t *testing.T) {
	captureCh := make(chan []byte, 4)
	engine := &fakeVoiceEngine{captureCh: captureCh}
	wakeWord := &fakeWakeWordDetector{}
	log := logger.New("test")

	mic := NewMicrophone(engine, wakeWord, &mockSTTProvider{}, log)
	if err := mic.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer mic.Stop()

	captureCh <- []byte("chunk-1")

	var ch <-chan []byte
	for i := 0; i < 50 && ch == nil; i++ {
		ch = wakeWord.audioChannel()
		if ch == nil {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if ch == nil {
		t.Fatal("WakeWordDetector.Start() was never called with an audio channel")
	}

	select {
	case got := <-ch:
		if string(got) != "chunk-1" {
			t.Errorf("got %q, want %q", got, "chunk-1")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for captured audio to reach the wake word detector")
	}
}

// TestAudioEngine_InitializeShutdown needs no subprocess: Initialize/Shutdown
// only touch in-memory state until Capture/Playback actually spawn Python.
func TestAudioEngine_InitializeShutdown(t *testing.T) {
	cfg := config.Defaults()
	log := logger.New("test", logger.WithMinLevel(logger.LevelDebug))

	engine, err := NewAudioEngine(&cfg.Voice, log)
	if err != nil {
		t.Fatalf("NewAudioEngine() error = %v", err)
	}
	if err := engine.Initialize(&cfg.Voice, log); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if err := engine.Shutdown(); err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}
}

func TestAudioEngine_Capture_BeforeInitialize(t *testing.T) {
	cfg := config.Defaults()
	log := logger.New("test")

	engine, err := NewAudioEngine(&cfg.Voice, log)
	if err != nil {
		t.Fatalf("NewAudioEngine() error = %v", err)
	}

	if _, err := engine.Capture(); err == nil {
		t.Fatal("Capture() error = nil, want an error before Initialize")
	}
}

func TestAudioEngine_Playback_BeforeInitialize(t *testing.T) {
	cfg := config.Defaults()
	log := logger.New("test")

	engine, err := NewAudioEngine(&cfg.Voice, log)
	if err != nil {
		t.Fatalf("NewAudioEngine() error = %v", err)
	}

	if err := engine.Playback([]byte{1, 2, 3}); err == nil {
		t.Fatal("Playback() error = nil, want an error before Initialize")
	}
}

func TestAudioEngine_Shutdown_NotRunning(t *testing.T) {
	cfg := config.Defaults()
	log := logger.New("test")

	engine, err := NewAudioEngine(&cfg.Voice, log)
	if err != nil {
		t.Fatalf("NewAudioEngine() error = %v", err)
	}

	if err := engine.Shutdown(); err != nil {
		t.Errorf("Shutdown() on a never-initialized engine error = %v, want nil", err)
	}
}

func TestWakeWordDetector_Stop_NotStarted(t *testing.T) {
	detector, err := NewWakeWordDetector("models/hey_jarvis.onnx", "python")
	if err != nil {
		t.Fatalf("NewWakeWordDetector() error = %v", err)
	}

	if err := detector.Stop(); err != nil {
		t.Errorf("Stop() on a never-started detector error = %v, want nil", err)
	}
}

// TestWakeWordDetector_StopDoesNotDeadlockWithoutClosingAudioCh is a
// regression test for the bug this session fixed: Stop() must always be
// able to terminate writeAudioLoop even if the caller never closes audioCh
// or cancels the ctx it passed to Start. Bounded at 5s so a real regression
// fails the test rather than hanging the suite.
func TestWakeWordDetector_StopDoesNotDeadlockWithoutClosingAudioCh(t *testing.T) {
	requirePython(t, "python")

	detector, err := NewWakeWordDetector("nonexistent-model.onnx", "python")
	if err != nil {
		t.Fatalf("NewWakeWordDetector() error = %v", err)
	}

	audioCh := make(chan []byte) // deliberately never written to or closed
	if err := detector.Start(context.Background(), audioCh, func() {}); err != nil {
		t.Skipf("skipping: Start() error = %v", err)
	}

	done := make(chan struct{})
	go func() {
		detector.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not return within 5s - writeAudioLoop likely deadlocked on an audio channel the caller never closed")
	}
}

func TestAudioEngine_Capture_RequiresPython(t *testing.T) {
	requirePython(t, "python")

	cfg := config.Defaults()
	log := logger.New("test", logger.WithMinLevel(logger.LevelDebug))

	engine, err := NewAudioEngine(&cfg.Voice, log)
	if err != nil {
		t.Fatalf("NewAudioEngine() error = %v", err)
	}
	if err := engine.Initialize(&cfg.Voice, log); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	defer engine.Shutdown()

	ch, err := engine.Capture()
	if err != nil {
		t.Skipf("skipping: Capture() error = %v (likely missing sounddevice or an audio device)", err)
	}

	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Skip("skipping: no audio captured within timeout (likely no working audio input device in this environment)")
	}
}

func TestMicrophone_StartStop(t *testing.T) {
	requirePython(t, "python")

	cfg := config.Defaults()
	log := logger.New("test", logger.WithMinLevel(logger.LevelDebug))

	engine, err := NewAudioEngine(&cfg.Voice, log)
	if err != nil {
		t.Fatalf("NewAudioEngine() error = %v", err)
	}
	if err := engine.Initialize(&cfg.Voice, log); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	defer engine.Shutdown()

	detector, err := NewWakeWordDetector(cfg.Voice.WakeWordModelPath, cfg.Voice.PythonPath)
	if err != nil {
		t.Fatalf("NewWakeWordDetector() error = %v", err)
	}

	mic := NewMicrophone(engine, detector, &mockSTTProvider{}, log)

	if err := mic.Start(); err != nil {
		t.Skipf("skipping: Start() error = %v (likely missing sounddevice/openwakeword or an audio device)", err)
	}

	time.Sleep(200 * time.Millisecond)

	if err := mic.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}
