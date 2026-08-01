package core

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	cfgpkg "jarvis-pa/packages/config"
	"jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
)

// testModelConfig returns a small ModelConfig with a default model, a
// coding model, and an agent override, used by every test below.
func testModelConfig() cfgpkg.ModelConfig {
	return cfgpkg.ModelConfig{
		Models: map[string]cfgpkg.Model{
			"general": {Provider: "ollama", Name: "qwen"},
			"coding":  {Provider: "ollama", Name: "qwen-coder"},
			"fast":    {Provider: "ollama", Name: "qwen-fast"},
		},
		DefaultModel: "general",
		AgentModels: map[string]string{
			"developer-agent": "coding",
		},
	}
}

// TestModelRouter_CorrectModelsAreSelected verifies Route resolves the
// right model for each SPEC-0029 input in priority order (testing
// criterion 1).
func TestModelRouter_CorrectModelsAreSelected(t *testing.T) {
	taskModels := map[string]string{
		"coding":       "coding",
		"conversation": "fast",
	}

	tests := []struct {
		name    string
		req     RouteRequest
		wantKey string
		wantRsn RouteReason
	}{
		{
			name:    "task type routes coding to coding model",
			req:     RouteRequest{TaskType: "coding"},
			wantKey: "coding",
			wantRsn: ReasonTaskType,
		},
		{
			name:    "task type routes conversation to fast model",
			req:     RouteRequest{TaskType: "conversation"},
			wantKey: "fast",
			wantRsn: ReasonTaskType,
		},
		{
			name:    "agent type overrides task type",
			req:     RouteRequest{TaskType: "conversation", AgentType: "developer-agent"},
			wantKey: "coding",
			wantRsn: ReasonAgentOverride,
		},
		{
			name:    "user preference overrides everything",
			req:     RouteRequest{TaskType: "conversation", AgentType: "developer-agent", UserPreference: "fast"},
			wantKey: "fast",
			wantRsn: ReasonUserPreference,
		},
		{
			name:    "unknown user preference falls through instead of failing",
			req:     RouteRequest{TaskType: "coding", UserPreference: "does-not-exist"},
			wantKey: "coding",
			wantRsn: ReasonTaskType,
		},
		{
			name:    "no matching input falls back to default",
			req:     RouteRequest{TaskType: "unmapped-type"},
			wantKey: "general",
			wantRsn: ReasonDefault,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewModelRouter(testModelConfig(), nil, nil, taskModels)
			decision, err := router.Route(context.Background(), tt.req)
			if err != nil {
				t.Fatalf("Route() returned error: %v", err)
			}
			if decision.Key != tt.wantKey {
				t.Errorf("Route() Key = %q, want %q", decision.Key, tt.wantKey)
			}
			if decision.Reason != tt.wantRsn {
				t.Errorf("Route() Reason = %q, want %q", decision.Reason, tt.wantRsn)
			}
			if decision.Fallback {
				t.Errorf("Route() Fallback = true, want false")
			}
		})
	}
}

// TestModelRouter_DanglingOverridesFallThrough verifies that an
// AgentModels or task-type entry pointing at a key absent from
// ModelConfig.Models is treated as no match, not an error - resolution
// falls through to the next input instead of resolving to a broken key.
func TestModelRouter_DanglingOverridesFallThrough(t *testing.T) {
	cfg := testModelConfig()
	cfg.AgentModels["ghost-agent"] = "no-such-model"

	tests := []struct {
		name       string
		req        RouteRequest
		taskModels map[string]string
		wantKey    string
		wantRsn    RouteReason
	}{
		{
			name:    "dangling agent override falls through to default",
			req:     RouteRequest{AgentType: "ghost-agent"},
			wantKey: "general",
			wantRsn: ReasonDefault,
		},
		{
			name:       "dangling task-type mapping falls through to default",
			req:        RouteRequest{TaskType: "ghost-task"},
			taskModels: map[string]string{"ghost-task": "no-such-model"},
			wantKey:    "general",
			wantRsn:    ReasonDefault,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewModelRouter(cfg, nil, nil, tt.taskModels)
			decision, err := router.Route(context.Background(), tt.req)
			if err != nil {
				t.Fatalf("Route() returned error: %v", err)
			}
			if decision.Key != tt.wantKey || decision.Reason != tt.wantRsn {
				t.Errorf("Route() = {Key: %q, Reason: %q}, want {Key: %q, Reason: %q}",
					decision.Key, decision.Reason, tt.wantKey, tt.wantRsn)
			}
		})
	}
}

// TestModelRouter_NoResolvableModelReturnsError verifies Route reports an
// error, rather than a zero-value decision, when no input resolves to a
// configured model.
func TestModelRouter_NoResolvableModelReturnsError(t *testing.T) {
	cfg := cfgpkg.ModelConfig{Models: map[string]cfgpkg.Model{}}
	router := NewModelRouter(cfg, nil, nil, nil)

	_, err := router.Route(context.Background(), RouteRequest{TaskType: "coding"})
	if err == nil {
		t.Fatal("Route() returned nil error, want error")
	}
	if !errors.HasCode(err, "MODEL_ROUTER_NO_MODEL") {
		t.Errorf("missing code MODEL_ROUTER_NO_MODEL: %v", err)
	}
	if !errors.Is(err, errors.TypeNotFound) {
		t.Errorf("error type = %v, want TypeNotFound", err)
	}
}

