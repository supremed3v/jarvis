package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// Load builds a Config by layering, in order of increasing precedence:
// built-in defaults, an optional JSON config file, then environment
// variables. path may be empty, in which case the file layer is skipped.
//
// Load never panics and never returns a partially-applied Config: a
// malformed file, an invalid environment override, or a value that fails
// validation all produce a descriptive error with a nil Config instead.
func Load(path string) (*Config, error) {
	cfg := Defaults()

	if path != "" {
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			if err := json.Unmarshal(data, &cfg); err != nil {
				return nil, fmt.Errorf("config: invalid config file %s: %w", path, err)
			}
		case os.IsNotExist(err):
			// Config file is optional; fall back to defaults + env.
		default:
			return nil, fmt.Errorf("config: reading config file %s: %w", path, err)
		}
	}

	if err := applyEnvOverrides(&cfg); err != nil {
		return nil, err
	}

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func applyEnvOverrides(cfg *Config) error {
	if v, ok := os.LookupEnv("JARVIS_ENV"); ok {
		cfg.App.Environment = v
	}
	if v, ok := os.LookupEnv("LOG_LEVEL"); ok {
		cfg.App.LogLevel = v
	}
	if v, ok := os.LookupEnv("OLLAMA_HOST"); ok {
		cfg.Model.OllamaHost = v
	}
	if v, ok := os.LookupEnv("OLLAMA_PORT"); ok {
		port, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("config: invalid OLLAMA_PORT %q: %w", v, err)
		}
		cfg.Model.OllamaPort = port
	}
	if v, ok := os.LookupEnv("NVIDIA_API_KEY"); ok {
		cfg.Model.NvidiaAPIKey = v
	}
	if v, ok := os.LookupEnv("NVIDIA_API_BASE_URL"); ok {
		cfg.Model.NvidiaBaseURL = v
	}

	return nil
}

func validate(cfg *Config) error {
	if cfg.App.Environment == "" {
		return fmt.Errorf("config: app.environment must not be empty")
	}
	if cfg.App.LogLevel == "" {
		return fmt.Errorf("config: app.logLevel must not be empty")
	}
	if cfg.Model.OllamaPort < 1 || cfg.Model.OllamaPort > 65535 {
		return fmt.Errorf("config: model.ollamaPort must be between 1 and 65535, got %d", cfg.Model.OllamaPort)
	}

	return nil
}
