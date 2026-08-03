// tool_manifest.go implements SPEC-0044: the Tool Manifest System -
// configuration-based tool definitions loaded from YAML rather than
// constructed in code. A ToolManifest declares a tool's Identity,
// Description, Capabilities, Input requirements, Permissions, and
// Configuration (SPEC-0044's Requirements); NewToolFromManifest turns a
// validated ToolManifest plus a caller-supplied ToolExecutor into a
// concrete SPEC-0043 Tool, since a manifest only declares what a tool is,
// not the Execute logic behind it - mirroring SPEC-0019's Manifest /
// NewAgentFromManifest precedent (agent_manifest.go) for the Agent side of
// the same pattern.
package core

import (
	"context"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"jarvis-pa/packages/errors"
)

// ManifestInputField is one declared input field in a tool manifest's
// `input:` list (SPEC-0044's "Input requirements"), converted into a
// SchemaField (tool.go) by ToolManifest.Metadata.
type ManifestInputField struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Required bool   `yaml:"required"`
}

// ToolManifest is the SPEC-0044 configuration-based description of a tool:
// identity (Name/Description), Capabilities, Input requirements,
// Permissions, and free-form Configuration.
type ToolManifest struct {
	Name         string               `yaml:"name"`
	Description  string               `yaml:"description"`
	Capabilities []string             `yaml:"capabilities"`
	Input        []ManifestInputField `yaml:"input"`
	Permissions  []string             `yaml:"permissions"`
	Config       map[string]any       `yaml:"config"`
}

// LoadToolManifest reads and parses the YAML tool manifest file at path,
// then validates it - SPEC-0044's "tool manifests load correctly" /
// "invalid manifests fail" testing criteria.
func LoadToolManifest(path string) (*ToolManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, errors.TypeNotFound, "TOOL_MANIFEST_READ_FAILED", "core.tool_manifest",
			fmt.Sprintf("reading tool manifest file %s", path))
	}

	var m ToolManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, errors.Wrap(err, errors.TypeInvalidInput, "TOOL_MANIFEST_PARSE_ERROR", "core.tool_manifest",
			fmt.Sprintf("parsing tool manifest file %s", path))
	}

	if err := m.Validate(); err != nil {
		return nil, err
	}

	return &m, nil
}

// Validate reports whether m is well-formed: it must declare a Name
// (SPEC-0044's Identity requirement), and every declared input field must
// itself have a Name.
func (m ToolManifest) Validate() error {
	if m.Name == "" {
		return errors.New(errors.TypeInvalidInput, "TOOL_MANIFEST_MISSING_NAME", "core.tool_manifest",
			"tool manifest is missing a name")
	}
	for i, f := range m.Input {
		if f.Name == "" {
			return errors.New(errors.TypeInvalidInput, "TOOL_MANIFEST_INPUT_MISSING_NAME", "core.tool_manifest",
				"tool manifest input field is missing a name").With("manifest", m.Name).With("index", i)
		}
	}
	return nil
}

// Metadata derives the SPEC-0043 ToolMetadata this manifest declares: Name
// doubles as ID (mirroring agent_manifest.go's Manifest.Metadata
// precedent), Description and Permissions map straight across, and Input
// becomes an InputSchema of SchemaFields. Capabilities and Config stay on
// the ToolManifest itself - ToolMetadata's contract, like AgentMetadata's,
// carries only identity and declared requirements, not this richer
// manifest-only detail.
func (m ToolManifest) Metadata() ToolMetadata {
	schema := make(Schema, len(m.Input))
	for i, f := range m.Input {
		schema[i] = SchemaField{Name: f.Name, Type: f.Type, Required: f.Required}
	}

	return ToolMetadata{
		ID:          m.Name,
		Name:        m.Name,
		Description: m.Description,
		InputSchema: schema,
		Permissions: m.Permissions,
	}
}

// ToolExecutor implements a manifest-described tool's actual behavior - the
// Execute logic no manifest can carry declaratively. Its signature matches
// Tool.Execute (tool.go) exactly, so no adapter is needed.
type ToolExecutor func(ctx context.Context, input map[string]any) (map[string]any, error)

// manifestTool is the Tool produced by NewToolFromManifest: its Metadata
// comes from the ToolManifest, its behavior from the caller-supplied
// ToolExecutor.
type manifestTool struct {
	metadata ToolMetadata
	execute  ToolExecutor
}

func (t *manifestTool) Metadata() ToolMetadata { return t.metadata }

func (t *manifestTool) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	return t.execute(ctx, input)
}

// NewToolFromManifest builds a SPEC-0043 Tool from a validated ToolManifest
// and a ToolExecutor implementing the tool's actual behavior - SPEC-0044's
// "tools can be created from manifests" testing criterion. The manifest
// supplies identity/capabilities/input/permissions/config declaratively,
// but Execute logic is still code the caller provides: nothing in
// SPEC-0044 asks a manifest to carry executable behavior, mirroring
// SPEC-0019/NewAgentFromManifest's identical precedent on the Agent side
// (agent_manifest.go).
func NewToolFromManifest(m *ToolManifest, execute ToolExecutor) (Tool, error) {
	if m == nil {
		return nil, errors.New(errors.TypeInvalidInput, "TOOL_MANIFEST_NIL", "core.tool_manifest",
			"cannot create a tool from a nil manifest")
	}
	if execute == nil {
		return nil, errors.New(errors.TypeInvalidInput, "TOOL_MANIFEST_MISSING_EXECUTOR", "core.tool_manifest",
			"cannot create a tool without an executor")
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}

	metadata := m.Metadata()
	if err := metadata.Validate(); err != nil {
		return nil, err
	}

	return &manifestTool{metadata: metadata, execute: execute}, nil
}
