package core

import (
	"context"
	"testing"
	"time"

	"jarvis-pa/packages/errors"
	types "jarvis-pa/packages/shared-types"
)

// TestNewExecutionLoop_RequiresPlanner verifies an ExecutionLoop cannot be
// constructed without a Planner (Create Plan has no default behavior).
func TestNewExecutionLoop_RequiresPlanner(t *testing.T) {
	_, err := NewExecutionLoop(nil)
	if !errors.HasCode(err, "EXECUTION_LOOP_MISSING_PLANNER") {
		t.Errorf("NewExecutionLoop(nil) error = %v, want code EXECUTION_LOOP_MISSING_PLANNER", err)
	}
}

// TestExecutionLoop_Run_NilTask verifies Receive Task rejects a nil task
// rather than panicking downstream.
func TestExecutionLoop_Run_NilTask(t *testing.T) {
	l, err := NewExecutionLoop(func(ctx context.Context, task *types.Task, analysis map[string]any) (Plan, error) {
		return Plan{}, nil
	})
	if err != nil {
		t.Fatalf("NewExecutionLoop returned error: %v", err)
	}

	_, err = l.Run(context.Background(), nil)
	if !errors.HasCode(err, "EXECUTION_LOOP_NIL_TASK") {
		t.Errorf("Run(nil task) error = %v, want code EXECUTION_LOOP_NIL_TASK", err)
	}
}

