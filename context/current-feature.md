# Current Feature

## Overview
Not started — no feature currently loaded. Next candidate per docs/execution/JARVIS_IMPLEMENTATION_ORDER.md: SPEC-0029 (Model Router), which SPEC-0028 (Model Configuration System) was blocking and which is now Completed.

## Status
Not Started

## Goals
_None yet._

## Files Modified
_None yet._

## Notes
SPEC-0028 (Model Configuration System) is now Completed (see docs/agents/JARVIS_BUILD_TRACKER.md, SPEC-0028 row, for full implementation/review rationale). SPEC-0029 (Model Router) is unblocked and can consume `packages/config`'s `ModelConfig.Models`/`DefaultModel`/`AgentModels`/`ModelFor` to pick a model per task.

## History

- 2026-08-02 SPEC-0028 Model Configuration System — Completed. Added `packages/config/config.go` and `load.go` extensions: `ModelConfig` gained `Models map[string]Model` (named model profiles - `general`/`coding`, matching SPEC-0028's own YAML example verbatim, defaulting to `qwen`/`qwen-coder`), `DefaultModel` (default model selection), and `AgentModels map[string]string` (agent-specific model overrides), plus a new `ModelConfig.ModelFor(agentName string) (Model, error)` method resolving an agent's override key if set, else falling back to `DefaultModel`, erroring if the resolved key has no `Models` entry - covering SPEC-0028's "Default model selection" and "Agent-specific models" requirements directly. `Model` (Provider/Name/Temperature/MaxTokens/Options) covers "Temperature settings", "Token limits" (`MaxTokens` - deliberately not `ContextSize`, since context-window sizing is SPEC-0032/SPEC-0033's job and both are still `Planned`), and "Runtime parameters" (`Options map[string]any`, a free-form catch-all not yet consumed by any provider). `load.go`'s existing `validate()` gained the checks needed for SPEC-0028's third testing criterion ("Invalid configuration is rejected"): empty or non-`ollama` provider on any named model (ADR-0004: Ollama is the only supported runtime), empty model name, temperature outside [0, 2], negative `MaxTokens`, `DefaultModel` unset while `Models` is non-empty, an unresolvable `DefaultModel`, and any `AgentModels` value with no matching `Models` entry - so `Load` fails closed with a descriptive error instead of silently accepting bad model config or deferring the failure to a later `ModelFor` call. Built entirely on the SPEC-0003 `Load`/`Defaults`/`validate` pattern already in `packages/config`, with no new dependency and no reach into `services/core`'s SPEC-0026 `Provider` interface or SPEC-0027 `OllamaProvider` - resolving a `Model` into an actual provider call is left to SPEC-0029 Model Router, which this spec unblocks. `go build ./...`, `go vet ./...`, `go test ./...` clean across all 5 go.work modules (9 new tests plus an extended `TestLoad_Defaults`: SPEC-0028's three explicit testing criteria - config loads, agent overrides work, invalid configuration rejected via seven `TestLoad_ValidationRejects*` cases). `gofmt -l` clean on both changed files. Reviewed against `docs/agents/CODE_REVIEW_PROTOCOL.md` (Architecture/Code Quality/Security/Testing): review pass found and fixed one real Code Quality gap - a `models` block with an empty `defaultModel` passed `validate()` and would only fail later at `ModelFor()` call time - by adding the missing check and a regression test; re-verified clean after fix. Approved after fix. Built on feature/model-configuration-system (off master, post SPEC-0027 merge). Full rationale: see docs/agents/JARVIS_BUILD_TRACKER.md (SPEC-0028 row).

Earlier entries (SPEC-0001 through SPEC-0027): see docs/agents/JARVIS_BUILD_TRACKER.md and `git log` — this file's History section is reset on each feature completion rather than accumulated indefinitely.
