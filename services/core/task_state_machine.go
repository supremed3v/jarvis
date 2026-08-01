// task_state_machine.go implements SPEC-0012: the Task lifecycle state
// machine. It validates transitions between the TaskStatus values defined
// in packages/shared-types (data shape only) and records each accepted
// transition so a Task's full history can be inspected.
package core

import (
	"fmt"
	"sync"
	"time"

	"jarvis-pa/packages/errors"
	types "jarvis-pa/packages/shared-types"
)

// validTransitions is the closed set of allowed TaskStatus transitions.
// completed, failed, and cancelled are terminal: once reached, a Task
// cannot leave that state.
var validTransitions = map[types.TaskStatus][]types.TaskStatus{
	types.TaskStatusCreated:   {types.TaskStatusPlanning, types.TaskStatusCancelled},
	types.TaskStatusPlanning:  {types.TaskStatusQueued, types.TaskStatusFailed, types.TaskStatusCancelled},
	types.TaskStatusQueued:    {types.TaskStatusExecuting, types.TaskStatusCancelled},
	types.TaskStatusExecuting: {types.TaskStatusCompleted, types.TaskStatusWaiting, types.TaskStatusFailed, types.TaskStatusCancelled},
	types.TaskStatusWaiting:   {types.TaskStatusExecuting, types.TaskStatusFailed, types.TaskStatusCancelled},
	types.TaskStatusCompleted: {},
	types.TaskStatusFailed:    {},
	types.TaskStatusCancelled: {},
}

// CanTransition reports whether a Task may move from "from" to "to"
// directly.
func CanTransition(from, to types.TaskStatus) bool {
	for _, allowed := range validTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// StateMachine validates Task status transitions and records each
// accepted transition per Task. StateMachine's own bookkeeping (History)
// is safe for concurrent use, but Transition mutates the *Task passed to
// it; callers must not call Transition concurrently for the same *Task
// without their own synchronization.
type StateMachine struct {
	mu      sync.Mutex
	history map[string][]types.TaskTransition
}

// NewStateMachine creates a ready-to-use StateMachine with no recorded
// history.
func NewStateMachine() *StateMachine {
	return &StateMachine{history: make(map[string][]types.TaskTransition)}
}

// Transition attempts to move task from its current Status to "to". On
// success it updates task.Status and task.UpdatedAt, records the
// transition in history, and returns the recorded TaskTransition. On an
// invalid transition, task is left unmodified and Transition returns a
// packages/errors error typed TypeInvalidInput.
func (m *StateMachine) Transition(task *types.Task, to types.TaskStatus) (types.TaskTransition, error) {
	from := task.Status
	if !CanTransition(from, to) {
		return types.TaskTransition{}, errors.New(
			errors.TypeInvalidInput,
			"TASK_INVALID_TRANSITION",
			"core.taskstatemachine",
			fmt.Sprintf("invalid task transition from %q to %q", from, to),
		).With("taskId", task.ID).With("from", from).With("to", to)
	}

	now := time.Now().UTC()
	record := types.TaskTransition{TaskID: task.ID, From: from, To: to, Timestamp: now}

	task.Status = to
	task.UpdatedAt = now

	m.mu.Lock()
	m.history[task.ID] = append(m.history[task.ID], record)
	m.mu.Unlock()

	return record, nil
}

// History returns the recorded transitions for taskID in the order they
// were applied. It returns nil if taskID has no recorded transitions.
func (m *StateMachine) History(taskID string) []types.TaskTransition {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]types.TaskTransition(nil), m.history[taskID]...)
}
