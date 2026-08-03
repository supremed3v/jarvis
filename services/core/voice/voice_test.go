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
	types "jarvis-pa/packages/shared-types"
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

func (f *fakeVoiceEngine) ListDevices() ([]core.AudioDevice, error) { return nil, nil }

func (f *fakeVoiceEngine) Shutdown() error { return nil }

var _ core.VoiceEngine = (*fakeVoiceEngine)(nil)

// fakeWakeWordDetector is a core.WakeWordDetector test double that records
// the audioCh and onDetect callback it was started with, so a test can
// prove a real Microphone actually wires captured audio through to it and
// can simulate a detection by invoking the recorded callback.
type fakeWakeWordDetector struct {
	mu        sync.Mutex
	startedCh <-chan []byte
	onDetect  func()
}

func (f *fakeWakeWordDetector) Start(ctx context.Context, audioCh <-chan []byte, onDetect func()) error {
	f.mu.Lock()
	f.startedCh = audioCh
	f.onDetect = onDetect
	f.mu.Unlock()
	return nil
}

func (f *fakeWakeWordDetector) audioChannel() <-chan []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.startedCh
}

// triggerDetect invokes the onDetect callback Start() was given, simulating
// a real detection, once Start() has actually been called.
func (f *fakeWakeWordDetector) triggerDetect() bool {
	f.mu.Lock()
	onDetect := f.onDetect
	f.mu.Unlock()
	if onDetect == nil {
		return false
	}
	onDetect()
	return true
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

	bus := core.NewBus()
	mic := NewMicrophone(engine, wakeWord, &mockSTTProvider{}, bus, log)
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

// TestMicrophone_PublishesWakeWordDetectedEvent proves a wake word
// detection actually reaches the EventBus (SPEC-0055's "detection events
// are emitted" testing criterion), not just a log line.
func TestMicrophone_PublishesWakeWordDetectedEvent(t *testing.T) {
	engine := &fakeVoiceEngine{captureCh: make(chan []byte, 4)}
	wakeWord := &fakeWakeWordDetector{}
	log := logger.New("test")
	bus := core.NewBus()

	received := make(chan types.Event, 1)
	bus.Subscribe(EventWakeWordDetected, func(event types.Event) {
		received <- event
	})

	mic := NewMicrophone(engine, wakeWord, &mockSTTProvider{}, bus, log)
	if err := mic.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer mic.Stop()

	if !waitForDetectCallback(wakeWord, 2*time.Second) {
		t.Fatal("WakeWordDetector.Start() was never called with an onDetect callback")
	}
	if !wakeWord.triggerDetect() {
		t.Fatal("triggerDetect() found no onDetect callback to invoke")
	}

	select {
	case event := <-received:
		if event.Source != "core.voice.microphone" {
			t.Errorf("event.Source = %q, want %q", event.Source, "core.voice.microphone")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for WAKE_WORD_DETECTED on the event bus")
	}
}

// waitForDetectCallback polls until Start() has recorded an onDetect
// callback on w, or timeout elapses.
func waitForDetectCallback(w *fakeWakeWordDetector, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		w.mu.Lock()
		ok := w.onDetect != nil
		w.mu.Unlock()
		if ok {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
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

// TestAudioEngine_ListDevices_RequiresPython proves ListDevices actually
// round-trips through the embedded Python subprocess and parses real JSON
// (SPEC-0053/SPEC-0054's device discovery requirement), rather than just
// type-checking against an empty stub.
func TestAudioEngine_ListDevices_RequiresPython(t *testing.T) {
	requirePython(t, "python")

	cfg := config.Defaults()
	log := logger.New("test", logger.WithMinLevel(logger.LevelDebug))

	engine, err := NewAudioEngine(&cfg.Voice, log)
	if err != nil {
		t.Fatalf("NewAudioEngine() error = %v", err)
	}

	devices, err := engine.ListDevices()
	if err != nil {
		t.Skipf("skipping: ListDevices() error = %v (likely missing sounddevice or PortAudio)", err)
	}
	t.Logf("ListDevices() found %d device(s)", len(devices))
}

// TestAudioEngine_Capture_RecoversFromSubprocessCrash proves SPEC-0054's
// "recover from device failures": if the capture subprocess dies
// unexpectedly (simulated here by killing it directly, bypassing Shutdown),
// AudioEngine's supervisor restarts it automatically instead of leaving
// captureCh silently dead.
func TestAudioEngine_Capture_RecoversFromSubprocessCrash(t *testing.T) {
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

	if _, err := engine.Capture(); err != nil {
		t.Skipf("skipping: Capture() error = %v (likely missing sounddevice or an audio device)", err)
	}

	engine.mu.Lock()
	originalCmd := engine.captureCmd
	engine.mu.Unlock()
	if originalCmd == nil || originalCmd.Process == nil {
		t.Fatal("Capture() did not record a running subprocess")
	}

	if err := originalCmd.Process.Kill(); err != nil {
		t.Skipf("skipping: could not kill capture subprocess to simulate a crash: %v", err)
	}

	deadline := time.Now().Add(captureRestartBackoff + 5*time.Second)
	for time.Now().Before(deadline) {
		engine.mu.Lock()
		restarted := engine.captureCmd != nil && engine.captureCmd != originalCmd
		engine.mu.Unlock()
		if restarted {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("AudioEngine did not restart the capture subprocess after it was killed")
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

	mic := NewMicrophone(engine, detector, &mockSTTProvider{}, core.NewBus(), log)

	if err := mic.Start(); err != nil {
		t.Skipf("skipping: Start() error = %v (likely missing sounddevice/openwakeword or an audio device)", err)
	}

	time.Sleep(200 * time.Millisecond)

	if err := mic.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}
