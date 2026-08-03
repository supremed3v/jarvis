# Current Feature: SPEC-0061 Voice Streaming Pipeline

## Working In

Voice branch (Phase 4/5, parallel to the still-unstarted Agent Execution
Loop branch) — `services/core/voice` (`session_manager.go` SPEC-0060,
`whisper_provider.go`/`piper_provider.go` SPEC-0057/0059,
`microphone.go`/`audio_engine.go` SPEC-0053/0054/0055) plus
`services/core` (`stt_provider.go`/`tts_provider.go` SPEC-0056/0058,
`llm_provider.go`/`stream_handler.go`, `container.go`'s `VoiceEngine`
interface).

## Status

In Progress

## Goals

- Streaming microphone input
- Streaming transcription
- Streaming responses
- Streaming speech output
- Optimize latency, responsiveness, natural conversation

## Dependencies

- SPEC-0053/0054/0055 Audio Engine / Microphone / Wake Word (status:
  Completed) — `Microphone` already streams captured audio continuously
  over a channel and already drives `STTProvider.StreamTranscribe`
  internally (`microphone.go:98`).
- SPEC-0056/0057 STT Provider / Whisper (status: Completed) —
  `StreamTranscribe` exists and is already exercised by `Microphone`.
- SPEC-0058/0059 TTS Provider / Piper (status: Completed) —
  `StreamSynthesize` exists on `core.TTSProvider` and is implemented by
  `PiperProvider`, but is currently unused: `SessionManager.processRequest`
  calls the blocking whole-buffer `Synthesize`, not `StreamSynthesize`.
- SPEC-0060 Voice Session Manager (status: Completed) — sequences a full
  interaction, but batch-style at both ends: `RequestHandler` is
  `func(ctx, transcript) (string, error)`, returning only once the full
  response text exists (no seam for incremental chunks), and
  `processRequest` waits for `Synthesize`'s complete audio buffer before a
  single `engine.Playback(audio, sampleRate)` call — `core.VoiceEngine`
  (`container.go:14-30`) has no streaming `Playback` variant, only
  `Playback(audio []byte, sampleRate int) error`.
- LLM streaming (`services/core/llm_provider.go`'s `Stream`/`StreamChunk`,
  `stream_handler.go`) already exists and establishes the incremental-chunk
  pattern SPEC-0061's "streaming responses" would reuse — but nothing calls
  `SessionManager.RequestHandler` with a real agent yet. No
  `Communicator`/`ExecutionLoop` exists (SPEC-0022 Agent Execution Loop is
  `Planned`, not started), and `NewSessionManager` itself is only
  constructed in tests today — never wired into a bootstrap/main.

## Notes

Specification: `context/features/SPEC-0061-voice-streaming-pipeline.md`.
Unusually thin spec — just Overview/Requirements/Goals/Testing bullet
lists, no further detail.

Every individual streaming primitive this spec names already exists
somewhere in the codebase. The actual gap is that `SessionManager`'s fixed
sequence (wait for a final transcript -> wait for `RequestHandler`'s full
response -> wait for `Synthesize`'s full audio buffer -> one `Playback`
call) never composes them into a low-latency streaming path end-to-end.
Likely shape, by inspection:

1. Streaming mic input + streaming transcription: already true end-to-end
   (`Microphone`'s continuous capture channel + `StreamTranscribe`) — no
   work needed here unless SPEC-0061 wants `SessionManager` itself to act
   on interim (non-final) transcript chunks, which `handleTranscript`
   currently ignores until `final == true`.
2. Streaming responses: needs `RequestHandler`'s contract widened (or a new
   streaming variant alongside it) so response text can arrive
   incrementally, mirroring `llm_provider.go`'s `Stream`/`StreamChunk`
   shape.
3. Streaming speech output: needs a streaming `Playback` on
   `core.VoiceEngine` (currently whole-buffer only), fed by
   `TTSProvider.StreamSynthesize`'s `audioCh` — likely synthesizing/playing
   per-sentence or per-chunk as response text streams in, rather than
   waiting for the complete response before speaking any of it.
4. Pipeline failure recovery: `SessionManager` already has failure/timeout
   handling for the non-streaming path (`finishSession`, `watchTimeout`); a
   streaming path needs the equivalent for a mid-stream chunk failure.

Since no `Communicator`/agent dispatch exists yet (SPEC-0022 not started),
this spec may end up widening `SessionManager`'s contracts without a real
production caller to prove them against — the same "seam, not full wiring"
position SPEC-0059's Piper->Playback gap was in before SPEC-0060 closed it.

