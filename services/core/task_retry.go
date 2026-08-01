// task_retry.go implements SPEC-0015: the Task Retry System. RetryManager
// tracks, per Task, how many times execution has been attempted and why
// each attempt failed, and decides whether a Worker (SPEC-0014) should
// retry a failed Task or fail it terminally. It builds on the Waiting
// TaskStatus already defined by SPEC-0012's StateMachine - a retried Task
// moves Executing -> Waiting -> (re-queued) -> Executing rather than
// requiring any new transitions - so it needs no changes to StateMachine
// or Queue.
package core

import (
	"sync"
	"time"

	types "jarvis-pa/packages/shared-types"
)

// EventTaskRetryScheduled is published when a failed Task is being retried
// rather than failed terminally (SPEC-0015). types.EventType has no
// hardcoded constants (SPEC-0004: shapes only); RetryManager owns this
// event name, mirroring Worker's precedent for EventTaskStarted /
// EventTaskFailed in task_worker.go.
const EventTaskRetryScheduled types.EventType = "TASK_RETRY_SCHEDULED"

// RetryPolicy configures how a RetryManager retries failed Tasks.
type RetryPolicy struct {
	// MaxAttempts is how many total execution attempts a Task gets before
	// it is failed terminally. A Task that fails on attempt N is retried
	// only if N < MaxAttempts.
	MaxAttempts int
	// Delay is how long a Worker waits before re-queuing a failed Task for
	// another attempt.
	Delay time.Duration
}

// DefaultRetryPolicy is a reasonable default for callers that don't need to
// tune retry behavior: up to 3 attempts, 1 second apart.
var DefaultRetryPolicy = RetryPolicy{MaxAttempts: 3, Delay: time.Second}

// RetryManager tracks retry attempts and failure reasons per Task ID, and
// applies a RetryPolicy to decide whether a failed Task should be retried.
// RetryManager is safe for concurrent use.
type RetryManager struct {
	mu       sync.Mutex
	policy   RetryPolicy
	attempts map[string]int
	reasons  map[string][]string
}

// NewRetryManager creates a ready-to-use RetryManager that applies policy.
func NewRetryManager(policy RetryPolicy) *RetryManager {
	return &RetryManager{
		policy:   policy,
		attempts: make(map[string]int),
		reasons:  make(map[string][]string),
	}
}

// RecordFailure records that taskID's most recent execution attempt failed
// with the given reason. attempt is the 1-based count of attempts recorded
// so far for taskID, including this one. retry reports whether another
// attempt should be made - true iff attempt has not yet reached the
// RetryManager's MaxAttempts - so callers can avoid the infinite retry
// loops SPEC-0015 requires against.
func (r *RetryManager) RecordFailure(taskID, reason string) (attempt int, retry bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.attempts[taskID]++
	r.reasons[taskID] = append(r.reasons[taskID], reason)
	attempt = r.attempts[taskID]
	retry = attempt < r.policy.MaxAttempts
	return attempt, retry
}

// Attempts reports how many failed execution attempts have been recorded
// for taskID so far.
func (r *RetryManager) Attempts(taskID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.attempts[taskID]
}

// FailureReasons returns the recorded failure reasons for taskID, in the
// order they occurred. It returns nil if taskID has no recorded failures.
func (r *RetryManager) FailureReasons(taskID string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.reasons[taskID]...)
}

// Reset clears any recorded attempts and failure reasons for taskID, e.g.
// after it completes successfully.
func (r *RetryManager) Reset(taskID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.attempts, taskID)
	delete(r.reasons, taskID)
}

// Delay returns how long a Worker should wait before re-queuing a failed
// Task for another attempt.
func (r *RetryManager) Delay() time.Duration {
	return r.policy.Delay
}
