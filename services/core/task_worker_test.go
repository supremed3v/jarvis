package core

import (
	"context"
	"sync"
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
