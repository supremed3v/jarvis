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
	if v, ok := os.LookupEnv("JARVIS_VOICE_WAKE_WORD_MODEL"); ok {
		cfg.Voice.WakeWordModelPath = v
	}
	if v, ok := os.LookupEnv("JARVIS_VOICE_STT_MODEL"); ok {
		cfg.Voice.STTModel = v
	}
	if v, ok := os.LookupEnv("JARVIS_VOICE_STT_LANGUAGE"); ok {
		cfg.Voice.STTLanguage = v
	}
	if v, ok := os.LookupEnv("JARVIS_VOICE_STT_DEVICE"); ok {
		cfg.Voice.STTDevice = v
	}
	if v, ok := os.LookupEnv("JARVIS_VOICE_TTS_MODEL"); ok {
		cfg.Voice.TTSModel = v
	}
	if v, ok := os.LookupEnv("JARVIS_VOICE_AUDIO_DEVICE"); ok {
		cfg.Voice.AudioDevice = v
	}
	if v, ok := os.LookupEnv("JARVIS_VOICE_SAMPLE_RATE"); ok {
		rate, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("config: invalid JARVIS_VOICE_SAMPLE_RATE %q: %w", v, err)
		}
		cfg.Voice.SampleRate = rate
	}
	if v, ok := os.LookupEnv("JARVIS_VOICE_TTS_SAMPLE_RATE"); ok {
		rate, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("config: invalid JARVIS_VOICE_TTS_SAMPLE_RATE %q: %w", v, err)
		}
		cfg.Voice.TTSSampleRate = rate
	}
	if v, ok := os.LookupEnv("JARVIS_VOICE_PYTHON_PATH"); ok {
		cfg.Voice.PythonPath = v
	}
	if v, ok := os.LookupEnv("JARVIS_VOICE_PIPER_BINARY"); ok {
		cfg.Voice.PiperBinary = v
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

	for name, model := range cfg.Model.Models {
		if model.Provider == "" {
			return fmt.Errorf("config: model.models[%q].provider must not be empty", name)
		}
		if model.Provider != "ollama" {
			return fmt.Errorf("config: model.models[%q].provider %q is not supported (only \"ollama\" is, per ADR-0004)", name, model.Provider)
		}
		if model.Name == "" {
			return fmt.Errorf("config: model.models[%q].name must not be empty", name)
		}
		if model.Temperature < 0 || model.Temperature > 2 {
			return fmt.Errorf("config: model.models[%q].temperature must be between 0 and 2, got %v", name, model.Temperature)
		}
		if model.MaxTokens < 0 {
			return fmt.Errorf("config: model.models[%q].maxTokens must not be negative, got %d", name, model.MaxTokens)
		}
	}

	if len(cfg.Model.Models) > 0 && cfg.Model.DefaultModel == "" {
		return fmt.Errorf("config: model.defaultModel must be set when model.models is non-empty")
	}
	if cfg.Model.DefaultModel != "" {
		if _, ok := cfg.Model.Models[cfg.Model.DefaultModel]; !ok {
			return fmt.Errorf("config: model.defaultModel %q has no entry in model.models", cfg.Model.DefaultModel)
		}
	}

	for agent, key := range cfg.Model.AgentModels {
		if _, ok := cfg.Model.Models[key]; !ok {
			return fmt.Errorf("config: model.agentModels[%q] references unknown model %q", agent, key)
		}
	}

	return nil
}
