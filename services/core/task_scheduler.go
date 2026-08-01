// task_scheduler.go implements SPEC-0016: the Background Scheduler.
// Scheduler produces Tasks on a schedule - one-time, delayed, or recurring -
// and enqueues them onto a Queue (SPEC-0013) for a Worker (SPEC-0014) to
// pick up, the same way any other Task producer would: it drives each
// fired Task through StateMachine (SPEC-0012) transitions before queuing,
// and publishes lifecycle events on an EventBus (SPEC-0009). Scheduler does
// not execute work itself.
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

// Concrete scheduler event types (SPEC-0016). packages/shared-types defines
// EventType as a bare string (SPEC-0004: shapes only); Scheduler owns this
// closed set, mirroring Worker's precedent for EventTaskStarted /
// EventTaskFailed in task_worker.go.
const (
	EventTaskScheduled         types.EventType = "TASK_SCHEDULED"
	EventScheduledTaskFired    types.EventType = "SCHEDULED_TASK_FIRED"
	EventScheduledTaskCanceled types.EventType = "SCHEDULED_TASK_CANCELED"
)

// defaultSchedulerTick is how often a Scheduler checks its schedules for
// due entries.
const defaultSchedulerTick = 10 * time.Millisecond

// TaskFactory builds a fresh Task for a Schedule to enqueue each time it
// fires. Each call must return a Task with a unique ID; a Task whose Status
// is left unset is treated as TaskStatusCreated and driven to Queued by the
// Scheduler before it reaches the Queue.
type TaskFactory func() *types.Task

// scheduleKind is the closed set of recurrence behaviors a Schedule
// supports (SPEC-0016). Build a Schedule with ScheduleOnce, ScheduleAfter,
// or ScheduleEvery rather than constructing one directly.
type scheduleKind int

const (
	kindOnce scheduleKind = iota
	kindAfter
	kindEvery
)

// Schedule is one entry registered with a Scheduler (SPEC-0016).
type Schedule struct {
	// ID identifies the Schedule for later cancellation. Must be unique
	// across a Scheduler's active schedules.
	ID string
	// Factory builds the Task enqueued each time this Schedule fires.
	Factory TaskFactory

	kind     scheduleKind
	at       time.Time
	delay    time.Duration
	interval time.Duration
}

// ScheduleOnce builds a Schedule that fires exactly once, at the given
// time, then removes itself.
func ScheduleOnce(id string, at time.Time, factory TaskFactory) Schedule {
	return Schedule{ID: id, Factory: factory, kind: kindOnce, at: at}
}

// ScheduleAfter builds a Schedule that fires exactly once, after delay has
// elapsed, then removes itself.
func ScheduleAfter(id string, delay time.Duration, factory TaskFactory) Schedule {
	return Schedule{ID: id, Factory: factory, kind: kindAfter, delay: delay}
}

// ScheduleEvery builds a Schedule that fires repeatedly, waiting interval
// between each firing, until it is canceled.
func ScheduleEvery(id string, interval time.Duration, factory TaskFactory) Schedule {
	return Schedule{ID: id, Factory: factory, kind: kindEvery, interval: interval}
}

// scheduleEntry is a Schedule plus the Scheduler's own bookkeeping for it.
type scheduleEntry struct {
	schedule Schedule
	nextRun  time.Time
}

