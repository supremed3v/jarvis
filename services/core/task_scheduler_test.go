package core

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
	types "jarvis-pa/packages/shared-types"
)

// newSchedulerTestFactory returns a TaskFactory that hands out fresh Tasks
// with sequential IDs prefixed by prefix, so repeated firings never collide
// on Queue.Add's duplicate-ID check.
func newSchedulerTestFactory(prefix string) TaskFactory {
	var n int32
	return func() *types.Task {
		id := atomic.AddInt32(&n, 1)
		return &types.Task{
			ID:    fmt.Sprintf("%s-%d", prefix, id),
			Title: "scheduled task",
			Type:  "test",
		}
	}
}

func TestScheduler_OnceFiresAndEnqueuesTask(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	s := NewScheduler(q, sm, bus, WithSchedulerTick(2*time.Millisecond))

	if err := s.Schedule(ScheduleOnce("s1", time.Now().Add(5*time.Millisecond), newSchedulerTestFactory("once"))); err != nil {
		t.Fatalf("Schedule returned error: %v", err)
	}

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer s.Stop(context.Background())

	waitFor(t, time.Second, func() bool { return q.Len() == 1 })

	task, err := q.Next()
	if err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	if task.Status != types.TaskStatusQueued {
		t.Errorf("task.Status = %v, want Queued", task.Status)
	}
	if task.Source != types.TaskSourceScheduled {
		t.Errorf("task.Source = %v, want Scheduled", task.Source)
	}

	// A one-time schedule fires exactly once - give it a few more ticks and
	// confirm nothing further is enqueued.
	time.Sleep(20 * time.Millisecond)
	if got := q.Len(); got != 0 {
		t.Errorf("queue.Len() = %d after one-time firing settled, want 0", got)
	}
}

func TestScheduler_AfterFiresOnceAfterDelay(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	s := NewScheduler(q, sm, bus, WithSchedulerTick(2*time.Millisecond))

	if err := s.Schedule(ScheduleAfter("s1", 10*time.Millisecond, newSchedulerTestFactory("after"))); err != nil {
		t.Fatalf("Schedule returned error: %v", err)
	}

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer s.Stop(context.Background())

	// Not due yet.
	time.Sleep(4 * time.Millisecond)
	if got := q.Len(); got != 0 {
		t.Errorf("queue.Len() = %d before delay elapsed, want 0", got)
	}

	waitFor(t, time.Second, func() bool { return q.Len() == 1 })
}

func TestScheduler_EveryRepeats(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	s := NewScheduler(q, sm, bus, WithSchedulerTick(2*time.Millisecond))

	if err := s.Schedule(ScheduleEvery("s1", 5*time.Millisecond, newSchedulerTestFactory("every"))); err != nil {
		t.Fatalf("Schedule returned error: %v", err)
	}

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer s.Stop(context.Background())

	waitFor(t, time.Second, func() bool { return q.Len() >= 3 })
}

func TestScheduler_CancelStopsFutureFiring(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	s := NewScheduler(q, sm, bus, WithSchedulerTick(2*time.Millisecond))

	if err := s.Schedule(ScheduleEvery("s1", 5*time.Millisecond, newSchedulerTestFactory("cancel"))); err != nil {
		t.Fatalf("Schedule returned error: %v", err)
	}

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer s.Stop(context.Background())

	waitFor(t, time.Second, func() bool { return q.Len() >= 1 })

	if err := s.Cancel("s1"); err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}

	settled := q.Len()
	time.Sleep(30 * time.Millisecond)
	if got := q.Len(); got != settled {
		t.Errorf("queue.Len() = %d after Cancel settled, want unchanged %d", got, settled)
	}
}

func TestScheduler_EmitsScheduledAndFiredEvents(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	s := NewScheduler(q, sm, bus, WithSchedulerTick(2*time.Millisecond))

	scheduledCount, unsubScheduled := subscribeCollector(bus, EventTaskScheduled)
	defer unsubScheduled()
	firedCount, unsubFired := subscribeCollector(bus, EventScheduledTaskFired)
	defer unsubFired()

	if err := s.Schedule(ScheduleAfter("s1", 2*time.Millisecond, newSchedulerTestFactory("events"))); err != nil {
		t.Fatalf("Schedule returned error: %v", err)
	}
	// Bus.Publish delivers asynchronously on the subscriber's own goroutine,
	// so poll rather than asserting the count immediately after Schedule.
	waitFor(t, time.Second, func() bool { return scheduledCount() == 1 })

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer s.Stop(context.Background())

	waitFor(t, time.Second, func() bool { return firedCount() == 1 })
}

func TestScheduler_CancelEmitsCanceledEvent(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	s := NewScheduler(q, sm, bus)

	canceledCount, unsub := subscribeCollector(bus, EventScheduledTaskCanceled)
	defer unsub()

	if err := s.Schedule(ScheduleEvery("s1", time.Hour, newSchedulerTestFactory("noop"))); err != nil {
		t.Fatalf("Schedule returned error: %v", err)
	}
	if err := s.Cancel("s1"); err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}
	waitFor(t, time.Second, func() bool { return canceledCount() == 1 })
}

