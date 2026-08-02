// container.go implements SPEC-0008: the dependency container. Container
// gives runtime components typed access to shared services through an
// explicit, instance-scoped struct rather than package-level globals.
package core

import (
	cfgpkg "jarvis-pa/packages/config"
	"jarvis-pa/packages/logger"
)

// TaskManager is a placeholder slot for the Task Execution layer
// (SPEC-0011..0017). Not yet implemented.
type TaskManager interface{}

// ToolRegistry is a placeholder slot for the SPEC-0045 Tool Registry.
// Not yet implemented.
type ToolRegistry interface{}

// Container holds the shared services a Core Runtime component may depend
// on. Config, Logger, EventBus, AgentRegistry, Provider, Router,
// StreamHandler, PromptRegistry, WindowManager, BudgetManager, and Memory are
// wired to their real SPEC-0003, SPEC-0005, SPEC-0009, SPEC-0020,
// SPEC-0026/0027, SPEC-0029, SPEC-0030, SPEC-0031, SPEC-0032, SPEC-0033, and
// SPEC-0034/0035 implementations; the remaining slots are typed placeholders
// until their owning specs are implemented. Every slot stays nil unless
// supplied via options.
type Container struct {
	Config *cfgpkg.Config
	Logger *logger.Logger

	EventBus       EventBus
	TaskManager    TaskManager
	ToolRegistry   ToolRegistry
	AgentRegistry  AgentRegistry
	Provider       Provider
	Router         *ModelRouter
	StreamHandler  *StreamHandler
	PromptRegistry *PromptRegistry
	WindowManager  *WindowManager
	BudgetManager  *BudgetManager
	Memory         Memory
}

// ContainerOption configures a Container created by NewContainer.
type ContainerOption func(*Container)

// WithEventBus sets the Container's Event Bus slot.
func WithEventBus(b EventBus) ContainerOption {
	return func(c *Container) { c.EventBus = b }
}

// WithTaskManager sets the Container's Task Manager slot.
func WithTaskManager(m TaskManager) ContainerOption {
	return func(c *Container) { c.TaskManager = m }
}

// WithToolRegistry sets the Container's Tool Registry slot.
func WithToolRegistry(r ToolRegistry) ContainerOption {
	return func(c *Container) { c.ToolRegistry = r }
}

// WithAgentRegistry sets the Container's Agent Registry slot.
func WithAgentRegistry(r AgentRegistry) ContainerOption {
	return func(c *Container) { c.AgentRegistry = r }
}

// WithProvider sets the Container's LLM Provider slot.
func WithProvider(p Provider) ContainerOption {
	return func(c *Container) { c.Provider = p }
}

// WithRouter sets the Container's Model Router slot.
func WithRouter(r *ModelRouter) ContainerOption {
	return func(c *Container) { c.Router = r }
}

// WithStreamHandler sets the Container's Streaming Response Handler slot.
func WithStreamHandler(h *StreamHandler) ContainerOption {
	return func(c *Container) { c.StreamHandler = h }
}

// WithPromptRegistry sets the Container's Prompt Registry slot.
func WithPromptRegistry(r *PromptRegistry) ContainerOption {
	return func(c *Container) { c.PromptRegistry = r }
}

// WithWindowManager sets the Container's Context Window Manager slot.
func WithWindowManager(w *WindowManager) ContainerOption {
	return func(c *Container) { c.WindowManager = w }
}

// WithBudgetManager sets the Container's Token Budget Manager slot.
func WithBudgetManager(m *BudgetManager) ContainerOption {
	return func(c *Container) { c.BudgetManager = m }
}

// WithMemory sets the Container's Memory slot.
func WithMemory(m Memory) ContainerOption {
	return func(c *Container) { c.Memory = m }
}

// NewContainer creates a Container wired to the given Config and Logger.
// cfg and log are required; the remaining slots default to nil and are set
// only via the supplied options.
func NewContainer(cfg *cfgpkg.Config, log *logger.Logger, opts ...ContainerOption) *Container {
	c := &Container{
		Config: cfg,
		Logger: log,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}
