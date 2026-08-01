# Current Feature

## Working In

Not Started

## Status

Not Started

## Goals

-

## Dependencies

-

## Notes

-

## History

- 2026-08-01 SPEC-0018 Agent Interface — Completed. Added `services/core/agent.go`: `AgentMetadata` (ID/Name/Description/Instructions/Tools/MemoryAccess/Permissions — the declarative half of the contract the runtime can inspect without invoking the agent) and `Agent` (a two-method interface: `Metadata() AgentMetadata`, `Execute(ctx, *types.Task) (map[string]any, error)`). `Tools`/`MemoryAccess`/`Permissions` are plain `[]string` identifiers, not resolved capabilities, mirroring `TaskSource`/`TaskPriority`'s precedent of being bare strings other not-yet-built specs (Tool Registry, memory system, permission system) own the meaning of — this spec does not depend on those siblings existing first, resolving the ambiguity between `JARVIS_DEPENDENCY_GRAPH.md`'s "Critical Dependencies" note (Agents need LLM/Tools/Memory) and `JARVIS_IMPLEMENTATION_ORDER.md`'s phase ordering (Agent Interface first in Phase 4) in favor of the latter. `AgentMetadata.Validate()` enforces required ID/Name via `packages/errors` `TypeInvalidInput`. `Execute`'s signature exactly matches the existing `Executor` type (`task_worker.go`, SPEC-0014), so a `Worker` can drive any conforming `Agent` via its bound `Execute` method with no adapter — proven by an integration test running a stub `Agent` through a real `Queue`+`StateMachine`+`EventBus`+`Worker`. `packages/shared-types/agent.go`'s existing `Agent` struct (SPEC-0004, annotated for SPEC-0018/SPEC-0020) is the separate registry-record shape for a *registered* agent's live state and was left untouched — that's SPEC-0020 Agent Registry's concern. `container.go`'s `AgentRegistry interface{}` placeholder (SPEC-0020) was also left untouched, for the same reason `TaskManager`/`ToolRegistry` were left alone by SPEC-0012-0017. No concrete agent was added under `agents/core-agent`/`agents/developer-agent`/`agents/research-agent` (still `.gitkeep`-only scaffolds) — not required by SPEC-0018's Requirements/Testing sections; `agent_test.go`'s `stubAgent` fills the "sample agent" testing role. `go build ./...`, `go vet ./...`, `go test ./... -v` clean across all 5 go.work modules (3 new tests: interface implementability, Worker-drives-Agent integration, table-driven `AgentMetadata.Validate`). `go test -race` unavailable in this environment (no cgo toolchain), same constraint noted under SPEC-0005/0007/0009/0012-0017. Built on feature/agent-interface (off master, post SPEC-0017 merge). Full rationale: see docs/agents/JARVIS_BUILD_TRACKER.md (SPEC-0018 row).

Earlier entries (SPEC-0001 through SPEC-0017): see docs/agents/JARVIS_BUILD_TRACKER.md and `git log` — this file's History section is reset on each feature completion rather than accumulated indefinitely.