**Decision (2026-08-03, user):** implement in dependency order — real-agent
wiring is out of scope for this spec. Build and test the streaming
plumbing (widened/streaming `RequestHandler` contract, streaming
`Synthesize`/`Playback` consumption, mid-stream failure recovery) against a
fake/stub handler, the same way `session_manager_test.go` already tests
SPEC-0060's non-streaming path. `SessionManager` stays an unwired seam in
production — not connected to a real agent — until SPEC-0022 (Agent
Execution Loop) exists to provide one. Do not pull SPEC-0022 forward to
give this spec a real end-to-end demo.

**Implementation (2026-08-03, start action), on `feature/voice-streaming-pipeline`
off master:**

- `core.VoiceEngine` (`container.go`) gained `PlaybackStream(ctx, audioCh
  <-chan []byte, sampleRate int) error` alongside the existing whole-buffer
  `Playback`, implemented in `AudioEngine`
  (`services/core/voice/audio_engine.go`) by writing each chunk to the same
  one-shot Python playback subprocess's stdin as it arrives instead of
  requiring the full buffer up front, and killing the subprocess promptly
  on `ctx` cancellation.
- `session_manager.go` gained the streaming half of the pipeline: `ResponseChunk`
  (`Text`/`Done`, mirroring `core.StreamChunk`), `StreamingRequestHandler`
  (`func(ctx, transcript, onChunk func(ResponseChunk) error) error`), and
  a `WithStreamingHandler` option. When set, `processRequest` routes through
  the new `processRequestStreaming`: response text arriving via `onChunk` is
  split into complete sentences (`flushCompleteSentences`, on `.`/`!`/`?`/`\n`)
  and handed off over a buffered channel to `speakSentences`, which
  synthesizes+plays each one in turn via `tts.StreamSynthesize` piped into
  `engine.PlaybackStream` (`speakSentence`) - so playback of one sentence
  overlaps with the handler still producing the next one, and speaking the
  first sentence can start before the full response exists. A `speakSentences`
  failure cancels the shared `ctx`, unblocking any pending sentence hand-off
  and the streaming handler itself, so a mid-stream TTS/playback failure
  can't deadlock or continue producing text nobody will speak
  (SPEC-0061's "pipeline failures recover" testing criterion). The
  SPEC-0060 batch path (`RequestHandler`, unchanged) remains the default
  when no streaming handler is configured - existing callers/tests are
  unaffected.
- Streaming mic input / streaming transcription: confirmed already
  complete end-to-end via existing `Microphone`/`StreamTranscribe`
  machinery, per the Notes above - no changes made there, per the decision
  not to expand scope beyond the response/speech-output gap.
- Per the decision above, `streamingHandler` is exercised only by new
  fake/stub-driven tests (`session_manager_test.go`): no real
  agent/Communicator wiring was added, and `SessionManager` still isn't
  constructed anywhere outside tests.
- Tests added: `TestSessionManager_StreamingHandler_SpeaksSentencesAsTheyComplete`
  (multi-fragment scripted handler -> correct per-sentence `StreamSynthesize`
  calls and `PlaybackStream` count, batch handler not invoked),
  `TestSessionManager_StreamingHandler_FailsOnSynthesisError` (mid-stream
  TTS failure -> `EventSessionFailed` with the real error, no hang),
  `TestSessionManager_StreamingHandler_FailsWhenHandlerErrors` (handler
  fails before producing text -> failure event, TTS never invoked), and a
  pure-function `TestFlushCompleteSentences` covering multi-sentence and
  cross-chunk sentence reconstruction. Existing `fakeVoiceEngine`/
  `recordingVoiceEngine`/`fakeTTSProvider` test doubles extended with
  `PlaybackStream`/`StreamSynthesize` implementations to satisfy the widened
  interfaces and to record streamed output the same way their non-streaming
  counterparts already did.
- Verification: `scripts/go_all.ps1 build|vet|test` clean across all 5
  workspace modules (`services/core` and `services/core/voice` both `ok`,
  no regressions in any SPEC-0060 or earlier voice test). `gofmt -l` flags
  every touched file - confirmed via `gofmt -d` to be the known repo-wide
  CRLF-only false positive (documented since SPEC-0008/0027/0053/0058/0059),
  not a real formatting issue in the new code.
- Committed to `feature/voice-streaming-pipeline` (`ad290fb`).

**Review (2026-08-03, `/feature complete`'s own initiated `/code-review medium`
pass, before merging):** 8-angle review found a real, CONFIRMED deadlock:
`speakSentence` could hang forever if `PlaybackStream` returned (e.g. a
stdin write failure) while `tts.StreamSynthesize`'s producer goroutine was
still trying to send more than `streamAudioBufferSize` (8) buffered chunks -
nothing cancelled `ctx` until `speakSentence` returned, which couldn't
happen until that same blocked goroutine unblocked (circular). A second
CONFIRMED issue: `PlaybackStream`'s `stdin.Write` was a plain blocking call
a `select` couldn't preempt, so a hung (not exited) subprocess could ignore
`ctx` cancellation entirely, contradicting the method's own doc comment.

Fixed:
- `speakSentence` now derives a per-sentence child context and cancels it
  immediately after `PlaybackStream` returns (success or failure), before
  waiting on `synthDone` - breaking the deadlock unconditionally, not
  relying on the outer session-level cancellation chain.
- Fixing that surfaced a related precedence bug the deadlock fix's own
  regression test caught: cancelling to unblock the producer makes
  `StreamSynthesize` return `context.Canceled`, which was masking the real
  `playbackErr` (e.g. "device busy"). `speakSentence` now prefers
  `playbackErr` over `synthErr` when both are non-nil.
- `PlaybackStream` switched from plain `exec.Command` + manual
  `select`/`Kill()` to `exec.CommandContext(ctx, ...)`, matching the
  established convention in this same package
  (`whisper_provider.go`/`wake_word.go`) - the runtime's own context
  watcher now kills the subprocess independent of whatever this goroutine
  is doing, fixing the blocking-write gap too.
- `Playback` and `PlaybackStream` now share a `startPlaybackProcess`
  helper instead of duplicating the subprocess-construction boilerplate
  (a 3x-corroborated cleanup finding from the review).
- New regression test:
  `TestSessionManager_StreamingHandler_PlaybackFailureDoesNotDeadlock`
  (fails via `waitForEvent`'s own 2s timeout if the deadlock ever
  regresses, rather than hanging the suite).

Two PLAUSIBLE (not CONFIRMED) findings left as documented, not-yet-fixed
limitations, since neither has a concrete reproduction without a real
`StreamingRequestHandler` (still out of scope per the dependency-order
decision above):
- `sentenceBoundary` treats a bare `\n` as a sentence end, which could
  split a real LLM's markdown-formatted streaming output mid-clause.
- `processRequestStreaming`'s `speakErr`-over-`handlerErr` precedence
  (a different, higher-level precedence than the `speakSentence` one fixed
  above) could in theory report a less-useful "context canceled" reason if
  `speakErr` were ever just a cancellation side effect unrelated to its own
  real cause - not currently reachable, since nothing external cancels
  `processRequestStreaming`'s context today, but worth another look if that
  changes.

Also left as a known, undone architectural gap (not a bug): `speakSentence`
spawns a brand-new playback subprocess per sentence rather than reusing one
long-lived subprocess across a multi-sentence response - partially offsets
this spec's own latency goal, flagged by 2 independent review angles, but
fixing it means widening `core.VoiceEngine`'s contract further (e.g. an
explicit playback-session lifecycle) - a bigger design decision than this
review pass, worth raising with the user as a deliberate follow-up rather
than folding into this fix pass.

