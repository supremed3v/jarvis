// session_manager.go implements SPEC-0060: the Voice Session Manager - the
// component that sequences a complete voice interaction (Wake Word ->
// Capture Audio -> Transcribe -> Process Request -> Generate Response ->
// Speak) into one bounded Session, on top of the already-continuous audio
// pipeline Microphone (SPEC-0054) drives. Microphone publishes
// EventWakeWordDetected and EventVoiceTranscript regardless of any session
// state; SessionManager is what turns those two events into a gated
// request/response cycle: a session starts on wake word detection, ends
// once the resulting transcript has been handed to the agent layer (via
// RequestHandler) and its response has been spoken, and any transcript
// arriving with no active session is ignored (ambient audio the wake word
// never gated).
package voice

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	cfgpkg "jarvis-pa/packages/config"
	"jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
	types "jarvis-pa/packages/shared-types"
	"jarvis-pa/services/core"
)

// defaultSessionTimeout bounds how long a Session waits in
// SessionStateListening for a final transcript before it is abandoned
// (SPEC-0060's "session cleanup"/"resources are released" requirement
// covering the case where the wake word fires but no speech follows).
const defaultSessionTimeout = 15 * time.Second

// Session lifecycle event types, published on the SessionManager's
// EventBus (SPEC-0060's "sessions start correctly"/"sessions complete
// correctly"/"resources are released" testing criteria).
const (
	EventSessionStarted   types.EventType = "VOICE_SESSION_STARTED"
	EventSessionCompleted types.EventType = "VOICE_SESSION_COMPLETED"
	EventSessionFailed    types.EventType = "VOICE_SESSION_FAILED"
)

// SessionState is where a Session currently sits in the SPEC-0060 flow.
type SessionState string

const (
	// SessionStateListening: a wake word was detected and the session is
	// waiting for a final transcript.
	SessionStateListening SessionState = "listening"
	// SessionStateProcessing: a transcript was received and RequestHandler
	// is running.
	SessionStateProcessing SessionState = "processing"
	// SessionStateResponding: RequestHandler returned a response and it is
	// being synthesized/played back.
	SessionStateResponding SessionState = "responding"
)

// Session is one complete voice interaction, from wake word detection
// through its spoken response (or failure).
type Session struct {
	ID         string
	State      SessionState
	StartedAt  time.Time
	Transcript string
}

// RequestHandler processes a session's transcribed utterance and returns
// the agent's response text (SPEC-0060's "Process Request"/"Generate
// Response" stages). This is the seam to the Agent layer - e.g. a closure
// over core.Communicator.Request or an ExecutionLoop.Run - so
// SessionManager stays agnostic of which agent or dispatch mechanism
// produced the response. RequestHandler must respect ctx cancellation.
type RequestHandler func(ctx context.Context, transcript string) (string, error)

// ResponseChunk is one incremental piece of a session's agent response,
// delivered to the callback StreamingRequestHandler invokes - the
// streaming-response counterpart of core.StreamChunk, whose Text/Done shape
// it mirrors. Done marks the final chunk; it may carry empty Text if the
// handler has nothing left to flush.
type ResponseChunk struct {
	Text string
	Done bool
}

// StreamingRequestHandler is RequestHandler's streaming counterpart
// (SPEC-0061's "streaming responses" requirement): instead of returning the
// complete response text in one call, it invokes onChunk for each
// incremental piece as it becomes available - e.g. a closure over
// core.Provider.Stream or core.StreamHandler.Stream - so SessionManager can
// begin synthesizing/speaking a response before the agent has finished
// producing all of it. StreamingRequestHandler must respect ctx
// cancellation, must invoke onChunk with a final Done chunk once the
// response is complete, and must return promptly if onChunk returns an
// error.
type StreamingRequestHandler func(ctx context.Context, transcript string, onChunk func(ResponseChunk) error) error

// sentenceBufferSize bounds how many complete sentences processRequestStreaming
// may have queued for speaking (via sentenceCh) ahead of what speakSentences
// has actually spoken. Buffered rather than synchronous so the streaming
// handler can keep producing the next sentence's text while the current one
// is still being synthesized/played - the pipelining that gives SPEC-0061's
// streaming path its latency advantage over SPEC-0060's batch path.
const sentenceBufferSize = 4

// streamAudioBufferSize bounds how many PCM chunks speakSentence may have
// buffered from TTSProvider.StreamSynthesize ahead of what
// core.VoiceEngine.PlaybackStream has actually played.
const streamAudioBufferSize = 8

