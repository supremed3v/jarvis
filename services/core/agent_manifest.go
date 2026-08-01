// agent_manifest.go implements SPEC-0019: the Agent Manifest System. A
// Manifest is the configuration-based description of an agent - identity,
// capabilities, tools, permissions, model preferences, and free-form
// configuration - loaded from a YAML file rather than constructed in code.
// NewAgentFromManifest turns a validated Manifest plus a caller-supplied
// Executor into a concrete SPEC-0018 Agent, since a manifest only declares
// what an agent is, not the Task-handling logic behind Execute.
package core

import (
	"context"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"

	"jarvis-pa/packages/errors"
	types "jarvis-pa/packages/shared-types"
)

// ManifestPermission is a single tool's permission rule declared in an
// agent manifest (e.g. `require_confirmation: true` under `permissions:
// terminal:`).
type ManifestPermission struct {
	RequireConfirmation bool `yaml:"require_confirmation"`
}

// ManifestModel captures an agent's model preferences (SPEC-0019's "Model
// preferences" requirement).
type ManifestModel struct {
	Provider    string  `yaml:"provider"`
	Name        string  `yaml:"name"`
	Temperature float64 `yaml:"temperature"`
}

// Manifest is the SPEC-0019 configuration-based description of an agent:
// identity (Name/Description), Capabilities, Tools, Permissions, Model
// preferences, and free-form Configuration.
type Manifest struct {
	Name         string                        `yaml:"name"`
	Description  string                        `yaml:"description"`
	Capabilities []string                      `yaml:"capabilities"`
	Tools        []string                      `yaml:"tools"`
	Permissions  map[string]ManifestPermission `yaml:"permissions"`
	Model        ManifestModel                 `yaml:"model"`
	Config       map[string]any                `yaml:"config"`
}

// LoadManifest reads and parses the YAML manifest file at path, then
// validates it - SPEC-0019's "manifest loads correctly" / "invalid
// manifests fail validation" testing criteria.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, errors.TypeNotFound, "MANIFEST_READ_FAILED", "core.manifest",
			fmt.Sprintf("reading manifest file %s", path))
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, errors.Wrap(err, errors.TypeInvalidInput, "MANIFEST_PARSE_ERROR", "core.manifest",
			fmt.Sprintf("parsing manifest file %s", path))
	}

	if err := m.Validate(); err != nil {
		return nil, err
	}

	return &m, nil
}

// Validate reports whether m is well-formed: it must declare a Name, and
// every tool named under Permissions must also be declared in Tools (a
// permission rule for a tool the agent never uses is treated as a manifest
// error rather than silently ignored).
func (m Manifest) Validate() error {
	if m.Name == "" {
		return errors.New(errors.TypeInvalidInput, "MANIFEST_MISSING_NAME", "core.manifest",
			"agent manifest is missing a name")
	}

	declared := make(map[string]bool, len(m.Tools))
	for _, t := range m.Tools {
		declared[t] = true
	}
	for tool := range m.Permissions {
		if !declared[tool] {
			return errors.New(errors.TypeInvalidInput, "MANIFEST_PERMISSION_UNKNOWN_TOOL", "core.manifest",
				fmt.Sprintf("permission declared for tool %q which is not listed under tools", tool)).
				With("manifest", m.Name).With("tool", tool)
		}
	}

	return nil
}

// Metadata derives the SPEC-0018 AgentMetadata identity/requirements this
// manifest declares. Permissions is the sorted set of tool names the
// manifest gates behind a permission rule; the richer per-tool rule detail
// (e.g. RequireConfirmation) stays on the Manifest itself, since
// AgentMetadata's own contract only carries bare identifiers - mirroring
// SPEC-0018's existing precedent for Tools/MemoryAccess/Permissions.
func (m Manifest) Metadata() AgentMetadata {
	var perms []string
	for tool := range m.Permissions {
		perms = append(perms, tool)
	}
	sort.Strings(perms)

	return AgentMetadata{
		ID:          m.Name,
		Name:        m.Name,
		Description: m.Description,
		Tools:       m.Tools,
		Permissions: perms,
	}
}

// manifestAgent is the Agent produced by NewAgentFromManifest: its Metadata
// comes from the Manifest, its behavior from the caller-supplied Executor.
type manifestAgent struct {
	metadata AgentMetadata
	execute  Executor
}

func (a *manifestAgent) Metadata() AgentMetadata { return a.metadata }

func (a *manifestAgent) Execute(ctx context.Context, task *types.Task) (map[string]any, error) {
	return a.execute(ctx, task)
}

// NewAgentFromManifest builds a SPEC-0018 Agent from a validated Manifest
// and an Executor implementing the agent's actual behavior - SPEC-0019's
// "agents can be created from manifests" testing criterion. The manifest
// supplies identity/capabilities/tools/permissions/model/config
// declaratively, but Task execution logic is still code the caller
// provides: nothing in SPEC-0019 asks a manifest to carry executable
// behavior, and Agent's own Execute is deliberately opaque (SPEC-0018).
func NewAgentFromManifest(m *Manifest, execute Executor) (Agent, error) {
	if m == nil {
		return nil, errors.New(errors.TypeInvalidInput, "MANIFEST_NIL", "core.manifest",
			"cannot create an agent from a nil manifest")
	}
	if execute == nil {
		return nil, errors.New(errors.TypeInvalidInput, "MANIFEST_MISSING_EXECUTOR", "core.manifest",
			"cannot create an agent without an executor")
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}

	metadata := m.Metadata()
	if err := metadata.Validate(); err != nil {
		return nil, err
	}

	return &manifestAgent{metadata: metadata, execute: execute}, nil
}
