# Current Feature: Whisper Integration

## Working In

services/core/voice/ (or services/core/stt_provider.go's package `core`) -
a new concrete `core.STTProvider` implementation wrapping Whisper, alongside
the existing `audio_engine.go`/`microphone.go`/`wake_word.go` voice pipeline
files. `services/voice/` is still an empty `.gitkeep`-only scaffold with no
prior spec requiring it, so default to `services/core/voice/` unless
implementation reveals a reason to stand up `services/voice/` instead.

## Status

In Progress

## Goals

- Local inference
- Model configuration
- Streaming transcription
- Language settings

## Dependencies

- SPEC-0056 Speech To Text Provider (status: Completed) - defines the
  `core.STTProvider` contract (`Transcribe`/`StreamTranscribe`,
  `TranscriptionResult`/`TranscriptionChunk`) in `services/core/stt_provider.go`
  that this spec's Whisper provider must implement; that file's own doc
  comment names SPEC-0057 as supplying its first concrete implementation.
- SPEC-0053/0054/0055 Audio Engine/Microphone/Wake Word (status: Completed) -
  the capture pipeline (`audio_engine.go`, `microphone.go`) that produces the
  mono int16 LE PCM audio `STTProvider.Transcribe`/`StreamTranscribe` expect
  as input.
- SPEC-0003 Configuration System (status: Completed) - this spec's own
  "Model configuration"/"Device selection"/"Processing settings"
  requirements are configuration surface, expected to route through
  `packages/config` rather than a bespoke mechanism.

## Notes

Specification:

context/features/SPEC-0057-whisper-integration.md

Index status at load time: Planned

Dependency resolution source: Requirements inference (FEATURE_INDEX.md
carries no explicit Dependencies field yet; JARVIS_IMPLEMENTATION_ORDER.md's
Phase 4 section is stale/high-level and doesn't enumerate Voice-branch specs
individually - cross-referenced against stt_provider.go's own doc comment
and JARVIS_BUILD_TRACKER.md's SPEC-0053-0056 entries instead).

## History

- 2026-08-03 10:17 setup_feature.ps1 loaded SPEC-0057 (SPEC-0057-whisper-integration.md)
- 2026-08-03 /feature load resolved dependencies (SPEC-0056, SPEC-0053/0054/0055,
  SPEC-0003) and implementation area (services/core/voice/) since
  FEATURE_INDEX.md and JARVIS_IMPLEMENTATION_ORDER.md don't carry this
  spec-level detail; setup_feature.ps1's automated pass only had time to
  record the spec itself.
- 2026-08-03 /feature start implemented all four goals on branch
  feature/whisper-integration: new `services/core/voice/whisper_provider.go`
  (`WhisperProvider`, the first concrete `core.STTProvider`) plus
  `services/core/voice/scripts/whisper_stt.py` (faster-whisper subprocess,
  `transcribe`/`stream` subcommands, following the audio_engine.go/wake_word.go
  Python-subprocess-over-stdio precedent). Added `STTLanguage`/`STTDevice` to
  `packages/config.VoiceConfig` (defaults `en`/`cpu`) plus their
  `JARVIS_VOICE_STT_LANGUAGE`/`JARVIS_VOICE_STT_DEVICE` env overrides and new
  config tests. Self-review caught and fixed a real protocol bug in
  `read_frame()` (a valid zero-length frame was being conflated with
  end-of-stream, which would have truncated live transcription streams
  early) before considering the work done. `go build`/`go vet`/`go test`
  clean via scripts/go_all.ps1 across all 5 workspace modules; `gofmt -l`
  clean on both new Go files. No Python interpreter is installed in this
  environment (only a Windows Store execution-alias stub), so the two
  subprocess-dependent tests only exercised subprocess-start/pipe-error
  behavior before skipping/completing - not a real faster-whisper model
  load - consistent with how SPEC-0053/0054/0055's own Python-dependent
  tests were validated in this same environment. Tracker updated to
  Completed on SPEC-0057; FEATURE_INDEX.md regenerated.
- 2026-08-03 /feature review against docs/agents/CODE_REVIEW_PROTOCOL.md
  (Architecture/Code Quality/Security/Testing) found and fixed one real Code
  Quality/Error Handling gap: both `Transcribe` and `StreamTranscribe` were
  discarding the whisper subprocess's stderr via wake_word.go's bare
  `drainReader` helper, so whisper_stt.py's one truly actionable failure (a
  missing faster-whisper install, reported only via stderr before exit 1)
  reached callers as an opaque "exit status 1". Fixed: `Transcribe` now
  captures stderr into a buffer via `cmd.Stderr` and folds it into the
  returned error on failure; `StreamTranscribe` (no synchronous return path
  to fold a later failure into) now logs each stderr line at Debug level via
  a new `drainStderr` method, matching audio_engine.go's
  `drainCaptureStderr` precedent instead of silently discarding. Added two
  new deterministic tests (`TestWhisperProvider_Transcribe_InvalidPythonPathReturnsError`,
  `TestWhisperProvider_StreamTranscribe_InvalidPythonPathReturnsError`)
  directly covering SPEC-0057's "runtime errors are handled" testing
  criterion without depending on python being installed - the
  requirePython-guarded tests can't reach this path since they skip
  entirely when python is absent. No architecture, scope, or security
  issues found: no new Go dependency (go.mod/go.sum/go.work untouched), no
  upward dependency violations, subprocess argv construction (not shell
  strings) rules out injection, no audio/speech content logged, Container's
  STTProvider slot (SPEC-0056) needed no changes. `go build`/`go vet`/`go
  test` clean via scripts/go_all.ps1 across all 5 workspace modules after
  the fix; `gofmt -l` clean. Verdict: Ready to complete.
- 2026-08-03 User installed a real Python 3.14 (at
  C:\Users\maddy\AppData\Local\Python\bin, shadowed on PATH by a
  non-functional Windows Store execution-alias stub at the same "python"
  name - the same stub every prior skipped test in this session's history
  was actually hitting). Used it to genuinely verify what the previous
  review could only reason about: `pip install numpy faster-whisper`
  succeeded, and running whisper_stt.py directly against real
  Windows-SAPI-synthesized speech ("The quick brown fox jumps over the lazy
  dog") produced accurate transcriptions from both subcommands - `transcribe`
  returned the exact sentence at 0.74 confidence, `stream` (2s segments)
  split it into "the quick brown fox jumps" / "over the lazy dog." across
  two `done:true` chunks - the first real proof of SPEC-0057's "speech
  transcribes accurately" testing criterion. This surfaced one real bug the
  review pass couldn't have caught without a live interpreter: `numpy` was
  imported unguarded (unlike sounddevice/openwakeword in the sibling
  scripts), so a numpy-less environment got a raw traceback instead of the
  established "X is required" stderr convention - fixed by wrapping it in
  the same try/except pattern. With PATH pointed at the real interpreter,
  `TestWhisperProvider_Transcribe_RequiresPython`/
  `TestWhisperProvider_StreamTranscribe_RequiresPython` both genuinely pass
  (no longer skip) once base.en/tiny.en are cached locally - confirming the
  Go<->Python subprocess wiring itself, not just the raw script, works
  end-to-end. `go build`/`go vet`/`go test` clean via scripts/go_all.ps1
  across all 5 workspace modules with the real interpreter on PATH (no
  regressions in audio_engine/wake_word's own Python-dependent tests, which
  still skip correctly since sounddevice/openwakeword weren't installed).