// TestExecutionLoop_Run_CompletesSimpleTask exercises SPEC-0022's first
// testing criterion: a task with a single-step plan and no tool calls
// completes successfully, exercising Analyze Context, Create Plan, and
// Return Response end to end.
func TestExecutionLoop_Run_CompletesSimpleTask(t *testing.T) {
	l, err := NewExecutionLoop(
		func(ctx context.Context, task *types.Task, analysis map[string]any) (Plan, error) {
			if analysis["greeting"] != "hi" {
				t.Errorf("planner saw analysis = %+v, want greeting=hi", analysis)
			}
			return Plan{Steps: []Step{{Name: "answer", Input: map[string]any{"reply": "done"}}}}, nil
		},
		WithContextAnalyzer(func(ctx context.Context, task *types.Task) (map[string]any, error) {
			return map[string]any{"greeting": "hi"}, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewExecutionLoop returned error: %v", err)
	}

	result, err := l.Run(context.Background(), &types.Task{ID: "task-1"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got, ok := result["result"].(map[string]any)
	if !ok || got["reply"] != "done" {
		t.Errorf("result[\"result\"] = %+v, want reply=done", result["result"])
	}
	steps, ok := result["steps"].([]map[string]any)
	if !ok || len(steps) != 1 || steps[0]["name"] != "answer" {
		t.Errorf("result[\"steps\"] = %+v, want one step named answer", result["steps"])
	}
}

// TestExecutionLoop_Run_ToolExecutionWorks exercises SPEC-0022's second
// testing criterion: a multi-step plan invokes the configured ToolCaller for
// each Step naming a Tool, with the right tool name and input, and records
// each call's output as an intermediate result.
func TestExecutionLoop_Run_ToolExecutionWorks(t *testing.T) {
	var calls []struct {
		tool  string
		input map[string]any
	}

	l, err := NewExecutionLoop(
		func(ctx context.Context, task *types.Task, analysis map[string]any) (Plan, error) {
			return Plan{Steps: []Step{
				{Name: "search", Tool: "web_search", Input: map[string]any{"query": "jarvis"}},
				{Name: "summarize", Tool: "summarizer", Input: map[string]any{"text": "results"}},
			}}, nil
		},
		WithToolCaller(func(ctx context.Context, tool string, input map[string]any) (map[string]any, error) {
			calls = append(calls, struct {
				tool  string
				input map[string]any
			}{tool, input})
			return map[string]any{"tool": tool}, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewExecutionLoop returned error: %v", err)
	}

	result, err := l.Run(context.Background(), &types.Task{ID: "task-1"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("ToolCaller invoked %d times, want 2: %+v", len(calls), calls)
	}
	if calls[0].tool != "web_search" || calls[0].input["query"] != "jarvis" {
		t.Errorf("first call = %+v, want tool=web_search query=jarvis", calls[0])
	}
	if calls[1].tool != "summarizer" || calls[1].input["text"] != "results" {
		t.Errorf("second call = %+v, want tool=summarizer text=results", calls[1])
	}

	steps := result["steps"].([]map[string]any)
	if len(steps) != 2 || steps[0]["output"].(map[string]any)["tool"] != "web_search" {
		t.Errorf("result[\"steps\"] = %+v, want step outputs recorded", result["steps"])
	}
}

// TestExecutionLoop_Run_MissingToolCallerFails verifies a Step naming a Tool
// fails clearly, rather than panicking, when no ToolCaller is configured.
func TestExecutionLoop_Run_MissingToolCallerFails(t *testing.T) {
	l, err := NewExecutionLoop(func(ctx context.Context, task *types.Task, analysis map[string]any) (Plan, error) {
		return Plan{Steps: []Step{{Name: "search", Tool: "web_search"}}}, nil
	})
	if err != nil {
		t.Fatalf("NewExecutionLoop returned error: %v", err)
	}

	_, err = l.Run(context.Background(), &types.Task{ID: "task-1"})
	if !errors.HasCode(err, "EXECUTION_LOOP_STEP_FAILED") {
		t.Errorf("Run error = %v, want code EXECUTION_LOOP_STEP_FAILED", err)
	}
	if !errors.HasCode(err, "EXECUTION_LOOP_NO_TOOL_CALLER") {
		t.Errorf("Run error chain = %v, want wrapped code EXECUTION_LOOP_NO_TOOL_CALLER", err)
	}
}

// TestExecutionLoop_Run_FailuresReturnUsefulResults exercises SPEC-0022's
// third testing criterion: a failing Step stops the loop (failure handling)
// but the returned error carries step/tool/task context and the returned
// result still reports every Step attempted so far, including the failed
// one.
func TestExecutionLoop_Run_FailuresReturnUsefulResults(t *testing.T) {
	toolErr := errors.New(errors.TypeUnavailable, "TOOL_DOWN", "test", "tool is unavailable")

	l, err := NewExecutionLoop(
		func(ctx context.Context, task *types.Task, analysis map[string]any) (Plan, error) {
			return Plan{Steps: []Step{
				{Name: "first", Input: map[string]any{"ok": true}},
				{Name: "second", Tool: "flaky_tool"},
				{Name: "never-runs", Input: map[string]any{"ok": true}},
			}}, nil
		},
		WithToolCaller(func(ctx context.Context, tool string, input map[string]any) (map[string]any, error) {
			return nil, toolErr
		}),
	)
	if err != nil {
		t.Fatalf("NewExecutionLoop returned error: %v", err)
	}

	task := &types.Task{ID: "task-1"}
	result, err := l.Run(context.Background(), task)
	if !errors.HasCode(err, "EXECUTION_LOOP_STEP_FAILED") {
		t.Fatalf("Run error = %v, want code EXECUTION_LOOP_STEP_FAILED", err)
	}
	if !errors.Is(err, errors.TypeInternal) {
		t.Errorf("Run error type = %v, want TypeInternal", err)
	}

	wrapped, ok := err.(*errors.Error)
	if !ok {
		t.Fatalf("Run error = %T, want *errors.Error", err)
	}
	if wrapped.Context["step"] != "second" || wrapped.Context["tool"] != "flaky_tool" || wrapped.Context["taskId"] != "task-1" {
		t.Errorf("Run error context = %+v, want step=second tool=flaky_tool taskId=task-1", wrapped.Context)
	}

	steps := result["steps"].([]map[string]any)
	if len(steps) != 2 {
		t.Fatalf("len(steps) = %d, want 2 (stopped after the failing step)", len(steps))
	}
	if steps[0]["name"] != "first" || steps[0]["error"] != "" {
		t.Errorf("steps[0] = %+v, want first step recorded as successful", steps[0])
	}
	if steps[1]["name"] != "second" || steps[1]["error"] == "" {
		t.Errorf("steps[1] = %+v, want second step recorded with an error", steps[1])
	}
}

// TestExecutionLoop_Run_PlanFailureIsReported verifies a Planner error
// (Create Plan) is wrapped and reported without running any Step.
func TestExecutionLoop_Run_PlanFailureIsReported(t *testing.T) {
	planErr := errors.New(errors.TypeInvalidInput, "BAD_PLAN", "test", "cannot plan this task")
	l, err := NewExecutionLoop(func(ctx context.Context, task *types.Task, analysis map[string]any) (Plan, error) {
		return Plan{}, planErr
	})
	if err != nil {
		t.Fatalf("NewExecutionLoop returned error: %v", err)
	}

	_, err = l.Run(context.Background(), &types.Task{ID: "task-1"})
	if !errors.HasCode(err, "EXECUTION_LOOP_PLAN_FAILED") {
		t.Errorf("Run error = %v, want code EXECUTION_LOOP_PLAN_FAILED", err)
	}
}

// TestExecutionLoop_Run_AnalysisFailureIsReported verifies a ContextAnalyzer
// error (Analyze Context) is wrapped and reported without creating a plan.
func TestExecutionLoop_Run_AnalysisFailureIsReported(t *testing.T) {
	analysisErr := errors.New(errors.TypeInternal, "ANALYSIS_BOOM", "test", "could not analyze task")
	plannerCalled := false

	l, err := NewExecutionLoop(
		func(ctx context.Context, task *types.Task, analysis map[string]any) (Plan, error) {
			plannerCalled = true
			return Plan{}, nil
		},
		WithContextAnalyzer(func(ctx context.Context, task *types.Task) (map[string]any, error) {
			return nil, analysisErr
		}),
	)
	if err != nil {
		t.Fatalf("NewExecutionLoop returned error: %v", err)
	}

	_, err = l.Run(context.Background(), &types.Task{ID: "task-1"})
	if !errors.HasCode(err, "EXECUTION_LOOP_ANALYSIS_FAILED") {
		t.Errorf("Run error = %v, want code EXECUTION_LOOP_ANALYSIS_FAILED", err)
	}
	if plannerCalled {
		t.Error("planner was called despite analysis failing")
	}
}

// TestExecutionLoop_Run_ResultEvaluatorRejectsSuccessfulStep verifies
// Evaluate Result can fail a Step whose action itself returned no error,
// e.g. because the output content is unacceptable.
func TestExecutionLoop_Run_ResultEvaluatorRejectsSuccessfulStep(t *testing.T) {
	l, err := NewExecutionLoop(
		func(ctx context.Context, task *types.Task, analysis map[string]any) (Plan, error) {
			return Plan{Steps: []Step{{Name: "check", Input: map[string]any{"count": 0}}}}, nil
		},
		WithResultEvaluator(func(task *types.Task, step Step, output map[string]any, stepErr error) error {
			if output["count"] == 0 {
				return errors.New(errors.TypeInvalidInput, "EMPTY_RESULT", "test", "count must be non-zero")
			}
			return stepErr
		}),
	)
	if err != nil {
		t.Fatalf("NewExecutionLoop returned error: %v", err)
	}

	_, err = l.Run(context.Background(), &types.Task{ID: "task-1"})
	if !errors.HasCode(err, "EXECUTION_LOOP_STEP_FAILED") {
		t.Errorf("Run error = %v, want code EXECUTION_LOOP_STEP_FAILED", err)
	}
	if !errors.HasCode(err, "EMPTY_RESULT") {
		t.Errorf("Run error chain = %v, want wrapped code EMPTY_RESULT", err)
	}
}

// TestExecutionLoop_Run_EmptyPlanSucceeds verifies a Plan with no Steps is a
// valid, trivially successful outcome (e.g. a task that needs no action).
func TestExecutionLoop_Run_EmptyPlanSucceeds(t *testing.T) {
	l, err := NewExecutionLoop(func(ctx context.Context, task *types.Task, analysis map[string]any) (Plan, error) {
		return Plan{}, nil
	})
	if err != nil {
		t.Fatalf("NewExecutionLoop returned error: %v", err)
	}

	result, err := l.Run(context.Background(), &types.Task{ID: "task-1"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if steps := result["steps"].([]map[string]any); len(steps) != 0 {
		t.Errorf("result[\"steps\"] = %+v, want empty", steps)
	}
	if r, ok := result["result"].(map[string]any); !ok || len(r) != 0 {
		t.Errorf("result[\"result\"] = %+v, want an empty/nil map (no step ran)", result["result"])
	}
}

// TestExecutionLoop_Run_SatisfiesExecutorAndAgent verifies Run's signature
// matches Executor/Agent.Execute exactly, so a Worker can drive an
// ExecutionLoop-backed Agent with no adapter - the same integration
// TestAgent_RuntimeCanExecuteSampleAgent (SPEC-0018) proved for a plain
// stubAgent.
func TestExecutionLoop_Run_SatisfiesExecutorAndAgent(t *testing.T) {
	loop, err := NewExecutionLoop(func(ctx context.Context, task *types.Task, analysis map[string]any) (Plan, error) {
		return Plan{Steps: []Step{{Name: "answer", Tool: "responder", Input: map[string]any{"in": task.ID}}}}, nil
	}, WithToolCaller(func(ctx context.Context, tool string, input map[string]any) (map[string]any, error) {
		return map[string]any{"summary": "done"}, nil
	}))
	if err != nil {
		t.Fatalf("NewExecutionLoop returned error: %v", err)
	}

	manifest := &Manifest{Name: "loop-agent"}
	agent, err := NewAgentFromManifest(manifest, loop.Run)
	if err != nil {
		t.Fatalf("NewAgentFromManifest returned error: %v", err)
	}

	q := NewQueue()
	sm := NewStateMachine()
	bus := NewBus()

	task := &types.Task{ID: "task-1", Source: types.TaskSourceAgent, Type: "test", Status: types.TaskStatusQueued}
	if err := q.Add(task); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	w := NewWorker("worker-1", q, sm, bus, agent.Execute, WithPollInterval(time.Millisecond))
	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer w.Stop(context.Background())

	waitFor(t, time.Second, func() bool { return task.Status == types.TaskStatusCompleted })

	steps := task.Result["steps"].([]map[string]any)
	if len(steps) != 1 || steps[0]["output"].(map[string]any)["summary"] != "done" {
		t.Errorf("task.Result[\"steps\"] = %+v, want one step with summary=done", task.Result["steps"])
	}
}
