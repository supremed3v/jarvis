// Package config implements SPEC-0003: centralized configuration handling
// for application settings, model configuration, tool permissions, feature
// flags, and environment variables.
package config

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
// request logic here.
type ModelConfig struct {
	Provider      string `json:"provider"`
	OllamaHost    string `json:"ollamaHost"`
	OllamaPort    int    `json:"ollamaPort"`
	NvidiaAPIKey  string `json:"nvidiaApiKey"`
	NvidiaBaseURL string `json:"nvidiaBaseUrl"`
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
		},
		Tools: ToolPermissions{
			FilesystemEnabled: false,
			TerminalEnabled:   false,
			BrowserEnabled:    false,
		},
		Features: map[string]bool{},
	}
}
