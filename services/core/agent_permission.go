// agent_permission.go implements SPEC-0024: the Agent Permission Model -
// security boundaries controlling an agent's tool access, file access,
// command execution, and external communication (JARVIS_MASTER_ARCHITECTURE
// .md's "Permission controlled" design principle, and the Tool System's
// filesystem/terminal/browser/external-integration responsibilities).
//
// SPEC-0019 already lets a single agent's Manifest declare
// require_confirmation per tool (agent_manifest.go), but nothing enforces
// it and it cannot express an outright denial. SPEC-0024 is a distinct,
// centralized security policy - one table covering every agent, with a
// three-state PermissionLevel (allowed/approval_required/denied) matching
// SPEC-0024's own YAML example - checked and logged at the point an agent
// actually tries to use a category, rather than declared once and never
// consulted. PermissionEnforcedToolCaller wires it into the SPEC-0022
// Agent Execution Loop's existing ToolCaller seam (agent_execution_loop.go)
// without modifying the loop itself.
package core

import (
	"context"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"

	"jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
)

// PermissionLevel is the three-state decision SPEC-0024 grants an agent for
// one permission category: PermissionAllowed executes unconditionally,
// PermissionApprovalRequired needs explicit approval first, and
// PermissionDenied always blocks.
type PermissionLevel string

const (
	PermissionAllowed          PermissionLevel = "allowed"
	PermissionApprovalRequired PermissionLevel = "approval_required"
	PermissionDenied           PermissionLevel = "denied"
)

// valid reports whether l is one of the three recognized PermissionLevel
// values.
func (l PermissionLevel) valid() bool {
	switch l {
	case PermissionAllowed, PermissionApprovalRequired, PermissionDenied:
		return true
	}
	return false
}

// AgentPermissions is one agent's declared PermissionLevel per category
// (e.g. "filesystem", "terminal", "browser" - SPEC-0024's own example).
// Category names are plain identifiers rather than a closed enum,
// mirroring AgentMetadata.Tools/Permissions' existing precedent (agent.go)
// of leaving what an identifier resolves to as the caller's concern.
type AgentPermissions map[string]PermissionLevel

// PermissionModel is the SPEC-0024 security boundary table for every agent,
// keyed by agent ID (matching AgentMetadata.ID / the Agent Registry key) -
// the loaded form of the YAML documented in SPEC-0024's Example.
type PermissionModel map[string]AgentPermissions

// LoadPermissionModel reads and parses the YAML permission table at path,
// then validates it.
func LoadPermissionModel(path string) (PermissionModel, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, errors.TypeNotFound, "PERMISSION_MODEL_READ_FAILED", "core.agentpermission",
			fmt.Sprintf("reading permission model file %s", path))
	}

	var model PermissionModel
	if err := yaml.Unmarshal(data, &model); err != nil {
		return nil, errors.Wrap(err, errors.TypeInvalidInput, "PERMISSION_MODEL_PARSE_ERROR", "core.agentpermission",
			fmt.Sprintf("parsing permission model file %s", path))
	}

	if err := model.Validate(); err != nil {
		return nil, err
	}

	return model, nil
}

// Validate reports whether every PermissionLevel declared in model is one
// of the three recognized values. Iteration is in sorted agent/category
// order so a model with more than one invalid entry always reports the
// same one first.
func (model PermissionModel) Validate() error {
	agentIDs := make([]string, 0, len(model))
	for id := range model {
		agentIDs = append(agentIDs, id)
	}
	sort.Strings(agentIDs)

	for _, id := range agentIDs {
		perms := model[id]
		categories := make([]string, 0, len(perms))
		for cat := range perms {
			categories = append(categories, cat)
		}
		sort.Strings(categories)

		for _, cat := range categories {
			level := perms[cat]
			if !level.valid() {
				return errors.New(errors.TypeInvalidInput, "PERMISSION_MODEL_INVALID_LEVEL", "core.agentpermission",
					fmt.Sprintf("agent %q category %q has invalid permission level %q", id, cat, level)).
					With("agentId", id).With("category", cat).With("level", string(level))
			}
		}
	}
	return nil
}

// Level reports agentID's declared PermissionLevel for category. An agent
// absent from model, or with no entry for category, defaults to
// PermissionDenied - SPEC-0024 security boundaries fail closed rather than
// silently allowing an undeclared category.
func (model PermissionModel) Level(agentID, category string) PermissionLevel {
	perms, ok := model[agentID]
	if !ok {
		return PermissionDenied
	}
	level, ok := perms[category]
	if !ok {
		return PermissionDenied
	}
	return level
}

