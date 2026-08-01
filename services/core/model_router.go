// model_router.go implements SPEC-0029: the Model Router. ModelRouter picks
// which packages/config Model a request should use based on task type,
// agent type, user preference, and current model availability, with a
// deterministic fallback and a logged decision for every route - the
// "Routing Inputs" and "Testing" requirements SPEC-0029 lists. It sits on
// top of SPEC-0026's Provider (for availability) and SPEC-0028's ModelConfig
// (for the configured model set) without changing either.
package core

import (
	"context"

	cfgpkg "jarvis-pa/packages/config"
	"jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
)

// RouteReason identifies which routing input decided a RouteDecision.
type RouteReason string

const (
	// ReasonUserPreference means the request's explicit UserPreference
	// named a configured model key.
	ReasonUserPreference RouteReason = "user_preference"
	// ReasonAgentOverride means ModelConfig.AgentModels had an entry for
	// the request's AgentType (SPEC-0028).
	ReasonAgentOverride RouteReason = "agent_override"
	// ReasonTaskType means the router's task-type table had an entry for
	// the request's TaskType.
	ReasonTaskType RouteReason = "task_type"
	// ReasonDefault means no more specific input matched, so
	// ModelConfig.DefaultModel was used.
	ReasonDefault RouteReason = "default"
)

// RouteRequest carries the SPEC-0029 "Routing Inputs" a caller supplies for
// one routing decision. Any field may be left empty; Route falls through to
// the next input in priority order (UserPreference, then AgentType, then
// TaskType, then the configured default).
type RouteRequest struct {
	// TaskType is the Task.Type (SPEC-0011) the request is executing,
	// e.g. "coding" or "conversation".
	TaskType string
	// AgentType is the Agent.Type (SPEC-0018) making the request.
	AgentType string
	// UserPreference, if set, is a ModelConfig.Models key the user
	// explicitly asked for. It wins over every other input when it names
	// a model that exists in the configuration.
	UserPreference string
}

// RouteDecision is the resolved outcome of a Route call: which configured
// model key was chosen, its Model definition, why it was chosen, and
// whether Route had to fall back off that choice due to availability.
type RouteDecision struct {
	Key      string
	Model    cfgpkg.Model
	Reason   RouteReason
	Fallback bool
}

// ModelRouter resolves a RouteRequest to a configured Model. Create one with
// NewModelRouter.
type ModelRouter struct {
	cfg        cfgpkg.ModelConfig
	provider   Provider
	log        *logger.Logger
	taskModels map[string]string
}

// NewModelRouter creates a ModelRouter over cfg's model definitions.
// provider is used for the "Model availability" input (its ListModels is
// consulted so Route can fall back off an unavailable choice); a nil
// provider disables the availability check and fallback. log receives one
// entry per Route call recording the routing decision; a nil log disables
// logging. taskModels maps a Task.Type value to a cfg.Models key (e.g.
// "coding" -> "coding"); a nil map means task-type routing is not
// configured and Route falls through to agent/default resolution.
func NewModelRouter(cfg cfgpkg.ModelConfig, provider Provider, log *logger.Logger, taskModels map[string]string) *ModelRouter {
	return &ModelRouter{cfg: cfg, provider: provider, log: log, taskModels: taskModels}
}

// Route resolves req to a RouteDecision. It first picks a model key using,
// in priority order: req.UserPreference, ModelConfig.AgentModels[req.AgentType],
// the router's taskModels[req.TaskType], then ModelConfig.DefaultModel.
// Route returns an error only if none of those resolve to a key with a
// matching entry in ModelConfig.Models.
//
// If a Provider was supplied and the resolved model's Name is absent from
// Provider.ListModels, Route falls back to ModelConfig.DefaultModel (when
// that names a different, configured model) and marks the decision
// Fallback. A ListModels failure is not treated as unavailability - Route
// keeps the original decision rather than guessing at an unreachable
// provider's state.
//
// Every call logs its outcome (the SPEC-0029 "Routing decisions are logged"
// testing requirement) before returning.
func (r *ModelRouter) Route(ctx context.Context, req RouteRequest) (RouteDecision, error) {
	key, reason, err := r.resolveKey(req)
	if err != nil {
		return RouteDecision{}, err
	}

	model := r.cfg.Models[key]
	decision := RouteDecision{Key: key, Model: model, Reason: reason}

	if r.provider != nil {
		if available, availErr := r.isAvailable(ctx, model.Name); availErr == nil && !available {
			if fallbackKey := r.cfg.DefaultModel; fallbackKey != "" && fallbackKey != key {
				if fallbackModel, ok := r.cfg.Models[fallbackKey]; ok {
					decision = RouteDecision{Key: fallbackKey, Model: fallbackModel, Reason: reason, Fallback: true}
				}
			}
		}
	}

	r.logDecision(req, decision)
	return decision, nil
}

// resolveKey picks a ModelConfig.Models key for req and reports which input
// decided it, without regard to provider availability.
func (r *ModelRouter) resolveKey(req RouteRequest) (string, RouteReason, error) {
	if req.UserPreference != "" {
		if _, ok := r.cfg.Models[req.UserPreference]; ok {
			return req.UserPreference, ReasonUserPreference, nil
		}
	}

	if req.AgentType != "" {
		if key, ok := r.cfg.AgentModels[req.AgentType]; ok {
			if _, ok := r.cfg.Models[key]; ok {
				return key, ReasonAgentOverride, nil
			}
		}
	}

	if req.TaskType != "" && r.taskModels != nil {
		if key, ok := r.taskModels[req.TaskType]; ok {
			if _, ok := r.cfg.Models[key]; ok {
				return key, ReasonTaskType, nil
			}
		}
	}

	if r.cfg.DefaultModel != "" {
		if _, ok := r.cfg.Models[r.cfg.DefaultModel]; ok {
			return r.cfg.DefaultModel, ReasonDefault, nil
		}
	}

	return "", "", errors.New(errors.TypeNotFound, "MODEL_ROUTER_NO_MODEL", "core.modelrouter",
		"no model could be resolved: no matching user preference, agent override, task-type mapping, or default model").
		With("taskType", req.TaskType).With("agentType", req.AgentType).With("userPreference", req.UserPreference)
}

// isAvailable reports whether modelName appears in the Provider's current
// ListModels result.
func (r *ModelRouter) isAvailable(ctx context.Context, modelName string) (bool, error) {
	models, err := r.provider.ListModels(ctx)
	if err != nil {
		return false, err
	}
	for _, m := range models {
		if m.Name == modelName {
			return true, nil
		}
	}
	return false, nil
}

// logDecision records one routing decision as a structured log entry.
func (r *ModelRouter) logDecision(req RouteRequest, d RouteDecision) {
	if r.log == nil {
		return
	}
	r.log.Info("model router decision", map[string]any{
		"taskType":       req.TaskType,
		"agentType":      req.AgentType,
		"userPreference": req.UserPreference,
		"modelKey":       d.Key,
		"modelName":      d.Model.Name,
		"modelProvider":  d.Model.Provider,
		"reason":         string(d.Reason),
		"fallback":       d.Fallback,
	})
}
