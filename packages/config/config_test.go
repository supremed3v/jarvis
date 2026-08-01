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
	if cfg.App != want.App {
		t.Errorf("App = %+v, want %+v", cfg.App, want.App)
	}
	if cfg.Model != want.Model {
		t.Errorf("Model = %+v, want %+v", cfg.Model, want.Model)
	}
	if cfg.Tools != want.Tools {
		t.Errorf("Tools = %+v, want %+v", cfg.Tools, want.Tools)
	}
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

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}
	return path
}
