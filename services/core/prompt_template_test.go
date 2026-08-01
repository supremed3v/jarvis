package core

import (
	"strings"
	"testing"

	"jarvis-pa/packages/errors"
	types "jarvis-pa/packages/shared-types"
)

// TestPromptTemplate_Render_InjectsVariables verifies "Prompts render
// correctly" and "Variables inject correctly": every referenced
// PromptVariables field appears in the rendered output.
func TestPromptTemplate_Render_InjectsVariables(t *testing.T) {
	tmpl := PromptTemplate{
		Name:    "agent-turn",
		Version: 1,
		Kind:    PromptKindSystem,
		Body:    "User context: {{.UserContext}}\nTask: {{.TaskInformation}}\nMemories: {{.Memories}}\nTools: {{.AvailableTools}}\nInstructions: {{.Instructions}}",
	}
	vars := PromptVariables{
		UserContext:     "What's on my calendar today?",
		TaskInformation: `task "t1" (demo): Do the thing [status=executing]`,
		Memories:        "user prefers concise answers",
		AvailableTools:  "filesystem",
		Instructions:    "Be helpful and concise.",
	}

	got, err := tmpl.Render(vars)
	if err != nil {
		t.Fatalf("Render() error = %v, want nil", err)
	}

	for _, want := range []string{vars.UserContext, vars.TaskInformation, vars.Memories, vars.AvailableTools, vars.Instructions} {
		if !strings.Contains(got, want) {
			t.Errorf("Render() output %q does not contain %q", got, want)
		}
	}
}

// TestPromptTemplate_Render_InjectsExtraVariables verifies a Body can
// reference Extra's map entries directly.
func TestPromptTemplate_Render_InjectsExtraVariables(t *testing.T) {
	tmpl := PromptTemplate{
		Name:    "extra-vars",
		Version: 1,
		Body:    "Agent: {{.Extra.agentName}}",
	}
	vars := PromptVariables{Extra: map[string]string{"agentName": "core-agent"}}

	got, err := tmpl.Render(vars)
	if err != nil {
		t.Fatalf("Render() error = %v, want nil", err)
	}
	if got != "Agent: core-agent" {
		t.Errorf("Render() = %q, want %q", got, "Agent: core-agent")
	}
}

// TestPromptTemplate_Render_UnknownFieldFails verifies a Body referencing a
// field PromptVariables doesn't have fails Render instead of silently
// rendering "<no value>".
func TestPromptTemplate_Render_UnknownFieldFails(t *testing.T) {
	tmpl := PromptTemplate{Name: "bad", Version: 1, Body: "{{.NotAField}}"}

	_, err := tmpl.Render(PromptVariables{})
	if err == nil {
		t.Fatal("Render() error = nil, want error for unknown field")
	}
	if !errors.Is(err, errors.TypeInvalidInput) {
		t.Errorf("Render() error type = %v, want TypeInvalidInput", err)
	}
}

// TestPromptTemplate_Render_MalformedBodyFailsToParse verifies a Body with
// invalid template syntax fails Render with a TypeInvalidInput error rather
// than panicking.
func TestPromptTemplate_Render_MalformedBodyFailsToParse(t *testing.T) {
	tmpl := PromptTemplate{Name: "malformed", Version: 1, Body: "{{.UserContext"}

	_, err := tmpl.Render(PromptVariables{})
	if err == nil {
		t.Fatal("Render() error = nil, want error for malformed body")
	}
	if !errors.Is(err, errors.TypeInvalidInput) {
		t.Errorf("Render() error type = %v, want TypeInvalidInput", err)
	}
}

// TestPromptTemplate_Render_InvalidTemplateFails verifies Render refuses an
// invalid PromptTemplate (e.g. missing Name) before ever touching
// text/template.
func TestPromptTemplate_Render_InvalidTemplateFails(t *testing.T) {
	_, err := PromptTemplate{Version: 1, Body: "hello"}.Render(PromptVariables{})
	if err == nil {
		t.Fatal("Render() error = nil, want error for missing Name")
	}
	if !errors.Is(err, errors.TypeInvalidInput) {
		t.Errorf("Render() error type = %v, want TypeInvalidInput", err)
	}
}

