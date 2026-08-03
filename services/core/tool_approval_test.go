package core

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	pkgerrors "jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
)

// TestApprovalQueue_Request_GeneratesPendingRequest verifies SPEC-0048's
// "Approval requests are generated" criterion: calling Request creates a
// visible pending entry before it is resolved.
func TestApprovalQueue_Request_GeneratesPendingRequest(t *testing.T) {
	q := NewApprovalQueue()

	done := make(chan struct{})
	go func() {
		q.Request(context.Background(), "developer_agent", "terminal")
		close(done)
	}()

	var pending []ApprovalRequest
	for i := 0; i < 100; i++ {
		pending = q.Pending()
		if len(pending) == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	if len(pending) != 1 {
		t.Fatalf("Pending() = %d requests, want 1", len(pending))
	}
	got := pending[0]
	if got.AgentID != "developer_agent" || got.Category != "terminal" {
		t.Errorf("pending request = %+v, want agent=developer_agent category=terminal", got)
	}
	if got.Status != ApprovalPending {
		t.Errorf("Status = %v, want pending", got.Status)
	}

	if err := q.Resolve(got.ID, true); err != nil {
		t.Fatalf("Resolve() = %v, want nil", err)
	}
	<-done
}

// TestApprovalQueue_Resolve_ApprovedActionsExecute verifies SPEC-0048's
// "Approved actions execute" criterion: Resolve(id, true) unblocks Request
// with true.
func TestApprovalQueue_Resolve_ApprovedActionsExecute(t *testing.T) {
	q := NewApprovalQueue()

	type result struct {
		approved bool
		err      error
	}
	resultCh := make(chan result, 1)
	go func() {
		approved, err := q.Request(context.Background(), "developer_agent", "terminal")
		resultCh <- result{approved, err}
	}()

	id := waitForPending(t, q)
	if err := q.Resolve(id, true); err != nil {
		t.Fatalf("Resolve() = %v, want nil", err)
	}

	res := <-resultCh
	if res.err != nil {
		t.Fatalf("Request() error = %v, want nil", res.err)
	}
	if !res.approved {
		t.Error("Request() approved = false, want true")
	}
}

// TestApprovalQueue_Resolve_RejectedActionsStop verifies SPEC-0048's
// "Rejected actions stop" criterion: Resolve(id, false) unblocks Request
// with false.
func TestApprovalQueue_Resolve_RejectedActionsStop(t *testing.T) {
	q := NewApprovalQueue()

	type result struct {
		approved bool
		err      error
	}
	resultCh := make(chan result, 1)
	go func() {
		approved, err := q.Request(context.Background(), "research_agent", "browser.search")
		resultCh <- result{approved, err}
	}()

	id := waitForPending(t, q)
	if err := q.Resolve(id, false); err != nil {
		t.Fatalf("Resolve() = %v, want nil", err)
	}

	res := <-resultCh
	if res.err != nil {
		t.Fatalf("Request() error = %v, want nil", res.err)
	}
	if res.approved {
		t.Error("Request() approved = true, want false")
	}
}

// TestApprovalQueue_Resolve_UnknownIDFailsNotFound verifies resolving an ID
// that was never requested (or already resolved) reports TypeNotFound
// rather than silently succeeding.
func TestApprovalQueue_Resolve_UnknownIDFailsNotFound(t *testing.T) {
	q := NewApprovalQueue()

	err := q.Resolve("no-such-id", true)
	if !pkgerrors.HasCode(err, "APPROVAL_REQUEST_NOT_FOUND") {
		t.Errorf("missing code APPROVAL_REQUEST_NOT_FOUND: %v", err)
	}
	if !pkgerrors.Is(err, pkgerrors.TypeNotFound) {
		t.Errorf("Type = %v, want TypeNotFound", err)
	}
}

// TestApprovalQueue_Resolve_DoubleResolveFailsSecondTime verifies a request
// can only be resolved once - a second Resolve call for the same ID reports
// not-found rather than re-delivering the outcome.
func TestApprovalQueue_Resolve_DoubleResolveFailsSecondTime(t *testing.T) {
	q := NewApprovalQueue()

	go q.Request(context.Background(), "developer_agent", "terminal")
	id := waitForPending(t, q)

	if err := q.Resolve(id, true); err != nil {
		t.Fatalf("first Resolve() = %v, want nil", err)
	}
	if err := q.Resolve(id, true); !pkgerrors.HasCode(err, "APPROVAL_REQUEST_NOT_FOUND") {
		t.Errorf("second Resolve() missing code APPROVAL_REQUEST_NOT_FOUND: %v", err)
	}
}

// TestApprovalQueue_Request_ContextCanceledStopsWaiting verifies a Request
// call respects ctx cancellation instead of blocking forever, and that the
// canceled request no longer appears as pending.
func TestApprovalQueue_Request_ContextCanceledStopsWaiting(t *testing.T) {
	q := NewApprovalQueue()

	ctx, cancel := context.WithCancel(context.Background())
	type result struct {
		approved bool
		err      error
	}
	resultCh := make(chan result, 1)
	go func() {
		approved, err := q.Request(ctx, "developer_agent", "terminal")
		resultCh <- result{approved, err}
	}()

	id := waitForPending(t, q)
	cancel()

	res := <-resultCh
	if res.err == nil {
		t.Fatal("Request() error = nil, want context-canceled error")
	}
	if !pkgerrors.HasCode(res.err, "APPROVAL_REQUEST_CANCELED") {
		t.Errorf("missing code APPROVAL_REQUEST_CANCELED: %v", res.err)
	}

	if err := q.Resolve(id, true); !pkgerrors.HasCode(err, "APPROVAL_REQUEST_NOT_FOUND") {
		t.Errorf("Resolve() after cancellation missing code APPROVAL_REQUEST_NOT_FOUND: %v", err)
	}
}

// TestApprovalQueue_LogsRequestAndResolution verifies every stage (request,
// approval, rejection) is logged, mirroring SPEC-0024's own
// "permission checks are logged" precedent for the human-approval side of
// the flow.
func TestApprovalQueue_LogsRequestAndResolution(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New("test", logger.WithOutput(&buf))
	q := NewApprovalQueue(WithApprovalQueueLogger(log))

	go q.Request(context.Background(), "developer_agent", "terminal")
	id := waitForPending(t, q)
	if err := q.Resolve(id, true); err != nil {
		t.Fatalf("Resolve() = %v, want nil", err)
	}

	out := buf.String()
	for _, want := range []string{`"event":"requested"`, `"event":"approved"`} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %s; got %s", want, out)
		}
	}
}

