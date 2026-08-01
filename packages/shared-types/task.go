package types

import "time"

// TaskStatus is the lifecycle state of a Task (SPEC-0012 Task State
// Machine).
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

// TaskSource identifies where a Task originated (SPEC-0011).
type TaskSource string

const (
	TaskSourceVoice     TaskSource = "voice"
	TaskSourceDesktop   TaskSource = "desktop"
	TaskSourceAgent     TaskSource = "agent"
	TaskSourceScheduled TaskSource = "scheduled"
)

// TaskPriority ranks a Task relative to others. Concrete priority levels
// are producer/consumer-specific and belong to SPEC-0013 Task Queue, not
// to this shared contract - mirrors EventType's precedent of having no
// hardcoded constants in SPEC-0004.
type TaskPriority string

// Task is a unit of work tracked by the JARVIS task system (SPEC-0011).
type Task struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Source      TaskSource     `json:"source"`
	Priority    TaskPriority   `json:"priority,omitempty"`
	Type        string         `json:"type"`
	Status      TaskStatus     `json:"status"`
	Input       map[string]any `json:"input,omitempty"`
	Result      map[string]any `json:"result,omitempty"`
	Error       string         `json:"error,omitempty"`
	ParentID    string         `json:"parentId,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}
