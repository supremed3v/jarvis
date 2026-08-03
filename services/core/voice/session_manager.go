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

// SessionManager implements SPEC-0060: it owns a Microphone's audio
// lifecycle (SPEC-0053/0054, including its STT state per SPEC-0056) and
// gates the EventWakeWordDetected/EventVoiceTranscript events it publishes
// into bounded Sessions, each ending with a RequestHandler call (agent
// communication) and a spoken TTSProvider response. SessionManager is safe
// for concurrent use.
type SessionManager struct {
	mic     *Microphone
	engine  core.VoiceEngine
	tts     core.TTSProvider
	cfg     *cfgpkg.VoiceConfig
	handler RequestHandler
	bus     core.EventBus
	log     *logger.Logger

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
// Response", and "Speak" stages for session: RequestHandler produces the
// response text (agent communication), tts.Synthesize converts it to audio,
// and engine.Playback speaks it. Any failure ends the session as failed
// rather than continuing to the next stage.
func (sm *SessionManager) processRequest(session *Session, text string) {
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
