// Package errors implements SPEC-0006: consistent backend error handling.
// It defines a closed set of error Types, a component-scoped Error carrying
// a stable Code and free-form Context, wrapping that preserves the full
// cause chain via the standard errors.Is / errors.As contract, and a
// Report format suitable for structured reporting (e.g. as log metadata
// alongside SPEC-0005's logger).
package errors

import (
	"errors"
	"fmt"
	"time"
)

// Error is JARVIS's structured error type. Code identifies the exact
// failure site (e.g. "CONFIG_MISSING_FIELD") and is stable across
// releases; Type classifies it into one of the closed categories in
// type.go for programmatic handling; Context carries arbitrary
// diagnostic key/value pairs; Cause is the wrapped underlying error, if
// any.
type Error struct {
	Type      Type
	Code      string
	Component string
	Message   string
	Context   map[string]any
	Cause     error
}

// New creates a root Error with no wrapped cause.
func New(t Type, code, component, message string) *Error {
	return &Error{Type: t, Code: code, Component: component, Message: message}
}

// Wrap creates an Error around cause, preserving it so errors.Is and
// errors.As can still traverse to it (or anything further down its own
// chain) through Error's Unwrap method.
func Wrap(cause error, t Type, code, component, message string) *Error {
	return &Error{Type: t, Code: code, Component: component, Message: message, Cause: cause}
}

// Wrapf is Wrap with a formatted message.
func Wrapf(cause error, t Type, code, component, format string, args ...any) *Error {
	return Wrap(cause, t, code, component, fmt.Sprintf(format, args...))
}

// Error implements the error interface.
func (e *Error) Error() string {
	msg := fmt.Sprintf("%s: [%s] %s", e.Component, e.Code, e.Message)
	if e.Cause != nil {
		msg += ": " + e.Cause.Error()
	}
	return msg
}

// Unwrap exposes the wrapped cause so the standard errors.Is / errors.As
// functions traverse the full chain.
func (e *Error) Unwrap() error {
	return e.Cause
}

// With returns a copy of e with the given context key/value attached. e
// itself is left untouched, so a shared base error can't be mutated by one
// caller and observed by another.
func (e *Error) With(key string, value any) *Error {
	clone := *e
	clone.Context = make(map[string]any, len(e.Context)+1)
	for k, v := range e.Context {
		clone.Context[k] = v
	}
	clone.Context[key] = value
	return &clone
}

// Is reports whether err is, or wraps, an *Error whose Type equals t.
func Is(err error, t Type) bool {
	for err != nil {
		if e, ok := err.(*Error); ok && e.Type == t {
			return true
		}
		err = errors.Unwrap(err)
	}
	return false
}

// HasCode reports whether err is, or wraps, an *Error whose Code equals
// code.
func HasCode(err error, code string) bool {
	for err != nil {
		if e, ok := err.(*Error); ok && e.Code == code {
			return true
		}
		err = errors.Unwrap(err)
	}
	return false
}

// Report is the structured, JSON-serializable reporting form of an Error
// (SPEC-0006's "reporting format" requirement). Causes flattens the full
// wrapped-error chain into readable strings so a report is self-contained
// even when consumed outside this package.
type Report struct {
	Timestamp time.Time      `json:"timestamp"`
	Type      Type           `json:"type"`
	Code      string         `json:"code"`
	Component string         `json:"component"`
	Message   string         `json:"message"`
	Context   map[string]any `json:"context,omitempty"`
	Causes    []string       `json:"causes,omitempty"`
}

// Report produces the structured reporting form of e.
func (e *Error) Report() Report {
	return Report{
		Timestamp: time.Now().UTC(),
		Type:      e.Type,
		Code:      e.Code,
		Component: e.Component,
		Message:   e.Message,
		Context:   e.Context,
		Causes:    causeChain(e.Cause),
	}
}

// causeChain flattens err's wrap chain (err itself, then everything
// errors.Unwrap reaches) into its component error messages, innermost
// cause last.
func causeChain(err error) []string {
	if err == nil {
		return nil
	}
	var chain []string
	for err != nil {
		chain = append(chain, err.Error())
		err = errors.Unwrap(err)
	}
	return chain
}