// TestModelRouter_FallbackModelsWork verifies Route falls back to the
// default model when the resolved model is absent from the provider's
// available models (SPEC-0029 testing criterion 2).
func TestModelRouter_FallbackModelsWork(t *testing.T) {
	provider := &stubProvider{
		name:   "ollama",
		models: []ModelInfo{{Name: "qwen"}}, // only the default model is available
	}
	router := NewModelRouter(testModelConfig(), provider, nil, map[string]string{"coding": "coding"})

	decision, err := router.Route(context.Background(), RouteRequest{TaskType: "coding"})
	if err != nil {
		t.Fatalf("Route() returned error: %v", err)
	}
	if decision.Key != "general" {
		t.Errorf("Route() Key = %q, want %q (fallback to default)", decision.Key, "general")
	}
	if !decision.Fallback {
		t.Error("Route() Fallback = false, want true")
	}
	if decision.Reason != ReasonTaskType {
		t.Errorf("Route() Reason = %q, want %q (fallback keeps the original reason)", decision.Reason, ReasonTaskType)
	}
}

// TestModelRouter_AvailableModelIsNotFalledBackFrom verifies Route keeps
// its original choice when that model is available.
func TestModelRouter_AvailableModelIsNotFalledBackFrom(t *testing.T) {
	provider := &stubProvider{
		name:   "ollama",
		models: []ModelInfo{{Name: "qwen-coder"}, {Name: "qwen"}},
	}
	router := NewModelRouter(testModelConfig(), provider, nil, map[string]string{"coding": "coding"})

	decision, err := router.Route(context.Background(), RouteRequest{TaskType: "coding"})
	if err != nil {
		t.Fatalf("Route() returned error: %v", err)
	}
	if decision.Key != "coding" || decision.Fallback {
		t.Errorf("Route() = %+v, want Key=coding Fallback=false", decision)
	}
}

// TestModelRouter_DefaultModelUnavailableStaysWithoutFallback verifies
// that when the resolved key already equals ModelConfig.DefaultModel and
// that model is unavailable, Route does not report a fallback (there is
// nothing further to fall back to) rather than looping or clearing the
// decision.
func TestModelRouter_DefaultModelUnavailableStaysWithoutFallback(t *testing.T) {
	provider := &stubProvider{name: "ollama", models: []ModelInfo{{Name: "qwen-coder"}}} // "general" (qwen) is absent
	router := NewModelRouter(testModelConfig(), provider, nil, nil)

	decision, err := router.Route(context.Background(), RouteRequest{})
	if err != nil {
		t.Fatalf("Route() returned error: %v", err)
	}
	if decision.Key != "general" {
		t.Errorf("Route() Key = %q, want %q", decision.Key, "general")
	}
	if decision.Fallback {
		t.Error("Route() Fallback = true, want false (already at the default, nothing to fall back to)")
	}
}

// TestModelRouter_ConcurrentRouteCallsAreSafe verifies Route can be called
// concurrently without a race, since ModelRouter holds no state that
// mutates after construction.
func TestModelRouter_ConcurrentRouteCallsAreSafe(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New("model-router", logger.WithOutput(&buf))
	provider := &stubProvider{name: "ollama", models: []ModelInfo{{Name: "qwen"}, {Name: "qwen-coder"}}}
	router := NewModelRouter(testModelConfig(), provider, log, map[string]string{"coding": "coding"})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := router.Route(context.Background(), RouteRequest{TaskType: "coding"}); err != nil {
				t.Errorf("Route() returned error: %v", err)
			}
		}()
	}
	wg.Wait()
}

// TestModelRouter_ProviderErrorDoesNotForceFallback verifies a failing
// availability check is not treated as unavailability - Route keeps its
// original decision rather than guessing at an unreachable provider.
func TestModelRouter_ProviderErrorDoesNotForceFallback(t *testing.T) {
	provider := &stubProvider{
		name: "ollama",
		err:  errors.New(errors.TypeUnavailable, "OLLAMA_CONNECTION_FAILED", "core.ollama", "unreachable"),
	}
	router := NewModelRouter(testModelConfig(), provider, nil, map[string]string{"coding": "coding"})

	decision, err := router.Route(context.Background(), RouteRequest{TaskType: "coding"})
	if err != nil {
		t.Fatalf("Route() returned error: %v", err)
	}
	if decision.Key != "coding" || decision.Fallback {
		t.Errorf("Route() = %+v, want Key=coding Fallback=false", decision)
	}
}

// TestModelRouter_RoutingDecisionsAreLogged verifies every Route call
// produces a structured log entry describing the decision (SPEC-0029
// testing criterion 3).
func TestModelRouter_RoutingDecisionsAreLogged(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New("model-router", logger.WithOutput(&buf))
	router := NewModelRouter(testModelConfig(), nil, log, map[string]string{"coding": "coding"})

	_, err := router.Route(context.Background(), RouteRequest{TaskType: "coding", AgentType: "unknown-agent"})
	if err != nil {
		t.Fatalf("Route() returned error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d log lines, want 1: %v", len(lines), lines)
	}

	var entry logger.Entry
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("Unmarshal(log entry) returned error: %v, raw=%s", err, lines[0])
	}
	if entry.Message != "model router decision" {
		t.Errorf("Message = %q, want %q", entry.Message, "model router decision")
	}
	if entry.Metadata["modelKey"] != "coding" {
		t.Errorf("Metadata[modelKey] = %v, want %q", entry.Metadata["modelKey"], "coding")
	}
	if entry.Metadata["reason"] != string(ReasonTaskType) {
		t.Errorf("Metadata[reason] = %v, want %q", entry.Metadata["reason"], ReasonTaskType)
	}
	if entry.Metadata["taskType"] != "coding" {
		t.Errorf("Metadata[taskType] = %v, want %q", entry.Metadata["taskType"], "coding")
	}
}
