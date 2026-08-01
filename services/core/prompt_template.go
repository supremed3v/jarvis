// prompt_template.go implements SPEC-0031: the Prompt Template System.
// PromptTemplate turns a named, versioned template body plus a
// PromptVariables value into the final string a GenerateRequest.Prompt
// (SPEC-0026) can use directly. It covers SPEC-0031's four requirements:
// system prompts and agent instructions are both just PromptTemplates
// distinguished by Kind, dynamic variables are PromptVariables' fields
// (with VariablesFromContext bridging SPEC-0023's ContextBuilder output for
// the "user context, task information, memories, available tools" example
// variables), and prompt versions are tracked by PromptRegistry, which keeps
// every registered Version of a template rather than overwriting on
// re-registration.
package core

import (
	"sort"
	"strings"
	"sync"
	"text/template"

	"jarvis-pa/packages/errors"
)

// PromptKind identifies what role a PromptTemplate plays, per SPEC-0031's
// "System prompts" and "Agent instructions" requirements. It is metadata
// only - Render behaves identically regardless of Kind.
type PromptKind string

const (
	PromptKindSystem       PromptKind = "system"
	PromptKindInstructions PromptKind = "instructions"
)

// PromptTemplate is one named, versioned prompt template. Body is
// text/template source referencing PromptVariables' fields (e.g.
// "{{.UserContext}}", "{{.Extra.foo}}"); Version distinguishes this
// revision of Name from any other registered under the same name (SPEC-0031's
// "Prompt versions" requirement).
type PromptTemplate struct {
	Name    string
	Version int
	Kind    PromptKind
	Body    string
}

// Validate reports whether t has the minimum fields a PromptRegistry or
// Render needs: a non-empty Name, a Version of at least 1, and a non-empty
// Body. It returns a packages/errors error typed TypeInvalidInput naming the
// first missing field, or nil if t is valid.
func (t PromptTemplate) Validate() error {
	if t.Name == "" {
		return errors.New(errors.TypeInvalidInput, "PROMPT_TEMPLATE_MISSING_NAME", "core.prompttemplate",
			"prompt template is missing a Name")
	}
	if t.Version < 1 {
		return errors.New(errors.TypeInvalidInput, "PROMPT_TEMPLATE_INVALID_VERSION", "core.prompttemplate",
			"prompt template Version must be at least 1").With("template", t.Name).With("version", t.Version)
	}
	if t.Body == "" {
		return errors.New(errors.TypeInvalidInput, "PROMPT_TEMPLATE_MISSING_BODY", "core.prompttemplate",
			"prompt template is missing a Body").With("template", t.Name).With("version", t.Version)
	}
	return nil
}

// PromptVariables carries the SPEC-0031 "Dynamic variables" a PromptTemplate
// renders against. UserContext, TaskInformation, Memories, and
// AvailableTools are SPEC-0031's own example variables; Instructions holds
// an agent's AgentMetadata.Instructions (SPEC-0018) or a Manifest's
// description (SPEC-0019) for a template that injects "agent instructions"
// into a larger prompt. Extra carries anything else a caller wants to
// inject under its own key.
type PromptVariables struct {
	UserContext     string
	TaskInformation string
	Memories        string
	AvailableTools  string
	Instructions    string
	Extra           map[string]string
}

// VariablesFromContext derives a PromptVariables' UserContext,
// TaskInformation, Memories, and AvailableTools fields from a SPEC-0023
// Context's items, joining each mapped section's ContextItem.Content with
// newlines. UserContext combines ContextSectionUserMessage and
// ContextSectionConversationHistory, since together they are what "context
// about the user's request" means; PreviousResults has no named variable in
// SPEC-0031's example list, so it is left out here - a caller who needs it
// can still read it from the Context directly. Instructions and Extra are
// left zero-valued, since Context carries neither.
func VariablesFromContext(c Context) PromptVariables {
	return PromptVariables{
		UserContext:     joinSections(c, ContextSectionUserMessage, ContextSectionConversationHistory),
		TaskInformation: joinSections(c, ContextSectionTask),
		Memories:        joinSections(c, ContextSectionMemories),
		AvailableTools:  joinSections(c, ContextSectionAvailableTools),
	}
}

// joinSections concatenates the Content of every Item in c belonging to any
// of sections, in c.Items order, one per line.
func joinSections(c Context, sections ...ContextSection) string {
	want := make(map[ContextSection]bool, len(sections))
	for _, s := range sections {
		want[s] = true
	}

	var lines []string
	for _, item := range c.Items {
		if want[item.Section] {
			lines = append(lines, item.Content)
		}
	}
	return strings.Join(lines, "\n")
}

