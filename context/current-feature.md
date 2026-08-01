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

- 2026-08-01 SPEC-0020 Agent Registry — Completed. Added `services/core/agent_registry.go`: `AgentRegistry` interface (`Register(Agent) error`, `Lookup(id string) (Agent, error)`, `Remove(id string) error`, `List() []Agent`) — replacing `container.go`'s `AgentRegistry interface{}` placeholder in-place (mirroring `EventBus`'s precedent of the real interface living in its own file, not container.go) — and `Registry`, an in-memory, mutex-protected implementation (`NewRegistry()`). `Register` validates the agent's `AgentMetadata` (SPEC-0018) via its existing `Validate()` and rejects a duplicate ID with a `packages/errors` `TypeAlreadyExists`/`AGENT_REGISTRY_DUPLICATE_AGENT` error without overwriting the existing registration, matching `Queue.Add`'s established convention for duplicate IDs (`task_queue.go`, SPEC-0013). `Lookup`/`Remove` return `TypeNotFound`/`AGENT_REGISTRY_AGENT_NOT_FOUND` for an unknown ID; `List` returns agents sorted by ID for deterministic ordering. Deliberately did not add a separate "check capabilities" method: SPEC-0020's "Registry Usage" list (find by ID / check capabilities / route tasks) is satisfied by `Lookup` plus the returned `Agent`'s own `Metadata()`/`Execute()` (SPEC-0018) — no new capability concept needed, since `AgentMetadata` has no `Capabilities` field (SPEC-0019 already established Capabilities stays manifest-only). Updated `container_test.go`: the `AgentRegistry` nil-check comment no longer cites SPEC-0020 as a future condition, and `TestNewContainer_OptionsWireStubSlots`'s fake `agentRegistry` (previously a bare `struct{name string}`, valid only against the old empty-interface placeholder) is now a real `NewRegistry()`, since a bare struct no longer satisfies the real interface. Reviewed against `docs/agents/CODE_REVIEW_PROTOCOL.md` (Architecture/Code Quality/Security/Testing) — no findings; approved. `go build ./...`, `go vet ./...`, `go test ./... -v` clean across all 5 go.work modules (8 new tests in `agent_registry_test.go`: register+lookup, invalid-metadata rejection, discovery via Lookup/List, lookup-not-found, duplicate-rejection-does-not-overwrite, remove+double-remove, empty-list, 50-goroutine concurrent registration). `gofmt -l` flags `container.go`/`container_test.go` in full, but `gofmt -d` confirms it's the pre-existing CRLF/LF artifact noted under SPEC-0007/0008/0019, not new formatting issues. Root `go build ./...` still fails with the same pre-existing go.work resolution error noted since SPEC-0009. Built on feature/agent-registry (off master, post SPEC-0019 merge). Full rationale: see docs/agents/JARVIS_BUILD_TRACKER.md (SPEC-0020 row).

Earlier entries (SPEC-0001 through SPEC-0019): see docs/agents/JARVIS_BUILD_TRACKER.md and `git log` — this file's History section is reset on each feature completion rather than accumulated indefinitely.