// SessionManager implements SPEC-0060: it owns a Microphone's audio
// lifecycle (SPEC-0053/0054, including its STT state per SPEC-0056) and
// gates the EventWakeWordDetected/EventVoiceTranscript events it publishes
// into bounded Sessions, each ending with a RequestHandler call (agent
// communication) and a spoken TTSProvider response. SessionManager is safe
// for concurrent use.
type SessionManager struct {
	mic              *Microphone
	engine           core.VoiceEngine
	tts              core.TTSProvider
	cfg              *cfgpkg.VoiceConfig
	handler          RequestHandler
	streamingHandler StreamingRequestHandler
	bus              core.EventBus
	log              *logger.Logger

	sessionTimeout time.Duration

	mu         sync.Mutex
	running    bool
	session    *Session
	activeDone chan struct{}
	nextID     int

	unsubWake       func()
	unsubTranscript func()
}

// SessionManagerOption configures a SessionManager created by
// NewSessionManager.
type SessionManagerOption func(*SessionManager)

// WithSessionTimeout overrides how long a Session waits for a final
// transcript before being abandoned. A value <= 0 disables the timeout
// (a session then waits indefinitely for speech). Optional; defaults to
// defaultSessionTimeout.
func WithSessionTimeout(d time.Duration) SessionManagerOption {
	return func(sm *SessionManager) { sm.sessionTimeout = d }
}

// WithStreamingHandler configures a StreamingRequestHandler
// (SPEC-0061). When set, processRequest uses it instead of the
// constructor's required RequestHandler: response text is synthesized and
// spoken sentence-by-sentence as it streams in (processRequestStreaming),
// rather than only after the complete response text is available
// (SPEC-0060's batch behavior, which NewSessionManager's handler parameter
// still exists to support when this option isn't used). Optional; unset by
// default.
func WithStreamingHandler(h StreamingRequestHandler) SessionManagerOption {
	return func(sm *SessionManager) { sm.streamingHandler = h }
}

// NewSessionManager creates a ready-to-use SessionManager. mic, engine, tts,
// cfg, bus, and handler are all required: it returns a packages/errors error
// typed TypeInvalidInput naming the first missing one. bus must be the same
// EventBus mic publishes EventWakeWordDetected/EventVoiceTranscript on -
// unlike Microphone's own optional bus, SessionManager cannot function
// without one, since subscribing to those events is its only way to learn
// about wake word detections and transcripts. cfg supplies TTSSampleRate,
// the rate tts's synthesized audio is played back at (see
// core.VoiceEngine.Playback's doc comment).
func NewSessionManager(mic *Microphone, engine core.VoiceEngine, tts core.TTSProvider, cfg *cfgpkg.VoiceConfig, bus core.EventBus, handler RequestHandler, log *logger.Logger, opts ...SessionManagerOption) (*SessionManager, error) {
	if mic == nil {
		return nil, errors.New(errors.TypeInvalidInput, "SESSION_MANAGER_MISSING_MICROPHONE", "core.voice.sessionmanager",
			"cannot create a session manager without a microphone")
	}
	if engine == nil {
		return nil, errors.New(errors.TypeInvalidInput, "SESSION_MANAGER_MISSING_ENGINE", "core.voice.sessionmanager",
			"cannot create a session manager without a voice engine")
	}
	if tts == nil {
		return nil, errors.New(errors.TypeInvalidInput, "SESSION_MANAGER_MISSING_TTS", "core.voice.sessionmanager",
			"cannot create a session manager without a TTS provider")
	}
	if cfg == nil {
		return nil, errors.New(errors.TypeInvalidInput, "SESSION_MANAGER_MISSING_CONFIG", "core.voice.sessionmanager",
			"cannot create a session manager without a voice config")
	}
	if bus == nil {
		return nil, errors.New(errors.TypeInvalidInput, "SESSION_MANAGER_MISSING_BUS", "core.voice.sessionmanager",
			"cannot create a session manager without an event bus")
	}
	if handler == nil {
		return nil, errors.New(errors.TypeInvalidInput, "SESSION_MANAGER_MISSING_HANDLER", "core.voice.sessionmanager",
			"cannot create a session manager without a request handler")
	}

	sm := &SessionManager{
		mic:            mic,
		engine:         engine,
		tts:            tts,
		cfg:            cfg,
		bus:            bus,
		handler:        handler,
		log:            log,
		sessionTimeout: defaultSessionTimeout,
	}
	for _, opt := range opts {
		opt(sm)
	}
	return sm, nil
}

