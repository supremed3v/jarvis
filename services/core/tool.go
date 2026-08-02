// tool.go implements SPEC-0043: the Tool Interface - the foundational
// contract for all JARVIS tools. Tools are controlled capabilities that let
// agents interact with the computer, files, services, and external systems
// (per the spec overview, and JARVIS_MASTER_ARCHITECTURE.md's Tool System
// responsibility set). This is the concrete contract the SPEC-0045 Tool
// Registry will hold instances of, and the shape the SPEC-0044 Tool Manifest
// System must produce when it loads a manifest into a runnable Tool.
//
// SPEC-0018's AgentMetadata.Tools and SPEC-0022's Step.Tool are bare string
// identifiers today; SPEC-0043 alone does not give them meaning - it only
// defines the contract that an eventual Tool Registry (SPEC-0045) will key
// those identifiers to. Mirroring AgentMetadata.Tools' existing precedent
// (agent.go), Permissions is likewise a []string of bare capability
// identifiers (e.g. "filesystem.read", "terminal.exec") that align with but
// do not constrain SPEC-0024's permission categories - SPEC-0024's own
// permission table maps agent+category to a PermissionLevel; this spec's
// Permissions field merely enumerates which categories a Tool *requires* an
// agent to be allowed, leaving the actual allow/approval/deny decision to
// SPEC-0024's PermissionChecker.
package core

import (
	"context"

	"jarvis-pa/packages/errors"
)

// SchemaField describes one field in a Tool's input or output schema
// (SPEC-0043's "Input schema" / "Output schema" requirements). A field is
// declared by Name and free-form Type (e.g. "string", "integer", "array",
// "object"); Required flags whether the field must be present on input. This
// is a deliberately minimal shape - SPEC-0044's manifest format may enrich
// it, and a future spec may move it to packages/shared-types if other layers
// need to share it - but for SPEC-0043 alone an inline definition is enough
// to satisfy the spec's "validate inputs" responsibility and no more.
type SchemaField struct {
	Name     string
	Type     string
	Required bool
}

// Schema is the ordered set of fields describing the structure of a Tool's
// input or output. An empty Schema (no fields) is valid: it describes a Tool
// whose input/output is free-form, mirroring the loose map[string]any contract
// the existing ToolCaller seam (agent_execution_loop.go) already uses so a
// Tool implementation can be wired straight into that seam without an adapter.
type Schema []SchemaField

// Validate reports whether input satisfies s: every Required field must be
// present and non-nil. Field values themselves are not type-checked here -
// the spec's "Validate inputs" responsibility is satisfied by requiring
// declared inputs to be present; deeper validation (types, ranges, formats)
// belongs to SPEC-0044's manifest system and beyond, not to this contract.
// Returns a packages/errors error typed TypeInvalidInput naming the first
// missing required field, or nil if input is valid.
func (s Schema) Validate(input map[string]any) error {
	for _, f := range s {
		if !f.Required {
			continue
		}
		v, ok := input[f.Name]
		if !ok || v == nil {
			return errors.New(errors.TypeInvalidInput, "TOOL_INPUT_MISSING_REQUIRED", "core.tool",
				"required input field is missing").With("field", f.Name)
		}
	}
	return nil
}

// ToolMetadata is a Tool's static, declarative identity and declared
// requirements - the SPEC-0043 contract fields the runtime can inspect
// without ever invoking the tool (e.g. to check permissions or discover
// capabilities before a Step naming this Tool is executed). This is the
// direct Tool-side counterpart to AgentMetadata (agent.go, SPEC-0018): the
// same shape (ID/Name/Description + declared requirements) so the runtime
// can manage either layer symmetrically. Permissions is the list of
// SPEC-0024 permission categories this Tool requires an invoking agent to
// be allowed (e.g. "filesystem.read"); the empty slice means the Tool needs
// no special permission, mirroring AgentMetadata's existing precedent that
// these identifiers are descriptive, not self-enforcing.
type ToolMetadata struct {
	ID           string
	Name         string
	Description  string
	InputSchema  Schema
	OutputSchema Schema
	Permissions  []string
}

// Validate reports whether m has the minimum identity a runtime needs to
// discover and manage the tool: a non-empty ID and Name. It returns a
// packages/errors error typed TypeInvalidInput naming the first missing
// field, or nil if m is valid. This mirrors AgentMetadata.Validate exactly
// (agent.go), so a runtime discovering tools uses the same validation
// contract it already uses for discovering agents.
func (m ToolMetadata) Validate() error {
	if m.ID == "" {
		return errors.New(errors.TypeInvalidInput, "TOOL_METADATA_MISSING_ID", "core.tool",
			"tool metadata is missing an ID")
	}
	if m.Name == "" {
		return errors.New(errors.TypeInvalidInput, "TOOL_METADATA_MISSING_NAME", "core.tool",
			"tool metadata is missing a Name").With("toolId", m.ID)
	}
	return nil
}

// Tool is the SPEC-0043 base contract every JARVIS tool implements. Metadata
// lets the runtime discover and validate the tool without running it;
// Execute carries out the tool's specific action on input and returns a
// structured result (Tool Responsibilities: perform a specific action,
// validate inputs, return structured results, report failures safely).
//
// Execute's signature matches the existing ToolCaller type
// (agent_execution_loop.go, SPEC-0022): by dropping the tool name argument
// (which the caller already knows - it named the tool to begin with) a
// Tool's Execute method value can be adapted to a ToolCaller by a small
// wrapper in SPEC-0045's Registry - the same "signature alignment, no
// adapter needed" precedent set by Agent.Execute matching Executor
// (agent.go) and ExecutionLoop.Run matching both (agent_execution_loop.go).
// Execute must respect ctx cancellation (SPEC-0043's "report failures
// safely" responsibility under cancellation: return an error rather than
// block forever).
type Tool interface {
	// Metadata returns the Tool's static identity and declared
	// requirements.
	Metadata() ToolMetadata

	// Execute performs the tool's action on input and returns its structured
	// output on success, or an error if execution failed. Execute must
	// respect ctx cancellation.
	Execute(ctx context.Context, input map[string]any) (map[string]any, error)
}

// ValidateToolInput is the convenience helper a Tool implementation's Execute
// can call to satisfy SPEC-0043's "Validate inputs" responsibility: it runs
// input against tool's InputSchema and returns the same typed error Schema
// .Validate produces, so a Tool's Execute boilerplate for input validation
// reduces to a single call (mirroring how AgentMetadata.Validate centralizes
// agent identity validation in agent.go). Returns nil if input satisfies
// the schema.
func ValidateToolInput(tool Tool, input map[string]any) error {
	return tool.Metadata().InputSchema.Validate(input)
}
