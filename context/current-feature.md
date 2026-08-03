# Current Feature: Text To Speech Provider

## Working In

services/core (Voice branch of Phase 4 Intelligence) - mirrors SPEC-0056's
STT Provider abstraction: a new stt_provider.go-shaped tts_provider.go
defining a TTSProvider interface plus its container.go wiring
(WithTTSProvider option, Container.TTSProvider slot), following the
STTProvider precedent exactly. No concrete engine yet - SPEC-0059 (Piper TTS
Integration) supplies the first implementation, same relationship SPEC-0057
(Whisper) has to SPEC-0056.

## Status

In Progress

## Goals

- Text input
- Voice generation
- Streaming audio output
- Voice configuration

## Dependencies

- SPEC-0053 Audio Engine (status: Completed) - VoiceEngine.Playback and its
  raw PCM mono/int16 LE audio format is what "streaming audio output" must
  produce/feed into
- SPEC-0056 Speech To Text Provider (status: Completed) - not a functional
  dependency, but the direct structural precedent: TTSProvider should mirror
  STTProvider's shape (interface-only, no engine, ctx-based, channel-based
  streaming) and its container.go wiring pattern

## Notes

Specification:

context/features/SPEC-0058-text-to-speech-provider.md

Index status at load time: Planned

Dependency resolution source: Requirements inference (Step 4 fallback #3) -
JARVIS_IMPLEMENTATION_ORDER.md and JARVIS_DEPENDENCY_GRAPH.md are both
phase-level only ("Voice" as a single node) and name no per-spec
dependencies for SPEC-0058. Cross-referenced against services/core/container.go
and stt_provider.go directly to confirm the STTProvider precedent's actual
shape.

Related specs: SPEC-0059 Piper TTS Integration (Planned, not yet loaded) -
the concrete engine this interface exists to support.

## History

- 2026-08-03 18:55 setup_feature.ps1 loaded SPEC-0058 (SPEC-0058-text-to-speech-provider.md)
- 2026-08-03 load action completed manual dependency resolution (Step 4) per
  .claude/skills/feature/actions/load.md; script's index-only pass found no
  declared dependencies, so resolved via requirements inference against
  container.go/stt_provider.go instead.
- 2026-08-03 start action: created branch feature/tts-provider-interface.
  Implemented services/core/tts_provider.go (TTSProvider interface +
  VoiceOptions, mirroring SPEC-0056's stt_provider.go pattern) and
  services/core/tts_provider_test.go (interface-satisfaction, streaming
  order, error handling, context-cancellation tests, mirroring
  stt_provider_test.go). Removed the stale inline TTSProvider placeholder
  from container.go (mistagged SPEC-0059; that's Piper, the concrete
  engine, not this interface) - same treatment SPEC-0056 gave STTProvider's
  prior placeholder. go build/vet/test clean for services/core and
  services/core/voice except one pre-existing flaky test
  (TestWorker_FailsTerminallyAfterMaxAttempts, unrelated task-worker code,
  passes in isolation/repeat runs) and a repo-wide gofmt CRLF false-positive
  from Windows core.autocrlf=true (affects all 101 .go files in the module,
  not something this change introduced).
