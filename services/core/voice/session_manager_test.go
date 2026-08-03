// session_manager_test.go tests SessionManager (SPEC-0060). Tests drive
// SessionManager directly through the EventBus - publishing
// EventWakeWordDetected/EventVoiceTranscript as Microphone itself would -
// rather than through a real audio/wake-word/STT pipeline, since that
// pipeline's own behavior is already covered by voice_test.go.
package voice

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"jarvis-pa/packages/config"
	"jarvis-pa/packages/logger"
	types "jarvis-pa/packages/shared-types"
	"jarvis-pa/services/core"
)

// recordingVoiceEngine wraps fakeVoiceEngine (voice_test.go), additionally
// recording every Playback call so a test can assert what audio was
// actually spoken.
type recordingVoiceEngine struct {
	*fakeVoiceEngine
	mu          sync.Mutex
	played      [][]byte
	sampleRates []int
}

func (r *recordingVoiceEngine) Playback(audio []byte, sampleRate int) error {
	r.mu.Lock()
	r.played = append(r.played, audio)
	r.sampleRates = append(r.sampleRates, sampleRate)
	r.mu.Unlock()
	return nil
}

// PlaybackStream records one played entry per call, concatenating every
// chunk it reads from audioCh before audioCh closes - the streaming
// equivalent of Playback's one-buffer-per-call recording, so streaming
// tests can assert both how many "utterances" were spoken and what audio
// each one carried using the same played/sampleRates fields and helpers as
// the non-streaming tests.
func (r *recordingVoiceEngine) PlaybackStream(ctx context.Context, audioCh <-chan []byte, sampleRate int) error {
	var buf []byte
	for {
		select {
		case chunk, ok := <-audioCh:
			if !ok {
				r.mu.Lock()
				r.played = append(r.played, buf)
				r.sampleRates = append(r.sampleRates, sampleRate)
				r.mu.Unlock()
				return nil
			}
			buf = append(buf, chunk...)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (r *recordingVoiceEngine) playbackCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.played)
}

func (r *recordingVoiceEngine) lastSampleRate() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.sampleRates) == 0 {
		return 0
	}
	return r.sampleRates[len(r.sampleRates)-1]
}

// fakeTTSProvider is a minimal core.TTSProvider test double recording every
// Synthesize/StreamSynthesize call. Synthesize and StreamSynthesize are
// controlled independently (err vs. streamErr, texts vs. streamTexts) since
// SessionManager's batch and streaming paths (SPEC-0060 and SPEC-0061
// respectively) each use exactly one of the two methods, never both in the
// same processRequest call.
type fakeTTSProvider struct {
	mu        sync.Mutex
	texts     []string
	audio     []byte
	err       error
	callCount int

	streamTexts []string
	streamAudio []byte
	streamErr   error
}

func (f *fakeTTSProvider) Synthesize(ctx context.Context, text string, opts core.VoiceOptions) ([]byte, error) {
	f.mu.Lock()
	f.texts = append(f.texts, text)
	f.callCount++
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return f.audio, nil
}

// StreamSynthesize records text, then - unless streamErr is set - writes
// streamAudio (or a default derived from text, if streamAudio is unset) to
// audioCh as a single chunk before returning, respecting ctx cancellation
// while sending. The caller owns and closes audioCh, matching
// core.TTSProvider.StreamSynthesize's real contract.
func (f *fakeTTSProvider) StreamSynthesize(ctx context.Context, text string, opts core.VoiceOptions, audioCh chan<- []byte) error {
	f.mu.Lock()
	f.streamTexts = append(f.streamTexts, text)
	streamErr := f.streamErr
	audio := f.streamAudio
	f.mu.Unlock()

	if streamErr != nil {
		return streamErr
	}
	if audio == nil {
		audio = []byte("audio:" + text)
	}
	select {
	case audioCh <- audio:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (f *fakeTTSProvider) synthesizeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.callCount
}

func (f *fakeTTSProvider) streamSynthesizeTexts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.streamTexts...)
}

var _ core.TTSProvider = (*fakeTTSProvider)(nil)