// Start begins the voice session lifecycle: it starts the underlying
// Microphone's audio capture (SPEC-0060's "audio lifecycle" requirement)
// and subscribes to its wake word/transcript events. Calling Start twice is
// a no-op.
func (sm *SessionManager) Start() error {
	sm.mu.Lock()
	if sm.running {
		sm.mu.Unlock()
		return nil
	}
	sm.running = true
	sm.mu.Unlock()

	if err := sm.mic.Start(); err != nil {
		sm.mu.Lock()
		sm.running = false
		sm.mu.Unlock()
		return err
	}

	sm.unsubWake = sm.bus.Subscribe(EventWakeWordDetected, sm.handleWakeWord)
	sm.unsubTranscript = sm.bus.Subscribe(EventVoiceTranscript, sm.handleTranscript)

	sm.log.Info("voice: session manager started", nil)
	return nil
}

// Stop ends the voice session lifecycle: it unsubscribes from the
// Microphone's events, abandons any in-flight Session (publishing
// EventSessionFailed so no session is silently left dangling), and stops
// the Microphone's audio capture - releasing every resource Start
// acquired (SPEC-0060's "resources are released" testing criterion).
// Calling Stop twice, or before Start, is a no-op.
func (sm *SessionManager) Stop() error {
	sm.mu.Lock()
	if !sm.running {
		sm.mu.Unlock()
		return nil
	}
	sm.running = false
	unsubWake := sm.unsubWake
	unsubTranscript := sm.unsubTranscript
	sm.unsubWake = nil
	sm.unsubTranscript = nil
	sm.mu.Unlock()

	if unsubWake != nil {
		unsubWake()
	}
	if unsubTranscript != nil {
		unsubTranscript()
	}

	if session := sm.clearSessionIfID(""); session != nil {
		sm.publish(EventSessionFailed, map[string]any{"sessionId": session.ID, "reason": "voice session manager stopped"})
	}

	sm.log.Info("voice: session manager stopped", nil)
	return sm.mic.Stop()
}

// CurrentSession returns a snapshot of the in-flight Session, or nil if no
// session is currently active.
func (sm *SessionManager) CurrentSession() *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.session == nil {
		return nil
	}
	snapshot := *sm.session
	return &snapshot
}

// handleWakeWord starts a new Session on EventWakeWordDetected (SPEC-0060's
// "session creation" requirement), unless one is already active - only one
// Session runs at a time, so a wake word detected mid-session is ignored.
func (sm *SessionManager) handleWakeWord(event types.Event) {
	sm.mu.Lock()
	if sm.session != nil {
		sm.mu.Unlock()
		sm.log.Debug("voice: wake word detected while a session is already active, ignoring", nil)
		return
	}
	sm.nextID++
	session := &Session{
		ID:        fmt.Sprintf("session-%d", sm.nextID),
		State:     SessionStateListening,
		StartedAt: time.Now().UTC(),
	}
	doneCh := make(chan struct{})
	sm.session = session
	sm.activeDone = doneCh
	sm.mu.Unlock()

	sm.log.Info("voice: session started", map[string]any{"sessionId": session.ID})
	sm.publish(EventSessionStarted, map[string]any{"sessionId": session.ID})

	if sm.sessionTimeout > 0 {
		go sm.watchTimeout(session.ID, doneCh)
	}
}

// watchTimeout abandons the Session identified by sessionID if doneCh
// hasn't been closed (by handleTranscript, once speech arrives) within
// sm.sessionTimeout.
func (sm *SessionManager) watchTimeout(sessionID string, doneCh <-chan struct{}) {
	timer := time.NewTimer(sm.sessionTimeout)
	defer timer.Stop()
	select {
	case <-timer.C:
		if session := sm.clearSessionIfID(sessionID); session != nil {
			sm.log.Info("voice: session timed out waiting for speech", map[string]any{"sessionId": session.ID})
			sm.publish(EventSessionFailed, map[string]any{"sessionId": session.ID, "reason": "timed out waiting for speech"})
		}
	case <-doneCh:
	}
}

// handleTranscript advances the active Session's STT state on a final
// EventVoiceTranscript (SPEC-0060's "STT state" requirement): a non-final
// chunk, or a transcript arriving with no Session in SessionStateListening
// (no wake word gated it), is ignored. An empty final transcript ends the
// session without invoking RequestHandler (nothing was said); otherwise the
// transcript is handed off to processRequest.
func (sm *SessionManager) handleTranscript(event types.Event) {
	final, _ := event.Payload["final"].(bool)
	if !final {
		return
	}

	sm.mu.Lock()
	session := sm.session
	if session == nil || session.State != SessionStateListening {
		sm.mu.Unlock()
		return
	}
	text, _ := event.Payload["text"].(string)
	session.State = SessionStateProcessing
	session.Transcript = text
	if sm.activeDone != nil {
		close(sm.activeDone)
		sm.activeDone = nil
	}
	sm.mu.Unlock()

	if strings.TrimSpace(text) == "" {
		sm.finishSession(session, EventSessionFailed, "empty transcript")
		return
	}

	sm.processRequest(session, text)
}

