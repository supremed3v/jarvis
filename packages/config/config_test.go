package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") returned error: %v", err)
	}

	want := Defaults()
	// Compare App fields
	if cfg.App != want.App {
		t.Errorf("App = %+v, want %+v", cfg.App, want.App)
	}
	// Compare Model fields (skip Maps as they can'"'"'t be directly compared)
	if cfg.Model.Provider != want.Model.Provider {
		t.Errorf("Model.Provider = %q, want %q", cfg.Model.Provider, want.Model.Provider)
	}
	if cfg.Model.OllamaHost != want.Model.OllamaHost {
		t.Errorf("Model.OllamaHost = %q, want %q", cfg.Model.OllamaHost, want.Model.OllamaHost)
	}
	if cfg.Model.OllamaPort != want.Model.OllamaPort {
		t.Errorf("Model.OllamaPort = %d, want %d", cfg.Model.OllamaPort, want.Model.OllamaPort)
	}
	if cfg.Model.NvidiaAPIKey != want.Model.NvidiaAPIKey {
		t.Errorf("Model.NvidiaAPIKey = %q, want %q", cfg.Model.NvidiaAPIKey, want.Model.NvidiaAPIKey)
	}
	if cfg.Model.NvidiaBaseURL != want.Model.NvidiaBaseURL {
		t.Errorf("Model.NvidiaBaseURL = %q, want %q", cfg.Model.NvidiaBaseURL, want.Model.NvidiaBaseURL)
	}
	if cfg.Model.DefaultModel != want.Model.DefaultModel {
		t.Errorf("Model.DefaultModel = %q, want %q", cfg.Model.DefaultModel, want.Model.DefaultModel)
	}
	if len(cfg.Model.Models) != len(want.Model.Models) {
		t.Fatalf("Model.Models = %+v, want %+v", cfg.Model.Models, want.Model.Models)
	}
	for name, wantModel := range want.Model.Models {
		got, ok := cfg.Model.Models[name]
		if !ok {
			t.Errorf("Model.Models[%q] missing, want %+v", name, wantModel)
			continue
		}
		if got.Provider != wantModel.Provider || got.Name != wantModel.Name || got.Temperature != wantModel.Temperature || got.MaxTokens != wantModel.MaxTokens {
			t.Errorf("Model.Models[%q] = %+v, want %+v", name, got, wantModel)
		}
	}
	// Compare Tools
	if cfg.Tools != want.Tools {
		t.Errorf("Tools = %+v, want %+v", cfg.Tools, want.Tools)
	}
	// Compare Features
	if len(cfg.Features) != 0 {
		t.Errorf("Features = %+v, want empty", cfg.Features)
	}
}

func TestLoad_MissingFileFallsBackToDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("Load with missing file returned error: %v", err)
	}
	if cfg.Model.Provider != "ollama" {
		t.Fatalf("expected defaults to apply when file is missing, got provider %q", cfg.Model.Provider)
	}
}

