# Current Feature: None

## Working In

(none)

## Status

Not Started

## Goals

- (none)

## Dependencies

- (none)

## Notes

No feature currently loaded. Use `/feature load <spec-name>` to begin.

Last completed: SPEC-0061 (Voice Streaming Pipeline) - see
docs/agents/JARVIS_BUILD_TRACKER.md for the full record and
services/core/voice/session_manager.go / services/core/voice/audio_engine.go
for the implementation (streaming responses + streaming speech output,
built against a fake/stub StreamingRequestHandler per the dependency-order
decision - no real agent wiring yet). A pre-merge `/code-review medium`
pass found and fixed a real deadlock (services/core/voice/session_manager.go's
speakSentence) plus a related ctx-cancellation gap
(services/core/voice/audio_engine.go's PlaybackStream) - see the tracker
entry for full detail. Branch feature/voice-streaming-pipeline merged to
master (5a6a430).

Also corrected this session: CLAUDE.md's Project Status section had gone
stale (claimed 21/182 specs Completed with SPEC-0022 "next"; actually
60/182 were Completed through SPEC-0060, including the full Agent/LLM/
Memory/Tools layers) - fixed in `88e7271`, discovered while answering "how
can we exercise SPEC-0022".

Known gap for whoever picks up voice/agent work next: no real agent exists
behind SessionManager's RequestHandler/StreamingRequestHandler yet.
SPEC-0022 (Agent Execution Loop) and SPEC-0025 (Communicator) are both
already Completed, but ExecutionLoop.Run has the same one-call/
one-final-result shape as the batch RequestHandler - nothing yet bridges
core.Provider.Stream's real token-level streaming through the Agent/Tool
orchestration layer into StreamingRequestHandler's incremental-chunk shape.
Also open: speakSentence spawns a new playback subprocess per sentence
rather than reusing one long-lived subprocess per response (a real, if
minor, latency cost); and sentenceBoundary's bare-'\n'-as-sentence-end rule
is unverified against real streamed LLM output.

## History

- 2026-08-03 complete action: SPEC-0061 (Voice Streaming Pipeline) marked
  Completed in docs/agents/JARVIS_BUILD_TRACKER.md;
  context/features/FEATURE_INDEX.md regenerated. File reset for next
  `/feature load`.