// newTestSessionManager builds a SessionManager wired to test doubles: a
// real Microphone (so Start/Stop exercises the actual audio lifecycle) atop
// fakeVoiceEngine/fakeWakeWordDetector/mockSTTProvider, plus the given
// handler/tts/opts.
func newTestSessionManager(t *testing.T, handler RequestHandler, tts core.TTSProvider, opts ...SessionManagerOption) (*SessionManager, *recordingVoiceEngine, core.EventBus) {
	t.Helper()

	engine := &recordingVoiceEngine{fakeVoiceEngine: &fakeVoiceEngine{captureCh: make(chan []byte, 4)}}
	wakeWord := &fakeWakeWordDetector{}
	bus := core.NewBus()
	log := logger.New("test")

	mic := NewMicrophone(engine, wakeWord, &mockSTTProvider{}, bus, log)
	cfg := config.Defaults()

	sm, err := NewSessionManager(mic, engine, tts, &cfg.Voice, bus, handler, log, opts...)
	if err != nil {
		t.Fatalf("NewSessionManager() error = %v", err)
	}
	return sm, engine, bus
}

// subscribeOnce returns a channel that receives the first Event of
// eventType published on bus.
func subscribeOnce(bus core.EventBus, eventType types.EventType) <-chan types.Event {
	ch := make(chan types.Event, 1)
	var unsub func()
	unsub = bus.Subscribe(eventType, func(event types.Event) {
		select {
		case ch <- event:
		default:
		}
		unsub()
	})
	return ch
}

func waitForEvent(t *testing.T, ch <-chan types.Event, what string) types.Event {
	t.Helper()
	select {
	case event := <-ch:
		return event
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		return types.Event{}
	}
}

func TestSessionManager_CompletesFullCycle(t *testing.T) {
	tts := &fakeTTSProvider{audio: []byte("synthesized-audio")}
	var handlerCalls []string
	handler := func(ctx context.Context, transcript string) (string, error) {
		handlerCalls = append(handlerCalls, transcript)
		return "response to: " + transcript, nil
	}

	sm, engine, bus := newTestSessionManager(t, handler, tts)
	started := subscribeOnce(bus, EventSessionStarted)
	completed := subscribeOnce(bus, EventSessionCompleted)

	if err := sm.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer sm.Stop()

	bus.Publish(types.Event{Type: EventWakeWordDetected, Source: "test"})
	waitForEvent(t, started, "EventSessionStarted")

	if got := sm.CurrentSession(); got == nil || got.State != SessionStateListening {
		t.Fatalf("CurrentSession() = %+v, want a session in state %q", got, SessionStateListening)
	}

	bus.Publish(types.Event{
		Type:    EventVoiceTranscript,
		Source:  "test",
		Payload: map[string]any{"text": "turn on the lights", "final": true},
	})

	waitForEvent(t, completed, "EventSessionCompleted")

	if len(handlerCalls) != 1 || handlerCalls[0] != "turn on the lights" {
		t.Errorf("handler calls = %v, want exactly one call with %q", handlerCalls, "turn on the lights")
	}
	if tts.synthesizeCount() != 1 || tts.texts[0] != "response to: turn on the lights" {
		t.Errorf("tts.texts = %v, want exactly one call with the handler's response", tts.texts)
	}
	if engine.playbackCount() != 1 {
		t.Errorf("engine.playbackCount() = %d, want 1", engine.playbackCount())
	}
	if want := config.Defaults().Voice.TTSSampleRate; engine.lastSampleRate() != want {
		t.Errorf("engine.lastSampleRate() = %d, want %d (VoiceConfig.TTSSampleRate, not the capture SampleRate)", engine.lastSampleRate(), want)
	}
	if got := sm.CurrentSession(); got != nil {
		t.Errorf("CurrentSession() = %+v, want nil after completion", got)
	}
}

func TestSessionManager_IgnoresTranscriptWithoutActiveSession(t *testing.T) {
	handlerCalled := false
	handler := func(ctx context.Context, transcript string) (string, error) {
		handlerCalled = true
		return "", nil
	}

	sm, _, bus := newTestSessionManager(t, handler, &fakeTTSProvider{})
	if err := sm.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer sm.Stop()

	bus.Publish(types.Event{
		Type:    EventVoiceTranscript,
		Source:  "test",
		Payload: map[string]any{"text": "stray audio", "final": true},
	})

	time.Sleep(200 * time.Millisecond)

	if handlerCalled {
		t.Error("handler was called for a transcript with no active session")
	}
}