func TestLoad_FromFileOverridesDefaults(t *testing.T) {
	path := writeTempConfig(t, `{"model":{"ollamaPort":9999},"features":{"voice":true}}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q) returned error: %v", path, err)
	}
	if cfg.Model.OllamaPort != 9999 {
		t.Errorf("OllamaPort = %d, want 9999", cfg.Model.OllamaPort)
	}
	if cfg.Model.Provider != "ollama" {
		t.Errorf("Provider = %q, want default %q to survive partial override", cfg.Model.Provider, "ollama")
	}
	if !cfg.Features["voice"] {
		t.Errorf("Features[voice] = false, want true")
	}
}

func TestLoad_InvalidJSONFailsSafely(t *testing.T) {
	path := writeTempConfig(t, `{not valid json`)

	cfg, err := Load(path)
	if err == nil {
		t.Fatalf("Load(%q) with malformed JSON returned no error, cfg=%+v", path, cfg)
	}
	if cfg != nil {
		t.Fatalf("Load(%q) with malformed JSON returned non-nil cfg: %+v", path, cfg)
	}
}

func TestLoad_EnvOverrideWinsOverFileAndDefaults(t *testing.T) {
	path := writeTempConfig(t, `{"model":{"ollamaPort":9999}}`)

	t.Setenv("OLLAMA_PORT", "7000")
	t.Setenv("JARVIS_ENV", "production")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q) returned error: %v", path, err)
	}
	if cfg.Model.OllamaPort != 7000 {
		t.Errorf("OllamaPort = %d, want env override 7000", cfg.Model.OllamaPort)
	}
	if cfg.App.Environment != "production" {
		t.Errorf("Environment = %q, want env override %q", cfg.App.Environment, "production")
	}
}

func TestLoad_InvalidEnvValueFailsSafely(t *testing.T) {
	t.Setenv("OLLAMA_PORT", "not-a-number")

	cfg, err := Load("")
	if err == nil {
		t.Fatalf("Load with invalid OLLAMA_PORT returned no error, cfg=%+v", cfg)
	}
	if cfg != nil {
		t.Fatalf("Load with invalid OLLAMA_PORT returned non-nil cfg: %+v", cfg)
	}
}

func TestLoad_ValidationRejectsOutOfRangePort(t *testing.T) {
	path := writeTempConfig(t, `{"model":{"ollamaPort":70000}}`)

	cfg, err := Load(path)
	if err == nil {
		t.Fatalf("Load(%q) with out-of-range port returned no error, cfg=%+v", path, cfg)
	}
	if cfg != nil {
		t.Fatalf("Load(%q) with out-of-range port returned non-nil cfg: %+v", path, cfg)
	}
}

func TestModelConfig_ModelFor(t *testing.T) {
	mc := Defaults().Model // general: qwen, coding: qwen-coder, default "general"
	mc.AgentModels = map[string]string{"developer-agent": "coding"}

	got, err := mc.ModelFor("developer-agent")
	if err != nil {
		t.Fatalf("ModelFor(developer-agent) returned error: %v", err)
	}
	if got.Name != "qwen-coder" {
		t.Errorf("ModelFor(developer-agent).Name = %q, want %q (agent override)", got.Name, "qwen-coder")
	}

	got, err = mc.ModelFor("core-agent")
	if err != nil {
		t.Fatalf("ModelFor(core-agent) returned error: %v", err)
	}
	if got.Name != "qwen" {
		t.Errorf("ModelFor(core-agent).Name = %q, want %q (falls back to DefaultModel)", got.Name, "qwen")
	}
}

func TestModelConfig_ModelFor_UnknownKeyFails(t *testing.T) {
	mc := ModelConfig{DefaultModel: "missing", Models: map[string]Model{}}

	if _, err := mc.ModelFor("any-agent"); err == nil {
		t.Fatalf("ModelFor with unresolvable key returned no error")
	}
}

func TestLoad_ValidationRejectsUnsupportedModelProvider(t *testing.T) {
	path := writeTempConfig(t, `{"model":{"models":{"general":{"provider":"openai","name":"gpt-4"}},"defaultModel":"general"}}`)

	if cfg, err := Load(path); err == nil {
		t.Fatalf("Load(%q) with non-ollama model provider returned no error, cfg=%+v", path, cfg)
	}
}

func TestLoad_ValidationRejectsMissingModelName(t *testing.T) {
	path := writeTempConfig(t, `{"model":{"models":{"general":{"provider":"ollama","name":""}},"defaultModel":"general"}}`)

	if cfg, err := Load(path); err == nil {
		t.Fatalf("Load(%q) with empty model name returned no error, cfg=%+v", path, cfg)
	}
}

func TestLoad_ValidationRejectsOutOfRangeTemperature(t *testing.T) {
	path := writeTempConfig(t, `{"model":{"models":{"general":{"provider":"ollama","name":"qwen","temperature":5}},"defaultModel":"general"}}`)

	if cfg, err := Load(path); err == nil {
		t.Fatalf("Load(%q) with out-of-range temperature returned no error, cfg=%+v", path, cfg)
	}
}

func TestLoad_ValidationRejectsUnresolvableDefaultModel(t *testing.T) {
	path := writeTempConfig(t, `{"model":{"defaultModel":"does-not-exist"}}`)

	if cfg, err := Load(path); err == nil {
		t.Fatalf("Load(%q) with unresolvable defaultModel returned no error, cfg=%+v", path, cfg)
	}
}

func TestLoad_ValidationRejectsMissingDefaultModel(t *testing.T) {
	path := writeTempConfig(t, `{"model":{"defaultModel":""}}`)

	if cfg, err := Load(path); err == nil {
		t.Fatalf("Load(%q) with empty defaultModel and non-empty models returned no error, cfg=%+v", path, cfg)
	}
}

func TestLoad_ValidationRejectsUnresolvableAgentModel(t *testing.T) {
	path := writeTempConfig(t, `{"model":{"agentModels":{"developer-agent":"does-not-exist"}}}`)

	if cfg, err := Load(path); err == nil {
		t.Fatalf("Load(%q) with unresolvable agentModels entry returned no error, cfg=%+v", path, cfg)
	}
}

func TestLoad_AgentModelOverrideAppliesFromFile(t *testing.T) {
	path := writeTempConfig(t, `{"model":{"agentModels":{"developer-agent":"coding"}}}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q) returned error: %v", path, err)
	}
	model, err := cfg.Model.ModelFor("developer-agent")
	if err != nil {
		t.Fatalf("ModelFor(developer-agent) returned error: %v", err)
	}
	if model.Name != "qwen-coder" {
		t.Errorf("ModelFor(developer-agent).Name = %q, want %q", model.Name, "qwen-coder")
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}
	return path
}
