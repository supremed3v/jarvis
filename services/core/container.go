// container.go implements SPEC-0008: the dependency container. Container
// gives runtime components typed access to shared services through an
// explicit, instance-scoped struct rather than package-level globals.
package core

import (
	cfgpkg "jarvis-pa/packages/config"
	"jarvis-pa/packages/logger"
)

// EventBus is a placeholder slot for the SPEC-0009 Event Bus contract.
// SPEC-0009 is not yet implemented, so this holds no methods; it exists so
// Container's shape does not change once a concrete Event Bus lands.
type EventBus interface{}

// TaskManager is a placeholder slot for the Task Execution layer
// (SPEC-0011..0017). Not yet implemented.
type TaskManager interface{}

// ToolRegistry is a placeholder slot for the SPEC-0045 Tool Registry.
// Not yet implemented.
type ToolRegistry interface{}

// AgentRegistry is a placeholder slot for the SPEC-0020 Agent Registry.
// Not yet implemented.
type AgentRegistry interface{}

// Container holds the shared services a Core Runtime component may depend
// on. Config and Logger are wired to their real SPEC-0003/SPEC-0005
// implementations; the remaining slots are typed placeholders until their
// owning specs are implemented, and stay nil unless supplied via options.
type Container struct {
	Config *cfgpkg.Config
	Logger *logger.Logger

	EventBus      EventBus
	TaskManager   TaskManager
	ToolRegistry  ToolRegistry
	AgentRegistry AgentRegistry
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
