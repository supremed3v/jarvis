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

Next candidate per docs/execution/JARVIS_IMPLEMENTATION_ORDER.md: SPEC-0056
(Speech To Text Provider), the fourth spec of the Voice branch of Phase 4
Intelligence, sitting on top of SPEC-0053/0054/0055 (all Completed) for its
`core.STTProvider` interface's audio input. Status: Planned per
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