// TestApprovalQueue_AsApprovalFunc_IntegratesWithPermissionChecker verifies
// AsApprovalFunc wires directly into SPEC-0024's PermissionChecker: an
// approved request lets Check succeed and a rejected request denies with
// AGENT_PERMISSION_DENIED, proving the SPEC-0048 workflow (Tool Request ->
// Permission Check -> User Approval -> Execution) works end to end through
// the real ApprovalFunc seam rather than only in isolation.
func TestApprovalQueue_AsApprovalFunc_IntegratesWithPermissionChecker(t *testing.T) {
	model := PermissionModel{"developer_agent": AgentPermissions{"terminal": PermissionApprovalRequired}}

	t.Run("approved", func(t *testing.T) {
		q := NewApprovalQueue()
		checker := NewPermissionChecker(model, WithApprovalFunc(q.AsApprovalFunc()))

		errCh := make(chan error, 1)
		go func() {
			errCh <- checker.Check(context.Background(), "developer_agent", "terminal")
		}()

		id := waitForPending(t, q)
		if err := q.Resolve(id, true); err != nil {
			t.Fatalf("Resolve() = %v, want nil", err)
		}
		if err := <-errCh; err != nil {
			t.Errorf("Check() = %v, want nil (approved actions execute)", err)
		}
	})

	t.Run("rejected", func(t *testing.T) {
		q := NewApprovalQueue()
		checker := NewPermissionChecker(model, WithApprovalFunc(q.AsApprovalFunc()))

		errCh := make(chan error, 1)
		go func() {
			errCh <- checker.Check(context.Background(), "developer_agent", "terminal")
		}()

		id := waitForPending(t, q)
		if err := q.Resolve(id, false); err != nil {
			t.Fatalf("Resolve() = %v, want nil", err)
		}
		err := <-errCh
		if !pkgerrors.HasCode(err, "AGENT_PERMISSION_DENIED") {
			t.Errorf("missing code AGENT_PERMISSION_DENIED (rejected actions stop): %v", err)
		}
	})
}

