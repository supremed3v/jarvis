# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Status

JARVIS is a local-first personal AI operating system, under active implementation following its spec library. Phases 1-3 (Foundation, Runtime, Task Execution: SPEC-0001 through SPEC-0017) are complete, and Phase 4 Intelligence's Agent layer is underway: SPEC-0018 through SPEC-0021 (Agent Interface, Agent Manifest System, Agent Registry, Agent Lifecycle Manager) are implemented. 21 of 182 specs are `Completed` per `docs/agents/JARVIS_BUILD_TRACKER.md`, which is the authoritative source for what's actually built — check it (or `context/features/FEATURE_INDEX.md`, generated from it) before assuming a spec is unimplemented.

The Go workspace (`go.work`, Go 1.23) currently has five real modules with code, tests, and (for `services/core`) one third-party dependency (`gopkg.in/yaml.v3`, since SPEC-0019):
- `packages/config`, `packages/errors`, `packages/logger`, `packages/shared-types` (Foundation layer)
- `services/core` (Runtime, Task Execution, and Agent layers so far — e.g. `runtime.go`, `eventbus.go`, `task_*.go`, `agent*.go`)

`agents/core-agent`, `agents/developer-agent`, `agents/research-agent`, `apps/desktop`, `services/memory`, `services/tools`, and `services/voice` are still empty scaffold directories (`.gitkeep` only) — no spec has required a concrete implementation in any of them yet.

Build/vet/test commands now exist: use `scripts/go_all.ps1` (see below) rather than `go build ./...` from the repo root, which fails for workspace-structural reasons unrelated to whether code exists. The next unit of work per `docs/execution/JARVIS_IMPLEMENTATION_ORDER.md` is SPEC-0022 (Agent Execution Loop), continuing Phase 4's Agent layer. Do not invent build tooling that isn't specified; implement exactly what the relevant SPEC describes.

The PowerShell utilities in `scripts/` (run from inside `scripts/`, since each uses relative `../` paths):
- `scripts/generate_feature_index.ps1` — regenerates `context/features/FEATURE_INDEX.md` from all `SPEC-*.md` files, deriving each spec's Status from `docs/agents/JARVIS_BUILD_TRACKER.md`.
- `scripts/rename_specs.ps1` — normalizes spec filenames to `SPEC-NNNN-kebab-case-title.md`.
- `scripts/go_all.ps1` — runs `go build`/`go vet`/`go test` (`./go_all.ps1 [all|build|vet|test]`, default `all`) inside every module listed in `go.work`, one at a time, with one aggregated pass/fail result. This is the standard way to build/vet/test the whole Go workspace: bare `go build ./...` from the repo root fails because the root has no `go.mod` of its own and isn't a workspace member — a Go workspace-mode constraint, not a bug — so don't try to "fix" that by adding a root `go.mod`; use this script (or `cd` into a specific module) instead.

## Required Workflow Before Coding

1. Read `README.md`.
2. Read the relevant document(s) in `docs/architecture/` (`JARVIS_MASTER_ARCHITECTURE.md`, `SYSTEM_OVERVIEW.md`).
3. Read `context/features/FEATURE_INDEX.md` to find the SPEC(s) relevant to the request — do not bulk-load every spec file.
4. Read the required `context/features/SPEC-NNNN-*.md` file(s) and their listed dependencies.
5. Check `docs/execution/JARVIS_IMPLEMENTATION_ORDER.md` and `docs/execution/JARVIS_DEPENDENCY_GRAPH.md` to confirm prerequisite specs are already implemented before starting a later one.

Each `SPEC-NNNN` file's actual status (`Planned`/`In Progress`/`Blocked`/`Completed`/`Verified`) is tracked in `docs/agents/JARVIS_BUILD_TRACKER.md` and mirrored into the feature index — check it rather than assuming a spec is unimplemented. When implementing a spec, use `docs/agents/SPEC_EXECUTION_TEMPLATE.md` as the shape for planning the work (objective, dependencies, files to create/modify, implementation plan, testing plan).

## Architecture (target design)

```
Desktop Application (Electron)
        |
Communication Layer
        |
Core Runtime (Go)
        |
--------------------------------
Agents | Tools | Memory | LLM Layer | Workflow Engine | Knowledge System | Security | Observability
--------------------------------
        |
Local Hardware / External Services
```