Re-verified after fixes: `scripts/go_all.ps1 all` (build+vet+test) clean
across all 5 workspace modules, including the new deadlock regression test.

## History

- 2026-08-03 complete action: SPEC-0060 (Voice Session Manager) marked
  Completed in docs/agents/JARVIS_BUILD_TRACKER.md;
  context/features/FEATURE_INDEX.md regenerated. File reset for next
  `/feature load`.
- 2026-08-03 load action: SPEC-0061 (Voice Streaming Pipeline) loaded as
  current feature, continuing the Voice branch after SPEC-0060.
- 2026-08-03 start action: branch `feature/voice-streaming-pipeline`
  created off master. Implemented streaming responses + streaming speech
  output (`core.VoiceEngine.PlaybackStream`,
  `voice.StreamingRequestHandler`/`WithStreamingHandler`,
  `processRequestStreaming`/`speakSentences`/`speakSentence`/
  `flushCompleteSentences`), built against a fake/stub streaming handler
  per the dependency-order decision above - no real agent wiring.
  `go_all.ps1 build|vet|test` clean across all 5 modules. Uncommitted,
  pending user review.
- 2026-08-03: CLAUDE.md's stale project-status claims (21/182,
  "next is SPEC-0022") corrected on master (`88e7271`), discovered while
  answering "how can we exercise SPEC-0022" - it, and everything through
  SPEC-0060, was already `Completed`. SPEC-0061 work committed to
  `feature/voice-streaming-pipeline` (`ad290fb`).
- 2026-08-03 `/feature complete` (in progress): ran a `/code-review medium`
  pass before merging (repo precedent: every prior spec's tracker entry
  describes a review before being marked Completed). Found and fixed a
  real deadlock plus a related error-masking bug and a ctx-cancellation
  gap - see the Notes section above for full detail. Re-verified clean.
  Fix commit pending; merge to master, tracker update, and
  FEATURE_INDEX.md regeneration not yet done.
