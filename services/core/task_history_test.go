package core

import (
	"context"
	"testing"
	"time"

	"jarvis-pa/packages/errors"
	types "jarvis-pa/packages/shared-types"
)

func newHistoryTestTask(id string) *types.Task {
	return &types.Task{
		ID:     id,
		Title:  "Test task " + id,
		Source: types.TaskSourceAgent,
		Type:   "test",
		Status: types.TaskStatusQueued,
	}
}

func TestHistoryStore_RecordsEventsPerTask(t *testing.T) {
	bus := NewBus()
	h := NewHistoryStore(bus)
	defer h.Close()

	bus.Publish(types.Event{
		Type:      EventTaskStarted,
		Source:    "core.taskworker",
		Timestamp: time.Now().UTC(),
		Payload:   map[string]any{"taskId": "task-1", "workerId": "worker-1"},
	})

	waitFor(t, time.Second, func() bool { return len(h.History("task-1")) == 1 })

	records := h.History("task-1")
	if records[0].EventType != EventTaskStarted {
		t.Errorf("records[0].EventType = %v, want %v", records[0].EventType, EventTaskStarted)
	}
	if records[0].Payload["workerId"] != "worker-1" {
		t.Errorf("records[0].Payload[workerId] = %v, want worker-1", records[0].Payload["workerId"])
	}
}

func TestHistoryStore_RecordedPayloadIsNotAliasedToPublishedPayload(t *testing.T) {
	bus := NewBus()
	h := NewHistoryStore(bus)
	defer h.Close()

	payload := map[string]any{"taskId": "task-1", "workerId": "worker-1"}
	bus.Publish(types.Event{
		Type:      EventTaskStarted,
		Source:    "test",
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	})
	waitFor(t, time.Second, func() bool { return len(h.History("task-1")) == 1 })

	// Mutate the map the publisher passed to Publish after the fact; the
	// recorded snapshot must not change.
	payload["workerId"] = "worker-2"

	if got := h.History("task-1")[0].Payload["workerId"]; got != "worker-1" {
		t.Errorf("recorded Payload[workerId] = %v, want worker-1 (must not alias the publisher's map)", got)
	}
}

func TestHistoryStore_UnknownTaskReturnsNil(t *testing.T) {
	bus := NewBus()
	h := NewHistoryStore(bus)
	defer h.Close()

	if got := h.History("does-not-exist"); got != nil {
		t.Errorf("History(does-not-exist) = %v, want nil", got)
	}
}

func TestHistoryStore_IgnoresEventsWithoutTaskID(t *testing.T) {
	bus := NewBus()
	h := NewHistoryStore(bus)
	defer h.Close()

	bus.Publish(types.Event{
		Type:      EventTaskStarted,
		Source:    "test",
		Timestamp: time.Now().UTC(),
		Payload:   map[string]any{"workerId": "worker-1"},
	})

	// Publish a second, recordable event and wait for it, so we know the
	// first (unrecordable) event already had its chance to be processed by
	// the same subscription goroutine before we assert on it.
	bus.Publish(types.Event{
		Type:      EventTaskStarted,
		Source:    "test",
		Timestamp: time.Now().UTC(),
		Payload:   map[string]any{"taskId": "task-1"},
	})
	waitFor(t, time.Second, func() bool { return len(h.History("task-1")) == 1 })

	if got := h.History(""); got != nil {
		t.Errorf("History(\"\") = %v, want nil (taskId-less event must not be recorded under an empty key)", got)
	}
}

func TestHistoryStore_ClosePreservesRecordedHistoryButStopsRecording(t *testing.T) {
	bus := NewBus()
	h := NewHistoryStore(bus)

	bus.Publish(types.Event{
		Type:      EventTaskStarted,
		Source:    "test",
		Timestamp: time.Now().UTC(),
		Payload:   map[string]any{"taskId": "task-1"},
	})
	waitFor(t, time.Second, func() bool { return len(h.History("task-1")) == 1 })

	h.Close()

	bus.Publish(types.Event{
		Type:      EventTaskCompleted,
		Source:    "test",
		Timestamp: time.Now().UTC(),
		Payload:   map[string]any{"taskId": "task-1"},
	})
	time.Sleep(20 * time.Millisecond)

	if got := len(h.History("task-1")); got != 1 {
		t.Errorf("History(task-1) after Close = %d records, want 1 (still-recorded event, no further recording)", got)
	}
}

func TestHistoryStore_WithNilBusRecordsNothing(t *testing.T) {
	h := NewHistoryStore(nil)
	defer h.Close()

	if got := h.History("task-1"); got != nil {
		t.Errorf("History(task-1) = %v, want nil", got)
	}
}