func TestSessionManager_FailsWhenHandlerErrors(t *testing.T) {
	tts := &fakeTTSProvider{audio: []byte("audio")}
	handler := func(ctx context.Context, transcript string) (string, error) {
		return "", errors.New("agent exploded")
	}

	sm, engine, bus := newTestSessionManager(t, handler, tts)
	started := subscribeOnce(bus, EventSessionStarted)
	failed := subscribeOnce(bus, EventSessionFailed)

	if err := sm.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer sm.Stop()

	bus.Publish(types.Event{Type: EventWakeWordDetected, Source: "test"})
	waitForEvent(t, started, "EventSessionStarted")

	bus.Publish(types.Event{
		Type:    EventVoiceTranscript,
		Source:  "test",
		Payload: map[string]any{"text": "do something", "final": true},
	})

	event := waitForEvent(t, failed, "EventSessionFailed")
	if reason, _ := event.Payload["reason"].(string); reason != "agent exploded" {
		t.Errorf("failure reason = %q, want %q", reason, "agent exploded")
	}
	if tts.synthesizeCount() != 0 {
		t.Errorf("tts.synthesizeCount() = %d, want 0 (handler failed before TTS)", tts.synthesizeCount())
	}
	if engine.playbackCount() != 0 {
		t.Errorf("engine.playbackCount() = %d, want 0", engine.playbackCount())
	}
	if got := sm.CurrentSession(); got != nil {
		t.Errorf("CurrentSession() = %+v, want nil after failure", got)
	}
}

func TestSessionManager_EmptyTranscriptEndsSessionWithoutHandler(t *testing.T) {
	handlerCalled := false
	handler := func(ctx context.Context, transcript string) (string, error) {
		handlerCalled = true
		return "", nil
	}

	sm, _, bus := newTestSessionManager(t, handler, &fakeTTSProvider{})
	started := subscribeOnce(bus, EventSessionStarted)
	failed := subscribeOnce(bus, EventSessionFailed)

	if err := sm.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer sm.Stop()

	bus.Publish(types.Event{Type: EventWakeWordDetected, Source: "test"})
	waitForEvent(t, started, "EventSessionStarted")

	bus.Publish(types.Event{
		Type:    EventVoiceTranscript,
		Source:  "test",
		Payload: map[string]any{"text": "", "final": true},
	})

	event := waitForEvent(t, failed, "EventSessionFailed")
	if reason, _ := event.Payload["reason"].(string); reason != "empty transcript" {
		t.Errorf("failure reason = %q, want %q", reason, "empty transcript")
	}
	if handlerCalled {
		t.Error("handler was called for an empty transcript")
	}
}

func TestSessionManager_IgnoresWakeWordWhileSessionActive(t *testing.T) {
	handler := func(ctx context.Context, transcript string) (string, error) {
		return "ok", nil
	}
	sm, _, bus := newTestSessionManager(t, handler, &fakeTTSProvider{audio: []byte("a")})
	started := subscribeOnce(bus, EventSessionStarted)

	if err := sm.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer sm.Stop()

	bus.Publish(types.Event{Type: EventWakeWordDetected, Source: "test"})
	waitForEvent(t, started, "EventSessionStarted")

	first := sm.CurrentSession()
	if first == nil {
		t.Fatal("CurrentSession() = nil after first wake word")
	}

	bus.Publish(types.Event{Type: EventWakeWordDetected, Source: "test"})
	time.Sleep(200 * time.Millisecond)

	second := sm.CurrentSession()
	if second == nil || second.ID != first.ID {
		t.Errorf("CurrentSession() = %+v, want the original session %+v unchanged", second, first)
	}
}

func TestSessionManager_StopReleasesActiveSession(t *testing.T) {
	handler := func(ctx context.Context, transcript string) (string, error) {
		return "ok", nil
	}
	sm, _, bus := newTestSessionManager(t, handler, &fakeTTSProvider{})
	started := subscribeOnce(bus, EventSessionStarted)
	failed := subscribeOnce(bus, EventSessionFailed)

	if err := sm.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	bus.Publish(types.Event{Type: EventWakeWordDetected, Source: "test"})
	waitForEvent(t, started, "EventSessionStarted")

	if err := sm.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	event := waitForEvent(t, failed, "EventSessionFailed")
	if reason, _ := event.Payload["reason"].(string); reason != "voice session manager stopped" {
		t.Errorf("failure reason = %q, want %q", reason, "voice session manager stopped")
	}
	if got := sm.CurrentSession(); got != nil {
		t.Errorf("CurrentSession() = %+v, want nil after Stop", got)
	}
}

