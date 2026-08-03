// tts_provider.go implements SPEC-0058: the Text To Speech Provider
// interface. TTSProvider is the contract the voice subsystem uses to turn
// text into speech audio without depending on any single speech synthesis
// engine, mirroring SPEC-0056's STTProvider for the reverse direction.
// SPEC-0059 (Piper TTS Integration) supplies the first concrete
// TTSProvider; this spec defines only the contract and its supporting
// types.
package core

import "context"

// VoiceOptions configures how a TTSProvider synthesizes speech (SPEC-0058's
// "voice configuration" requirement): which voice to speak as and how fast
// to speak it. This is per-call configuration distinct from VoiceConfig's
// engine-level settings (e.g. TTSModel) - a provider that doesn't support a
// given field, or is left at its zero value, falls back to its own default.
type VoiceOptions struct {
	Voice string
	Speed float64
}

// TTSProvider converts text to speech audio (SPEC-0058). Both methods
// return/stream raw PCM audio (mono, int16 LE, matching VoiceEngine.Playback's
// expected format) so a caller can feed the result directly to playback.
type TTSProvider interface {
	// Synthesize converts text to one complete audio clip in a single call
	// (the "voice generation" requirement).
	Synthesize(ctx context.Context, text string, opts VoiceOptions) ([]byte, error)

	// StreamSynthesize converts text to speech incrementally, delivering
	// audio chunks to audioCh as they're generated (the "streaming audio
	// output" requirement), until synthesis completes or ctx is cancelled.
	// StreamSynthesize must respect ctx cancellation and must not block
	// permanently on a full audioCh past ctx's lifetime. The caller owns
	// audioCh and closes it once StreamSynthesize returns, mirroring
	// STTProvider.StreamTranscribe's resultCh convention.
	StreamSynthesize(ctx context.Context, text string, opts VoiceOptions, audioCh chan<- []byte) error
}
