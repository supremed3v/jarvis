package errors

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"testing"
	"time"
)

func TestNewHasNoCause(t *testing.T) {
	e := New(TypeNotFound, "USER_NOT_FOUND", "userservice", "user does not exist")

	if e.Cause != nil {
		t.Fatalf("Cause = %v, want nil", e.Cause)
	}
	if e.Unwrap() != nil {
		t.Fatalf("Unwrap() = %v, want nil", e.Unwrap())
	}
}

func TestWrapPreservesCause(t *testing.T) {
	root := fmt.Errorf("connection refused")
	e := Wrap(root, TypeUnavailable, "DB_UNAVAILABLE", "storage", "could not reach database")

	if e.Cause != root {
		t.Fatalf("Cause = %v, want %v", e.Cause, root)
	}
	if !stderrors.Is(e, root) {
		t.Fatalf("errors.Is(e, root) = false, want true")
	}
}

func TestWrapfFormatsMessage(t *testing.T) {
	root := fmt.Errorf("boom")
	e := Wrapf(root, TypeInternal, "RENDER_FAILED", "ui", "failed to render %s (attempt %d)", "widget", 3)

	want := "failed to render widget (attempt 3)"
	if e.Message != want {
		t.Fatalf("Message = %q, want %q", e.Message, want)
	}
}