// ApprovalFunc requests explicit approval for agentID to use category, for
// a PermissionApprovalRequired check. It reports whether approval was
// granted, or an error if the approval process itself failed (e.g. a
// prompt could not be presented). ApprovalFunc must respect ctx
// cancellation.
type ApprovalFunc func(ctx context.Context, agentID, category string) (bool, error)

// PermissionChecker enforces a PermissionModel's security boundaries and
// logs every check (SPEC-0024's "permission checks are logged" testing
// criterion). PermissionChecker is safe for concurrent use - it holds no
// per-check state.
type PermissionChecker struct {
	model   PermissionModel
	approve ApprovalFunc
	log     *logger.Logger
}

// PermissionCheckerOption configures a PermissionChecker created by
// NewPermissionChecker.
type PermissionCheckerOption func(*PermissionChecker)

// WithApprovalFunc attaches the callback used to resolve an
// ApprovalRequired category. Optional; a checker with none configured
// treats ApprovalRequired the same as Denied, since it has no way to
// obtain approval on its own.
func WithApprovalFunc(a ApprovalFunc) PermissionCheckerOption {
	return func(c *PermissionChecker) { c.approve = a }
}

// WithPermissionCheckerLogger attaches a Logger used to record every
// permission check's outcome. Optional; a checker with no logger runs
// silently.
func WithPermissionCheckerLogger(log *logger.Logger) PermissionCheckerOption {
	return func(c *PermissionChecker) { c.log = log }
}

// NewPermissionChecker creates a ready-to-use PermissionChecker enforcing
// model.
func NewPermissionChecker(model PermissionModel, opts ...PermissionCheckerOption) *PermissionChecker {
	c := &PermissionChecker{model: model}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Check enforces agentID's declared permission for category: Allowed
// returns nil immediately (allowed tools execute); Denied - including an
// undeclared agent or category - returns a packages/errors error typed
// TypePermissionDenied (restricted tools are blocked); ApprovalRequired
// calls the configured ApprovalFunc and returns nil only if it grants
// approval, or a TypePermissionDenied error otherwise. Every outcome is
// logged before Check returns.
func (c *PermissionChecker) Check(ctx context.Context, agentID, category string) error {
	level := c.model.Level(agentID, category)

	switch level {
	case PermissionAllowed:
		c.record(agentID, category, level, "allowed")
		return nil

	case PermissionApprovalRequired:
		if c.approve == nil {
			c.record(agentID, category, level, "denied_no_approver")
			return c.denied(agentID, category, level)
		}
		granted, err := c.approve(ctx, agentID, category)
		if err != nil {
			c.record(agentID, category, level, "approval_error")
			return errors.Wrap(err, errors.TypeInternal, "PERMISSION_APPROVAL_FAILED", "core.agentpermission",
				fmt.Sprintf("resolving approval for agent %q category %q", agentID, category)).
				With("agentId", agentID).With("category", category)
		}
		if !granted {
			c.record(agentID, category, level, "denied_by_approver")
			return c.denied(agentID, category, level)
		}
		c.record(agentID, category, level, "approved")
		return nil

	default: // PermissionDenied, and any unrecognized level - fail closed.
		c.record(agentID, category, level, "denied")
		return c.denied(agentID, category, level)
	}
}

// denied builds the TypePermissionDenied error Check returns for a blocked
// category.
func (c *PermissionChecker) denied(agentID, category string, level PermissionLevel) error {
	return errors.New(errors.TypePermissionDenied, "AGENT_PERMISSION_DENIED", "core.agentpermission",
		fmt.Sprintf("agent %q is not permitted to use %q", agentID, category)).
		With("agentId", agentID).With("category", category).With("level", string(level))
}

// record logs a single permission check's outcome. A no-op if no Logger is
// configured.
func (c *PermissionChecker) record(agentID, category string, level PermissionLevel, decision string) {
	if c.log == nil {
		return
	}
	c.log.Info("agent permission check", map[string]any{
		"agentId":  agentID,
		"category": category,
		"level":    string(level),
		"decision": decision,
	})
}

// PermissionEnforcedToolCaller wraps next so every call first runs
// checker.Check(ctx, agentID, tool), refusing to invoke next if the check
// fails. This is the SPEC-0022 Agent Execution Loop's existing Execute
// Actions seam (agent_execution_loop.go's ToolCaller, invoked once per Step
// naming a Tool) - wrapping it here enforces SPEC-0024's Tool access
// control at the same point every tool call already passes through,
// without modifying ExecutionLoop itself.
func PermissionEnforcedToolCaller(checker *PermissionChecker, agentID string, next ToolCaller) ToolCaller {
	return func(ctx context.Context, tool string, input map[string]any) (map[string]any, error) {
		if err := checker.Check(ctx, agentID, tool); err != nil {
			return nil, err
		}
		return next(ctx, tool, input)
	}
}
