// agent_execution_loop.go implements SPEC-0022: the Agent Execution Loop -
// the seven-stage cycle (Receive Task -> Analyze Context -> Create Plan ->
// Select Tools -> Execute Actions -> Evaluate Result -> Return Response) an
// Agent's Execute logic follows. ExecutionLoop.Run has the same signature as
// Executor (task_worker.go, SPEC-0014) and Agent.Execute (agent.go,
// SPEC-0018), so it can be handed directly to NewWorker or
// NewAgentFromManifest with no adapter - the same "no adapter needed"
// precedent those two specs already established.
package core

import (
	"context"
	"fmt"

	"jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
	types "jarvis-pa/packages/shared-types"
)

// Step is a single action within a Plan. Tool names the tool Execute
// Actions should invoke via the loop's ToolCaller; a Step with no Tool is a
// direct action whose Input is returned as-is, for plans that need a step
// with no tool call (e.g. recording a fixed value). Select Tools (SPEC-0022's
// own stage) is therefore expressed by the Planner assigning Tool per Step,
// rather than as a separate pluggable hook - planning which tool a step
// needs is inseparable from planning the step itself, mirroring SPEC-0018's
// precedent that planning and tool use stay internal to an Agent's own
// Execute logic.
type Step struct {
	Name  string
	Tool  string
	Input map[string]any
}

// Plan is the ordered sequence of Steps an ExecutionLoop carries out for one
// Task, produced by a Planner. An empty Plan is valid: some tasks need no
// steps to complete.
type Plan struct {
	Steps []Step
}

// ContextAnalyzer inspects task and returns whatever contextual analysis a
// Planner needs (SPEC-0022's Analyze Context stage). Optional; a loop with
// no ContextAnalyzer configured uses an empty analysis map.
type ContextAnalyzer func(ctx context.Context, task *types.Task) (map[string]any, error)

// Planner produces the Plan an ExecutionLoop carries out for task, given the
// analysis ContextAnalyzer (or an empty map) produced (SPEC-0022's Create
// Plan stage). Required: an ExecutionLoop cannot be constructed without one.
type Planner func(ctx context.Context, task *types.Task, analysis map[string]any) (Plan, error)

// ToolCaller invokes the named tool with input on the loop's behalf,
// returning the tool's output (SPEC-0022's Execute Actions stage, for any
// Step naming a Tool). Required only if a Plan ever produces a Step with a
// non-empty Tool; a loop with no ToolCaller configured fails such a Step
// with EXECUTION_LOOP_NO_TOOL_CALLER rather than panicking.
type ToolCaller func(ctx context.Context, tool string, input map[string]any) (map[string]any, error)

// ResultEvaluator inspects a completed Step's outcome and reports whether it
// counts as a failure (SPEC-0022's Evaluate Result stage), letting a caller
// reject a Step whose action returned no error but whose output is still
// unacceptable (e.g. a tool call that "succeeded" with an empty result).
// Optional; a loop with no ResultEvaluator configured treats a non-nil
// stepErr as the only failure condition.
type ResultEvaluator func(task *types.Task, step Step, output map[string]any, stepErr error) error

// StepResult is one Step's recorded outcome (SPEC-0022's "intermediate
// results" requirement): Output is nil and Err is non-empty on failure.
type StepResult struct {
	Step   Step
	Output map[string]any
	Err    string
}

// ExecutionLoop drives the SPEC-0022 Agent Execution Loop for a single Task:
// Run implements every stage except Receive Task (the task argument itself)
// and Select Tools (folded into Plan/Step, see Step's doc comment).
// ExecutionLoop is safe for concurrent use - it holds no per-run state.
type ExecutionLoop struct {
	analyze  ContextAnalyzer
	plan     Planner
	call     ToolCaller
	evaluate ResultEvaluator
	log      *logger.Logger
}

// ExecutionLoopOption configures an ExecutionLoop created by
// NewExecutionLoop.
type ExecutionLoopOption func(*ExecutionLoop)

// WithContextAnalyzer attaches the Analyze Context stage's implementation.
// Optional; a loop with none configured uses an empty analysis map.
func WithContextAnalyzer(a ContextAnalyzer) ExecutionLoopOption {
	return func(l *ExecutionLoop) { l.analyze = a }
}

// WithToolCaller attaches the callback Execute Actions uses to run any Step
// naming a Tool. Required only if the configured Planner can ever produce
// such a Step.
func WithToolCaller(c ToolCaller) ExecutionLoopOption {
	return func(l *ExecutionLoop) { l.call = c }
}

// WithResultEvaluator attaches the Evaluate Result stage's implementation.
// Optional; a loop with none configured treats a non-nil Step error as the
// only failure condition.
func WithResultEvaluator(e ResultEvaluator) ExecutionLoopOption {
	return func(l *ExecutionLoop) { l.evaluate = e }
}

// WithExecutionLoopLogger attaches a Logger used to report step failures.
// Optional; a loop with no logger runs silently.
func WithExecutionLoopLogger(log *logger.Logger) ExecutionLoopOption {
	return func(l *ExecutionLoop) { l.log = log }
}

