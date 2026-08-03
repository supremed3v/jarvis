# Current Feature: (none active)

## Working In

(none)

## Status

Not Started

## Goals

(none)

## Dependencies

(none)

## Notes

Next candidate per docs/execution/JARVIS_IMPLEMENTATION_ORDER.md: SPEC-0057
(Whisper Integration), the fifth spec of the Voice branch of Phase 4
Intelligence, supplying the first concrete `core.STTProvider` implementation
against the abstraction SPEC-0056 completed. Status: Planned per
context/features/FEATURE_INDEX.md.

## History

- 2026-08-03 09:35 setup_feature.ps1 loaded SPEC-0053 (SPEC-0053-audio-engine-interface.md)
- 2026-08-03 /feature review found SPEC-0053/0054/0055 overclaimed completion
  in docs/agents/JARVIS_BUILD_TRACKER.md (device discovery, device-failure
  recovery, and wake-word/STT EventBus emission were all missing); tracker
  and FEATURE_INDEX.md corrected to In Progress.
- 2026-08-03 Implemented the three missing pieces: `core.VoiceEngine.ListDevices`
  (+ `audio_engine.py list-devices`) for device discovery; a
  `superviseCapture` goroutine in `audio_engine.go` for automatic
  restart-on-crash (bounded to 5 attempts); `Microphone` now takes a
  `core.EventBus` and publishes `WAKE_WORD_DETECTED`/`VOICE_TRANSCRIPT`
  events instead of only logging. New tests added (deterministic event-bus
  test plus two subprocess-dependent tests, one of which was actually
  exercised: killed a live capture subprocess and confirmed automatic
  restart). `go build`/`go vet`/`go test` clean via scripts/go_all.ps1.
  Tracker restored to Completed on SPEC-0053/0054/0055; FEATURE_INDEX.md
  regenerated. Feature reset to no active work; SPEC-0056 is next.
- 2026-08-03 /feature start SPEC-0056 (Speech To Text Provider): implemented
  the STTProvider abstraction layer as a contract-only spec (mirroring
  llm_provider.go's SPEC-0026 pattern - Whisper, the first concrete
  provider, arrives in SPEC-0057, not this spec). Moved the placeholder
  `STTProvider` interface out of `container.go` into a new
  `services/core/stt_provider.go`, adding `TranscriptionResult{Text,
  Confidence}` and `TranscriptionChunk{Text, Confidence, Done}` so both
  Transcribe and StreamTranscribe report the spec's required confidence
  score (the old interface returned bare strings with no confidence at
  all). Updated the two real consumers of the old shape:
  `services/core/voice/microphone.go` (StreamTranscribe's channel now
  carries TranscriptionChunk; the VOICE_TRANSCRIPT event payload gained
  confidence/final fields) and voice_test.go's mockSTTProvider. Added
  `services/core/stt_provider_test.go` with stub-based contract tests
  covering all three SPEC-0056 testing criteria (audio converts to text,
  streaming transcription works, errors are handled) plus a context-
  cancellation regression test. `go build`/`go vet`/`go test` clean via
  scripts/go_all.ps1 across all 5 workspace modules. Tracker updated to
  Completed on SPEC-0056; FEATURE_INDEX.md regenerated. Feature reset to no
  active work; SPEC-0057 (Whisper Integration) is next.