// TestApprovalQueue_IntegratesWithToolExecutionEngine verifies the full
// SPEC-0048 workflow diagram end to end - Agent -> Tool Request -> Permission
// Check -> User Approval -> Execution - through the real SPEC-0046
// ToolExecutionEngine and SPEC-0045 ToolRegistry, not just PermissionChecker
// in isolation: a tool requiring an ApprovalRequired category only runs after
// ApprovalQueue.Resolve grants it, and never runs if rejected.
func TestApprovalQueue_IntegratesWithToolExecutionEngine(t *testing.T) {
	tool := &stubTool{metadata: ToolMetadata{
		ID:          "delete-file",
		Name:        "Delete File",
		InputSchema: Schema{{Name: "path", Type: "string", Required: true}},
		Permissions: []string{"filesystem.delete"},
	}}
	model := PermissionModel{"developer_agent": AgentPermissions{"filesystem.delete": PermissionApprovalRequired}}

	t.Run("approved request executes the tool", func(t *testing.T) {
		registry := newExecutionEngineRegistry(t, tool)
		q := NewApprovalQueue()
		checker := NewPermissionChecker(model, WithApprovalFunc(q.AsApprovalFunc()))
		engine, err := NewToolExecutionEngine(registry, WithExecutionPermissionChecker(checker))
		if err != nil {
			t.Fatalf("NewToolExecutionEngine returned error: %v", err)
		}

		type execResult struct {
			out map[string]any
			err error
		}
		resultCh := make(chan execResult, 1)
		go func() {
			out, err := engine.Execute(context.Background(), "developer_agent", "delete-file", map[string]any{"path": "/tmp/x", "payload": "ok"})
			resultCh <- execResult{out, err}
		}()

		id := waitForPending(t, q)
		if err := q.Resolve(id, true); err != nil {
			t.Fatalf("Resolve() = %v, want nil", err)
		}

		res := <-resultCh
		if res.err != nil {
			t.Fatalf("Execute() = %v, want nil (approved tool executes)", res.err)
		}
		if res.out["echo"] != "ok" {
			t.Errorf("Execute output = %#v, want echo=ok", res.out)
		}
	})

	t.Run("rejected request stops the tool from executing", func(t *testing.T) {
		registry := newExecutionEngineRegistry(t, tool)
		q := NewApprovalQueue()
		checker := NewPermissionChecker(model, WithApprovalFunc(q.AsApprovalFunc()))
		engine, err := NewToolExecutionEngine(registry, WithExecutionPermissionChecker(checker))
		if err != nil {
			t.Fatalf("NewToolExecutionEngine returned error: %v", err)
		}

		type execResult struct {
			out map[string]any
			err error
		}
		resultCh := make(chan execResult, 1)
		go func() {
			out, err := engine.Execute(context.Background(), "developer_agent", "delete-file", map[string]any{"path": "/tmp/x"})
			resultCh <- execResult{out, err}
		}()

		id := waitForPending(t, q)
		if err := q.Resolve(id, false); err != nil {
			t.Fatalf("Resolve() = %v, want nil", err)
		}

		res := <-resultCh
		if !pkgerrors.HasCode(res.err, "AGENT_PERMISSION_DENIED") {
			t.Errorf("missing code AGENT_PERMISSION_DENIED (rejected tool must not execute): %v", res.err)
		}
		if res.out != nil {
			t.Errorf("Execute output = %#v, want nil on rejection", res.out)
		}
	})
}

// TestApprovalQueue_ConcurrentRequests verifies multiple simultaneous
// Request calls are tracked independently and can be resolved individually.
func TestApprovalQueue_ConcurrentRequests(t *testing.T) {
	q := NewApprovalQueue()

	const n = 10
	var wg sync.WaitGroup
	results := make([]bool, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			approved, err := q.Request(context.Background(), "developer_agent", "terminal")
			if err != nil {
				t.Errorf("Request() error = %v, want nil", err)
			}
			results[i] = approved
		}(i)
	}

	var ids []string
	for i := 0; i < 100 && len(ids) < n; i++ {
		ids = nil
		for _, req := range q.Pending() {
			ids = append(ids, req.ID)
		}
		if len(ids) < n {
			time.Sleep(time.Millisecond)
		}
	}
	if len(ids) != n {
		t.Fatalf("Pending() = %d requests, want %d", len(ids), n)
	}

	for _, id := range ids {
		if err := q.Resolve(id, true); err != nil {
			t.Errorf("Resolve(%q) = %v, want nil", id, err)
		}
	}
	wg.Wait()

	for i, approved := range results {
		if !approved {
			t.Errorf("results[%d] = false, want true", i)
		}
	}
}

// waitForPending blocks until exactly one ApprovalRequest is pending on q
// and returns its ID, failing the test if none appears in time.
func waitForPending(t *testing.T, q *ApprovalQueue) string {
	t.Helper()
	for i := 0; i < 100; i++ {
		pending := q.Pending()
		if len(pending) == 1 {
			return pending[0].ID
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("no pending approval request appeared in time")
	return ""
}
