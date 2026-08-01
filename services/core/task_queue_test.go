package core

import (
	"fmt"
	"sync"
	"testing"

	"jarvis-pa/packages/errors"
	types "jarvis-pa/packages/shared-types"
)

func newQueueTestTask(id string, priority types.TaskPriority) *types.Task {
	return &types.Task{
		ID:       id,
		Title:    "Test task " + id,
		Source:   types.TaskSourceAgent,
		Type:     "test",
		Status:   types.TaskStatusQueued,
		Priority: priority,
	}
}

func TestQueue_AddEnqueuesTask(t *testing.T) {
	q := NewQueue()
	task := newQueueTestTask("task-1", PriorityNormal)

	if err := q.Add(task); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if q.Len() != 1 {
		t.Errorf("Len() = %d, want 1", q.Len())
	}
}

func TestQueue_AddRejectsDuplicateTaskID(t *testing.T) {
	q := NewQueue()
	if err := q.Add(newQueueTestTask("task-1", PriorityNormal)); err != nil {
		t.Fatalf("first Add returned error: %v", err)
	}

	err := q.Add(newQueueTestTask("task-1", PriorityHigh))
	if err == nil {
		t.Fatal("second Add with same task ID returned no error")
	}
	if !errors.Is(err, errors.TypeAlreadyExists) {
		t.Errorf("error type = %v, want TypeAlreadyExists", err)
	}
	if !errors.HasCode(err, "TASK_QUEUE_DUPLICATE_TASK") {
		t.Errorf("missing code TASK_QUEUE_DUPLICATE_TASK: %v", err)
	}
	if q.Len() != 1 {
		t.Errorf("Len() = %d, want 1 (duplicate must not be enqueued)", q.Len())
	}
}

func TestQueue_AddRejectsUnknownPriority(t *testing.T) {
	q := NewQueue()
	err := q.Add(newQueueTestTask("task-1", types.TaskPriority("urgent")))
	if err == nil {
		t.Fatal("Add with unknown priority returned no error")
	}
	if !errors.Is(err, errors.TypeInvalidInput) {
		t.Errorf("error type = %v, want TypeInvalidInput", err)
	}
	if !errors.HasCode(err, "TASK_QUEUE_INVALID_PRIORITY") {
		t.Errorf("missing code TASK_QUEUE_INVALID_PRIORITY: %v", err)
	}
	if q.Len() != 0 {
		t.Errorf("Len() = %d, want 0", q.Len())
	}
}

func TestQueue_AddDefaultsUnsetPriorityToNormal(t *testing.T) {
	q := NewQueue()
	low := newQueueTestTask("low", PriorityLow)
	unset := newQueueTestTask("unset", "")

	if err := q.Add(low); err != nil {
		t.Fatalf("Add(low) returned error: %v", err)
	}
	if err := q.Add(unset); err != nil {
		t.Fatalf("Add(unset) returned error: %v", err)
	}

	// Normal outranks Low, so the unset-priority task must come first even
	// though it was queued after the low-priority one.
	task, err := q.Next()
	if err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	if task.ID != "unset" {
		t.Errorf("Next() = %q, want %q (default priority should be Normal)", task.ID, "unset")
	}
}

func TestQueue_PriorityOrderingHighestFirst(t *testing.T) {
	q := NewQueue()
	for _, task := range []*types.Task{
		newQueueTestTask("low", PriorityLow),
		newQueueTestTask("normal", PriorityNormal),
		newQueueTestTask("critical", PriorityCritical),
		newQueueTestTask("high", PriorityHigh),
	} {
		if err := q.Add(task); err != nil {
			t.Fatalf("Add(%s) returned error: %v", task.ID, err)
		}
	}

	want := []string{"critical", "high", "normal", "low"}
	for _, id := range want {
		task, err := q.Next()
		if err != nil {
			t.Fatalf("Next returned error: %v", err)
		}
		if task.ID != id {
			t.Errorf("Next() = %q, want %q", task.ID, id)
		}
	}
}

