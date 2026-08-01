package core

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"jarvis-pa/packages/errors"
	types "jarvis-pa/packages/shared-types"
)

func newWorkerTestTask(id string) *types.Task {
	return &types.Task{
		ID:     id,
		Title:  "Test task " + id,
		Source: types.TaskSourceAgent,
		Type:   "test",
		Status: types.TaskStatusQueued,
	}
}

// subscribeCollector subscribes to eventType on b and returns a function
// that reports how many events have been received so far.
func subscribeCollector(b *Bus, eventType types.EventType) (count func() int, unsubscribe func()) {
	var mu sync.Mutex
	var received []types.Event
	unsubscribe = b.Subscribe(eventType, func(event types.Event) {
		mu.Lock()
		received = append(received, event)
		mu.Unlock()
	})
	count = func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(received)
	}
	return count, unsubscribe
}

func TestWorker_ExecutesQueuedTaskAndTransitionsToCompleted(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	task := newWorkerTestTask("task-1")
	if err := q.Add(task); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	executor := func(ctx context.Context, task *types.Task) (map[string]any, error) {
		return map[string]any{"ok": true}, nil
	}
	w := NewWorker("worker-1", q, sm, bus, executor, WithPollInterval(time.Millisecond))

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer w.Stop(context.Background())

	waitFor(t, time.Second, func() bool { return task.Status == types.TaskStatusCompleted })

	if task.Result["ok"] != true {
		t.Errorf("task.Result = %+v, want ok=true", task.Result)
	}
	history := sm.History("task-1")
	if len(history) != 2 || history[0].To != types.TaskStatusExecuting || history[1].To != types.TaskStatusCompleted {
		t.Errorf("history = %+v, want [->executing, ->completed]", history)
	}
}

func TestWorker_ReportsExecutorFailure(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	task := newWorkerTestTask("task-1")
	if err := q.Add(task); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	executor := func(ctx context.Context, task *types.Task) (map[string]any, error) {
		return nil, errors.New(errors.TypeInternal, "BOOM", "test", "executor exploded")
	}
	w := NewWorker("worker-1", q, sm, bus, executor, WithPollInterval(time.Millisecond))

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer w.Stop(context.Background())

	waitFor(t, time.Second, func() bool { return task.Status == types.TaskStatusFailed })

	if task.Error == "" {
		t.Error("task.Error is empty, want the executor's error message recorded")
	}
}

func TestWorker_EmitsStartedAndCompletedEvents(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	task := newWorkerTestTask("task-1")
	if err := q.Add(task); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	startedCount, unsubStarted := subscribeCollector(bus, EventTaskStarted)
	defer unsubStarted()
	completedCount, unsubCompleted := subscribeCollector(bus, EventTaskCompleted)
	defer unsubCompleted()

	executor := func(ctx context.Context, task *types.Task) (map[string]any, error) {
		return nil, nil
	}
	w := NewWorker("worker-1", q, sm, bus, executor, WithPollInterval(time.Millisecond))

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer w.Stop(context.Background())

	waitFor(t, time.Second, func() bool { return startedCount() == 1 && completedCount() == 1 })
}

func TestWorker_EmitsFailedEventOnExecutorError(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	task := newWorkerTestTask("task-1")
	if err := q.Add(task); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	failedCount, unsubFailed := subscribeCollector(bus, EventTaskFailed)
	defer unsubFailed()

	executor := func(ctx context.Context, task *types.Task) (map[string]any, error) {
		return nil, errors.New(errors.TypeInternal, "BOOM", "test", "executor exploded")
	}
	w := NewWorker("worker-1", q, sm, bus, executor, WithPollInterval(time.Millisecond))

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer w.Stop(context.Background())

	waitFor(t, time.Second, func() bool { return failedCount() == 1 })
}

func TestWorker_InvalidTaskStatusFailsWithoutExecuting(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	// A task already in a terminal state has no valid transition to
	// Executing; Queue.Add does not enforce Status, so this can reach a
	// Worker in practice (e.g. a stale requeue).
	task := newWorkerTestTask("task-1")
	task.Status = types.TaskStatusCompleted

	if err := q.Add(task); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	executed := false
	executor := func(ctx context.Context, task *types.Task) (map[string]any, error) {
		executed = true
		return nil, nil
	}
	w := NewWorker("worker-1", q, sm, bus, executor, WithPollInterval(time.Millisecond))

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer w.Stop(context.Background())

	waitFor(t, time.Second, func() bool { return task.Error != "" })

	if executed {
		t.Error("executor was invoked for a task that could not transition to executing")
	}
}

func TestWorker_PollsWhenQueueEmpty(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()

	executor := func(ctx context.Context, task *types.Task) (map[string]any, error) {
		return nil, nil
	}
	w := NewWorker("worker-1", q, sm, bus, executor, WithPollInterval(5*time.Millisecond))

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer w.Stop(context.Background())

	// Give the worker a couple of poll cycles against an empty queue before
	// anything is enqueued.
	time.Sleep(20 * time.Millisecond)

	task := newWorkerTestTask("task-1")
	if err := q.Add(task); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	waitFor(t, time.Second, func() bool { return task.Status == types.TaskStatusCompleted })
}

func TestWorker_StopStopsProcessing(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()

	executor := func(ctx context.Context, task *types.Task) (map[string]any, error) {
		return nil, nil
	}
	w := NewWorker("worker-1", q, sm, bus, executor, WithPollInterval(time.Millisecond))

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if err := w.Stop(context.Background()); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}

	task := newWorkerTestTask("task-1")
	if err := q.Add(task); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if task.Status != types.TaskStatusQueued {
		t.Errorf("task.Status = %v, want still Queued after Stop", task.Status)
	}
}