// Render fills t's Body against vars, producing the final prompt string
// (the SPEC-0031 "Prompts render correctly" / "Variables inject correctly"
// testing criteria). Body is parsed as a text/template with
// "missingkey=error", so a Body referencing a PromptVariables field that
// doesn't exist fails Render rather than silently rendering "<no value>".
// Render returns a packages/errors TypeInvalidInput error, naming t, if
// Body fails to parse or execute.
func (t PromptTemplate) Render(vars PromptVariables) (string, error) {
	if err := t.Validate(); err != nil {
		return "", err
	}

	tmpl, err := template.New(t.Name).Option("missingkey=error").Parse(t.Body)
	if err != nil {
		return "", errors.Wrap(err, errors.TypeInvalidInput, "PROMPT_TEMPLATE_PARSE_ERROR", "core.prompttemplate",
			"parsing prompt template body").With("template", t.Name).With("version", t.Version)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", errors.Wrap(err, errors.TypeInvalidInput, "PROMPT_TEMPLATE_RENDER_ERROR", "core.prompttemplate",
			"rendering prompt template").With("template", t.Name).With("version", t.Version)
	}

	return buf.String(), nil
}

// PromptRegistry stores PromptTemplates by Name, keeping every registered
// Version rather than overwriting on re-registration, so a caller can
// render against a specific historical version as well as the latest
// (SPEC-0031's "Prompt versions" requirement, "Versions can be tracked"
// testing criterion). PromptRegistry is safe for concurrent use.
type PromptRegistry struct {
	mu        sync.RWMutex
	templates map[string]map[int]PromptTemplate
}

// NewPromptRegistry creates an empty PromptRegistry.
func NewPromptRegistry() *PromptRegistry {
	return &PromptRegistry{templates: make(map[string]map[int]PromptTemplate)}
}

// Register adds t to the registry under its Name and Version. It returns a
// packages/errors error if t fails Validate, or typed TypeAlreadyExists if
// that exact Name and Version were already registered - versions are
// immutable once registered rather than silently overwritten.
func (r *PromptRegistry) Register(t PromptTemplate) error {
	if err := t.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	versions, ok := r.templates[t.Name]
	if !ok {
		versions = make(map[int]PromptTemplate)
		r.templates[t.Name] = versions
	}
	if _, exists := versions[t.Version]; exists {
		return errors.New(errors.TypeAlreadyExists, "PROMPT_TEMPLATE_VERSION_EXISTS", "core.prompttemplate",
			"prompt template version already registered").With("template", t.Name).With("version", t.Version)
	}

	versions[t.Version] = t
	return nil
}

// Get returns the registered PromptTemplate matching name and version, or a
// packages/errors TypeNotFound error if no such name/version was
// registered.
func (r *PromptRegistry) Get(name string, version int) (PromptTemplate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	versions, ok := r.templates[name]
	if !ok {
		return PromptTemplate{}, errors.New(errors.TypeNotFound, "PROMPT_TEMPLATE_NOT_FOUND", "core.prompttemplate",
			"no prompt template registered under this name").With("template", name)
	}
	t, ok := versions[version]
	if !ok {
		return PromptTemplate{}, errors.New(errors.TypeNotFound, "PROMPT_TEMPLATE_VERSION_NOT_FOUND", "core.prompttemplate",
			"no such version registered for this prompt template").With("template", name).With("version", version)
	}
	return t, nil
}

// Latest returns the highest-Version PromptTemplate registered under name,
// or a packages/errors TypeNotFound error if no version of name was
// registered.
func (r *PromptRegistry) Latest(name string) (PromptTemplate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	versions, ok := r.templates[name]
	if !ok || len(versions) == 0 {
		return PromptTemplate{}, errors.New(errors.TypeNotFound, "PROMPT_TEMPLATE_NOT_FOUND", "core.prompttemplate",
			"no prompt template registered under this name").With("template", name)
	}

	best := -1
	for v := range versions {
		if v > best {
			best = v
		}
	}
	return versions[best], nil
}

// Versions returns every registered version number for name, sorted
// ascending, or a packages/errors TypeNotFound error if no version of name
// was registered.
func (r *PromptRegistry) Versions(name string) ([]int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	versions, ok := r.templates[name]
	if !ok || len(versions) == 0 {
		return nil, errors.New(errors.TypeNotFound, "PROMPT_TEMPLATE_NOT_FOUND", "core.prompttemplate",
			"no prompt template registered under this name").With("template", name)
	}

	result := make([]int, 0, len(versions))
	for v := range versions {
		result = append(result, v)
	}
	sort.Ints(result)
	return result, nil
}
