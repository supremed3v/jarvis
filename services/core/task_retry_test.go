package core

import (
	"testing"
	"time"
)

func TestRetryManager_RecordFailureRetriesUntilMaxAttempts(t *testing.T) {
	rm := NewRetryManager(RetryPolicy{MaxAttempts: 3, Delay: time.Millisecond})

	attempt, retry := rm.RecordFailure("task-1", "boom 1")
	if attempt != 1 || !retry {
		t.Errorf("1st failure: attempt=%d retry=%v, want attempt=1 retry=true", attempt, retry)
	}

	attempt, retry = rm.RecordFailure("task-1", "boom 2")
	if attempt != 2 || !retry {
		t.Errorf("2nd failure: attempt=%d retry=%v, want attempt=2 retry=true", attempt, retry)
	}

	attempt, retry = rm.RecordFailure("task-1", "boom 3")
	if attempt != 3 || retry {
		t.Errorf("3rd failure: attempt=%d retry=%v, want attempt=3 retry=false", attempt, retry)
	}
}

func TestRetryManager_TracksAttemptsAndReasonsPerTask(t *testing.T) {
	rm := NewRetryManager(RetryPolicy{MaxAttempts: 5, Delay: time.Millisecond})

	rm.RecordFailure("task-1", "reason A")
	rm.RecordFailure("task-1", "reason B")
	rm.RecordFailure("task-2", "reason C")

	if got := rm.Attempts("task-1"); got != 2 {
		t.Errorf("Attempts(task-1) = %d, want 2", got)
	}
	if got := rm.Attempts("task-2"); got != 1 {
		t.Errorf("Attempts(task-2) = %d, want 1", got)
	}
	if got := rm.Attempts("task-3"); got != 0 {
		t.Errorf("Attempts(task-3) = %d, want 0", got)
	}

	reasons := rm.FailureReasons("task-1")
	if len(reasons) != 2 || reasons[0] != "reason A" || reasons[1] != "reason B" {
		t.Errorf("FailureReasons(task-1) = %v, want [reason A reason B]", reasons)
	}
}

func TestRetryManager_ResetClearsAttemptsAndReasons(t *testing.T) {
	rm := NewRetryManager(RetryPolicy{MaxAttempts: 3, Delay: time.Millisecond})

	rm.RecordFailure("task-1", "boom")
	rm.Reset("task-1")

	if got := rm.Attempts("task-1"); got != 0 {
		t.Errorf("Attempts(task-1) after Reset = %d, want 0", got)
	}
	if reasons := rm.FailureReasons("task-1"); reasons != nil {
		t.Errorf("FailureReasons(task-1) after Reset = %v, want nil", reasons)
	}

	// A fresh failure after Reset starts back at attempt 1.
	attempt, retry := rm.RecordFailure("task-1", "boom again")
	if attempt != 1 || !retry {
		t.Errorf("post-reset failure: attempt=%d retry=%v, want attempt=1 retry=true", attempt, retry)
	}
}

func TestRetryManager_Delay(t *testing.T) {
	rm := NewRetryManager(RetryPolicy{MaxAttempts: 3, Delay: 42 * time.Millisecond})
	if got := rm.Delay(); got != 42*time.Millisecond {
		t.Errorf("Delay() = %v, want 42ms", got)
	}
}
