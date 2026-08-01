# Current Feature: Shared Types Package

## Working In

packages/shared-types (standalone Go module, per the SPEC-0003 precedent — likely `jarvis-pa/packages/shared-types`, go 1.23, stdlib-only)

## Status

In Progress

## Goals

- Define shared contracts: Events, Tasks, Agents, Tools
- No business logic in this package
- Framework independent
- Support serialization

## Dependencies

- SPEC-0001 Repository Foundation (status: Completed)
- SPEC-0002 Development Environment (status: Completed)
- SPEC-0003 Configuration System (status: Completed)

## Notes

Specification:

context/features/SPEC-0004-shared-types-package.md

Dependency resolution source: Implementation Order (Phase 1 Foundation lists 0001-0006 in order; 0004 follows 0001-0003 which are already Completed) + spec's own Requirements text (no config/logging dependency implied — this is a pure types package).

Precedent: packages/config (SPEC-0003) established the pattern for Foundation-layer packages — standalone go.mod (module `jarvis-pa/packages/<name>`, go 1.23), independent of the future SPEC-0007 root runtime module, stdlib-only where possible, no go.sum. Follow the same shape for packages/shared-types.

Relevant ADR: ADR-0001 (Go runtime choice) — package will be Go. No other ADR directly governs a types-only package.

## History

- 2026-08-01 load loaded SPEC-0004
- 2026-08-01 start began implementation on feature/shared-types-package
- 2026-08-01 test added error-handling/edge-case/boundary tests (malformed JSON, zero values, omitempty, unknown-field forward-compat, enum wire format); go build/vet/test all pass (12/12)
- 2026-08-01 review verdict: Ready to complete. No scope, architecture, or security issues found; see review output for detail.