// processRequest carries out SPEC-0060's "Process Request", "Generate
// Response", and "Speak" stages for session. If a StreamingRequestHandler
// is configured (WithStreamingHandler), it drives SPEC-0061's streaming
// path via processRequestStreaming; otherwise it falls back to SPEC-0060's
// batch path: RequestHandler produces the complete response text (agent
// communication), tts.Synthesize converts it to audio, and engine.Playback
// speaks it. Either way, any failure ends the session as failed rather than
// continuing to the next stage.
func (sm *SessionManager) processRequest(session *Session, text string) {
	if sm.streamingHandler != nil {
		sm.processRequestStreaming(session, text)
		return
	}

	ctx := context.Background()

	response, err := sm.handler(ctx, text)
	if err != nil {
		sm.log.Error("voice: request handler failed", map[string]any{"sessionId": session.ID, "error": err.Error()})
		sm.finishSession(session, EventSessionFailed, err.Error())
		return
	}

	sm.mu.Lock()
	if sm.session == session {
		session.State = SessionStateResponding
	}
	sm.mu.Unlock()

	audio, err := sm.tts.Synthesize(ctx, response, core.VoiceOptions{})
	if err != nil {
		sm.log.Error("voice: tts synthesis failed", map[string]any{"sessionId": session.ID, "error": err.Error()})
		sm.finishSession(session, EventSessionFailed, err.Error())
		return
	}

	if err := sm.engine.Playback(audio, sm.cfg.TTSSampleRate); err != nil {
		sm.log.Error("voice: playback failed", map[string]any{"sessionId": session.ID, "error": err.Error()})
		sm.finishSession(session, EventSessionFailed, err.Error())
		return
	}

	sm.finishSession(session, EventSessionCompleted, "")
}

// processRequestStreaming is processRequest's SPEC-0061 streaming path: it
// runs streamingHandler, splitting its incrementally-delivered response text
// into sentences (flushCompleteSentences) and handing each complete sentence
// to speakSentences as soon as it's ready, so speaking the start of a
// response can overlap with the handler still producing the rest of it -
// unlike processRequest's batch path, which waits for the complete response
// before synthesizing or speaking any of it.
//
// speakSentences runs concurrently, consuming sentenceCh; if it fails
// partway through (a TTS or playback error), ctx is cancelled so
// streamingHandler - and any pending, blocked send to sentenceCh in
// onChunk - stops promptly instead of continuing to produce text nothing
// will speak.
func (sm *SessionManager) processRequestStreaming(session *Session, text string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sentenceCh := make(chan string, sentenceBufferSize)
	speakDone := make(chan error, 1)
	go func() {
		err := sm.speakSentences(ctx, session, sentenceCh)
		if err != nil {
			cancel()
		}
		speakDone <- err
	}()

	var buf strings.Builder
	handlerErr := sm.streamingHandler(ctx, text, func(chunk ResponseChunk) error {
		buf.WriteString(chunk.Text)
		for _, sentence := range flushCompleteSentences(&buf) {
			if err := sendSentence(ctx, sentenceCh, sentence); err != nil {
				return err
			}
		}
		if chunk.Done {
			if rest := strings.TrimSpace(buf.String()); rest != "" {
				buf.Reset()
				return sendSentence(ctx, sentenceCh, rest)
			}
			buf.Reset()
		}
		return nil
	})
	close(sentenceCh)

	speakErr := <-speakDone

	// speakErr, not handlerErr, is reported when both are non-nil: a
	// speakSentences failure is what triggered cancel() above, so any
	// resulting handlerErr is just that cancellation surfacing through
	// streamingHandler, not the failure's root cause.
	switch {
	case speakErr != nil:
		sm.log.Error("voice: streaming speech failed", map[string]any{"sessionId": session.ID, "error": speakErr.Error()})
		sm.finishSession(session, EventSessionFailed, speakErr.Error())
	case handlerErr != nil:
		sm.log.Error("voice: streaming request handler failed", map[string]any{"sessionId": session.ID, "error": handlerErr.Error()})
		sm.finishSession(session, EventSessionFailed, handlerErr.Error())
	default:
		sm.finishSession(session, EventSessionCompleted, "")
	}
}

