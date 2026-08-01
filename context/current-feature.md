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

- 2026-08-01 SPEC-0004 Shared Types Package — Completed. Implemented packages/shared-types (standalone Go module, module jarvis-pa/packages/shared-types, go 1.23, stdlib-only, no go.sum), following the packages/config (SPEC-0003) precedent for Foundation-layer packages. Defines four framework-independent, serializable data contracts with no business logic: Event (id/type/source/timestamp/payload), Task (id/type/status via TaskStatus enum/input/result/error/timestamps), Agent (id/name/type/status via AgentStatus enum/capabilities), Tool (name/description/parameters/permissions). EventType intentionally has no hardcoded constants, since concrete event names are producer-specific and belong to future services (SPEC-0009 Event Bus onward), not to this shared contract. 12 tests cover JSON round-trips for all four types, omitempty/zero-value edge cases, malformed-JSON error handling (fails safely, never panics), and cross-service wire-compatibility (unknown fields ignored for forward-compat, enums encode as plain JSON strings for non-Go consumers). `go build ./...`, `go vet ./...`, `go test ./... -v`, `gofmt -l .` all clean (12/12 tests pass); coverage reports "no statements" as expected for a types-only package. Built on feature/shared-types-package, merged into master via commit 3c99277 (merge commit follows). Full rationale: see docs/agents/JARVIS_BUILD_TRACKER.md (SPEC-0004 row).