func TestSessionManager_SessionTimesOutWaitingForSpeech(t *testing.T) {
	handlerCalled := false
	handler := func(ctx context.Context, transcript string) (string, error) {
		handlerCalled = true
		return "", nil
	}

	sm, _, bus := newTestSessionManager(t, handler, &fakeTTSProvider{}, WithSessionTimeout(100*time.Millisecond))
	started := subscribeOnce(bus, EventSessionStarted)
	failed := subscribeOnce(bus, EventSessionFailed)

	if err := sm.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer sm.Stop()

	bus.Publish(types.Event{Type: EventWakeWordDetected, Source: "test"})
	waitForEvent(t, started, "EventSessionStarted")

	event := waitForEvent(t, failed, "EventSessionFailed")
	if reason, _ := event.Payload["reason"].(string); reason != "timed out waiting for speech" {
		t.Errorf("failure reason = %q, want %q", reason, "timed out waiting for speech")
	}
	if handlerCalled {
		t.Error("handler was called despite no transcript ever arriving")
	}
	if got := sm.CurrentSession(); got != nil {
		t.Errorf("CurrentSession() = %+v, want nil after timeout", got)
	}
}

// scriptedStreamingHandler returns a StreamingRequestHandler that delivers
// text in the given fragments - simulating tokens/words arriving one at a
// time, independent of where any sentence boundary falls within a single
// fragment - followed by a final Done chunk.
func scriptedStreamingHandler(fragments ...string) StreamingRequestHandler {
	return func(ctx context.Context, transcript string, onChunk func(ResponseChunk) error) error {
		for _, f := range fragments {
			if err := onChunk(ResponseChunk{Text: f}); err != nil {
				return err
			}
		}
		return onChunk(ResponseChunk{Done: true})
	}
}

func TestSessionManager_StreamingHandler_SpeaksSentencesAsTheyComplete(t *testing.T) {
	tts := &fakeTTSProvider{}
	batchHandlerCalled := false
	batchHandler := func(ctx context.Context, transcript string) (string, error) {
		batchHandlerCalled = true
		return "", nil
	}
	streamHandler := scriptedStreamingHandler("Turning on ", "the lights. ", "Anything else? ")

	sm, engine, bus := newTestSessionManager(t, batchHandler, tts, WithStreamingHandler(streamHandler))
	started := subscribeOnce(bus, EventSessionStarted)
	completed := subscribeOnce(bus, EventSessionCompleted)

	if err := sm.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer sm.Stop()

	bus.Publish(types.Event{Type: EventWakeWordDetected, Source: "test"})
	waitForEvent(t, started, "EventSessionStarted")

	bus.Publish(types.Event{
		Type:    EventVoiceTranscript,
		Source:  "test",
		Payload: map[string]any{"text": "turn on the lights", "final": true},
	})

	waitForEvent(t, completed, "EventSessionCompleted")

	if batchHandlerCalled {
		t.Error("batch RequestHandler was called despite a StreamingRequestHandler being configured")
	}
	wantSentences := []string{"Turning on the lights.", "Anything else?"}
	if got := tts.streamSynthesizeTexts(); !reflect.DeepEqual(got, wantSentences) {
		t.Errorf("tts.streamSynthesizeTexts() = %v, want %v", got, wantSentences)
	}
	if engine.playbackCount() != len(wantSentences) {
		t.Errorf("engine.playbackCount() = %d, want %d (one PlaybackStream call per sentence, not one for the whole response)", engine.playbackCount(), len(wantSentences))
	}
	if got := sm.CurrentSession(); got != nil {
		t.Errorf("CurrentSession() = %+v, want nil after completion", got)
	}
}

func TestSessionManager_StreamingHandler_FailsOnSynthesisError(t *testing.T) {
	tts := &fakeTTSProvider{streamErr: errors.New("piper exploded")}
	batchHandler := func(ctx context.Context, transcript string) (string, error) { return "", nil }
	streamHandler := scriptedStreamingHandler("Hello. ", "World. ")

	sm, _, bus := newTestSessionManager(t, batchHandler, tts, WithStreamingHandler(streamHandler))
	started := subscribeOnce(bus, EventSessionStarted)
	failed := subscribeOnce(bus, EventSessionFailed)

	if err := sm.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer sm.Stop()

	bus.Publish(types.Event{Type: EventWakeWordDetected, Source: "test"})
	waitForEvent(t, started, "EventSessionStarted")

	bus.Publish(types.Event{
		Type:    EventVoiceTranscript,
		Source:  "test",
		Payload: map[string]any{"text": "hello", "final": true},
	})

	event := waitForEvent(t, failed, "EventSessionFailed")
	if reason, _ := event.Payload["reason"].(string); reason != "piper exploded" {
		t.Errorf("failure reason = %q, want %q", reason, "piper exploded")
	}
	if got := sm.CurrentSession(); got != nil {
		t.Errorf("CurrentSession() = %+v, want nil after failure", got)
	}
}

