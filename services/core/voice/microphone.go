// microphone.go implements SPEC-0054: Microphone Capture System.
// Fans out audio capture to wake word detector and STT provider.
package voice

import (
	"context"
	"sync"

	"jarvis-pa/packages/logger"
	"jarvis-pa/services/core"
)

// Microphone captures audio and distributes to consumers.
type Microphone struct {
	engine   core.VoiceEngine
	wakeWord core.WakeWordDetector
	stt      core.STTProvider
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

// NewMicrophone creates a new Microphone.
func NewMicrophone(engine core.VoiceEngine, wakeWord core.WakeWordDetector, stt core.STTProvider, log *logger.Logger) *Microphone {
	ctx, cancel := context.WithCancel(context.Background())
	return &Microphone{
		engine:   engine,
		wakeWord: wakeWord,
		stt:      stt,
		log:      log,
		ctx:      ctx,
		cancel:   cancel,
		wakeCh:   make(chan []byte, 50),
		sttCh:    make(chan []byte, 50),
	}
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
		// EventBus event would be emitted here in integration
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
					// EventBus event would be emitted here
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
