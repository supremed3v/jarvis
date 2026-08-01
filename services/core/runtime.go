// Package core implements SPEC-0007: the Go runtime bootstrap. Runtime is
// the Core Runtime's lifecycle owner — it loads configuration, initializes
// logging, brings up dependencies in order, and tears them down in reverse
// on shutdown.
package core

import (
	"context"
	"fmt"
	"sync"

	cfgpkg "jarvis-pa/packages/config"
	"jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
)

// State is the Runtime's lifecycle state.
type State string

const (
	StateNotStarted State = "NOT_STARTED"
	StateStarting   State = "STARTING"
	StateRunning    State = "RUNNING"
	StateStopping   State = "STOPPING"
	StateStopped    State = "STOPPED"
	StateFailed     State = "FAILED"
)

// Dependency is a component the Runtime brings up during Start and tears
// down, in reverse initialization order, during Shutdown.
type Dependency interface {
	// Name identifies the dependency in logs and error context.
	Name() string
	// Init brings the dependency up. A non-nil error aborts Start.
	Init(ctx context.Context) error
	// Close tears the dependency down. Close is only called on
	// dependencies whose Init already succeeded.
	Close(ctx context.Context) error
}

// Runtime owns the Core Runtime's process lifecycle: configuration
// loading, logging initialization, and ordered dependency startup/
// shutdown. It is safe for concurrent use.
type Runtime struct {
	mu sync.Mutex

	state      State
	configPath string
	minLevel   logger.Level

	dependencies []Dependency
	started      []Dependency // prefix of dependencies successfully initialized, for reverse teardown

	Config *cfgpkg.Config
	Logger *logger.Logger
}

// Option configures a Runtime created by New.
type Option func(*Runtime)

// WithConfigPath sets the JSON config file Start loads. An empty path (the
// default) skips the file layer and uses config.Defaults plus environment
// overrides, per packages/config's Load contract.
func WithConfigPath(path string) Option {
	return func(r *Runtime) { r.configPath = path }
}

// WithDependencies registers the dependencies Start initializes, in the
// given order. Shutdown tears down the successfully-initialized subset in
// reverse order.
func WithDependencies(deps ...Dependency) Option {
	return func(r *Runtime) { r.dependencies = append(r.dependencies, deps...) }
}

// New creates a Runtime in StateNotStarted. It performs no I/O; Start does
// configuration loading, logging initialization, and dependency init.
func New(opts ...Option) *Runtime {
	r := &Runtime{state: StateNotStarted}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// State reports the Runtime's current lifecycle state.
func (r *Runtime) State() State {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

// Start brings the Runtime up: it loads configuration, initializes
// logging at the configured level, then initializes each registered
// dependency in order. If any step fails, Start rolls back the
// dependencies it already initialized (in reverse order) and returns a
// descriptive, wrapped error; the Runtime is left in StateFailed rather
// than StateRunning.
//
// Start may only be called from StateNotStarted or StateStopped (i.e. a
// previously shut-down Runtime may be restarted). Calling it from any
// other state returns an error without side effects.
func (r *Runtime) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.state != StateNotStarted && r.state != StateStopped {
		state := r.state
		r.mu.Unlock()
		return errors.New(errors.TypeInternal, "RUNTIME_ALREADY_STARTED", "core.runtime",
			fmt.Sprintf("cannot start runtime in state %s", state))
	}
	r.state = StateStarting
	r.started = nil
	r.mu.Unlock()

	cfg, err := cfgpkg.Load(r.configPath)
	if err != nil {
		r.fail()
		return errors.Wrap(err, errors.TypeInternal, "RUNTIME_CONFIG_LOAD_FAILED", "core.runtime",
			"failed to load configuration")
	}

	level, err := logger.ParseLevel(cfg.App.LogLevel)
	if err != nil {
		r.fail()
		return errors.Wrap(err, errors.TypeInvalidInput, "RUNTIME_LOG_LEVEL_INVALID", "core.runtime",
			"failed to parse configured log level")
	}

	r.mu.Lock()
	r.Config = cfg
	r.minLevel = level
	r.Logger = logger.New("core.runtime", logger.WithMinLevel(level))
	deps := append([]Dependency(nil), r.dependencies...)
	r.mu.Unlock()

	r.Logger.Info("runtime starting", map[string]any{"dependencies": len(deps)})

	var started []Dependency
	for _, dep := range deps {
		if err := dep.Init(ctx); err != nil {
			r.Logger.Error("dependency init failed", map[string]any{"dependency": dep.Name(), "error": err.Error()})
			r.rollback(ctx, started)
			r.fail()
			return errors.Wrap(err, errors.TypeInternal, "RUNTIME_DEPENDENCY_INIT_FAILED", "core.runtime",
				fmt.Sprintf("failed to initialize dependency %q", dep.Name())).With("dependency", dep.Name())
		}
		started = append(started, dep)
		r.Logger.Info("dependency initialized", map[string]any{"dependency": dep.Name()})
	}

	r.mu.Lock()
	r.started = started
	r.state = StateRunning
	r.mu.Unlock()

	r.Logger.Info("runtime started", nil)
	return nil
}

// Shutdown tears the Runtime down: it closes each successfully-initialized
// dependency in reverse order, collecting rather than stopping on the
// first failure, so one broken dependency can't prevent the others from
// releasing their resources. Shutdown only succeeds from StateRunning.
func (r *Runtime) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	if r.state != StateRunning {
		state := r.state
		r.mu.Unlock()
		return errors.New(errors.TypeInternal, "RUNTIME_NOT_RUNNING", "core.runtime",
			fmt.Sprintf("cannot shut down runtime in state %s", state))
	}
	r.state = StateStopping
	started := r.started
	log := r.Logger
	r.mu.Unlock()

	log.Info("runtime stopping", nil)

	closeErr := r.rollback(ctx, started)

	r.mu.Lock()
	r.started = nil
	r.state = StateStopped
	r.mu.Unlock()

	if closeErr != nil {
		log.Error("runtime stopped with errors", map[string]any{"error": closeErr.Error()})
		return errors.Wrap(closeErr, errors.TypeInternal, "RUNTIME_SHUTDOWN_FAILED", "core.runtime",
			"one or more dependencies failed to close cleanly")
	}

	log.Info("runtime stopped", nil)
	return nil
}

// Run starts the Runtime, blocks until ctx is done, then shuts down using
// a fresh background context (ctx is already canceled by the time
// shutdown begins). Callers that want OS signal-triggered shutdown should
// pass a context from signal.NotifyContext.
func (r *Runtime) Run(ctx context.Context) error {
	if err := r.Start(ctx); err != nil {
		return err
	}
	<-ctx.Done()
	return r.Shutdown(context.Background())
}

// fail transitions the Runtime to StateFailed. Called with r.mu unlocked.
func (r *Runtime) fail() {
	r.mu.Lock()
	r.state = StateFailed
	r.mu.Unlock()
}

// rollback closes deps in reverse order, joining every Close error
// (rather than stopping at the first) into a single error so no
// dependency is skipped because an earlier one failed to close.
func (r *Runtime) rollback(ctx context.Context, deps []Dependency) error {
	var errs []error
	for i := len(deps) - 1; i >= 0; i-- {
		dep := deps[i]
		if err := dep.Close(ctx); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", dep.Name(), err))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	msg := errs[0].Error()
	for _, e := range errs[1:] {
		msg += "; " + e.Error()
	}
	return fmt.Errorf("%s", msg)
}