// NewExecutionLoop creates a ready-to-use ExecutionLoop whose Create Plan
// stage is implemented by planner. It returns a packages/errors error typed
// TypeInvalidInput if planner is nil.
func NewExecutionLoop(planner Planner, opts ...ExecutionLoopOption) (*ExecutionLoop, error) {
	if planner == nil {
		return nil, errors.New(errors.TypeInvalidInput, "EXECUTION_LOOP_MISSING_PLANNER", "core.agentexecutionloop",
			"cannot create an execution loop without a planner")
	}

	l := &ExecutionLoop{plan: planner}
	for _, opt := range opts {
		opt(l)
	}
	return l, nil
}

// Run carries out the SPEC-0022 execution cycle for task: Analyze Context,
// Create Plan, then Select Tools/Execute Actions/Evaluate Result for each
// Step of the resulting Plan in order (multi-step execution), stopping at
// the first Step whose evaluated outcome fails (failure handling). The
// returned map always carries the analysis and every StepResult recorded so
// far (intermediate results), whether or not err is nil, so a failure still
// returns a useful result describing what was attempted and why it failed.
//
// Run's signature matches Executor (task_worker.go) and Agent.Execute
// (agent.go), so it can be passed directly wherever either is expected.
func (l *ExecutionLoop) Run(ctx context.Context, task *types.Task) (map[string]any, error) {
	if task == nil {
		return nil, errors.New(errors.TypeInvalidInput, "EXECUTION_LOOP_NIL_TASK", "core.agentexecutionloop",
			"cannot run the execution loop for a nil task")
	}

	analysis, err := l.analyzeContext(ctx, task)
	if err != nil {
		return l.response(analysis, nil), errors.Wrap(err, errors.TypeInternal, "EXECUTION_LOOP_ANALYSIS_FAILED", "core.agentexecutionloop",
			fmt.Sprintf("analyzing context for task %q", task.ID)).With("taskId", task.ID)
	}

	plan, err := l.plan(ctx, task, analysis)
	if err != nil {
		return l.response(analysis, nil), errors.Wrap(err, errors.TypeInternal, "EXECUTION_LOOP_PLAN_FAILED", "core.agentexecutionloop",
			fmt.Sprintf("creating plan for task %q", task.ID)).With("taskId", task.ID)
	}

	results := make([]StepResult, 0, len(plan.Steps))
	for i, step := range plan.Steps {
		output, stepErr := l.executeStep(ctx, step)

		evalErr := stepErr
		if l.evaluate != nil {
			evalErr = l.evaluate(task, step, output, stepErr)
		}

		result := StepResult{Step: step, Output: output}
		if evalErr != nil {
			result.Err = evalErr.Error()
		}
		results = append(results, result)

		if evalErr != nil {
			if l.log != nil {
				l.log.Error("execution loop step failed", map[string]any{
					"taskId":    task.ID,
					"step":      step.Name,
					"tool":      step.Tool,
					"stepIndex": i,
					"error":     evalErr.Error(),
				})
			}
			return l.response(analysis, results), errors.Wrap(evalErr, errors.TypeInternal, "EXECUTION_LOOP_STEP_FAILED", "core.agentexecutionloop",
				fmt.Sprintf("step %q failed for task %q", step.Name, task.ID)).
				With("taskId", task.ID).With("step", step.Name).With("tool", step.Tool).With("stepIndex", i)
		}
	}

	return l.response(analysis, results), nil
}

// analyzeContext runs the Analyze Context stage, defaulting to an empty
// analysis map when no ContextAnalyzer is configured.
func (l *ExecutionLoop) analyzeContext(ctx context.Context, task *types.Task) (map[string]any, error) {
	if l.analyze == nil {
		return map[string]any{}, nil
	}
	return l.analyze(ctx, task)
}

// executeStep runs the Select Tools/Execute Actions stages for a single
// Step: a Step naming a Tool is run through the configured ToolCaller; a
// Step with no Tool is a direct action whose Input is its own output.
func (l *ExecutionLoop) executeStep(ctx context.Context, step Step) (map[string]any, error) {
	if step.Tool == "" {
		return step.Input, nil
	}
	if l.call == nil {
		return nil, errors.New(errors.TypeInvalidInput, "EXECUTION_LOOP_NO_TOOL_CALLER", "core.agentexecutionloop",
			fmt.Sprintf("step %q names tool %q but no ToolCaller is configured", step.Name, step.Tool)).
			With("step", step.Name).With("tool", step.Tool)
	}
	return l.call(ctx, step.Tool, step.Input)
}

// response builds the Return Response stage's payload: the Analyze Context
// output, every StepResult recorded so far, and the last successful Step's
// output as the loop's top-level result (nil if no Step ran).
func (l *ExecutionLoop) response(analysis map[string]any, results []StepResult) map[string]any {
	steps := make([]map[string]any, len(results))
	var last map[string]any
	for i, r := range results {
		steps[i] = map[string]any{"name": r.Step.Name, "tool": r.Step.Tool, "output": r.Output, "error": r.Err}
		if r.Err == "" {
			last = r.Output
		}
	}
	return map[string]any{"analysis": analysis, "steps": steps, "result": last}
}
