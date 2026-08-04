# Current Feature: SPEC-0062 Voice Interruptions And Barge-In

## Status: Complete

## Spec

context/features/SPEC-0062-voice-interruptions-barge-in.md

## Overview

Implement natural conversation interruption handling — "barge-in" — so a user
can speak while JARVIS is still responding, causing JARVIS to stop its current
TTS/playback immediately, capture the new utterance, and continue the
conversation with context intact.

## Requirements (from spec)

1. User interrupting speech output
2. Cancelling active generation
3. Restarting listening mode
4. Maintaining conversation context

## Flow (from spec)

    JARVIS Speaking → User Interrupts → Stop TTS → Capture User Speech → Continue Conversation

## Testing Criteria (from spec)

1. User can interrupt JARVIS
2. TTS stops correctly
3. Context remains intact

## Dependencies (all Completed)

- SPEC-0053 Audio Engine Interface (capture/playback/device discovery)
- SPEC-0054 Microphone Management (continuous capture, wake word fan-out)
- SPEC-0055 Wake Word Detection (detection events on EventBus)
- SPEC-0056 STT Provider Interface (TranscriptionChunk with confidence)
- SPEC-0057 Whisper Integration (concrete STT)
- SPEC-0058 TTS Provider Interface (VoiceOptions, Synthesize/StreamSynthesize)
- SPEC-0059 Piper Integration (concrete TTS)
- SPEC-0060 Voice Session Manager (session lifecycle, batch path)
- SPEC-0061 Voice Streaming Pipeline (streaming path, sentence-by-sentence TTS)

## Key Files

- `services/core/voice/session_manager.go` — SessionManager (primary change target)
- `services/core/voice/audio_engine.go` — AudioEngine (Playback/PlaybackStream may need stop support)
- `services/core/voice/microphone.go` — Microphone (always-on capture, wake word/STT events)
- `services/core/container.go` — core.VoiceEngine/TTSProvider/VoiceSessionManager interfaces

## Architecture Analysis

### Current state

- `SessionManager.processRequest` (batch) and `processRequestStreaming` (streaming)
  both use `context.Background()` — there is no mechanism to cancel an in-flight
  TTS/playback/handler from outside.
- `handleWakeWord` ignores a wake word while a session is already active (logs
  "ignoring" and returns).
- `Microphone` runs continuously — audio capture and wake word detection never
  stop, even while JARVIS is speaking. This means the wake word detector can
  fire during playback if the user says the wake word.
- `AudioEngine.Playback`/`PlaybackStream` are cancellable via ctx (PlaybackStream)
  or blocking (Playback). Neither has an explicit "stop now" method.

### What needs to change

1. **Interruption detection**: `handleWakeWord` must recognize a wake word during
   `SessionStateProcessing` or `SessionStateResponding` as an interruption signal,
   not ignore it.

2. **Cancellation plumbing**: `processRequest`/`processRequestStreaming` need a
   cancellable context that the interruption path can cancel, stopping:
   - The RequestHandler/StreamingRequestHandler (active generation)
   - TTS synthesis (StreamSynthesize/Synthesize)
   - Audio playback (PlaybackStream/Playback)

3. **Session transition**: On interruption, the current session should be ended
   (as interrupted, not failed), and a new session started in listening mode
   to capture the user's new utterance.

4. **Context preservation**: The interrupted session's transcript and partial
   response should be available to the next session's RequestHandler so the
   conversation flow isn't lost.

## Implementation Plan

### 1. Add cancellation plumbing to SessionManager

- Store a `cancelRequest context.CancelFunc` on SessionManager (set when
  processRequest/processRequestStreaming starts, cleared when it ends).
- Both `processRequest` and `processRequestStreaming` derive their ctx from
  a new cancellable context instead of `context.Background()`.

### 2. Add interruption handling to handleWakeWord

- When a wake word arrives during `SessionStateProcessing` or
  `SessionStateResponding`, call the stored `cancelRequest()` to abort the
  current pipeline, wait for it to finish (via a done channel), then start
  a new session.

### 3. Add a new session state and event

- `SessionStateInterrupted` — transitional state for a session being
  interrupted (between the cancel and the new session starting).
- `EventSessionInterrupted` event type — distinct from failed/completed.

### 4. Convert batch Playback to be cancellable

- `AudioEngine.Playback` currently uses `context.Background()`. Change it to
  accept/use a context so it can be killed mid-utterance, or convert the
  batch path to use PlaybackStream (which already takes ctx).

### 5. Maintain conversation context

- Add `InterruptedTurn *ConversationTurn` to SessionManager (transcript +
  partial response of the last interrupted session), accessible via a method
  so the agent layer can incorporate it into the next session's context.

### 6. Tests

- User interrupts during batch playback → TTS stops, new session starts
- User interrupts during streaming playback → pipeline stops, new session starts
- User interrupts during handler (active generation) → handler cancelled
- Context from interrupted session available to next session
- Multiple rapid interruptions don't deadlock or panic
