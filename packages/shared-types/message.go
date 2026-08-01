package types

import "time"

// MessageType identifies which category of internal communication a
// Message carries (SPEC-0010).
type MessageType string

const (
	MessageTypeAgentCommunication MessageType = "agent_communication"
	MessageTypeToolRequest        MessageType = "tool_request"
	MessageTypeEventNotification  MessageType = "event_notification"
)

// Message is the internal communication envelope exchanged between Core
// Runtime components (SPEC-0010 Internal Message Protocol): agent-to-agent
// communication, tool requests, and event notifications all share this one
// contract. Destination is left empty for broadcast-style traffic (e.g. an
// event notification with no single addressed recipient).
type Message struct {
	ID          string         `json:"id"`
	Timestamp   time.Time      `json:"timestamp"`
	Source      string         `json:"source"`
	Destination string         `json:"destination,omitempty"`
	Type        MessageType    `json:"type"`
	Payload     map[string]any `json:"payload,omitempty"`
}
