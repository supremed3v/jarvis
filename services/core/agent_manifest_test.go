package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"jarvis-pa/packages/errors"
	types "jarvis-pa/packages/shared-types"
)

func writeManifest(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "manifest.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	return path
}

// TestLoadManifest_LoadsCorrectly verifies a well-formed manifest (matching
// SPEC-0019's own example) loads with every declared field populated.
func TestLoadManifest_LoadsCorrectly(t *testing.T) {
	path := writeManifest(t, `
name: developer_agent
description: Writes and edits code
capabilities:
  - code_generation
tools:
  - filesystem
  - terminal
permissions:
  terminal:
    require_confirmation: true
model:
  provider: ollama
  name: llama3
  temperature: 0.2
config:
  max_steps: 10
`)

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest returned error: %v", err)
	}

	if m.Name != "developer_agent" {
		t.Errorf("Name = %q, want developer_agent", m.Name)
	}
	if len(m.Tools) != 2 || m.Tools[0] != "filesystem" || m.Tools[1] != "terminal" {
		t.Errorf("Tools = %v, want [filesystem terminal]", m.Tools)
	}
	perm, ok := m.Permissions["terminal"]
	if !ok || !perm.RequireConfirmation {
		t.Errorf("Permissions[terminal] = %+v, ok=%v, want RequireConfirmation=true", perm, ok)
	}
	if m.Model.Provider != "ollama" || m.Model.Name != "llama3" {
		t.Errorf("Model = %+v, want provider=ollama name=llama3", m.Model)
	}
	if m.Config["max_steps"] != 10 {
		t.Errorf("Config[max_steps] = %v, want 10", m.Config["max_steps"])
	}
}

// TestLoadManifest_InvalidManifestsFailValidation verifies malformed or
// semantically invalid manifests are rejected rather than silently loaded.
func TestLoadManifest_InvalidManifestsFailValidation(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		wantCode string
	}{
		{
			name:     "missing name",
			contents: "tools:\n  - filesystem\n",
			wantCode: "MANIFEST_MISSING_NAME",
		},
		{
			name: "permission for undeclared tool",
			contents: `
name: developer_agent
tools:
  - filesystem
permissions:
  terminal:
    require_confirmation: true
`,
			wantCode: "MANIFEST_PERMISSION_UNKNOWN_TOOL",
		},
		{
			name:     "malformed yaml",
			contents: "name: [unterminated\n",
			wantCode: "MANIFEST_PARSE_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeManifest(t, tt.contents)
			_, err := LoadManifest(path)
			if err == nil {
				t.Fatalf("LoadManifest() = nil, want error with code %s", tt.wantCode)
			}
			if !errors.HasCode(err, tt.wantCode) {
				t.Errorf("missing code %s: %v", tt.wantCode, err)
			}
		})
	}
}

// TestLoadManifest_MissingFile verifies a missing manifest path is reported
// as a distinct not-found error rather than a parse failure.
func TestLoadManifest_MissingFile(t *testing.T) {
	_, err := LoadManifest(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("LoadManifest() = nil, want error")
	}
	if !errors.HasCode(err, "MANIFEST_READ_FAILED") {
		t.Errorf("missing code MANIFEST_READ_FAILED: %v", err)
	}
}

// TestNewAgentFromManifest_AgentsCanBeCreatedFromManifests verifies a valid
// Manifest plus an Executor produces a working SPEC-0018 Agent.
func TestNewAgentFromManifest_AgentsCanBeCreatedFromManifests(t *testing.T) {
	path := writeManifest(t, `
name: developer_agent
tools:
  - filesystem
  - terminal
permissions:
  terminal:
    require_confirmation: true
`)
	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest returned error: %v", err)
	}

	var agent Agent
	agent, err = NewAgentFromManifest(m, func(ctx context.Context, task *types.Task) (map[string]any, error) {
		return map[string]any{"ok": true}, nil
	})
	if err != nil {
		t.Fatalf("NewAgentFromManifest returned error: %v", err)
	}

	meta := agent.Metadata()
	if meta.ID != "developer_agent" || meta.Name != "developer_agent" {
		t.Errorf("Metadata() = %+v, want ID=Name=developer_agent", meta)
	}
	if len(meta.Permissions) != 1 || meta.Permissions[0] != "terminal" {
		t.Errorf("Metadata().Permissions = %v, want [terminal]", meta.Permissions)
	}

	result, err := agent.Execute(context.Background(), &types.Task{ID: "task-1"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result["ok"] != true {
		t.Errorf("Execute result = %+v, want ok=true", result)
	}
}

// TestNewAgentFromManifest_RejectsInvalidInput verifies nil manifests,
// missing executors, and manifests that fail Validate are all rejected.
func TestNewAgentFromManifest_RejectsInvalidInput(t *testing.T) {
	noop := func(ctx context.Context, task *types.Task) (map[string]any, error) { return nil, nil }

	if _, err := NewAgentFromManifest(nil, noop); !errors.HasCode(err, "MANIFEST_NIL") {
		t.Errorf("nil manifest: missing code MANIFEST_NIL: %v", err)
	}

	if _, err := NewAgentFromManifest(&Manifest{Name: "a"}, nil); !errors.HasCode(err, "MANIFEST_MISSING_EXECUTOR") {
		t.Errorf("nil executor: missing code MANIFEST_MISSING_EXECUTOR: %v", err)
	}

	invalid := &Manifest{Name: ""}
	if _, err := NewAgentFromManifest(invalid, noop); !errors.HasCode(err, "MANIFEST_MISSING_NAME") {
		t.Errorf("invalid manifest: missing code MANIFEST_MISSING_NAME: %v", err)
	}
}
