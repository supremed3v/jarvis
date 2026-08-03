// microphone.go implements SPEC-0054: Microphone Capture System.
// Fans out audio capture to wake word detector and STT provider.
package voice

import (
	"context"
	"sync"
	"time"

	"jarvis-pa/packages/logger"
	types "jarvis-pa/packages/shared-types"
	"jarvis-pa/services/core"
)

// Event types Microphone publishes on its EventBus (SPEC-0055's "detection
// events are emitted" testing criterion, plus the STT transcript equivalent
// for SPEC-0054's "provide audio streams to STT" once transcribed).
const (
	EventWakeWordDetected types.EventType = "WAKE_WORD_DETECTED"
	EventVoiceTranscript  types.EventType = "VOICE_TRANSCRIPT"
)

// Microphone captures audio and distributes to consumers.
type Microphone struct {
	engine   core.VoiceEngine
	wakeWord core.WakeWordDetector
	stt      core.STTProvider
	bus      core.EventBus
	log      *logger.Logger

	captureCh <-chan []byte
	wakeCh    chan []byte
	sttCh     chan []byte

	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool
	mu      sync.Mutex
}

// NewMicrophone creates a new Microphone. bus may be nil, in which case
// Microphone still functions but publishes no events.
func NewMicrophone(engine core.VoiceEngine, wakeWord core.WakeWordDetector, stt core.STTProvider, bus core.EventBus, log *logger.Logger) *Microphone {
	ctx, cancel := context.WithCancel(context.Background())
	return &Microphone{
		engine:   engine,
		wakeWord: wakeWord,
		stt:      stt,
		bus:      bus,
		log:      log,
		ctx:      ctx,
		cancel:   cancel,
		wakeCh:   make(chan []byte, 50),
		sttCh:    make(chan []byte, 50),
	}
}

// publish emits an Event of eventType on the Microphone's EventBus, if one
// is configured.
func (m *Microphone) publish(eventType types.EventType, payload map[string]any) {
	if m.bus == nil {
		return
	}
	m.bus.Publish(types.Event{
		Type:      eventType,
		Source:    "core.voice.microphone",
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	})
}

// Start begins audio capture and distribution.
func (m *Microphone) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return nil
	}

	captureCh, err := m.engine.Capture()
	if err != nil {
		return err
	}
	m.captureCh = captureCh

	// Start wake word detector, fed by the distribution loop's wakeCh below.
	if err := m.wakeWord.Start(m.ctx, m.wakeCh, func() {
		m.log.Info("voice: wake word detected", nil)
		m.publish(EventWakeWordDetected, nil)
	}); err != nil {
		return err
	}

	// Start STT streaming
	textCh := make(chan string, 10)
	if err := m.stt.StreamTranscribe(m.ctx, m.sttCh, textCh); err != nil {
		return err
	}

	// Handle transcriptions
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		for {
			select {
			case text := <-textCh:
				if text != "" {
					m.log.Info("voice: transcript", map[string]any{"text": text})
					m.publish(EventVoiceTranscript, map[string]any{"text": text})
				}
			case <-m.ctx.Done():
				return
			}
		}
	}()

	// Distribution loop
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		for {
			select {
			case chunk := <-m.captureCh:
				if chunk == nil {
					return
				}
				// Fan out to wake word and STT
				select {
				case m.wakeCh <- chunk:
				default:
				}
				select {
				case m.sttCh <- chunk:
				default:
				}
			case <-m.ctx.Done():
				return
			}
		}
	}()

	m.running = true
	m.log.Info("voice: microphone started", nil)
	return nil
}

// Stop stops audio capture.
func (m *Microphone) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	m.cancel()
	m.wg.Wait()

	m.wakeWord.Stop()
	close(m.wakeCh)
	close(m.sttCh)

	m.running = false
	m.log.Info("voice: microphone stopped", nil)
	return nil
}
