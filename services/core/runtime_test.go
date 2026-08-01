package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeDependency is a test Dependency that records Init/Close order and can
// be configured to fail either call.
type fakeDependency struct {
	name       string
	failInit   bool
	failClose  bool
	initCalled bool
	log        *callLog
}

func (f *fakeDependency) Name() string { return f.name }

func (f *fakeDependency) Init(ctx context.Context) error {
	f.initCalled = true
	f.log.record("init:" + f.name)
	if f.failInit {
		return errString("init failed for " + f.name)
	}
	return nil
}

func (f *fakeDependency) Close(ctx context.Context) error {
	f.log.record("close:" + f.name)
	if f.failClose {
		return errString("close failed for " + f.name)
	}
	return nil
}

type errString string

func (e errString) Error() string { return string(e) }

// callLog records call order across goroutines.
type callLog struct {
	mu    sync.Mutex
	calls []string
}

func (c *callLog) record(s string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, s)
}

func (c *callLog) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.calls...)
}

func writeConfigFile(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing test config file: %v", err)
	}
	return path
}

func TestNew_DefaultsToNotStarted(t *testing.T) {
	r := New()
	if got := r.State(); got != StateNotStarted {
		t.Fatalf("State() = %s, want %s", got, StateNotStarted)
	}
}

func TestStart_CleanStartupNoDependencies(t *testing.T) {
	r := New()
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if got := r.State(); got != StateRunning {
		t.Fatalf("State() = %s, want %s", got, StateRunning)
	}
	if r.Config == nil {
		t.Fatal("Config not populated after Start")
	}
	if r.Logger == nil {
		t.Fatal("Logger not populated after Start")
	}
}

func TestStart_InitializesDependenciesInOrder(t *testing.T) {
	log := &callLog{}
	depA := &fakeDependency{name: "a", log: log}
	depB := &fakeDependency{name: "b", log: log}

	r := New(WithDependencies(depA, depB))
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	want := []string{"init:a", "init:b"}
	if got := log.snapshot(); !equalSlices(got, want) {
		t.Fatalf("init order = %v, want %v", got, want)
	}
}

func TestStart_DependencyInitFailureRollsBackAndFails(t *testing.T) {
	log := &callLog{}
	depA := &fakeDependency{name: "a", log: log}
	depB := &fakeDependency{name: "b", log: log, failInit: true}
	depC := &fakeDependency{name: "c", log: log}

	r := New(WithDependencies(depA, depB, depC))
	err := r.Start(context.Background())
	if err == nil {
		t.Fatal("Start() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "b") {
		t.Fatalf("error %q does not reference failing dependency", err.Error())
	}
	if got := r.State(); got != StateFailed {
		t.Fatalf("State() = %s, want %s", got, StateFailed)
	}
	if depC.initCalled {
		t.Fatal("dependency c should not have been initialized after b failed")
	}

	want := []string{"init:a", "init:b", "close:a"}
	if got := log.snapshot(); !equalSlices(got, want) {
		t.Fatalf("call order = %v, want %v (a should be rolled back, c never started)", got, want)
	}
}

func TestStart_InvalidConfigFile(t *testing.T) {
	path := writeConfigFile(t, "{not valid json")
	r := New(WithConfigPath(path))

	err := r.Start(context.Background())
	if err == nil {
		t.Fatal("Start() error = nil, want error")
	}
	if got := r.State(); got != StateFailed {
		t.Fatalf("State() = %s, want %s", got, StateFailed)
	}
}

func TestStart_InvalidLogLevel(t *testing.T) {
	path := writeConfigFile(t, `{"app":{"logLevel":"NOPE"}}`)
	r := New(WithConfigPath(path))

	err := r.Start(context.Background())
	if err == nil {
		t.Fatal("Start() error = nil, want error")
	}
	if got := r.State(); got != StateFailed {
		t.Fatalf("State() = %s, want %s", got, StateFailed)
	}
}

func TestStart_AlreadyRunningReturnsError(t *testing.T) {
	r := New()
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	if err := r.Start(context.Background()); err == nil {
		t.Fatal("second Start() error = nil, want error")
	}
	if got := r.State(); got != StateRunning {
		t.Fatalf("State() = %s, want %s (unaffected by rejected restart)", got, StateRunning)
	}
}

func TestShutdown_TearsDownInReverseOrder(t *testing.T) {
	log := &callLog{}
	depA := &fakeDependency{name: "a", log: log}
	depB := &fakeDependency{name: "b", log: log}

	r := New(WithDependencies(depA, depB))
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := r.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if got := r.State(); got != StateStopped {
		t.Fatalf("State() = %s, want %s", got, StateStopped)
	}

	want := []string{"init:a", "init:b", "close:b", "close:a"}
	if got := log.snapshot(); !equalSlices(got, want) {
		t.Fatalf("call order = %v, want %v", got, want)
	}
}

func TestShutdown_NotRunningReturnsError(t *testing.T) {
	r := New()
	if err := r.Shutdown(context.Background()); err == nil {
		t.Fatal("Shutdown() error = nil, want error")
	}
}

func TestShutdown_CollectsCloseErrorsFromAllDependencies(t *testing.T) {
	log := &callLog{}
	depA := &fakeDependency{name: "a", log: log, failClose: true}
	depB := &fakeDependency{name: "b", log: log, failClose: true}

	r := New(WithDependencies(depA, depB))
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	err := r.Shutdown(context.Background())
	if err == nil {
		t.Fatal("Shutdown() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "a") || !strings.Contains(err.Error(), "b") {
		t.Fatalf("error %q does not reference both failing dependencies", err.Error())
	}
	if got := r.State(); got != StateStopped {
		t.Fatalf("State() = %s, want %s (shutdown completes best-effort despite Close errors)", got, StateStopped)
	}
}

func TestStart_RestartAfterCleanShutdown(t *testing.T) {
	r := New()
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	if err := r.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("restart Start() error = %v", err)
	}
	if got := r.State(); got != StateRunning {
		t.Fatalf("State() = %s, want %s", got, StateRunning)
	}
}

func TestRun_ShutsDownWhenContextIsDone(t *testing.T) {
	log := &callLog{}
	dep := &fakeDependency{name: "a", log: log}
	r := New(WithDependencies(dep))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()

	// Give Run a moment to reach StateRunning before triggering shutdown.
	deadline := time.Now().Add(2 * time.Second)
	for r.State() != StateRunning && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := r.State(); got != StateRunning {
		t.Fatalf("State() before cancel = %s, want %s", got, StateRunning)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}

	if got := r.State(); got != StateStopped {
		t.Fatalf("State() = %s, want %s", got, StateStopped)
	}
	want := []string{"init:a", "close:a"}
	if got := log.snapshot(); !equalSlices(got, want) {
		t.Fatalf("call order = %v, want %v", got, want)
	}
}

func TestRun_ReturnsErrorIfStartFails(t *testing.T) {
	path := writeConfigFile(t, "{not valid json")
	r := New(WithConfigPath(path))

	err := r.Run(context.Background())
	if err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	if got := r.State(); got != StateFailed {
		t.Fatalf("State() = %s, want %s", got, StateFailed)
	}
}

func TestStart_AppliesEnvironmentOverrides(t *testing.T) {
	t.Setenv("LOG_LEVEL", "ERROR")

	r := New()
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if r.Config.App.LogLevel != "ERROR" {
		t.Fatalf("Config.App.LogLevel = %q, want %q (env override should flow through config.Load)", r.Config.App.LogLevel, "ERROR")
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