// TestPromptTemplate_Validate verifies each required field is checked.
func TestPromptTemplate_Validate(t *testing.T) {
	cases := []struct {
		name    string
		tmpl    PromptTemplate
		wantErr bool
	}{
		{"valid", PromptTemplate{Name: "n", Version: 1, Body: "b"}, false},
		{"missing name", PromptTemplate{Version: 1, Body: "b"}, true},
		{"zero version", PromptTemplate{Name: "n", Body: "b"}, true},
		{"negative version", PromptTemplate{Name: "n", Version: -1, Body: "b"}, true},
		{"missing body", PromptTemplate{Name: "n", Version: 1}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.tmpl.Validate()
			if (err != nil) != c.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}

// TestVariablesFromContext_MapsSpecNamedSections verifies SPEC-0031's four
// example variables map to their corresponding SPEC-0023 Context sections.
func TestVariablesFromContext_MapsSpecNamedSections(t *testing.T) {
	task := &types.Task{ID: "t1", Title: "Do the thing", Type: "demo", Status: types.TaskStatusExecuting}
	input := ContextInput{
		UserMessage:         "What's on my calendar today?",
		ConversationHistory: []string{"user: hi", "assistant: hello"},
		Memories:            []string{"user prefers concise answers"},
		Task:                task,
		AvailableTools:      []string{"filesystem", "browser"},
	}
	ctx := NewContextBuilder().Build(input)

	vars := VariablesFromContext(ctx)

	if !strings.Contains(vars.UserContext, input.UserMessage) {
		t.Errorf("UserContext = %q, want to contain %q", vars.UserContext, input.UserMessage)
	}
	if !strings.Contains(vars.UserContext, "user: hi") {
		t.Errorf("UserContext = %q, want to contain conversation history", vars.UserContext)
	}
	if !strings.Contains(vars.TaskInformation, "Do the thing") {
		t.Errorf("TaskInformation = %q, want to contain task title", vars.TaskInformation)
	}
	if vars.Memories != "user prefers concise answers" {
		t.Errorf("Memories = %q, want %q", vars.Memories, "user prefers concise answers")
	}
	if !strings.Contains(vars.AvailableTools, "filesystem") || !strings.Contains(vars.AvailableTools, "browser") {
		t.Errorf("AvailableTools = %q, want to contain both tools", vars.AvailableTools)
	}
}

// TestVariablesFromContext_EmptyContextProducesEmptyVariables verifies an
// empty Context yields empty variable strings, not placeholder text.
func TestVariablesFromContext_EmptyContextProducesEmptyVariables(t *testing.T) {
	vars := VariablesFromContext(Context{})
	if vars.UserContext != "" || vars.TaskInformation != "" || vars.Memories != "" || vars.AvailableTools != "" {
		t.Errorf("VariablesFromContext(empty) = %+v, want all empty", vars)
	}
}

// TestPromptRegistry_RegisterAndGet verifies a registered template can be
// retrieved by its exact name and version.
func TestPromptRegistry_RegisterAndGet(t *testing.T) {
	r := NewPromptRegistry()
	tmpl := PromptTemplate{Name: "system", Version: 1, Body: "hello"}

	if err := r.Register(tmpl); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	got, err := r.Get("system", 1)
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got != tmpl {
		t.Errorf("Get() = %+v, want %+v", got, tmpl)
	}
}

// TestPromptRegistry_Register_DuplicateVersionFails verifies re-registering
// the same name+version is rejected rather than silently overwriting.
func TestPromptRegistry_Register_DuplicateVersionFails(t *testing.T) {
	r := NewPromptRegistry()
	tmpl := PromptTemplate{Name: "system", Version: 1, Body: "hello"}
	if err := r.Register(tmpl); err != nil {
		t.Fatalf("first Register() error = %v, want nil", err)
	}

	err := r.Register(PromptTemplate{Name: "system", Version: 1, Body: "different body"})
	if err == nil {
		t.Fatal("second Register() error = nil, want TypeAlreadyExists error")
	}
	if !errors.Is(err, errors.TypeAlreadyExists) {
		t.Errorf("Register() error type = %v, want TypeAlreadyExists", err)
	}
}

// TestPromptRegistry_Register_InvalidTemplateFails verifies Register
// delegates to PromptTemplate.Validate.
func TestPromptRegistry_Register_InvalidTemplateFails(t *testing.T) {
	r := NewPromptRegistry()
	if err := r.Register(PromptTemplate{Version: 1, Body: "b"}); err == nil {
		t.Fatal("Register() error = nil, want error for missing Name")
	}
}

// TestPromptRegistry_Get_UnknownNameOrVersionFails verifies Get reports
// TypeNotFound for both an unregistered name and an unregistered version of
// a known name.
func TestPromptRegistry_Get_UnknownNameOrVersionFails(t *testing.T) {
	r := NewPromptRegistry()
	if err := r.Register(PromptTemplate{Name: "system", Version: 1, Body: "hello"}); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	if _, err := r.Get("unknown", 1); !errors.Is(err, errors.TypeNotFound) {
		t.Errorf("Get(unknown name) error = %v, want TypeNotFound", err)
	}
	if _, err := r.Get("system", 2); !errors.Is(err, errors.TypeNotFound) {
		t.Errorf("Get(unknown version) error = %v, want TypeNotFound", err)
	}
}

// TestPromptRegistry_Latest verifies "Versions can be tracked": Latest
// returns the highest registered version regardless of registration order.
func TestPromptRegistry_Latest(t *testing.T) {
	r := NewPromptRegistry()
	for _, v := range []int{2, 1, 3} {
		if err := r.Register(PromptTemplate{Name: "system", Version: v, Body: "hello"}); err != nil {
			t.Fatalf("Register(version %d) error = %v, want nil", v, err)
		}
	}

	got, err := r.Latest("system")
	if err != nil {
		t.Fatalf("Latest() error = %v, want nil", err)
	}
	if got.Version != 3 {
		t.Errorf("Latest().Version = %d, want 3", got.Version)
	}
}

// TestPromptRegistry_Latest_UnknownNameFails verifies Latest reports
// TypeNotFound for a name with no registered versions.
func TestPromptRegistry_Latest_UnknownNameFails(t *testing.T) {
	r := NewPromptRegistry()
	if _, err := r.Latest("unknown"); !errors.Is(err, errors.TypeNotFound) {
		t.Errorf("Latest(unknown) error = %v, want TypeNotFound", err)
	}
}

// TestPromptRegistry_Versions verifies every registered version is
// returned, sorted ascending.
func TestPromptRegistry_Versions(t *testing.T) {
	r := NewPromptRegistry()
	for _, v := range []int{3, 1, 2} {
		if err := r.Register(PromptTemplate{Name: "system", Version: v, Body: "hello"}); err != nil {
			t.Fatalf("Register(version %d) error = %v, want nil", v, err)
		}
	}

	got, err := r.Versions("system")
	if err != nil {
		t.Fatalf("Versions() error = %v, want nil", err)
	}
	want := []int{1, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("Versions() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Versions()[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

// TestPromptRegistry_Versions_UnknownNameFails verifies Versions reports
// TypeNotFound for a name with no registered versions.
func TestPromptRegistry_Versions_UnknownNameFails(t *testing.T) {
	r := NewPromptRegistry()
	if _, err := r.Versions("unknown"); !errors.Is(err, errors.TypeNotFound) {
		t.Errorf("Versions(unknown) error = %v, want TypeNotFound", err)
	}
}

// TestPromptRegistry_IndependentNamesDoNotCollide verifies versions of one
// name never appear under another.
func TestPromptRegistry_IndependentNamesDoNotCollide(t *testing.T) {
	r := NewPromptRegistry()
	if err := r.Register(PromptTemplate{Name: "system", Version: 1, Body: "a"}); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	if err := r.Register(PromptTemplate{Name: "instructions", Version: 1, Body: "b"}); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	if _, err := r.Get("system", 1); err != nil {
		t.Errorf("Get(system, 1) error = %v, want nil", err)
	}
	if _, err := r.Get("instructions", 1); err != nil {
		t.Errorf("Get(instructions, 1) error = %v, want nil", err)
	}
}