func TestSessionManager_StreamingHandler_FailsWhenHandlerErrors(t *testing.T) {
	tts := &fakeTTSProvider{}
	batchHandler := func(ctx context.Context, transcript string) (string, error) { return "", nil }
	streamHandler := func(ctx context.Context, transcript string, onChunk func(ResponseChunk) error) error {
		return errors.New("agent exploded")
	}

	sm, _, bus := newTestSessionManager(t, batchHandler, tts, WithStreamingHandler(streamHandler))
	started := subscribeOnce(bus, EventSessionStarted)
	failed := subscribeOnce(bus, EventSessionFailed)

	if err := sm.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer sm.Stop()

	bus.Publish(types.Event{Type: EventWakeWordDetected, Source: "test"})
	waitForEvent(t, started, "EventSessionStarted")

	bus.Publish(types.Event{
		Type:    EventVoiceTranscript,
		Source:  "test",
		Payload: map[string]any{"text": "do something", "final": true},
	})

	event := waitForEvent(t, failed, "EventSessionFailed")
	if reason, _ := event.Payload["reason"].(string); reason != "agent exploded" {
		t.Errorf("failure reason = %q, want %q", reason, "agent exploded")
	}
	if n := len(tts.streamSynthesizeTexts()); n != 0 {
		t.Errorf("tts was invoked %d times despite the streaming handler failing before producing any text", n)
	}
}

func TestFlushCompleteSentences(t *testing.T) {
	var buf strings.Builder

	buf.WriteString("Hello there. How are")
	got := flushCompleteSentences(&buf)
	want := []string{"Hello there."}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("flushCompleteSentences() = %v, want %v", got, want)
	}
	if remaining := buf.String(); remaining != " How are" {
		t.Fatalf("buf after flush = %q, want %q", remaining, " How are")
	}

	buf.WriteString(" you? I'm great!\n")
	got = flushCompleteSentences(&buf)
	want = []string{"How are you?", "I'm great!"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("flushCompleteSentences() = %v, want %v", got, want)
	}
	if remaining := buf.String(); remaining != "" {
		t.Fatalf("buf after flush = %q, want empty", remaining)
	}
}

func TestNewSessionManager_RequiresDependencies(t *testing.T) {
	engine := &recordingVoiceEngine{fakeVoiceEngine: &fakeVoiceEngine{}}
	tts := &fakeTTSProvider{}
	bus := core.NewBus()
	log := logger.New("test")
	handler := func(ctx context.Context, transcript string) (string, error) { return "", nil }
	mic := NewMicrophone(engine, &fakeWakeWordDetector{}, &mockSTTProvider{}, bus, log)
	cfg := config.Defaults()

	cases := []struct {
		name string
		fn   func() (*SessionManager, error)
	}{
		{"nil microphone", func() (*SessionManager, error) {
			return NewSessionManager(nil, engine, tts, &cfg.Voice, bus, handler, log)
		}},
		{"nil engine", func() (*SessionManager, error) {
			return NewSessionManager(mic, nil, tts, &cfg.Voice, bus, handler, log)
		}},
		{"nil tts", func() (*SessionManager, error) {
			return NewSessionManager(mic, engine, nil, &cfg.Voice, bus, handler, log)
		}},
		{"nil config", func() (*SessionManager, error) { return NewSessionManager(mic, engine, tts, nil, bus, handler, log) }},
		{"nil bus", func() (*SessionManager, error) {
			return NewSessionManager(mic, engine, tts, &cfg.Voice, nil, handler, log)
		}},
		{"nil handler", func() (*SessionManager, error) { return NewSessionManager(mic, engine, tts, &cfg.Voice, bus, nil, log) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := c.fn(); err == nil {
				t.Errorf("%s: error = nil, want a validation error", c.name)
			}
		})
	}
}
