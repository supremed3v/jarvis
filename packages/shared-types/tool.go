package types

// Tool describes a capability an agent can invoke (SPEC-0043 Tool
// Interface, SPEC-0044 Tool Manifest System).
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
	Permissions []string       `json:"permissions,omitempty"`
}
