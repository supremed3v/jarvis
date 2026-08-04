# Current Feature: Voice Interface UI

## Working In

apps/desktop renderer (Voice Interface UI, orb/status view)

## Status

In Progress

## Goals

- Render voice states: IDLE, LISTENING, THINKING, SPEAKING, ERROR
- Display voice sessions (visible/listable)
- Display errors properly

## Dependencies

- SPEC-0053 Audio Engine Interface (status: Completed)
- SPEC-0054 Microphone Capture System (status: Completed)
- SPEC-0055 Wake Word Detection (status: Completed)
- SPEC-0056 Speech to Text Provider (status: Completed)
- SPEC-0057 Whisper Integration (status: Completed)
- SPEC-0058 Text to Speech Provider (status: Completed)
- SPEC-0059 Piper TTS Integration (status: Completed)
- SPEC-0060 Voice Session Manager (status: Completed)
- SPEC-0061 Voice Streaming Pipeline (status: Completed)
- SPEC-0062 Voice Interruptions Barge In (status: Completed)
- SPEC-0063 Electron Application Bootstrap (status: Completed)
- SPEC-0064 Desktop IPC Architecture (status: Completed)
- SPEC-0065 Core Runtime Communication Bridge (status: Completed)

Not a dependency: SPEC-0066 (Planned; full chat interface is deferred past MVP
per `docs/execution/JARVIS_MVP_SCOPE.md` Interface section — voice-first MVP).

## Notes

Specification:

context/features/SPEC-0067-voice-interface-ui.md

Dependency resolution source: Implementation Order (Phase 5 Applications/Voice)
+ Dependency Graph (Voice -> Applications) + Requirements inference (display of
voice states requires voice session events; display in the desktop app requires
the Electron bootstrap/IPC/bridge chain).

State sources already exist in core: SessionManager states
`listening`/`processing`/`responding` and lifecycle events
VOICE_SESSION_STARTED/COMPLETED/FAILED/INTERRUPTED
(services/core/voice/session_manager.go). The WS bridge forwards those events
(services/core/ws_bridge.go defaultForwardedEvents) and the desktop IPC layer
already exposes the `jarvis:voice:event` channel via preload
`voiceEvent` (apps/desktop/src/shared/ipc.ts, preload.ts). SPEC-0067's UI states
(IDLE/LISTENING/THINKING/SPEAKING/ERROR) map onto these events.

## History

- 2026-08-04 load loaded SPEC-0067
- 2026-08-04 start branch feature/voice-interface-ui; implementation and tests complete: core now emits VOICE_SESSION_PROCESSING/SPEAKING (session_manager.go, forwarded via ws_bridge.go); new apps/desktop/src/shared/voice.ts model + reducer (16 TS tests); main.ts reduces VOICE_SESSION_* events into a VoiceUiSnapshot pushed on jarvis:voice:event; renderer index.html/renderer.ts render the orb/status UI, session list, and error banner. Verification: scripts/go_all.ps1 clean (5 modules), npm run build clean, npm test 52/52. Tracker: SPEC-0067 marked Completed (docs/agents/JARVIS_BUILD_TRACKER.md). Awaiting review/complete actions.
- 2026-08-04 review verdict: Ready to complete — all goals/architecture/scope/security/error-handling/tests checks pass; const-block alignment corrected per gofmt group rules (pre-existing struct misalignment in HEAD left untouched).
