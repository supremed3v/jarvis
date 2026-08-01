// Package types implements SPEC-0004: framework-independent, serializable
// data contracts (Event, Task, Agent, Tool) shared across JARVIS services.
// It defines shapes only — no business logic, no I/O, no behavior.
package types

import "time"

// EventType identifies the kind of event flowing through the JARVIS event
// bus (SPEC-0009).
type EventType string

// Event is a single message on the JARVIS event bus.
type Event struct {
	ID        string         `json:"id"`
	Type      EventType      `json:"type"`
	Source    string         `json:"source"`
	Timestamp time.Time      `json:"timestamp"`
	Payload   map[string]any `json:"payload,omitempty"`
}
