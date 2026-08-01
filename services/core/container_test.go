package core

import (
	"testing"

	cfgpkg "jarvis-pa/packages/config"
	"jarvis-pa/packages/logger"
)

func TestNewContainer_WiresConfigAndLogger(t *testing.T) {
	cfg := &cfgpkg.Config{}
	log := logger.New("test")

	c := NewContainer(cfg, log)

	if c.Config != cfg {
		t.Fatal("Config not wired to the value passed to NewContainer")
	}
	if c.Logger != log {
		t.Fatal("Logger not wired to the value passed to NewContainer")
	}
}

func TestNewContainer_UnwiredSlotsDefaultToNil(t *testing.T) {
	c := NewContainer(&cfgpkg.Config{}, logger.New("test"))

	if c.EventBus != nil {
		t.Fatal("EventBus should be nil until SPEC-0009 is implemented and wired")
	}
	if c.TaskManager != nil {
		t.Fatal("TaskManager should be nil until its owning spec is implemented and wired")
	}
	if c.ToolRegistry != nil {
		t.Fatal("ToolRegistry should be nil until SPEC-0045 is implemented and wired")
	}
	if c.AgentRegistry != nil {
		t.Fatal("AgentRegistry should be nil until wired via WithAgentRegistry")
	}
}

func TestNewContainer_OptionsWireStubSlots(t *testing.T) {
	eventBus := NewBus()
	taskManager := struct{ name string }{name: "fake-task-manager"}
	toolRegistry := struct{ name string }{name: "fake-tool-registry"}
	agentRegistry := NewRegistry()

	c := NewContainer(&cfgpkg.Config{}, logger.New("test"),
		WithEventBus(eventBus),
		WithTaskManager(taskManager),
		WithToolRegistry(toolRegistry),
		WithAgentRegistry(agentRegistry),
	)

	if c.EventBus != eventBus {
		t.Fatal("WithEventBus did not wire the given value")
	}
	if c.TaskManager != taskManager {
		t.Fatal("WithTaskManager did not wire the given value")
	}
	if c.ToolRegistry != toolRegistry {
		t.Fatal("WithToolRegistry did not wire the given value")
	}
	if c.AgentRegistry != agentRegistry {
		t.Fatal("WithAgentRegistry did not wire the given value")
	}
}

func TestContainer_NoPackageLevelState(t *testing.T) {
	c1 := NewContainer(&cfgpkg.Config{App: cfgpkg.AppConfig{Environment: "dev"}}, logger.New("one"))
	c2 := NewContainer(&cfgpkg.Config{App: cfgpkg.AppConfig{Environment: "prod"}}, logger.New("two"))

	if c1.Config.App.Environment == c2.Config.App.Environment {
		t.Fatal("two Containers should hold independent Config instances")
	}
}
