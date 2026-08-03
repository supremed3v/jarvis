// tool_approval.go implements SPEC-0048: the Tool Approval Workflow - the
// human-in-the-loop step the spec's own diagram places between Permission
// Check and Execution (Agent -> Tool Request -> Permission Check -> User
// Approval -> Execution). SPEC-0024's PermissionChecker (agent_permission.go,
// Completed) already calls a configurable ApprovalFunc(ctx, agentID,
// category) (bool, error) whenever a category resolves to
// PermissionApprovalRequired, but no concrete ApprovalFunc existed anywhere
// in the codebase - callers only ever supplied inline test stubs that
// resolve immediately. ApprovalQueue is that missing concrete
// implementation: it turns an approval check into a real pending request a
// human can list and resolve, blocking the requesting call until that
// happens (or until ctx is canceled), which is what makes "sensitive
// operations" (dangerous commands, file deletion, external messages, system
// changes - SPEC-0048's own examples of what ends up ApprovalRequired in the
// PermissionModel) actually wait on a person rather than an inline yes/no
// function.
//
// ApprovalQueue deliberately does not add new permission categories or
// reimplement any part of PermissionChecker/ToolExecutionEngine - it plugs
// into the exact ApprovalFunc seam those already provide, via AsApprovalFunc.
// It also does not implement a concrete UI: apps/desktop is still an empty
// scaffold (no spec has required a concrete implementation there yet, per
// CLAUDE.md), so Resolve/Pending are the surface a future UI, CLI, or voice
// interface calls into - the same "define the mechanism now, let a later
// spec supply the concrete front-end" precedent PermissionChecker's own
// ApprovalFunc already set.
package core

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
)

// ApprovalStatus is the lifecycle state of one ApprovalRequest.
type ApprovalStatus string

const (
	ApprovalPending  ApprovalStatus = "pending"
	ApprovalApproved ApprovalStatus = "approved"
	ApprovalRejected ApprovalStatus = "rejected"
)

// ApprovalRequest is a single SPEC-0048 "User Approval" step instance: one
// agent's attempt to use one PermissionApprovalRequired category, waiting on
// a human decision.
type ApprovalRequest struct {
	ID          string
	AgentID     string
	Category    string
	Status      ApprovalStatus
	RequestedAt time.Time
	ResolvedAt  time.Time
}

// pendingApproval pairs a stored ApprovalRequest with the channel its
// blocked Request call is waiting on.
type pendingApproval struct {
	request  ApprovalRequest
	resultCh chan bool
}

// ApprovalQueue tracks pending SPEC-0048 approval requests and lets a human
// (via Pending/Resolve) grant or deny them. ApprovalQueue is safe for
// concurrent use.
type ApprovalQueue struct {
	mu      sync.Mutex
	nextID  int
	pending map[string]*pendingApproval
	log     *logger.Logger
}

// ApprovalQueueOption configures an ApprovalQueue created by
// NewApprovalQueue.
type ApprovalQueueOption func(*ApprovalQueue)

// WithApprovalQueueLogger attaches a Logger used to record every request,
// approval, rejection, and cancellation. Optional; a queue with no logger
// runs silently.
func WithApprovalQueueLogger(log *logger.Logger) ApprovalQueueOption {
	return func(q *ApprovalQueue) { q.log = log }
}

// NewApprovalQueue creates a ready-to-use, empty ApprovalQueue.
func NewApprovalQueue(opts ...ApprovalQueueOption) *ApprovalQueue {
	q := &ApprovalQueue{pending: make(map[string]*pendingApproval)}
	for _, opt := range opts {
		opt(q)
	}
	return q
}

// Request files a new ApprovalRequest for agentID's use of category and
// blocks until a human calls Resolve for it, or ctx is canceled - the
// SPEC-0048 "User Approval" step. Request's signature matches
// agent_permission.go's ApprovalFunc exactly, so a queue can be wired
// straight into a PermissionChecker via AsApprovalFunc without an adapter,
// the same "signature alignment" precedent Tool.Execute/ToolCaller and
// ExecutionLoop.Run/Executor already set.
func (q *ApprovalQueue) Request(ctx context.Context, agentID, category string) (bool, error) {
	q.mu.Lock()
	q.nextID++
	id := fmt.Sprintf("approval-%d", q.nextID)
	req := ApprovalRequest{
		ID:          id,
		AgentID:     agentID,
		Category:    category,
		Status:      ApprovalPending,
		RequestedAt: time.Now(),
	}
	resultCh := make(chan bool, 1)
	q.pending[id] = &pendingApproval{request: req, resultCh: resultCh}
	q.mu.Unlock()

	q.record(req, "requested")

	select {
	case approved := <-resultCh:
		return approved, nil
	case <-ctx.Done():
		q.mu.Lock()
		delete(q.pending, id)
		q.mu.Unlock()

		errType, _ := ctxErrType(ctx)
		err := errors.Wrap(ctx.Err(), errType, "APPROVAL_REQUEST_CANCELED", "core.toolapproval",
			fmt.Sprintf("approval request %q canceled before resolution", id)).
			With("approvalId", id).With("agentId", agentID).With("category", category)
		q.record(req, "canceled")
		return false, err
	}
}

// Resolve grants or denies the pending ApprovalRequest with the given id,
// unblocking the Request call waiting on it. It returns a packages/errors
// error typed TypeNotFound if id names no currently pending request
// (including one already resolved or canceled).
func (q *ApprovalQueue) Resolve(id string, approved bool) error {
	q.mu.Lock()
	p, ok := q.pending[id]
	if !ok {
		q.mu.Unlock()
		return errors.New(errors.TypeNotFound, "APPROVAL_REQUEST_NOT_FOUND", "core.toolapproval",
			fmt.Sprintf("no pending approval request %q", id)).With("approvalId", id)
	}
	delete(q.pending, id)
	q.mu.Unlock()

	status := ApprovalRejected
	if approved {
		status = ApprovalApproved
	}
	p.request.Status = status
	p.request.ResolvedAt = time.Now()
	q.record(p.request, string(status))

	p.resultCh <- approved
	return nil
}

// Pending returns every currently outstanding ApprovalRequest, ordered by
// ID, for a human-facing surface (future UI/CLI) to list and act on.
func (q *ApprovalQueue) Pending() []ApprovalRequest {
	q.mu.Lock()
	defer q.mu.Unlock()

	ids := make([]string, 0, len(q.pending))
	for id := range q.pending {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]ApprovalRequest, 0, len(ids))
	for _, id := range ids {
		out = append(out, q.pending[id].request)
	}
	return out
}

// AsApprovalFunc adapts q to agent_permission.go's ApprovalFunc type, so it
// can be passed directly to WithApprovalFunc - the concrete wiring point
// between SPEC-0024's PermissionChecker and this spec's human approval
// workflow.
func (q *ApprovalQueue) AsApprovalFunc() ApprovalFunc {
	return q.Request
}

// record logs a single ApprovalRequest state transition. A no-op if no
// Logger is configured.
func (q *ApprovalQueue) record(req ApprovalRequest, event string) {
	if q.log == nil {
		return
	}
	q.log.Info("tool approval", map[string]any{
		"approvalId": req.ID,
		"agentId":    req.AgentID,
		"category":   req.Category,
		"event":      event,
	})
}
