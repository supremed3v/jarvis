// Package config implements SPEC-0003: centralized configuration handling
// for application settings, model configuration, tool permissions, feature
// flags, and environment variables.
package config

import "fmt"

// Config is the root configuration structure for JARVIS.
type Config struct {
	App      AppConfig       `json:"app"`
	Model    ModelConfig     `json:"model"`
	Tools    ToolPermissions `json:"tools"`
	Features map[string]bool `json:"features"`
}

// AppConfig holds general application settings.
type AppConfig struct {
	Environment string `json:"environment"`
	LogLevel    string `json:"logLevel"`
}

// ModelConfig holds local AI model runtime settings. Ollama is the only
// active provider per ADR-0004; the Nvidia fields are reserved for a future
// hybrid local+cloud path (SPEC-0026 / SPEC-0029) and are not wired to any
// request logic here. Models/DefaultModel/AgentModels (SPEC-0028) let
// different use cases and agents select a distinct named model instead of
// a single global one.
type ModelConfig struct {
	Provider      string `json:"provider"`
	OllamaHost    string `json:"ollamaHost"`
	OllamaPort    int    `json:"ollamaPort"`
	NvidiaAPIKey  string `json:"nvidiaApiKey"`
	NvidiaBaseURL string `json:"nvidiaBaseUrl"`

	// Models holds named model configurations (e.g. "general", "coding").
	Models map[string]Model `json:"models,omitempty"`
	// DefaultModel is the Models key used when an agent has no entry in
	// AgentModels.
	DefaultModel string `json:"defaultModel,omitempty"`
	// AgentModels maps an agent name to the Models key it should use
	// instead of DefaultModel.
	AgentModels map[string]string `json:"agentModels,omitempty"`
}

// Model is a single named model configuration: which provider/model to
// use, its sampling temperature, an output token limit, and any
// provider-specific runtime parameters.
type Model struct {
	Provider    string  `json:"provider"`
	Name        string  `json:"name"`
	Temperature float64 `json:"temperature,omitempty"`
	// MaxTokens caps generated output length (0 means use the provider's
	// default).
	MaxTokens int `json:"maxTokens,omitempty"`
	// Options carries additional provider-specific runtime parameters.
	Options map[string]any `json:"options,omitempty"`
}

// ModelFor resolves the Model an agent should use: its AgentModels
// override if one is set, otherwise DefaultModel. It returns an error if
// the resolved key has no entry in Models.
func (m ModelConfig) ModelFor(agentName string) (Model, error) {
	key := m.DefaultModel
	if override, ok := m.AgentModels[agentName]; ok {
		key = override
	}
	model, ok := m.Models[key]
	if !ok {
		return Model{}, fmt.Errorf("config: no model configured for key %q", key)
	}
	return model, nil
}

// ToolPermissions gates which MVP tool categories are enabled.
type ToolPermissions struct {
	FilesystemEnabled bool `json:"filesystemEnabled"`
	TerminalEnabled   bool `json:"terminalEnabled"`
	BrowserEnabled    bool `json:"browserEnabled"`
}

// Defaults returns the built-in configuration used when no file or
// environment override is present.
func Defaults() Config {
	return Config{
		App: AppConfig{
			Environment: "development",
			LogLevel:    "info",
		},
		Model: ModelConfig{
			Provider:      "ollama",
			OllamaHost:    "127.0.0.1",
			OllamaPort:    11434,
			NvidiaAPIKey:  "",
			NvidiaBaseURL: "https://integrate.api.nvidia.com/v1",
			Models: map[string]Model{
				"general": {
					Provider:    "ollama",
					Name:        "qwen",
					Temperature: 0.7,
				},
				"coding": {
					Provider:    "ollama",
					Name:        "qwen-coder",
					Temperature: 0.2,
				},
			},
			DefaultModel: "general",
		},
		Tools: ToolPermissions{
			FilesystemEnabled: false,
			TerminalEnabled:   false,
			BrowserEnabled:    false,
		},
		Features: map[string]bool{},
	}
}
