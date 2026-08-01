// task_queue.go implements SPEC-0013: the Task Queue. Queue holds Tasks
// awaiting worker pickup, ordered by TaskPriority (CRITICAL first, LOW
// last) and, within a priority tier, by insertion order (FIFO). It builds
// on the Task shape from packages/shared-types (SPEC-0011); nothing here
// enforces TaskStatus transitions - callers combine Queue with
// services/core's StateMachine (SPEC-0012) if they need to keep a Task's
// Status in sync with queue membership.
package core

import (
	"fmt"
	"sync"

	"jarvis-pa/packages/errors"
	types "jarvis-pa/packages/shared-types"
)

// Concrete TaskPriority levels (SPEC-0013). packages/shared-types defines
// TaskPriority as a bare string (SPEC-0004: shapes only); Queue owns the
// closed set of values it understands, mirroring EventBus's precedent for
// EventType constants in eventbus.go.
const (
	PriorityLow      types.TaskPriority = "low"
	PriorityNormal   types.TaskPriority = "normal"
	PriorityHigh     types.TaskPriority = "high"
	PriorityCritical types.TaskPriority = "critical"
)

// DefaultPriority is used for a Task whose Priority is unset when it is
// added to the Queue.
const DefaultPriority = PriorityNormal

// priorityOrder ranks priority tiers from first-served to last-served.
var priorityOrder = []types.TaskPriority{PriorityCritical, PriorityHigh, PriorityNormal, PriorityLow}

func isKnownPriority(p types.TaskPriority) bool {
	for _, known := range priorityOrder {
		if p == known {
			return true
		}
	}
	return false
}

// Queue is an in-memory, priority-ordered holding area for Tasks awaiting
// worker pickup. Queue is safe for concurrent use.
type Queue struct {
	mu    sync.Mutex
	tiers map[types.TaskPriority][]*types.Task
}

// NewQueue creates a ready-to-use, empty Queue.
func NewQueue() *Queue {
	return &Queue{tiers: make(map[types.TaskPriority][]*types.Task)}
}

// Add enqueues task. If task.Priority is unset, task is queued at
// DefaultPriority; task.Priority itself is left unmodified. Add rejects a
// task whose ID is already present in the queue, and a task whose Priority
// is set to a value Queue does not recognize.
func (q *Queue) Add(task *types.Task) error {
	priority := task.Priority
	if priority == "" {
		priority = DefaultPriority
	}
	if !isKnownPriority(priority) {
		return errors.New(errors.TypeInvalidInput, "TASK_QUEUE_INVALID_PRIORITY", "core.taskqueue",
			fmt.Sprintf("unknown task priority %q", priority)).With("taskId", task.ID).With("priority", priority)
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	for _, tasks := range q.tiers {
		for _, t := range tasks {
			if t.ID == task.ID {
				return errors.New(errors.TypeAlreadyExists, "TASK_QUEUE_DUPLICATE_TASK", "core.taskqueue",
					fmt.Sprintf("task %q is already queued", task.ID)).With("taskId", task.ID)
			}
		}
	}

	q.tiers[priority] = append(q.tiers[priority], task)
	return nil
}

// Next removes and returns the highest-priority, longest-waiting Task in
// the queue, for a worker to consume. It returns a packages/errors error
// typed TypeNotFound if the queue is empty.
func (q *Queue) Next() (*types.Task, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, p := range priorityOrder {
		tasks := q.tiers[p]
		if len(tasks) == 0 {
			continue
		}
		task := tasks[0]
		q.tiers[p] = tasks[1:]
		return task, nil
	}
	return nil, errors.New(errors.TypeNotFound, "TASK_QUEUE_EMPTY", "core.taskqueue", "task queue is empty")
}

// Remove removes the queued task with the given ID without returning it
// (e.g. when a Task is cancelled before a worker picks it up). It returns a
// packages/errors error typed TypeNotFound if no queued task has that ID.
func (q *Queue) Remove(taskID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for p, tasks := range q.tiers {
		for i, t := range tasks {
			if t.ID == taskID {
				q.tiers[p] = append(tasks[:i:i], tasks[i+1:]...)
				return nil
			}
		}
	}
	return errors.New(errors.TypeNotFound, "TASK_QUEUE_TASK_NOT_FOUND", "core.taskqueue",
		fmt.Sprintf("no queued task with id %q", taskID)).With("taskId", taskID)
}

// List returns the currently queued Tasks in the order Next would return
// them - highest priority tier first, FIFO within a tier - without removing
// them from the queue. It returns nil if the queue is empty.
func (q *Queue) List() []*types.Task {
	q.mu.Lock()
	defer q.mu.Unlock()

	var out []*types.Task
	for _, p := range priorityOrder {
		out = append(out, q.tiers[p]...)
	}
	return out
}

// Len reports how many Tasks are currently queued.
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	n := 0
	for _, tasks := range q.tiers {
		n += len(tasks)
	}
	return n
}