func TestWorker_StartTwiceReturnsError(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	executor := func(ctx context.Context, task *types.Task) (map[string]any, error) { return nil, nil }
	w := NewWorker("worker-1", q, sm, bus, executor)

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("first Start returned error: %v", err)
	}
	defer w.Stop(context.Background())

	err := w.Start(context.Background())
	if err == nil {
		t.Fatal("second Start returned no error")
	}
	if !errors.Is(err, errors.TypeInternal) {
		t.Errorf("error type = %v, want TypeInternal", err)
	}
	if !errors.HasCode(err, "WORKER_ALREADY_STARTED") {
		t.Errorf("missing code WORKER_ALREADY_STARTED: %v", err)
	}
}

func TestWorker_StopIsIdempotentAndSafeWithoutStart(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	executor := func(ctx context.Context, task *types.Task) (map[string]any, error) { return nil, nil }
	w := NewWorker("worker-1", q, sm, bus, executor)

	if err := w.Stop(context.Background()); err != nil {
		t.Fatalf("Stop on unstarted worker returned error: %v", err)
	}

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if err := w.Stop(context.Background()); err != nil {
		t.Fatalf("first Stop returned error: %v", err)
	}
	if err := w.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop returned error: %v", err)
	}
}

func TestWorker_RetriesFailedTaskUntilItSucceeds(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	rm := NewRetryManager(RetryPolicy{MaxAttempts: 3, Delay: time.Millisecond})
	task := newWorkerTestTask("task-1")
	if err := q.Add(task); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	var calls int32
	executor := func(ctx context.Context, task *types.Task) (map[string]any, error) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			return nil, errors.New(errors.TypeInternal, "BOOM", "test", "transient failure")
		}
		return map[string]any{"ok": true}, nil
	}
	w := NewWorker("worker-1", q, sm, bus, executor, WithPollInterval(time.Millisecond), WithRetryManager(rm))

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer w.Stop(context.Background())

	waitFor(t, time.Second, func() bool { return task.Status == types.TaskStatusCompleted })

	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("executor called %d times, want 3", got)
	}
	if task.Result["ok"] != true {
		t.Errorf("task.Result = %+v, want ok=true", task.Result)
	}
	if got := rm.Attempts("task-1"); got != 0 {
		t.Errorf("RetryManager.Attempts(task-1) after success = %d, want 0 (reset)", got)
	}
}

func TestWorker_FailsTerminallyAfterMaxAttempts(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	rm := NewRetryManager(RetryPolicy{MaxAttempts: 2, Delay: time.Millisecond})
	task := newWorkerTestTask("task-1")
	if err := q.Add(task); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	failedCount, unsubFailed := subscribeCollector(bus, EventTaskFailed)
	defer unsubFailed()
	retryCount, unsubRetry := subscribeCollector(bus, EventTaskRetryScheduled)
	defer unsubRetry()

	var calls int32
	executor := func(ctx context.Context, task *types.Task) (map[string]any, error) {
		atomic.AddInt32(&calls, 1)
		return nil, errors.New(errors.TypeInternal, "BOOM", "test", "permanent failure")
	}
	w := NewWorker("worker-1", q, sm, bus, executor, WithPollInterval(time.Millisecond), WithRetryManager(rm))

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer w.Stop(context.Background())

	waitFor(t, time.Second, func() bool { return task.Status == types.TaskStatusFailed })

	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("executor called %d times, want 2 (MaxAttempts)", got)
	}
	if task.Error == "" {
		t.Error("task.Error is empty, want the final failure reason recorded")
	}
	if got := failedCount(); got != 1 {
		t.Errorf("EventTaskFailed published %d times, want 1", got)
	}
	if got := retryCount(); got != 1 {
		t.Errorf("EventTaskRetryScheduled published %d times, want 1", got)
	}
	reasons := rm.FailureReasons("task-1")
	if len(reasons) != 2 {
		t.Errorf("FailureReasons(task-1) = %v, want 2 recorded reasons", reasons)
	}
}

func TestWorker_WithoutRetryManagerFailsOnFirstError(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	task := newWorkerTestTask("task-1")
	if err := q.Add(task); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	var calls int32
	executor := func(ctx context.Context, task *types.Task) (map[string]any, error) {
		atomic.AddInt32(&calls, 1)
		return nil, errors.New(errors.TypeInternal, "BOOM", "test", "executor exploded")
	}
	w := NewWorker("worker-1", q, sm, bus, executor, WithPollInterval(time.Millisecond))

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer w.Stop(context.Background())

	waitFor(t, time.Second, func() bool { return task.Status == types.TaskStatusFailed })

	time.Sleep(20 * time.Millisecond)
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("executor called %d times, want 1 (no retry manager configured)", got)
	}
}

func TestWorker_StopReturnsTimeoutWhenExecutorBlocks(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	task := newWorkerTestTask("task-1")
	if err := q.Add(task); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	executor := func(ctx context.Context, task *types.Task) (map[string]any, error) {
		close(entered)
		<-release
		return nil, nil
	}
	w := NewWorker("worker-1", q, sm, bus, executor, WithPollInterval(time.Millisecond))

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	<-entered // executor is now blocked inside release

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := w.Stop(ctx)
	if err == nil {
		t.Fatal("Stop should have returned an error while the executor was still blocked")
	}
	if !errors.HasCode(err, "WORKER_STOP_INCOMPLETE") {
		t.Errorf("missing code WORKER_STOP_INCOMPLETE: %v", err)
	}
	if !errors.Is(err, errors.TypeTimeout) {
		t.Errorf("error type = %v, want TypeTimeout", err)
	}

	close(release)
	waitFor(t, time.Second, func() bool { return task.Status == types.TaskStatusCompleted })
}