// Scheduler holds a set of Schedules and, once Started, fires each at its
// due time by building its Task and enqueuing it onto a Queue. Scheduler is
// safe for concurrent use.
type Scheduler struct {
	queue *Queue
	sm    *StateMachine
	bus   EventBus
	log   *logger.Logger
	tick  time.Duration

	mu      sync.Mutex
	entries map[string]*scheduleEntry
	running bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// SchedulerOption configures a Scheduler created by NewScheduler.
type SchedulerOption func(*Scheduler)

// WithSchedulerTick overrides how often a Scheduler checks its schedules
// for due entries. Defaults to defaultSchedulerTick.
func WithSchedulerTick(d time.Duration) SchedulerOption {
	return func(s *Scheduler) { s.tick = d }
}

// WithSchedulerLogger attaches a Logger used to report firing failures.
// Optional; a Scheduler with no logger runs silently.
func WithSchedulerLogger(log *logger.Logger) SchedulerOption {
	return func(s *Scheduler) { s.log = log }
}

// NewScheduler creates a ready-to-use, empty Scheduler that enqueues fired
// Tasks onto queue, drives them through sm, and publishes events on bus.
func NewScheduler(queue *Queue, sm *StateMachine, bus EventBus, opts ...SchedulerOption) *Scheduler {
	s := &Scheduler{
		queue:   queue,
		sm:      sm,
		bus:     bus,
		tick:    defaultSchedulerTick,
		entries: make(map[string]*scheduleEntry),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Schedule registers schedule so it fires at its due time once the
// Scheduler is started. It returns a packages/errors error typed
// TypeInvalidInput if schedule is missing an ID, a Factory, or (for
// ScheduleAfter/ScheduleEvery) a positive duration, and TypeAlreadyExists
// if schedule.ID is already registered.
func (s *Scheduler) Schedule(schedule Schedule) error {
	if schedule.ID == "" {
		return errors.New(errors.TypeInvalidInput, "SCHEDULE_INVALID", "core.scheduler",
			"schedule requires a non-empty ID")
	}
	if schedule.Factory == nil {
		return errors.New(errors.TypeInvalidInput, "SCHEDULE_INVALID", "core.scheduler",
			"schedule requires a TaskFactory").With("scheduleId", schedule.ID)
	}
	if schedule.kind == kindAfter && schedule.delay <= 0 {
		return errors.New(errors.TypeInvalidInput, "SCHEDULE_INVALID", "core.scheduler",
			"ScheduleAfter requires a positive delay").With("scheduleId", schedule.ID)
	}
	if schedule.kind == kindEvery && schedule.interval <= 0 {
		return errors.New(errors.TypeInvalidInput, "SCHEDULE_INVALID", "core.scheduler",
			"ScheduleEvery requires a positive interval").With("scheduleId", schedule.ID)
	}

	now := time.Now().UTC()
	var nextRun time.Time
	switch schedule.kind {
	case kindOnce:
		nextRun = schedule.at
	case kindAfter:
		nextRun = now.Add(schedule.delay)
	case kindEvery:
		nextRun = now.Add(schedule.interval)
	}

	s.mu.Lock()
	if _, exists := s.entries[schedule.ID]; exists {
		s.mu.Unlock()
		return errors.New(errors.TypeAlreadyExists, "SCHEDULE_DUPLICATE_ID", "core.scheduler",
			fmt.Sprintf("schedule %q is already registered", schedule.ID)).With("scheduleId", schedule.ID)
	}
	s.entries[schedule.ID] = &scheduleEntry{schedule: schedule, nextRun: nextRun}
	s.mu.Unlock()

	s.publish(EventTaskScheduled, schedule.ID, nil, nil)
	return nil
}

// Cancel stops schedule scheduleID from firing again. A Task already
// enqueued by a prior firing is unaffected. It returns a packages/errors
// error typed TypeNotFound if no schedule with that ID is registered.
func (s *Scheduler) Cancel(scheduleID string) error {
	s.mu.Lock()
	if _, exists := s.entries[scheduleID]; !exists {
		s.mu.Unlock()
		return errors.New(errors.TypeNotFound, "SCHEDULE_NOT_FOUND", "core.scheduler",
			fmt.Sprintf("no schedule with id %q", scheduleID)).With("scheduleId", scheduleID)
	}
	delete(s.entries, scheduleID)
	s.mu.Unlock()

	s.publish(EventScheduledTaskCanceled, scheduleID, nil, nil)
	return nil
}

// Start begins checking schedules for due entries on a background
// goroutine. It returns a packages/errors error typed TypeInternal if the
// Scheduler is already running.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New(errors.TypeInternal, "SCHEDULER_ALREADY_STARTED", "core.scheduler",
			"scheduler is already running")
	}

	loopCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.done = make(chan struct{})
	s.running = true
	done := s.done
	s.mu.Unlock()

	if s.log != nil {
		s.log.Info("scheduler started", nil)
	}

	go s.loop(loopCtx, done)
	return nil
}

