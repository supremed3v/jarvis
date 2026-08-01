# Current Feature

## Overview
Not started — no feature currently loaded. Next candidate per docs/execution/JARVIS_IMPLEMENTATION_ORDER.md: SPEC-0031 (Prompt Template System), the next spec in Phase 4 Intelligence's LLM branch now that SPEC-0030 (Streaming Response Handler) is Completed.

## Status
Not Started

## Goals
_None yet._

## Files Modified
_None yet._

## Notes
SPEC-0030 (Streaming Response Handler) is now Completed (see docs/agents/JARVIS_BUILD_TRACKER.md, SPEC-0030 row, for full implementation/review rationale). SPEC-0031 (Prompt Template System) is unblocked and can consume `services/core`'s `Provider`/`StreamHandler` (SPEC-0026/0027/0030) and `ModelRouter` (SPEC-0029) to render prompts before generating or streaming a response.

## History

- 2026-08-02 SPEC-0030 Streaming Response Handler — Completed. Added `services/core/stream_handler.go`: `StreamHandler.Stream(ctx, GenerateRequest, onEvent func(StreamEvent) error) (StreamResult, error)` sits on top of `Provider.Stream` (SPEC-0026/0027) without modifying `Provider`, `OllamaProvider`, or `ModelRouter` (SPEC-0029), and deliberately does not call `ModelRouter.Route` itself — routing and streaming stay separate, caller-composed concerns. `onEvent` fires once per `StreamChunk` (token streaming) alongside the full text accumulated so far (`StreamEvent.Partial`, partial responses). Cancellation is checked via `ctx.Err()` before starting and after every chunk, independent of whether the underlying Provider itself observes `ctx`, returning a `packages/errors` `TypeCanceled`/`STREAM_CANCELLED` error and marking `StreamResult.Cancelled`. `StreamResult.Text` always holds whatever was accumulated before a failure or cancellation rather than discarding it, and the handler holds no state across calls, so it's safe to retry immediately (error recovery). Every call logs its outcome (model, cancelled, accumulated text length — not the text itself) via an optional `packages/logger.Logger`. `Container` gained the same real-slot treatment every LLM-branch service gets as its spec completes: `StreamHandler *StreamHandler` + `WithStreamHandler` in `container.go`/`container_test.go`. Review pass found and fixed one real Code Quality bug: cancellation was originally detected only via `errors.Is(err, errors.TypeCanceled)` on the Provider's returned error, which relied on `StreamHandler`'s own per-chunk check producing that error first — but `OllamaProvider.mapConnectionError` classifies a cancelled in-flight HTTP request as `TypeUnavailable`/`OLLAMA_CONNECTION_FAILED` (`url.Error.Timeout()` is `false` for `context.Canceled`), so a real race where Ollama's own network layer notices cancellation first would have misreported a genuine cancellation as a plain failure. Fixed by checking `ctx.Err()` directly instead of the error's `Type`, normalizing any resulting error to `TypeCanceled` (wrapping the original as cause); regression test `TestStreamHandler_CancellationDetectedEvenWhenProviderMisclassifiesError` uses a `raceCancelProvider` stub mirroring Ollama's real misclassification. `go build ./...`, `go vet ./...`, `go test ./... -v` clean across all 5 go.work modules (9 new tests in `stream_handler_test.go`: SPEC-0030's three explicit testing criteria plus partial-response accumulation, already-cancelled-context, onEvent-error propagation, and outcome logging; 10x rerun clean). Built on feature/streaming-response-handler (off master, post SPEC-0029 merge).

Earlier entries (SPEC-0001 through SPEC-0029): see docs/agents/JARVIS_BUILD_TRACKER.md and `git log` — this file's History section is reset on each feature completion rather than accumulated indefinitely.
