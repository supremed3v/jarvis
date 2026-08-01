package core

import (
	"testing"
	"time"

	"jarvis-pa/packages/errors"
	types "jarvis-pa/packages/shared-types"
)

func newTestTask(status types.TaskStatus) *types.Task {
	return &types.Task{
		ID:        "task-1",
		Title:     "Test task",
		Source:    types.TaskSourceAgent,
		Type:      "test",
		Status:    status,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
}

func TestCanTransition_ValidTransitionsSucceed(t *testing.T) {
	cases := []struct {
		from, to types.TaskStatus
	}{
		{types.TaskStatusCreated, types.TaskStatusPlanning},
		{types.TaskStatusPlanning, types.TaskStatusQueued},
		{types.TaskStatusQueued, types.TaskStatusExecuting},
		{types.TaskStatusExecuting, types.TaskStatusCompleted},
		{types.TaskStatusExecuting, types.TaskStatusWaiting},
		{types.TaskStatusWaiting, types.TaskStatusExecuting},
		{types.TaskStatusExecuting, types.TaskStatusFailed},
		{types.TaskStatusCreated, types.TaskStatusCancelled},
	}
	for _, c := range cases {
		if !CanTransition(c.from, c.to) {
			t.Errorf("CanTransition(%q, %q) = false, want true", c.from, c.to)
		}
	}
}

func TestCanTransition_InvalidTransitionsFail(t *testing.T) {
	cases := []struct {
		from, to types.TaskStatus
	}{
		{types.TaskStatusCreated, types.TaskStatusExecuting},   // skips planning/queued
		{types.TaskStatusCompleted, types.TaskStatusExecuting}, // terminal
		{types.TaskStatusFailed, types.TaskStatusQueued},       // terminal
		{types.TaskStatusCancelled, types.TaskStatusCreated},   // terminal
		{types.TaskStatusCreated, types.TaskStatusCreated},     // self-transition
		{types.TaskStatusQueued, types.TaskStatusPlanning},     // backward
	}
	for _, c := range cases {
		if CanTransition(c.from, c.to) {
			t.Errorf("CanTransition(%q, %q) = true, want false", c.from, c.to)
		}
	}
}

func TestStateMachine_TransitionUpdatesTaskOnSuccess(t *testing.T) {
	m := NewStateMachine()
	task := newTestTask(types.TaskStatusCreated)
	// Deliberately stale so the assertion below can't pass due to clock
	// resolution happening to return the same instant twice.
	before := task.UpdatedAt.Add(-time.Hour)
	task.UpdatedAt = before

	record, err := m.Transition(task, types.TaskStatusPlanning)
	if err != nil {
		t.Fatalf("Transition returned error: %v", err)
	}
	if task.Status != types.TaskStatusPlanning {
		t.Errorf("task.Status = %q, want %q", task.Status, types.TaskStatusPlanning)
	}
	if !task.UpdatedAt.After(before) {
		t.Errorf("task.UpdatedAt was not advanced")
	}
	if record.From != types.TaskStatusCreated || record.To != types.TaskStatusPlanning {
		t.Errorf("record = %+v, want From=created To=planning", record)
	}
	if record.TaskID != task.ID {
		t.Errorf("record.TaskID = %q, want %q", record.TaskID, task.ID)
	}
}

func TestStateMachine_TransitionRejectsInvalidTransition(t *testing.T) {
	m := NewStateMachine()
	task := newTestTask(types.TaskStatusCreated)
	before := task.UpdatedAt

	_, err := m.Transition(task, types.TaskStatusExecuting)
	if err == nil {
		t.Fatal("Transition returned no error for an invalid transition")
	}
	if !errors.Is(err, errors.TypeInvalidInput) {
		t.Errorf("Transition error type = %v, want TypeInvalidInput", err)
	}
	if !errors.HasCode(err, "TASK_INVALID_TRANSITION") {
		t.Errorf("Transition error missing code TASK_INVALID_TRANSITION: %v", err)
	}
	if task.Status != types.TaskStatusCreated {
		t.Errorf("task.Status changed to %q after rejected transition", task.Status)
	}
	if task.UpdatedAt != before {
		t.Errorf("task.UpdatedAt changed after rejected transition")
	}
}

func TestStateMachine_HistoryIsTracked(t *testing.T) {
	m := NewStateMachine()
	task := newTestTask(types.TaskStatusCreated)

	steps := []types.TaskStatus{
		types.TaskStatusPlanning,
		types.TaskStatusQueued,
		types.TaskStatusExecuting,
		types.TaskStatusCompleted,
	}
	for _, to := range steps {
		if _, err := m.Transition(task, to); err != nil {
			t.Fatalf("Transition(%q) returned error: %v", to, err)
		}
	}

	history := m.History(task.ID)
	if len(history) != len(steps) {
		t.Fatalf("len(History) = %d, want %d", len(history), len(steps))
	}
	want := append([]types.TaskStatus{types.TaskStatusCreated}, steps[:len(steps)-1]...)
	for i, record := range history {
		if record.From != want[i] || record.To != steps[i] {
			t.Errorf("history[%d] = %+v, want From=%q To=%q", i, record, want[i], steps[i])
		}
	}
}

func TestStateMachine_FailedTransitionNotRecordedInHistory(t *testing.T) {
	m := NewStateMachine()
	task := newTestTask(types.TaskStatusCreated)

	if _, err := m.Transition(task, types.TaskStatusExecuting); err == nil {
		t.Fatal("expected invalid transition to fail")
	}

	if history := m.History(task.ID); len(history) != 0 {
		t.Errorf("History = %+v, want empty after only a rejected transition", history)
	}
}

func TestStateMachine_HistoryReturnsCopyNotInternalSlice(t *testing.T) {
	m := NewStateMachine()
	task := newTestTask(types.TaskStatusCreated)
	if _, err := m.Transition(task, types.TaskStatusPlanning); err != nil {
		t.Fatalf("Transition returned error: %v", err)
	}

	history := m.History(task.ID)
	history[0].To = types.TaskStatusCancelled

	fresh := m.History(task.ID)
	if fresh[0].To != types.TaskStatusPlanning {
		t.Errorf("mutating a returned History slice affected internal state: %+v", fresh)
	}
}

func TestStateMachine_HistoryForUnknownTaskIsEmpty(t *testing.T) {
	m := NewStateMachine()
	if history := m.History("no-such-task"); len(history) != 0 {
		t.Errorf("History for unknown task = %+v, want empty", history)
	}
}