func TestErrorStringIncludesCause(t *testing.T) {
	root := fmt.Errorf("disk full")
	e := Wrap(root, TypeInternal, "WRITE_FAILED", "storage", "could not persist file")

	got := e.Error()
	want := "storage: [WRITE_FAILED] could not persist file: disk full"
	if got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestErrorStringWithoutCause(t *testing.T) {
	e := New(TypeInvalidInput, "MISSING_FIELD", "config", "field 'port' is required")

	got := e.Error()
	want := "config: [MISSING_FIELD] field 'port' is required"
	if got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestWithDoesNotMutateOriginal(t *testing.T) {
	base := New(TypeInvalidInput, "BAD_INPUT", "api", "invalid request")
	derived := base.With("field", "email")

	if base.Context != nil {
		t.Fatalf("base.Context = %v, want nil (original must be untouched)", base.Context)
	}
	if derived.Context["field"] != "email" {
		t.Fatalf("derived.Context[field] = %v, want %q", derived.Context["field"], "email")
	}
}

func TestWithChainsMultipleKeys(t *testing.T) {
	e := New(TypeInvalidInput, "BAD_INPUT", "api", "invalid request").
		With("field", "email").
		With("value", "not-an-email")

	if e.Context["field"] != "email" || e.Context["value"] != "not-an-email" {
		t.Fatalf("Context = %v, want both field and value set", e.Context)
	}
}

func TestIsTraversesWrapChain(t *testing.T) {
	inner := New(TypeNotFound, "USER_NOT_FOUND", "userservice", "no such user")
	outer := Wrap(inner, TypeInternal, "LOOKUP_FAILED", "api", "user lookup failed")

	if !Is(outer, TypeNotFound) {
		t.Fatalf("Is(outer, TypeNotFound) = false, want true (should traverse to inner)")
	}
	if Is(outer, TypeUnauthenticated) {
		t.Fatalf("Is(outer, TypeUnauthenticated) = true, want false")
	}
}

func TestHasCodeTraversesWrapChain(t *testing.T) {
	inner := New(TypeNotFound, "USER_NOT_FOUND", "userservice", "no such user")
	outer := Wrap(inner, TypeInternal, "LOOKUP_FAILED", "api", "user lookup failed")

	if !HasCode(outer, "USER_NOT_FOUND") {
		t.Fatalf("HasCode(outer, USER_NOT_FOUND) = false, want true")
	}
	if HasCode(outer, "NOPE") {
		t.Fatalf("HasCode(outer, NOPE) = true, want false")
	}
}

func TestStdlibErrorsAsUnwrapsToError(t *testing.T) {
	inner := New(TypeNotFound, "USER_NOT_FOUND", "userservice", "no such user")
	outer := Wrap(inner, TypeInternal, "LOOKUP_FAILED", "api", "user lookup failed")

	var target *Error
	if !stderrors.As(outer, &target) {
		t.Fatalf("errors.As(outer, &target) = false, want true")
	}
	if target != outer {
		t.Fatalf("errors.As found %v, want the outermost *Error itself (%v)", target, outer)
	}
}

func TestReportFlattensCauseChain(t *testing.T) {
	root := fmt.Errorf("connection reset")
	mid := Wrap(root, TypeUnavailable, "DB_UNAVAILABLE", "storage", "database unreachable")
	top := Wrap(mid, TypeInternal, "SAVE_FAILED", "api", "could not save record").With("recordID", "42")

	report := top.Report()

	if report.Type != TypeInternal || report.Code != "SAVE_FAILED" || report.Component != "api" {
		t.Fatalf("Report top-level fields = %+v, unexpected", report)
	}
	if report.Context["recordID"] != "42" {
		t.Fatalf("Report.Context = %v, want recordID=42", report.Context)
	}
	wantCauses := []string{
		"storage: [DB_UNAVAILABLE] database unreachable: connection reset",
		"connection reset",
	}
	if len(report.Causes) != len(wantCauses) {
		t.Fatalf("Causes = %v, want %v", report.Causes, wantCauses)
	}
	for i := range wantCauses {
		if report.Causes[i] != wantCauses[i] {
			t.Fatalf("Causes[%d] = %q, want %q", i, report.Causes[i], wantCauses[i])
		}
	}
}

func TestReportOmitsEmptyContextAndCauses(t *testing.T) {
	e := New(TypeNotFound, "USER_NOT_FOUND", "userservice", "no such user")

	data, err := json.Marshal(e.Report())
	if err != nil {
		t.Fatalf("json.Marshal(Report) error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if _, present := decoded["context"]; present {
		t.Fatalf("decoded report has 'context' key, want omitted when empty: %s", data)
	}
	if _, present := decoded["causes"]; present {
		t.Fatalf("decoded report has 'causes' key, want omitted when empty: %s", data)
	}
}

func TestReportRoundTripsAsJSON(t *testing.T) {
	e := Wrap(fmt.Errorf("timeout"), TypeTimeout, "REQUEST_TIMEOUT", "http", "request timed out").
		With("url", "https://example.com")

	data, err := json.Marshal(e.Report())
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	var decoded Report
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if decoded.Code != "REQUEST_TIMEOUT" || decoded.Type != TypeTimeout {
		t.Fatalf("decoded = %+v, unexpected", decoded)
	}
	if decoded.Context["url"] != "https://example.com" {
		t.Fatalf("decoded.Context = %v, want url set", decoded.Context)
	}
}

func TestIsAndHasCodeOnNilError(t *testing.T) {
	if Is(nil, TypeInternal) {
		t.Fatalf("Is(nil, TypeInternal) = true, want false")
	}
	if HasCode(nil, "ANYTHING") {
		t.Fatalf("HasCode(nil, ANYTHING) = true, want false")
	}
}

func TestIsAndHasCodeOnPlainError(t *testing.T) {
	plain := fmt.Errorf("plain stdlib error")

	if Is(plain, TypeInternal) {
		t.Fatalf("Is(plain, TypeInternal) = true, want false (no *Error in chain)")
	}
	if HasCode(plain, "ANYTHING") {
		t.Fatalf("HasCode(plain, ANYTHING) = true, want false (no *Error in chain)")
	}
}

func TestIsAndHasCodeThroughStdlibWrapping(t *testing.T) {
	inner := New(TypeNotFound, "USER_NOT_FOUND", "userservice", "no such user")
	// A plain fmt.Errorf %w wrapper sits between the caller and our *Error,
	// exercising the integration boundary where JARVIS errors pass through
	// non-*Error wrapping (e.g. third-party libraries using %w).
	outer := fmt.Errorf("handling request: %w", inner)

	if !Is(outer, TypeNotFound) {
		t.Fatalf("Is(outer, TypeNotFound) = false, want true (should traverse through stdlib wrapper)")
	}
	if !HasCode(outer, "USER_NOT_FOUND") {
		t.Fatalf("HasCode(outer, USER_NOT_FOUND) = false, want true (should traverse through stdlib wrapper)")
	}
}

func TestReportTimestampIsPopulated(t *testing.T) {
	e := New(TypeInternal, "SOMETHING_FAILED", "core", "something failed")

	before := time.Now().UTC().Add(-time.Second)
	report := e.Report()
	after := time.Now().UTC().Add(time.Second)

	if report.Timestamp.Before(before) || report.Timestamp.After(after) {
		t.Fatalf("Report.Timestamp = %v, want between %v and %v", report.Timestamp, before, after)
	}
}

func TestReportOnRootErrorHasNoCauses(t *testing.T) {
	e := New(TypeInternal, "SOMETHING_FAILED", "core", "something failed")

	report := e.Report()
	if report.Causes != nil {
		t.Fatalf("Causes = %v, want nil for a root error with no wrapped cause", report.Causes)
	}
}
