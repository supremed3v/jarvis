// stream_handler.go implements SPEC-0030: the Streaming Response Handler.
// StreamHandler sits directly on top of SPEC-0026's Provider.Stream,
// turning a provider's raw chunk callback into SPEC-0030's four
// requirements - progressive tokens, accumulated partial responses,
// cooperative cancellation, and recoverable failures - without changing
// Provider itself.
package core

import (
	"context"
	"strings"

	"jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
)

// StreamEvent is delivered to the callback passed to StreamHandler.Stream
// for every chunk the underlying Provider produces. Chunk is that chunk as
// the Provider emitted it (the "Token streaming" requirement); Partial is
// the full response text accumulated so far, including Chunk.Text (the
// "Partial responses" requirement), so a caller that only wants the
// running total doesn't have to accumulate chunks itself.
type StreamEvent struct {
	Chunk   StreamChunk
	Partial string
}

// StreamResult is StreamHandler.Stream's return value once a stream stops,
// whether it completed normally, was cancelled, or failed partway through.
// Text is always whatever had been accumulated at the point the stream
// stopped, so a cancelled or failed stream doesn't discard output already
// produced (the "Error recovery" requirement).
type StreamResult struct {
	Model     string
	Text      string
	Cancelled bool
}

// StreamHandler implements SPEC-0030's streaming response handling on top
// of a Provider's raw Stream call.
type StreamHandler struct {
	provider Provider
	log      *logger.Logger
}

// NewStreamHandler creates a StreamHandler that streams through provider.
// log receives one entry when a stream ends, whether it completed,
// was cancelled, or failed; a nil log disables logging.
func NewStreamHandler(provider Provider, log *logger.Logger) *StreamHandler {
	return &StreamHandler{provider: provider, log: log}
}

// Stream runs req against the handler's Provider, invoking onEvent for
// every chunk produced.
//
// Stream observes ctx cancellation independent of whether the underlying
// Provider does so itself: if ctx is already done when Stream is called,
// or becomes done between chunks, Stream stops immediately, marks the
// result Cancelled, and returns a packages/errors TypeCanceled error
// instead of continuing to consume the stream (the "Stream cancellation"
// requirement).
//
// If the Provider's Stream call fails partway through - a network error, a
// mid-stream error chunk, or onEvent itself returning an error - Stream
// still returns the StreamResult accumulated up to that point alongside
// the error, so a caller can recover the partial output rather than losing
// it (the "Error recovery" requirement). StreamHandler holds no state
// across calls, so a failed or cancelled Stream call leaves it safe to
// call again immediately.
func (h *StreamHandler) Stream(ctx context.Context, req GenerateRequest, onEvent func(StreamEvent) error) (StreamResult, error) {
	result := StreamResult{Model: req.Model}

	if err := ctx.Err(); err != nil {
		result.Cancelled = true
		cancelErr := h.cancelErr(err)
		h.logOutcome(req, result, cancelErr)
		return result, cancelErr
	}

	var text strings.Builder
	err := h.provider.Stream(ctx, req, func(c StreamChunk) error {
		text.WriteString(c.Text)
		result.Model = c.Model
		result.Text = text.String()

		if err := onEvent(StreamEvent{Chunk: c, Partial: result.Text}); err != nil {
			return err
		}

		if err := ctx.Err(); err != nil {
			result.Cancelled = true
			return h.cancelErr(err)
		}

		return nil
	})

	// A Provider is only required to "respect ctx cancellation" (SPEC-0026),
	// not to classify the resulting error as TypeCanceled - OllamaProvider,
	// for instance, maps a cancelled in-flight request to TypeUnavailable
	// (mapConnectionError), since url.Error.Timeout() is false for
	// context.Canceled. So cancellation is detected from ctx itself, not
	// from the error's Type, and any resulting error is normalized to
	// TypeCanceled (preserving the original as its cause) so a caller can
	// rely on errors.Is(err, errors.TypeCanceled) regardless of which
	// Provider produced the failure.
	if err != nil && ctx.Err() != nil {
		result.Cancelled = true
		if !errors.Is(err, errors.TypeCanceled) {
			err = errors.Wrap(err, errors.TypeCanceled, "STREAM_CANCELLED", "core.streamhandler",
				"stream was cancelled")
		}
	}

	h.logOutcome(req, result, err)
	return result, err
}

// cancelErr wraps ctx's error (context.Canceled or context.DeadlineExceeded)
// into a packages/errors TypeCanceled error identifying the cancellation as
// SPEC-0030's own, so callers can distinguish it from a Provider failure
// via errors.Is(err, errors.TypeCanceled).
func (h *StreamHandler) cancelErr(cause error) error {
	return errors.Wrap(cause, errors.TypeCanceled, "STREAM_CANCELLED", "core.streamhandler",
		"stream was cancelled")
}

// logOutcome records how a Stream call ended.
func (h *StreamHandler) logOutcome(req GenerateRequest, result StreamResult, err error) {
	if h.log == nil {
		return
	}
	fields := map[string]any{
		"model":     req.Model,
		"cancelled": result.Cancelled,
		"textLen":   len(result.Text),
	}
	if err != nil {
		fields["error"] = err.Error()
		h.log.Error("stream ended with error", fields)
		return
	}
	h.log.Info("stream completed", fields)
}
