package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"jarvis-pa/packages/errors"
)

func writeToolManifest(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "manifest.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	return path
}

// TestLoadToolManifest_LoadsCorrectly verifies a well-formed manifest
// (matching SPEC-0044's own example) loads with every declared field
// populated.
func TestLoadToolManifest_LoadsCorrectly(t *testing.T) {
	path := writeToolManifest(t, `
name: filesystem
description: Reads and writes files on disk
capabilities:
  - read_file
  - write_file
input:
  - name: path
    type: string
    required: true
permissions:
  - filesystem.read
  - filesystem.write
config:
  max_file_size_mb: 10
`)

	m, err := LoadToolManifest(path)
	if err != nil {
		t.Fatalf("LoadToolManifest returned error: %v", err)
	}

	if m.Name != "filesystem" {
		t.Errorf("Name = %q, want filesystem", m.Name)
	}
	if len(m.Capabilities) != 2 || m.Capabilities[0] != "read_file" || m.Capabilities[1] != "write_file" {
		t.Errorf("Capabilities = %v, want [read_file write_file]", m.Capabilities)
	}
	if len(m.Input) != 1 || m.Input[0].Name != "path" || m.Input[0].Type != "string" || !m.Input[0].Required {
		t.Errorf("Input = %+v, want [{path string true}]", m.Input)
	}
	if len(m.Permissions) != 2 || m.Permissions[0] != "filesystem.read" || m.Permissions[1] != "filesystem.write" {
		t.Errorf("Permissions = %v, want [filesystem.read filesystem.write]", m.Permissions)
	}
	if m.Config["max_file_size_mb"] != 10 {
		t.Errorf("Config[max_file_size_mb] = %v, want 10", m.Config["max_file_size_mb"])
	}
}

// TestLoadToolManifest_InvalidManifestsFail verifies malformed or
// semantically invalid manifests are rejected rather than silently loaded.
func TestLoadToolManifest_InvalidManifestsFail(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		wantCode string
	}{
		{
			name:     "missing name",
			contents: "permissions:\n  - filesystem.read\n",
			wantCode: "TOOL_MANIFEST_MISSING_NAME",
		},
		{
			name: "input field missing name",
			contents: `
name: filesystem
input:
  - type: string
    required: true
`,
			wantCode: "TOOL_MANIFEST_INPUT_MISSING_NAME",
		},
		{
			name:     "malformed yaml",
			contents: "name: [unterminated\n",
			wantCode: "TOOL_MANIFEST_PARSE_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeToolManifest(t, tt.contents)
			_, err := LoadToolManifest(path)
			if err == nil {
				t.Fatalf("LoadToolManifest() = nil, want error with code %s", tt.wantCode)
			}
			if !errors.HasCode(err, tt.wantCode) {
				t.Errorf("missing code %s: %v", tt.wantCode, err)
			}
		})
	}
}

// TestLoadToolManifest_MinimalSpecExample verifies the literal minimal
// manifest from SPEC-0044's own example (just name + permissions, every
// other field left at its zero value) loads and converts to a valid
// ToolMetadata - the empty InputSchema edge case must validate any input
// as satisfied, since no fields were declared Required.
func TestLoadToolManifest_MinimalSpecExample(t *testing.T) {
	path := writeToolManifest(t, `
name: filesystem
permissions:
  - filesystem.read
  - filesystem.write
`)

	m, err := LoadToolManifest(path)
	if err != nil {
		t.Fatalf("LoadToolManifest returned error: %v", err)
	}
	if m.Description != "" || m.Capabilities != nil || m.Input != nil || m.Config != nil {
		t.Errorf("unset fields should stay zero-valued, got %+v", m)
	}

	meta := m.Metadata()
	if err := meta.Validate(); err != nil {
		t.Errorf("Metadata().Validate(): %v", err)
	}
	if err := meta.InputSchema.Validate(map[string]any{"anything": "goes"}); err != nil {
		t.Errorf("empty InputSchema.Validate() should accept any input, got: %v", err)
	}
	if err := meta.InputSchema.Validate(nil); err != nil {
		t.Errorf("empty InputSchema.Validate(nil) should accept nil input, got: %v", err)
	}
}

// TestLoadToolManifest_MissingFile verifies a missing manifest path is
// reported as a distinct not-found error rather than a parse failure.
func TestLoadToolManifest_MissingFile(t *testing.T) {
	_, err := LoadToolManifest(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("LoadToolManifest() = nil, want error")
	}
	if !errors.HasCode(err, "TOOL_MANIFEST_READ_FAILED") {
		t.Errorf("missing code TOOL_MANIFEST_READ_FAILED: %v", err)
	}
}

// TestNewToolFromManifest_ToolsCanBeCreatedFromManifests verifies a valid
// ToolManifest plus a ToolExecutor produces a working SPEC-0043 Tool.
func TestNewToolFromManifest_ToolsCanBeCreatedFromManifests(t *testing.T) {
	path := writeToolManifest(t, `
name: filesystem
description: Reads and writes files on disk
input:
  - name: path
    type: string
    required: true
permissions:
  - filesystem.read
`)
	m, err := LoadToolManifest(path)
	if err != nil {
		t.Fatalf("LoadToolManifest returned error: %v", err)
	}

	tool, err := NewToolFromManifest(m, func(ctx context.Context, input map[string]any) (map[string]any, error) {
		return map[string]any{"contents": "hello"}, nil
	})
	if err != nil {
		t.Fatalf("NewToolFromManifest returned error: %v", err)
	}

	meta := tool.Metadata()
	if meta.ID != "filesystem" || meta.Name != "filesystem" {
		t.Errorf("Metadata() = %+v, want ID=Name=filesystem", meta)
	}
	if len(meta.Permissions) != 1 || meta.Permissions[0] != "filesystem.read" {
		t.Errorf("Metadata().Permissions = %v, want [filesystem.read]", meta.Permissions)
	}
	if err := meta.InputSchema.Validate(map[string]any{"path": "/tmp/x"}); err != nil {
		t.Errorf("InputSchema.Validate with path set: %v", err)
	}
	if err := meta.InputSchema.Validate(map[string]any{}); err == nil {
		t.Error("InputSchema.Validate without path: want error, got nil")
	}

	result, err := tool.Execute(context.Background(), map[string]any{"path": "/tmp/x"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result["contents"] != "hello" {
		t.Errorf("Execute result = %+v, want contents=hello", result)
	}
}

// TestNewToolFromManifest_RejectsInvalidInput verifies nil manifests,
// missing executors, and manifests that fail Validate are all rejected.
func TestNewToolFromManifest_RejectsInvalidInput(t *testing.T) {
	noop := func(ctx context.Context, input map[string]any) (map[string]any, error) { return nil, nil }

	if _, err := NewToolFromManifest(nil, noop); !errors.HasCode(err, "TOOL_MANIFEST_NIL") {
		t.Errorf("nil manifest: missing code TOOL_MANIFEST_NIL: %v", err)
	}

	if _, err := NewToolFromManifest(&ToolManifest{Name: "a"}, nil); !errors.HasCode(err, "TOOL_MANIFEST_MISSING_EXECUTOR") {
		t.Errorf("nil executor: missing code TOOL_MANIFEST_MISSING_EXECUTOR: %v", err)
	}

	invalid := &ToolManifest{Name: ""}
	if _, err := NewToolFromManifest(invalid, noop); !errors.HasCode(err, "TOOL_MANIFEST_MISSING_NAME") {
		t.Errorf("invalid manifest: missing code TOOL_MANIFEST_MISSING_NAME: %v", err)
	}
}
