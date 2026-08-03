// stt_provider.go implements SPEC-0056: the Speech To Text Provider
// interface. STTProvider is the contract the voice subsystem (SPEC-0054's
// Microphone) uses to turn captured audio into text without depending on any
// single speech recognition engine. SPEC-0057 (Whisper Integration) supplies
// the first concrete STTProvider; this spec defines only the contract and
// its supporting types.
package core

import "context"

// TranscriptionResult is a completed, non-streamed transcription: the text
// an STTProvider recognized in an audio clip, and the provider's confidence
// in that result (the "confidence scores" requirement), on a 0-1 scale
// where 1 is fully confident. A provider that doesn't estimate confidence
// may leave Confidence at its zero value.
type TranscriptionResult struct {
	Text       string
	Confidence float64
}

// TranscriptionChunk is one incremental piece of a streamed transcription,
// delivered to StreamTranscribe's resultCh. Done marks the final chunk for
// the audio received so far; a provider may send further chunks later as
// more audio arrives on audioCh.
type TranscriptionChunk struct {
	Text       string
	Confidence float64
	Done       bool
}

// STTProvider converts speech audio to text (SPEC-0056). Both methods accept
// raw PCM audio (mono, int16 LE, matching VoiceEngine.Capture's chunk
// format) so a caller can choose either mode without changing audio format.
type STTProvider interface {
	// Transcribe converts one complete audio clip to text in a single call
	// (the "transcription" requirement).
	Transcribe(ctx context.Context, audio []byte) (TranscriptionResult, error)

	// StreamTranscribe consumes audio chunks from audioCh as they arrive and
	// delivers incremental transcription results to resultCh (the
	// "streaming results" requirement), until audioCh closes or ctx is
	// cancelled. StreamTranscribe must respect ctx cancellation and must not
	// block permanently on a full resultCh past ctx's lifetime.
	StreamTranscribe(ctx context.Context, audioCh <-chan []byte, resultCh chan<- TranscriptionChunk) error
}
