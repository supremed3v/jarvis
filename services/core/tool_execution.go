// tool_execution.go implements SPEC-0046: the Tool Execution Engine - the
// layer responsible for running tools safely, following the spec's fixed
// flow (Agent Request -> Validate Input -> Check Permission -> Execute Tool
// -> Return Result).
//
// Every stage of that flow already has a concrete implementation elsewhere
// in this package: SPEC-0045's ToolRegistry (tool_registry.go) resolves a
// tool by ID, SPEC-0043's ValidateToolInput (tool.go) validates input
// against the tool's declared InputSchema, and SPEC-0024's PermissionChecker
// (agent_permission.go) is the "actual allow/approval/deny decision" tool.go
// already documents as the intended home for the Permissions a ToolMetadata
// declares. ToolExecutionEngine's job is solely to sequence these existing
// pieces in the order SPEC-0046 specifies and report the outcome - it does
// not reimplement lookup, validation, or permission logic itself.
package core

import (
	"context"
	"fmt"

	"jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
)

// ToolExecutionEngine runs a registered tool through the SPEC-0046 flow:
// Validate Input, Check Permission, Execute Tool, Return Result.
// ToolExecutionEngine is safe for concurrent use - it holds no per-call
// state of its own (ToolRegistry and PermissionChecker are themselves
// already safe for concurrent use).
type ToolExecutionEngine struct {
	registry ToolRegistry
	checker  *PermissionChecker
	log      *logger.Logger
}

// ToolExecutionEngineOption configures a ToolExecutionEngine created by
// NewToolExecutionEngine.
type ToolExecutionEngineOption func(*ToolExecutionEngine)

// WithExecutionPermissionChecker attaches the SPEC-0024 PermissionChecker
// used for Check Permission. Required only if a tool ever declares a
// non-empty ToolMetadata.Permissions - a tool that declares no permission
// categories needs no checker, mirroring tool.go's own documented meaning
// of an empty Permissions slice ("the Tool needs no special permission").
func WithExecutionPermissionChecker(checker *PermissionChecker) ToolExecutionEngineOption {
	return func(e *ToolExecutionEngine) { e.checker = checker }
}

// WithToolExecutionEngineLogger attaches a Logger used to record every
// Execute outcome. Optional; an engine with no logger runs silently.
func WithToolExecutionEngineLogger(log *logger.Logger) ToolExecutionEngineOption {
	return func(e *ToolExecutionEngine) { e.log = log }
}

// NewToolExecutionEngine creates a ready-to-use ToolExecutionEngine that
// resolves tools through registry. It returns a packages/errors error typed
// TypeInvalidInput if registry is nil.
func NewToolExecutionEngine(registry ToolRegistry, opts ...ToolExecutionEngineOption) (*ToolExecutionEngine, error) {
	if registry == nil {
		return nil, errors.New(errors.TypeInvalidInput, "TOOL_EXECUTION_ENGINE_MISSING_REGISTRY", "core.toolexecution",
			"cannot create a tool execution engine without a registry")
	}

	e := &ToolExecutionEngine{registry: registry}
	for _, opt := range opts {
		opt(e)
	}
	return e, nil
}

// Execute runs the SPEC-0046 flow for toolID on behalf of agentID:
//
//  1. Validate Input is satisfied by looking toolID up in the registry
//     (a TypeNotFound error if unregistered) and validating input against
//     its declared InputSchema (ValidateToolInput, tool.go).
//  2. Check Permission is satisfied by checking every permission category
//     the tool declares against the configured PermissionChecker.
//  3. Execute Tool runs the tool's own Execute.
//  4. Return Result passes the tool's structured output back unchanged.
//
// Execute checks ctx for cancellation before doing any of the above, so a
// caller that cancels before dispatch gets a clear TypeCanceled/TypeTimeout
// error rather than a partially completed flow. Every outcome (success or
// failure, and which stage it failed at) is logged if a Logger is
// configured.
func (e *ToolExecutionEngine) Execute(ctx context.Context, agentID, toolID string, input map[string]any) (map[string]any, error) {
	if errType, canceled := ctxErrType(ctx); canceled {
		err := errors.Wrap(ctx.Err(), errType, "TOOL_EXECUTION_CANCELED", "core.toolexecution",
			fmt.Sprintf("tool execution canceled before running %q", toolID)).
			With("toolId", toolID).With("agentId", agentID)
		e.record(agentID, toolID, "canceled", err)
		return nil, err
	}

	tool, err := e.registry.Lookup(toolID)
	if err != nil {
		e.record(agentID, toolID, "lookup_failed", err)
		return nil, err
	}

	if err := ValidateToolInput(tool, input); err != nil {
		e.record(agentID, toolID, "validation_failed", err)
		return nil, err
	}

	if err := e.checkPermissions(ctx, agentID, tool.Metadata()); err != nil {
		e.record(agentID, toolID, "permission_denied", err)
		return nil, err
	}

	result, err := tool.Execute(ctx, input)
	if err != nil {
		wrapped := errors.Wrap(err, errors.TypeInternal, "TOOL_EXECUTION_FAILED", "core.toolexecution",
			fmt.Sprintf("executing tool %q", toolID)).With("toolId", toolID).With("agentId", agentID)
		e.record(agentID, toolID, "execution_failed", wrapped)
		return nil, wrapped
	}

	e.record(agentID, toolID, "executed", nil)
	return result, nil
}

// checkPermissions enforces Check Permission for every category metadata
// declares. A tool with no declared categories needs no checker at all; a
// tool that does declare categories but has no checker configured fails
// closed with a distinct, diagnosable error rather than silently skipping
// the check - mirroring PermissionModel.Level's own "fail closed on
// anything undeclared" precedent (agent_permission.go).
func (e *ToolExecutionEngine) checkPermissions(ctx context.Context, agentID string, metadata ToolMetadata) error {
	if len(metadata.Permissions) == 0 {
		return nil
	}
	if e.checker == nil {
		return errors.New(errors.TypePermissionDenied, "TOOL_EXECUTION_NO_PERMISSION_CHECKER", "core.toolexecution",
			fmt.Sprintf("tool %q requires permission categories but no PermissionChecker is configured", metadata.ID)).
			With("toolId", metadata.ID)
	}
	for _, category := range metadata.Permissions {
		if err := e.checker.Check(ctx, agentID, category); err != nil {
			return err
		}
	}
	return nil
}

// record logs a single Execute outcome. A no-op if no Logger is configured.
func (e *ToolExecutionEngine) record(agentID, toolID, outcome string, err error) {
	if e.log == nil {
		return
	}
	fields := map[string]any{"agentId": agentID, "toolId": toolID, "outcome": outcome}
	if err != nil {
		fields["error"] = err.Error()
		e.log.Error("tool execution", fields)
		return
	}
	e.log.Info("tool execution", fields)
}