func TestScheduler_ScheduleValidatesInput(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	s := NewScheduler(q, sm, bus)

	cases := []struct {
		name     string
		schedule Schedule
	}{
		{"empty ID", ScheduleOnce("", time.Now(), newSchedulerTestFactory("x"))},
		{"nil factory", ScheduleOnce("s1", time.Now(), nil)},
		{"non-positive delay", ScheduleAfter("s1", 0, newSchedulerTestFactory("x"))},
		{"non-positive interval", ScheduleEvery("s1", 0, newSchedulerTestFactory("x"))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := s.Schedule(tc.schedule)
			if err == nil {
				t.Fatal("Schedule returned no error")
			}
			if !errors.Is(err, errors.TypeInvalidInput) {
				t.Errorf("error type = %v, want TypeInvalidInput", err)
			}
			if !errors.HasCode(err, "SCHEDULE_INVALID") {
				t.Errorf("missing code SCHEDULE_INVALID: %v", err)
			}
		})
	}
}

func TestScheduler_ScheduleDuplicateIDReturnsError(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	s := NewScheduler(q, sm, bus)

	factory := newSchedulerTestFactory("dup")
	if err := s.Schedule(ScheduleEvery("s1", time.Hour, factory)); err != nil {
		t.Fatalf("first Schedule returned error: %v", err)
	}

	err := s.Schedule(ScheduleEvery("s1", time.Hour, factory))
	if err == nil {
		t.Fatal("second Schedule returned no error")
	}
	if !errors.Is(err, errors.TypeAlreadyExists) {
		t.Errorf("error type = %v, want TypeAlreadyExists", err)
	}
	if !errors.HasCode(err, "SCHEDULE_DUPLICATE_ID") {
		t.Errorf("missing code SCHEDULE_DUPLICATE_ID: %v", err)
	}
}

func TestScheduler_CancelUnknownIDReturnsError(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	s := NewScheduler(q, sm, bus)

	err := s.Cancel("does-not-exist")
	if err == nil {
		t.Fatal("Cancel returned no error")
	}
	if !errors.Is(err, errors.TypeNotFound) {
		t.Errorf("error type = %v, want TypeNotFound", err)
	}
	if !errors.HasCode(err, "SCHEDULE_NOT_FOUND") {
		t.Errorf("missing code SCHEDULE_NOT_FOUND: %v", err)
	}
}

func TestScheduler_StartTwiceReturnsError(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	s := NewScheduler(q, sm, bus)

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("first Start returned error: %v", err)
	}
	defer s.Stop(context.Background())

	err := s.Start(context.Background())
	if err == nil {
		t.Fatal("second Start returned no error")
	}
	if !errors.Is(err, errors.TypeInternal) {
		t.Errorf("error type = %v, want TypeInternal", err)
	}
	if !errors.HasCode(err, "SCHEDULER_ALREADY_STARTED") {
		t.Errorf("missing code SCHEDULER_ALREADY_STARTED: %v", err)
	}
}

func TestScheduler_StopIsIdempotentAndSafeWithoutStart(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	s := NewScheduler(q, sm, bus)

	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("Stop on unstarted scheduler returned error: %v", err)
	}

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("first Stop returned error: %v", err)
	}
	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop returned error: %v", err)
	}
}

func TestScheduler_ScheduleAfterStartIsPickedUpOnNextTick(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	s := NewScheduler(q, sm, bus, WithSchedulerTick(2*time.Millisecond))

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer s.Stop(context.Background())

	// Registering a schedule against an already-running Scheduler must be
	// picked up by the same loop, not require a restart.
	if err := s.Schedule(ScheduleAfter("s1", 5*time.Millisecond, newSchedulerTestFactory("dynamic"))); err != nil {
		t.Fatalf("Schedule returned error: %v", err)
	}

	waitFor(t, time.Second, func() bool { return q.Len() == 1 })
}

func TestScheduler_FailedFiringIsLoggedAndDropsWithoutCrashing(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	s := NewScheduler(q, sm, bus, WithSchedulerTick(2*time.Millisecond), WithSchedulerLogger(logger.New("test")))

	// A buggy factory that always returns the same Task ID: the first
	// firing succeeds, every firing after that fails Queue.Add's
	// duplicate-ID check since the original Task is still sitting in the
	// (unconsumed) queue. The schedule must keep running rather than
	// crashing or wedging.
	factory := func() *types.Task {
		return &types.Task{ID: "collides", Title: "buggy", Type: "test"}
	}
	if err := s.Schedule(ScheduleEvery("s1", 3*time.Millisecond, factory)); err != nil {
		t.Fatalf("Schedule returned error: %v", err)
	}

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer s.Stop(context.Background())

	waitFor(t, time.Second, func() bool { return q.Len() == 1 })

	// Give it several more ticks: only the first firing should ever have
	// made it into the queue.
	time.Sleep(30 * time.Millisecond)
	if got := q.Len(); got != 1 {
		t.Errorf("queue.Len() = %d, want 1 (later firings should fail silently on duplicate ID)", got)
	}
}

func TestScheduler_FiredTaskFlowsThroughToWorkerCompletion(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	s := NewScheduler(q, sm, bus, WithSchedulerTick(2*time.Millisecond))

	var executed int32
	executor := func(ctx context.Context, task *types.Task) (map[string]any, error) {
		atomic.AddInt32(&executed, 1)
		return map[string]any{"ok": true}, nil
	}
	w := NewWorker("worker-1", q, sm, bus, executor, WithPollInterval(time.Millisecond))

	if err := s.Schedule(ScheduleAfter("s1", 2*time.Millisecond, newSchedulerTestFactory("pipeline"))); err != nil {
		t.Fatalf("Schedule returned error: %v", err)
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("scheduler Start returned error: %v", err)
	}
	defer s.Stop(context.Background())
	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("worker Start returned error: %v", err)
	}
	defer w.Stop(context.Background())

	waitFor(t, time.Second, func() bool { return atomic.LoadInt32(&executed) == 1 })
}
