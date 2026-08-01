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

- 2026-08-01 SPEC-0019 Agent Manifest System — Completed. Added `services/core/agent_manifest.go`: `Manifest` (Name/Description/Capabilities/Tools/Permissions/Model/Config, YAML-tagged to match SPEC-0019's own `developer_agent` example), `LoadManifest(path)` (read + parse + validate), `Manifest.Validate()` (requires a Name; rejects a `Permissions` entry for a tool not listed in `Tools`), `Manifest.Metadata()` (derives a SPEC-0018 `AgentMetadata` — ID/Name from the manifest Name, Tools passed through, Permissions flattened to the sorted set of gated tool names — without modifying `AgentMetadata` itself, since Capabilities/Model/Configuration have no equivalent slot in that already-Completed contract and stay manifest-only), and `NewAgentFromManifest(m, execute Executor) (Agent, error)` (builds a concrete `Agent` from a validated manifest plus a caller-supplied Executor, since a manifest is declarative only — Task-handling logic is still code the caller provides). Parses YAML via `gopkg.in/yaml.v3` — the project's first third-party Go dependency (every prior module was stdlib-only) — chosen over a stdlib-only JSON format or a hand-rolled parser after asking the user, since it matches SPEC-0019's own YAML example exactly; `services/core/go.mod`/`go.sum` updated via `go get`/`go mod tidy`. `container.go`'s `AgentRegistry` placeholder (SPEC-0020) was left untouched, for the same reason SPEC-0018 left it alone. `go build ./...`, `go vet ./...`, `go test ./... -v` clean in services/core (105 tests pass; 6 new: manifest load, three invalid-manifest cases [missing name, permission for an undeclared tool, malformed YAML], missing-file, agent-construction round-trip, constructor input-rejection). Root `go build ./...`/`go test ./...` from the repo root fails with a workspace-resolution error ("pattern ./...: directory prefix . does not contain modules listed in go.work..."); reproduced even after `git stash`-ing back to a clean master, confirming this is a pre-existing environment quirk unrelated to this feature — verified instead per-module across all 5 go.work directories (all clean). A review pass also caught and fixed a UTF-8 BOM that `setup_feature.ps1` had introduced into this file (absent on master) — stripped, no content change. Built on feature/agent-manifest-system (off master, post SPEC-0018 merge). Full rationale: see docs/agents/JARVIS_BUILD_TRACKER.md (SPEC-0019 row).

Earlier entries (SPEC-0001 through SPEC-0018): see docs/agents/JARVIS_BUILD_TRACKER.md and `git log` — this file's History section is reset on each feature completion rather than accumulated indefinitely.