func TestHistoryStore_WithHistoryEventTypesOverridesDefaultSet(t *testing.T) {
	bus := NewBus()
	const customEvent types.EventType = "TOOL_EXECUTED"
	h := NewHistoryStore(bus, WithHistoryEventTypes([]types.EventType{customEvent}))
	defer h.Close()

	bus.Publish(types.Event{
		Type:      EventTaskStarted,
		Source:    "test",
		Timestamp: time.Now().UTC(),
		Payload:   map[string]any{"taskId": "task-1"},
	})
	bus.Publish(types.Event{
		Type:      customEvent,
		Source:    "test",
		Timestamp: time.Now().UTC(),
		Payload:   map[string]any{"taskId": "task-1"},
	})
	waitFor(t, time.Second, func() bool { return len(h.History("task-1")) == 1 })

	records := h.History("task-1")
	if records[0].EventType != customEvent {
		t.Errorf("records[0].EventType = %v, want %v (default event types must not be subscribed)", records[0].EventType, customEvent)
	}
}

func TestHistoryStore_IntegrationRecordsFullTimelineForCompletedTask(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	h := NewHistoryStore(bus)
	defer h.Close()

	task := newHistoryTestTask("task-1")
	if err := q.Add(task); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	executor := func(ctx context.Context, task *types.Task) (map[string]any, error) {
		// A small delay ensures EventTaskStarted and EventTaskCompleted get
		// distinguishable Timestamps even on a coarse system clock, so
		// History's chronological sort has something to sort by.
		time.Sleep(5 * time.Millisecond)
		return map[string]any{"ok": true}, nil
	}
	w := NewWorker("worker-1", q, sm, bus, executor, WithPollInterval(time.Millisecond))

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer w.Stop(context.Background())

	waitFor(t, time.Second, func() bool { return len(h.History("task-1")) == 2 })

	records := h.History("task-1")
	if records[0].EventType != EventTaskStarted {
		t.Errorf("records[0].EventType = %v, want %v", records[0].EventType, EventTaskStarted)
	}
	if records[1].EventType != EventTaskCompleted {
		t.Errorf("records[1].EventType = %v, want %v", records[1].EventType, EventTaskCompleted)
	}
	result, ok := records[1].Payload["result"].(map[string]any)
	if !ok || result["ok"] != true {
		t.Errorf("records[1].Payload[result] = %v, want map with ok=true", records[1].Payload["result"])
	}
}

func TestHistoryStore_IntegrationRecordsErrorForFailedTask(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	h := NewHistoryStore(bus)
	defer h.Close()

	task := newHistoryTestTask("task-1")
	if err := q.Add(task); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	executor := func(ctx context.Context, task *types.Task) (map[string]any, error) {
		time.Sleep(5 * time.Millisecond)
		return nil, errors.New(errors.TypeInternal, "BOOM", "test", "executor exploded")
	}
	w := NewWorker("worker-1", q, sm, bus, executor, WithPollInterval(time.Millisecond))

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer w.Stop(context.Background())

	waitFor(t, time.Second, func() bool { return len(h.History("task-1")) == 2 })

	records := h.History("task-1")
	if records[1].EventType != EventTaskFailed {
		t.Errorf("records[1].EventType = %v, want %v", records[1].EventType, EventTaskFailed)
	}
	wantErr := "test: [BOOM] executor exploded"
	if records[1].Payload["error"] != wantErr {
		t.Errorf("records[1].Payload[error] = %v, want %q", records[1].Payload["error"], wantErr)
	}
}

func TestHistoryStore_IntegrationRecordsRetryThenCompletion(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	h := NewHistoryStore(bus)
	defer h.Close()
	rm := NewRetryManager(RetryPolicy{MaxAttempts: 2, Delay: time.Millisecond})

	task := newHistoryTestTask("task-1")
	if err := q.Add(task); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	var calls int
	executor := func(ctx context.Context, task *types.Task) (map[string]any, error) {
		calls++
		if calls < 2 {
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
	// STARTED (attempt 1) -> RETRY_SCHEDULED -> STARTED (attempt 2) -> COMPLETED.
	waitFor(t, time.Second, func() bool { return len(h.History("task-1")) == 4 })

	var sawRetry bool
	for _, r := range h.History("task-1") {
		if r.EventType == EventTaskRetryScheduled {
			sawRetry = true
			wantErr := "test: [BOOM] transient failure"
			if r.Payload["error"] != wantErr {
				t.Errorf("retry record Payload[error] = %v, want %q", r.Payload["error"], wantErr)
			}
		}
	}
	if !sawRetry {
		t.Error("history has no EventTaskRetryScheduled record")
	}
}

func TestHistoryStore_IntegrationRecordsSchedulerLifecycle(t *testing.T) {
	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()
	h := NewHistoryStore(bus)
	defer h.Close()

	s := NewScheduler(q, sm, bus, WithSchedulerTick(2*time.Millisecond))
	factory := newSchedulerTestFactory("hist")
	if err := s.Schedule(ScheduleOnce("sched-1", time.Now().Add(5*time.Millisecond), factory)); err != nil {
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

	waitFor(t, time.Second, func() bool { return len(h.History(task.ID)) == 1 })

	records := h.History(task.ID)
	if records[0].EventType != EventScheduledTaskFired {
		t.Errorf("records[0].EventType = %v, want %v", records[0].EventType, EventScheduledTaskFired)
	}
}
