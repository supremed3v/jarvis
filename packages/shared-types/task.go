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

// Task is a unit of work tracked by the JARVIS task system (SPEC-0011).
type Task struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Status    TaskStatus     `json:"status"`
	Input     map[string]any `json:"input,omitempty"`
	Result    map[string]any `json:"result,omitempty"`
	Error     string         `json:"error,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}
