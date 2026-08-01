// task_history.go implements SPEC-0017: Task History Storage. HistoryStore
// subscribes to the lifecycle events already published across the Task
// Execution layer (SPEC-0009/0014/0015/0016) and records each one against
// its Task ID, building a per-task execution timeline - status changes,
// errors, and results all arrive as event Payload since each producer
// already attaches them - so it needs no changes to Queue, StateMachine,
// Worker, RetryManager, or Scheduler beyond the "result" payload key added
// to Worker's EventTaskCompleted publish. Recorded history is retrievable
// by Task ID for debugging, memory formation, and user activity review.
package core

import (
	"sort"
	"sync"
	"time"

	types "jarvis-pa/packages/shared-types"
)

// HistoryRecord is one recorded event in a Task's execution timeline
// (SPEC-0017).
type HistoryRecord struct {
	TaskID    string          `json:"taskId"`
	EventType types.EventType `json:"eventType"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   map[string]any  `json:"payload,omitempty"`
}

// defaultHistoryEventTypes is the closed set of Task Execution layer event
// types a HistoryStore records by default: task lifecycle (SPEC-0014),
// retry (SPEC-0015), and scheduling (SPEC-0016) events. A future producer
// scoped to a Task (e.g. a later Tool Execution Engine spec) can be
// historized too, either by reusing one of these event types or by passing
// its own via WithHistoryEventTypes, without HistoryStore itself changing.
var defaultHistoryEventTypes = []types.EventType{
	EventTaskScheduled,
	EventScheduledTaskFired,
	EventScheduledTaskCanceled,
	EventTaskStarted,
	EventTaskCompleted,
	EventTaskFailed,
	EventTaskRetryScheduled,
}

// HistoryStore records Task lifecycle events published on an EventBus and
// makes each Task's recorded timeline retrievable by ID. HistoryStore is
// safe for concurrent use.
type HistoryStore struct {
	mu      sync.Mutex
	records map[string][]HistoryRecord

	unsubscribe []func()
}

// HistoryStoreOption configures a HistoryStore created by NewHistoryStore.
type HistoryStoreOption func(*historyStoreConfig)

type historyStoreConfig struct {
	eventTypes []types.EventType
}

// WithHistoryEventTypes overrides the set of EventTypes a HistoryStore
// subscribes to. Defaults to defaultHistoryEventTypes.
func WithHistoryEventTypes(eventTypes []types.EventType) HistoryStoreOption {
	return func(c *historyStoreConfig) { c.eventTypes = eventTypes }
}

// NewHistoryStore creates a HistoryStore subscribed to bus for
// defaultHistoryEventTypes (or the types given via WithHistoryEventTypes)
// and begins recording immediately. A nil bus yields a HistoryStore that
// never records anything, for callers that don't need history. Call Close
// to stop recording.
func NewHistoryStore(bus EventBus, opts ...HistoryStoreOption) *HistoryStore {
	cfg := historyStoreConfig{eventTypes: defaultHistoryEventTypes}
	for _, opt := range opts {
		opt(&cfg)
	}

	h := &HistoryStore{records: make(map[string][]HistoryRecord)}
	if bus == nil {
		return h
	}
	for _, eventType := range cfg.eventTypes {
		h.unsubscribe = append(h.unsubscribe, bus.Subscribe(eventType, h.record))
	}
	return h
}

// record is the EventBus Handler that appends event to its Task's timeline.
// An event whose Payload carries no non-empty "taskId" string is ignored -
// none of defaultHistoryEventTypes currently omit it, but a caller-supplied
// event type (via WithHistoryEventTypes) might not be Task-scoped.
func (h *HistoryStore) record(event types.Event) {
	taskID, ok := event.Payload["taskId"].(string)
	if !ok || taskID == "" {
		return
	}

	// Payload is copied rather than referenced: event.Payload is a map, a
	// reference type, and every subscriber of this event.Type receives an
	// Event pointing at the same underlying map (Bus.Publish sends copies of
	// the Event struct, not deep copies of its Payload). Without this copy,
	// a HistoryRecord would not be the immutable snapshot its "persisted
	// history" role implies - it would still be aliased to whatever the
	// publisher (or another subscriber) does with that map afterward.
	payload := make(map[string]any, len(event.Payload))
	for k, v := range event.Payload {
		payload[k] = v
	}

	rec := HistoryRecord{
		TaskID:    taskID,
		EventType: event.Type,
		Timestamp: event.Timestamp,
		Payload:   payload,
	}

	h.mu.Lock()
	h.records[taskID] = append(h.records[taskID], rec)
	h.mu.Unlock()
}

// History returns taskID's recorded timeline in chronological order (by
// each event's own Timestamp, not the order record was called - different
// event types are delivered on independent EventBus subscription
// goroutines, so arrival order does not always match publish order). It
// returns nil if taskID has no recorded history.
func (h *HistoryStore) History(taskID string) []HistoryRecord {
	h.mu.Lock()
	records := append([]HistoryRecord(nil), h.records[taskID]...)
	h.mu.Unlock()

	sort.SliceStable(records, func(i, j int) bool {
		return records[i].Timestamp.Before(records[j].Timestamp)
	})
	return records
}

// Close unsubscribes the HistoryStore from its EventBus; no further events
// are recorded afterward. Already-recorded history is retained and still
// retrievable via History. Close is idempotent and safe for concurrent use.
func (h *HistoryStore) Close() {
	h.mu.Lock()
	unsub := h.unsubscribe
	h.unsubscribe = nil
	h.mu.Unlock()

	for _, fn := range unsub {
		fn()
	}
}
