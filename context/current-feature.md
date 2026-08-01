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

- 2026-08-01 SPEC-0021 Agent Lifecycle Manager — Completed. Added `services/core/agent_lifecycle.go`: `LifecycleManager`, filling the "Agent initialization/startup/execution/shutdown/cleanup" requirements and 7-state machine (`REGISTERED`, `INITIALIZING`, `READY`, `RUNNING`, `STOPPING`, `STOPPED`, `FAILED`) of SPEC-0021. Corrected `packages/shared-types/agent.go`'s `AgentStatus` (a SPEC-0004 placeholder explicitly annotated "SPEC-0021 Agent Lifecycle Manager" but carrying the wrong 4 values — `idle`/`running`/`stopped`/`error`) to the real 7 states, mirroring `TaskStatus`'s precedent (SPEC-0004/SPEC-0012) of shared-types declaring the closed state set while `services/core` owns transition validation; updated the two `types_test.go` cases using the old `AgentStatusIdle`. Added `AgentTransition` (AgentID/From/To/Reason/Timestamp) mirroring `TaskTransition`, with an extra `Reason` field since — unlike `Task` — there's no persistent `*Agent` struct to hang a failure message off; `LifecycleManager` tracks state by agent ID instead. `LifecycleManager` composes an `AgentRegistry` (SPEC-0020) rather than duplicating storage: `Register` delegates to it before recording `REGISTERED`; `Initialize`/`Stop` call `Lookup` to get the live `Agent` for two new optional capability interfaces, `AgentInitializer` (`Init(ctx) error`) and `AgentCleaner` (`Cleanup(ctx) error`) — an `io.Closer`-style optional-interface pattern chosen over changing the closed SPEC-0018 `Agent` contract. A closed `agentValidTransitions` table plus per-agent history directly mirrors `task_state_machine.go`'s `StateMachine`, adapted to a by-ID rather than by-pointer model. Deliberately did not add a `Container` slot: `container.go` has no pre-reserved `LifecycleManager` placeholder the way `AgentRegistry` had from SPEC-0008, and `TaskManager`'s slot stayed an untouched placeholder through SPEC-0011-0017 despite that whole layer being built — Container wiring is only added when a spec pre-reserves the slot. `go build ./...`, `go vet ./...`, `go test ./... -v` clean across all 5 go.work modules (12 new tests covering all three SPEC-0021 testing criteria — correct transitions, cleanup-on-shutdown, and Init/Cleanup-hook failures landing in FAILED — plus terminal-state rejection, not-registered lookups, duplicate-registration passthrough, and history copy-safety). `gofmt -l` flags only the pre-existing CRLF/LF artifact noted since SPEC-0007; confirmed via `gofmt -d` on the new files themselves. Root `go build ./...` still fails with the same pre-existing go.work resolution error noted since SPEC-0009. One pre-existing flaky test unrelated to this change (`TestWorker_FailsTerminallyAfterMaxAttempts`, SPEC-0015) was verified to reproduce on a clean master via `git stash -u` before this branch's changes — not touched. Reviewed against `docs/agents/CODE_REVIEW_PROTOCOL.md`: found and fixed two Code Quality issues (`Initialize`/`Stop` discarding the result of their own follow-up transition to `FAILED` on a hook failure — now explicit best-effort with a comment) and accepted one known limitation (a narrow TOCTOU window in `Register` between updating the `AgentRegistry` and the manager's own state map — no concurrent caller exists yet, and the codebase tolerates similarly narrow races elsewhere). Approved after fixes; re-verified `go build`/`go vet`/`go test -v` clean. Built on feature/agent-lifecycle-manager (off master, post SPEC-0020 merge). Full rationale: see docs/agents/JARVIS_BUILD_TRACKER.md (SPEC-0021 row).

Earlier entries (SPEC-0001 through SPEC-0020): see docs/agents/JARVIS_BUILD_TRACKER.md and `git log` — this file's History section is reset on each feature completion rather than accumulated indefinitely.