// Stop signals the Scheduler to stop checking schedules and waits for its
// current tick to finish, or for ctx to be done, whichever comes first.
// Stop on a Scheduler that was never started, or already stopped, is a
// no-op. Stop is idempotent and safe to call concurrently.
func (s *Scheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	cancel := s.cancel
	done := s.done
	s.mu.Unlock()

	cancel()

	select {
	case <-done:
		if s.log != nil {
			s.log.Info("scheduler stopped", nil)
		}
		return nil
	case <-ctx.Done():
		errType := errors.TypeCanceled
		if ctx.Err() == context.DeadlineExceeded {
			errType = errors.TypeTimeout
		}
		return errors.Wrap(ctx.Err(), errType, "SCHEDULER_STOP_INCOMPLETE", "core.scheduler",
			"scheduler did not stop before context was done")
	}
}

// loop is the Scheduler's main body: it checks for due schedules every
// tick, firing each one, until ctx is done.
func (s *Scheduler) loop(ctx context.Context, done chan struct{}) {
	defer close(done)

	ticker := time.NewTicker(s.tick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.fireDue()
		}
	}
}

// fireDue finds every schedule due at the current time, advances or
// removes each one (kindEvery reschedules for its next interval; kindOnce
// and kindAfter remove themselves), then fires them. Advancing/removing
// entries happens under s.mu so a concurrent Cancel can never race a
// firing already in flight; firing itself happens outside the lock since
// it calls out to the schedule's Factory, the StateMachine, and the Queue.
func (s *Scheduler) fireDue() {
	now := time.Now().UTC()

	s.mu.Lock()
	var due []Schedule
	for id, entry := range s.entries {
		if entry.nextRun.After(now) {
			continue
		}
		due = append(due, entry.schedule)
		if entry.schedule.kind == kindEvery {
			entry.nextRun = now.Add(entry.schedule.interval)
		} else {
			delete(s.entries, id)
		}
	}
	s.mu.Unlock()

	for _, schedule := range due {
		s.fire(schedule)
	}
}

// fire builds schedule's Task via its Factory, drives it to Queued, and
// enqueues it. A Task whose Factory left Status unset is treated as
// TaskStatusCreated; a Task returned with any other Status is enqueued as
// given, on the assumption its caller already positioned it correctly. A
// failure at any step is logged and the firing is dropped rather than
// retried - the Schedule itself (for kindEvery) still fires again at its
// next interval.
func (s *Scheduler) fire(schedule Schedule) {
	task := schedule.Factory()
	if task.Source == "" {
		task.Source = types.TaskSourceScheduled
	}
	if task.Status == "" {
		task.Status = types.TaskStatusCreated
	}

	if task.Status == types.TaskStatusCreated {
		if _, err := s.sm.Transition(task, types.TaskStatusPlanning); err != nil {
			s.failFire(schedule.ID, task, err)
			return
		}
		if _, err := s.sm.Transition(task, types.TaskStatusQueued); err != nil {
			s.failFire(schedule.ID, task, err)
			return
		}
	}

	if err := s.queue.Add(task); err != nil {
		s.failFire(schedule.ID, task, err)
		return
	}

	s.publish(EventScheduledTaskFired, schedule.ID, task, nil)
}

// failFire logs a firing failure for scheduleID, if a Logger is configured.
func (s *Scheduler) failFire(scheduleID string, task *types.Task, err error) {
	if s.log == nil {
		return
	}
	s.log.Error("scheduled task failed to fire", map[string]any{
		"scheduleId": scheduleID,
		"taskId":     task.ID,
		"error":      err.Error(),
	})
}

// publish emits an Event of eventType for scheduleID on the Scheduler's
// EventBus, if one is configured. task, if non-nil, contributes a taskId
// field; extra payload keys are merged in alongside it.
func (s *Scheduler) publish(eventType types.EventType, scheduleID string, task *types.Task, extra map[string]any) {
	if s.bus == nil {
		return
	}

	payload := map[string]any{"scheduleId": scheduleID}
	if task != nil {
		payload["taskId"] = task.ID
	}
	for k, v := range extra {
		payload[k] = v
	}

	s.bus.Publish(types.Event{
		Type:      eventType,
		Source:    "core.scheduler",
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	})
}
