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

- 2026-08-01 SPEC-0024 Agent Permission Model — Completed. Added `services/core/agent_permission.go`: `PermissionLevel`/`PermissionModel`/`PermissionChecker` filling the "security boundaries for agents" requirements of SPEC-0024, the sixth spec of Phase 4 Intelligence - directly closing the permission-checking gap SPEC-0022 and SPEC-0023 each explicitly called out and accepted as out-of-scope ("no permission-checking system exists anywhere in the codebase yet"). Distinct from SPEC-0019's existing per-manifest `Permissions map[string]ManifestPermission` (`require_confirmation` boolean, declared but never enforced): SPEC-0024 is a separate, centralized security policy table - `PermissionModel` (`map[string]AgentPermissions`, `AgentPermissions` a `map[string]PermissionLevel`) keyed by agent ID, loaded from YAML in the exact three-state shape (`allowed`/`approval_required`/`denied`) SPEC-0024's own example uses, via `LoadPermissionModel` (parses with the existing `gopkg.in/yaml.v3` dependency from SPEC-0019, then `Validate()`s every declared level). Category names (`filesystem`/`terminal`/`browser`/etc.) are plain identifiers, not a closed enum, mirroring `AgentMetadata.Tools`/`Permissions`' existing bare-string precedent (SPEC-0018) - matching `JARVIS_MASTER_ARCHITECTURE.md`'s Tool System responsibilities (filesystem access/terminal operations/browser automation/external integrations), which map onto SPEC-0024's four listed controls (Tool access/File access/Command execution/External communication). `PermissionModel.Level(agentID, category)` fails closed: an agent absent from the model, or present with no entry for that category, defaults to `PermissionDenied` rather than silently allowing an undeclared boundary. `PermissionChecker.Check(ctx, agentID, category)` is the enforcement point - `Allowed` returns nil immediately, `Denied` (including any fail-closed default) returns a `packages/errors` `TypePermissionDenied`/`AGENT_PERMISSION_DENIED` error (the `TypePermissionDenied` `Type` constant already existed in `packages/errors/type.go` since SPEC-0006 but had never been used until now), `ApprovalRequired` calls an optional `ApprovalFunc` and denies if none is configured, the func errors, or it declines - and every outcome (`allowed`/`approved`/`denied`/`denied_by_approver`/`denied_no_approver`/`approval_error`) is logged via an optional `packages/logger.Logger`, satisfying "permission checks are logged" without requiring a logger to be configured. `PermissionEnforcedToolCaller(checker, agentID, next ToolCaller)` wraps SPEC-0022's existing `ToolCaller` seam (`agent_execution_loop.go`'s Execute Actions stage) so enforcement sits exactly where every tool call already passes through - `ExecutionLoop`, `Agent`, `Manifest`, `Registry`, and `LifecycleManager` (SPEC-0018-0023) were left completely untouched, this spec is purely additive. `go build ./...`, `go vet ./...`, `go test ./... -v`, `gofmt -l` all clean across all 5 go.work modules (13 new top-level tests: SPEC-0024's three explicit testing criteria - restricted tools blocked, allowed tools execute, permission checks logged - plus YAML load/invalid-level/malformed/missing-file handling, fail-closed defaults for an absent agent and for an agent with zero declared categories, approval-required grant/decline/approver-error/no-approver paths, and a full integration test wiring `PermissionEnforcedToolCaller` into a real `ExecutionLoop.Run()` proving a denied step actually halts the loop rather than only working in isolation). `go test -race` unavailable in this environment (no cgo toolchain), same constraint noted since SPEC-0005 - moot here regardless, since neither `PermissionModel` nor `PermissionChecker` mutate any field after construction. Reviewed against `docs/agents/CODE_REVIEW_PROTOCOL.md` (Architecture/Code Quality/Security/Testing): no blocking issues found; approved without changes. Accepted one known, out-of-scope limitation: nothing yet constructs a `PermissionChecker` at real agent startup or wires it into `Registry`/`LifecycleManager`, since no spec has defined where the permission YAML lives at runtime or when checking should be wired in system-wide - this delivers the enforcement primitive, ready for that future wiring. Built on feature/agent-permission-model (off master, post SPEC-0023 merge). Full rationale: see docs/agents/JARVIS_BUILD_TRACKER.md (SPEC-0024 row).

Earlier entries (SPEC-0001 through SPEC-0023): see docs/agents/JARVIS_BUILD_TRACKER.md and `git log` — this file's History section is reset on each feature completion rather than accumulated indefinitely.
