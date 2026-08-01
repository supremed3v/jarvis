// task_worker.go implements SPEC-0014: the Task Worker System. A Worker
// pulls Tasks from a Queue (SPEC-0013), executes them via a caller-supplied
// Executor, drives status transitions through a StateMachine (SPEC-0012),
// and publishes lifecycle events on an EventBus (SPEC-0009).
package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	"jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
	types "jarvis-pa/packages/shared-types"
)

// Concrete task lifecycle event types a Worker publishes (SPEC-0014).
// packages/shared-types defines EventType as a bare string (SPEC-0004:
// shapes only); Worker owns this closed set, mirroring Queue's precedent
// for TaskPriority constants in task_queue.go. EventTaskCompleted is
// already defined in eventbus.go and is reused here rather than
// redeclared.
const (
	EventTaskStarted types.EventType = "TASK_STARTED"
	EventTaskFailed  types.EventType = "TASK_FAILED"
)

// defaultPollInterval is how long a Worker waits before checking an empty
// Queue again.
const defaultPollInterval = 20 * time.Millisecond

// Executor performs the actual work a Task describes. It returns the
// Task's result payload on success, or an error if execution failed.
// Executor must respect ctx cancellation.
type Executor func(ctx context.Context, task *types.Task) (map[string]any, error)

// Worker repeatedly pulls Tasks from a Queue and runs them through an
// Executor, keeping Task status and published events in sync with the
// outcome. A Worker is not safe to Start more than once concurrently, but
// Stop is idempotent and safe for concurrent use.
type Worker struct {
	id       string
	queue    *Queue
	sm       *StateMachine
	bus      EventBus
	executor Executor
	log      *logger.Logger

	pollInterval time.Duration

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// WorkerOption configures a Worker created by NewWorker.
type WorkerOption func(*Worker)

// WithPollInterval overrides how long a Worker waits before re-checking an
// empty Queue. Defaults to defaultPollInterval.
func WithPollInterval(d time.Duration) WorkerOption {
	return func(w *Worker) { w.pollInterval = d }
}

// WithWorkerLogger attaches a Logger used to report task lifecycle events
// and failures. Optional; a Worker with no logger runs silently.
func WithWorkerLogger(log *logger.Logger) WorkerOption {
	return func(w *Worker) { w.log = log }
}

// NewWorker creates a ready-to-use Worker identified by id, consuming from
// queue, transitioning Task status through sm, publishing events on bus,
// and running assigned work via executor.
func NewWorker(id string, queue *Queue, sm *StateMachine, bus EventBus, executor Executor, opts ...WorkerOption) *Worker {
	w := &Worker{
		id:           id,
		queue:        queue,
		sm:           sm,
		bus:          bus,
		executor:     executor,
		pollInterval: defaultPollInterval,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Start begins pulling Tasks from the Queue on a background goroutine. It
// returns a packages/errors error typed TypeInternal if the Worker is
// already running.
func (w *Worker) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return errors.New(errors.TypeInternal, "WORKER_ALREADY_STARTED", "core.taskworker",
			fmt.Sprintf("worker %q is already running", w.id)).With("workerId", w.id)
	}

	loopCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.done = make(chan struct{})
	w.running = true
	done := w.done
	w.mu.Unlock()

	if w.log != nil {
		w.log.Info("worker started", map[string]any{"workerId": w.id})
	}

	go w.loop(loopCtx, done)
	return nil
}

// Stop signals the Worker to stop pulling new Tasks and waits for its
// current iteration to finish, or for ctx to be done, whichever comes
// first. Stop on a Worker that was never started, or already stopped, is a
// no-op. Stop is idempotent and safe to call concurrently.
func (w *Worker) Stop(ctx context.Context) error {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = false
	cancel := w.cancel
	done := w.done
	w.mu.Unlock()

	cancel()

	select {
	case <-done:
		if w.log != nil {
			w.log.Info("worker stopped", map[string]any{"workerId": w.id})
		}
		return nil
	case <-ctx.Done():
		errType := errors.TypeCanceled
		if ctx.Err() == context.DeadlineExceeded {
			errType = errors.TypeTimeout
		}
		return errors.Wrap(ctx.Err(), errType, "WORKER_STOP_INCOMPLETE", "core.taskworker",
			fmt.Sprintf("worker %q did not stop before context was done", w.id)).With("workerId", w.id)
	}
}

// loop is the Worker's main body: it repeatedly pulls a Task from the
// Queue and processes it, polling at pollInterval when the Queue is empty,
// until ctx is done.
func (w *Worker) loop(ctx context.Context, done chan struct{}) {
	defer close(done)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		task, err := w.queue.Next()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(w.pollInterval):
			}
			continue
		}

		w.processTask(ctx, task)
	}
}

// processTask executes a single Task pulled from the Queue: it transitions
// the Task to Executing, runs the Executor, and transitions the Task to
// Completed or Failed based on the outcome - publishing a lifecycle Event
// at each step. A Task that cannot be transitioned to Executing (e.g. it
// was queued in an unexpected Status) is reported as failed without being
// handed to the Executor.
func (w *Worker) processTask(ctx context.Context, task *types.Task) {
	if _, err := w.sm.Transition(task, types.TaskStatusExecuting); err != nil {
		w.fail(task, err)
		return
	}
	w.publish(EventTaskStarted, task, nil)

	result, err := w.executor(ctx, task)
	if err != nil {
		w.fail(task, err)
		return
	}

	task.Result = result
	if _, terr := w.sm.Transition(task, types.TaskStatusCompleted); terr != nil {
		w.fail(task, terr)
		return
	}
	w.publish(EventTaskCompleted, task, nil)
}

// fail records err on task, transitions it to Failed, and publishes
// EventTaskFailed. If the Failed transition itself is rejected (task is
// already in a terminal Status), fail logs that and still publishes
// EventTaskFailed so the failure is not silently dropped.
func (w *Worker) fail(task *types.Task, taskErr error) {
	task.Error = taskErr.Error()

	if _, err := w.sm.Transition(task, types.TaskStatusFailed); err != nil && w.log != nil {
		w.log.Error("worker could not transition task to failed", map[string]any{
			"workerId": w.id,
			"taskId":   task.ID,
			"error":    err.Error(),
		})
	}

	if w.log != nil {
		w.log.Error("task failed", map[string]any{
			"workerId": w.id,
			"taskId":   task.ID,
			"error":    taskErr.Error(),
		})
	}

	w.publish(EventTaskFailed, task, map[string]any{"error": taskErr.Error()})
}

// publish emits an Event of eventType for task on the Worker's EventBus, if
// one is configured. extra payload keys are merged in alongside the
// standard taskId/workerId fields.
func (w *Worker) publish(eventType types.EventType, task *types.Task, extra map[string]any) {
	if w.bus == nil {
		return
	}

	payload := map[string]any{"taskId": task.ID, "workerId": w.id}
	for k, v := range extra {
		payload[k] = v
	}

	w.bus.Publish(types.Event{
		Type:      eventType,
		Source:    "core.taskworker",
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	})
}