func TestQueue_FIFOWithinPriorityTier(t *testing.T) {
	q := NewQueue()
	for _, id := range []string{"first", "second", "third"} {
		if err := q.Add(newQueueTestTask(id, PriorityNormal)); err != nil {
			t.Fatalf("Add(%s) returned error: %v", id, err)
		}
	}

	for _, id := range []string{"first", "second", "third"} {
		task, err := q.Next()
		if err != nil {
			t.Fatalf("Next returned error: %v", err)
		}
		if task.ID != id {
			t.Errorf("Next() = %q, want %q", task.ID, id)
		}
	}
}

func TestQueue_NextReturnsErrorWhenEmpty(t *testing.T) {
	q := NewQueue()
	_, err := q.Next()
	if err == nil {
		t.Fatal("Next on empty queue returned no error")
	}
	if !errors.Is(err, errors.TypeNotFound) {
		t.Errorf("error type = %v, want TypeNotFound", err)
	}
	if !errors.HasCode(err, "TASK_QUEUE_EMPTY") {
		t.Errorf("missing code TASK_QUEUE_EMPTY: %v", err)
	}
}

func TestQueue_RemoveDequeuesTask(t *testing.T) {
	q := NewQueue()
	if err := q.Add(newQueueTestTask("task-1", PriorityNormal)); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if err := q.Add(newQueueTestTask("task-2", PriorityNormal)); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	if err := q.Remove("task-1"); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if q.Len() != 1 {
		t.Errorf("Len() = %d, want 1", q.Len())
	}

	task, err := q.Next()
	if err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	if task.ID != "task-2" {
		t.Errorf("Next() = %q, want %q", task.ID, "task-2")
	}
}

func TestQueue_RemoveUnknownTaskReturnsError(t *testing.T) {
	q := NewQueue()
	err := q.Remove("no-such-task")
	if err == nil {
		t.Fatal("Remove on unknown task ID returned no error")
	}
	if !errors.Is(err, errors.TypeNotFound) {
		t.Errorf("error type = %v, want TypeNotFound", err)
	}
	if !errors.HasCode(err, "TASK_QUEUE_TASK_NOT_FOUND") {
		t.Errorf("missing code TASK_QUEUE_TASK_NOT_FOUND: %v", err)
	}
}

func TestQueue_ListReturnsQueueOrderWithoutRemoving(t *testing.T) {
	q := NewQueue()
	if err := q.Add(newQueueTestTask("low", PriorityLow)); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if err := q.Add(newQueueTestTask("critical", PriorityCritical)); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	list := q.List()
	if len(list) != 2 {
		t.Fatalf("len(List()) = %d, want 2", len(list))
	}
	if list[0].ID != "critical" || list[1].ID != "low" {
		t.Errorf("List() = [%q, %q], want [critical, low]", list[0].ID, list[1].ID)
	}
	if q.Len() != 2 {
		t.Errorf("Len() = %d after List, want 2 (List must not remove tasks)", q.Len())
	}
}

func TestQueue_ListEmptyQueueIsEmpty(t *testing.T) {
	q := NewQueue()
	if list := q.List(); len(list) != 0 {
		t.Errorf("List() = %+v, want empty", list)
	}
}

// TestQueue_WorkersReceiveTasksCorrectly simulates several concurrent
// workers draining the queue via Next and verifies every enqueued task is
// delivered to exactly one worker, with no duplicates and none dropped.
func TestQueue_WorkersReceiveTasksCorrectly(t *testing.T) {
	q := NewQueue()
	const taskCount = 200
	for i := 0; i < taskCount; i++ {
		id := fmt.Sprintf("task-%d", i)
		if err := q.Add(newQueueTestTask(id, PriorityNormal)); err != nil {
			t.Fatalf("Add(%s) returned error: %v", id, err)
		}
	}

	const workerCount = 8
	var (
		mu       sync.Mutex
		received = make(map[string]int)
		wg       sync.WaitGroup
	)

	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				task, err := q.Next()
				if err != nil {
					return
				}
				mu.Lock()
				received[task.ID]++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(received) != taskCount {
		t.Fatalf("received %d distinct tasks, want %d", len(received), taskCount)
	}
	for id, count := range received {
		if count != 1 {
			t.Errorf("task %q delivered %d times, want 1", id, count)
		}
	}
	if q.Len() != 0 {
		t.Errorf("Len() = %d after draining, want 0", q.Len())
	}
}