// sendSentence sends sentence on sentenceCh, returning ctx.Err() instead of
// blocking forever if ctx is cancelled first (e.g. by speakSentences having
// already failed).
func sendSentence(ctx context.Context, sentenceCh chan<- string, sentence string) error {
	select {
	case sentenceCh <- sentence:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// speakSentences reads sentences from sentenceCh in the order
// processRequestStreaming sends them and speaks each in turn via
// speakSentence, returning the first error encountered (having stopped
// consuming further sentences at that point - the caller is responsible for
// unblocking any sentence it still has left to send, via ctx cancellation).
func (sm *SessionManager) speakSentences(ctx context.Context, session *Session, sentenceCh <-chan string) error {
	responding := false
	for sentence := range sentenceCh {
		if !responding {
			sm.mu.Lock()
			if sm.session == session {
				session.State = SessionStateResponding
			}
			sm.mu.Unlock()
			responding = true
		}
		if err := sm.speakSentence(ctx, sentence); err != nil {
			return err
		}
	}
	return nil
}

// speakSentence synthesizes and speaks one sentence, pipelining
// tts.StreamSynthesize's output directly into engine.PlaybackStream (both
// running concurrently) so playback of the start of a sentence can begin
// before the rest of it has finished synthesizing.
func (sm *SessionManager) speakSentence(ctx context.Context, sentence string) error {
	audioCh := make(chan []byte, streamAudioBufferSize)
	synthDone := make(chan error, 1)
	go func() {
		defer close(audioCh)
		synthDone <- sm.tts.StreamSynthesize(ctx, sentence, core.VoiceOptions{}, audioCh)
	}()

	playbackErr := sm.engine.PlaybackStream(ctx, audioCh, sm.cfg.TTSSampleRate)
	if synthErr := <-synthDone; synthErr != nil {
		return synthErr
	}
	return playbackErr
}

// sentenceBoundary reports whether b ends a sentence a streamed response
// should be split on for speaking (flushCompleteSentences), so playback of
// one sentence can start as soon as it's complete rather than waiting for
// the entire response.
func sentenceBoundary(b byte) bool {
	switch b {
	case '.', '!', '?', '\n':
		return true
	default:
		return false
	}
}

// flushCompleteSentences extracts every complete sentence currently in buf
// (ending in a sentenceBoundary byte) and returns them in order, leaving any
// trailing incomplete sentence in buf for a later chunk to complete.
// Sentence-boundary bytes are all single-byte ASCII, so splitting on them
// never lands in the middle of a multi-byte UTF-8 rune.
func flushCompleteSentences(buf *strings.Builder) []string {
	s := buf.String()
	var sentences []string
	start := 0
	for i := 0; i < len(s); i++ {
		if !sentenceBoundary(s[i]) {
			continue
		}
		if sentence := strings.TrimSpace(s[start : i+1]); sentence != "" {
			sentences = append(sentences, sentence)
		}
		start = i + 1
	}
	buf.Reset()
	buf.WriteString(s[start:])
	return sentences
}

// finishSession ends session (SPEC-0060's "session cleanup" requirement):
// it clears sm.session so the next wake word can start a fresh session, and
// publishes eventType (EventSessionCompleted or EventSessionFailed, the
// latter carrying reason).
func (sm *SessionManager) finishSession(session *Session, eventType types.EventType, reason string) {
	sm.clearSessionIfID(session.ID)

	payload := map[string]any{"sessionId": session.ID, "transcript": session.Transcript}
	if reason != "" {
		payload["reason"] = reason
	}
	sm.publish(eventType, payload)
	sm.log.Info("voice: session finished", map[string]any{"sessionId": session.ID, "event": string(eventType)})
}

// clearSessionIfID clears sm.session and stops its timeout watchdog (if
// any), returning the cleared Session. If id is non-empty, sm.session is
// only cleared when its ID matches (a stale timeout firing for a session
// that has already finished is a no-op). Returns nil if there was nothing
// to clear.
func (sm *SessionManager) clearSessionIfID(id string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	session := sm.session
	if session == nil || (id != "" && session.ID != id) {
		return nil
	}
	sm.session = nil
	if sm.activeDone != nil {
		close(sm.activeDone)
		sm.activeDone = nil
	}
	return session
}

// publish broadcasts an Event of eventType on the SessionManager's
// EventBus.
func (sm *SessionManager) publish(eventType types.EventType, payload map[string]any) {
	sm.bus.Publish(types.Event{
		Type:      eventType,
		Source:    "core.voice.sessionmanager",
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	})
}

// Ensure SessionManager implements core.VoiceSessionManager.
var _ core.VoiceSessionManager = (*SessionManager)(nil)