Dependency direction is strict, bottom-up (`docs/execution/JARVIS_DEPENDENCY_GRAPH.md`):
`Foundation -> Runtime -> Tasks -> Agents -> {LLM, Memory, Tools, Voice, Research} -> Applications -> Automation -> Advanced Intelligence`.
Never implement a feature whose dependencies (per the spec's own dependency list, and this graph) don't exist yet.

### Locked technology decisions (`docs/decisions/ADR-*.md`) — do not relitigate without an ADR update

| Concern | Choice | ADR |
|---|---|---|
| Backend runtime | Go | ADR-0001 |
| Overall architecture | Local-first (privacy, offline, no mandatory paid services) | ADR-0002 |
| Desktop app | Electron | ADR-0003 |
| LLM runtime | Ollama (local models only, no API dependency) | ADR-0004 |
| Search/research | SearXNG (self-hosted) | ADR-0005 |
| Browser automation | Playwright | ADR-0006 |
| Storage | Relational (settings/tasks/config) + Vector (semantic memory/retrieval) | ADR-0007 |
| Services policy | No mandatory paid services — open source/local/self-hosted only | ADR-0008 |
| Dev workflow | Specification -> Planning -> Implementation -> Testing -> Review | ADR-0009 |

### MVP scope (`docs/execution/JARVIS_MVP_SCOPE.md`)

Before the full architecture, the MVP is: Go runtime + config + logging + events; Ollama integration + a basic agent + prompt system; conversation memory + user profile memory; filesystem/terminal/browser tools; voice (Whisper/Piper/wake word) and research (SearXNG/browser agent). Voice and research are core, required features, not optional or deferrable — do not deprioritize them to reach a desktop UI sooner. JARVIS is voice-first: voice is the primary MVP interaction surface, not a chat window. The MVP's Application-phase target is a minimal visual presence — System Tray (SPEC-0068) + Voice Interface UI (SPEC-0067, an orb/status view) — with the full desktop chat interface (SPEC-0066) deferred past MVP. See `docs/execution/JARVIS_MVP_SCOPE.md`'s `## Interface` section for the full rationale.

## Feature Specification System

- `context/features/SPEC-NNNN-<slug>.md` — 182 individual feature specs (SPEC-0001 through SPEC-0182), each defining overview/requirements/testing for one feature.
- `context/features/FEATURE_INDEX.md` — auto-generated navigation index over all specs; always consult this before opening individual specs (see `FEATURE_INDEX_SPEC.md` for the rationale/rules governing it).
- Specs are grouped into layers: Foundation, Runtime, Task Execution, Agent, Intelligence, Memory, Tools, Voice, Application, Advanced Systems — roughly corresponding to the phases in `JARVIS_IMPLEMENTATION_ORDER.md`.

## Agent Governance Documents (`docs/agents/`)

These define process rules for any coding agent (Claude Code or OpenCode) working in this repo — read the ones relevant to your current action:
- `AGENT_OPERATING_MODEL.md` / `CLAUDE_CODE_RULES.md` / `OPENCODE_AGENT_RULES.md` — baseline execution rules (read specs first, stay in scope, no silent architecture changes).
- `ARCHITECTURE_CHANGE_PROTOCOL.md` — required for any change touching core runtime, data models, agent system, or security: needs reason, impact analysis, alternatives, updated docs, explicit review.
- `CODE_REVIEW_PROTOCOL.md` — review checklist (architecture fit, code quality, security/permissions, tests).
- `AGENT_HANDOFF_PROTOCOL.md` — what one agent must hand off to the next (completed work, files changed, remaining work, decisions, test status).
- `FEATURE_COMPLETION_CHECKLIST.md` — pre-merge checklist per feature.
- `JARVIS_BUILD_TRACKER.md` / `docs/execution/BUILD_TRACKER.md` — status tracking table (Planned/In Progress/Blocked/Completed/Verified) per spec.
- `docs/JARVIS_AGENT_ASSIGNMENT_PLAN.md` — defines role split (Architecture, Backend, Frontend, Intelligence, Tooling, QA agents) for anyone dividing work across multiple agents.

## Rules

- Follow the specification for the feature being implemented; do not redesign architecture without going through `ARCHITECTURE_CHANGE_PROTOCOL.md`.
- Implement specs in dependency order — do not jump ahead to a later phase.
- Avoid unrelated changes; keep changes scoped to the spec being executed.
- Add tests and document decisions as part of the same change.

## Completion Report

Every task must report:
- Summary
- Files changed
- Tests executed
- Limitations
