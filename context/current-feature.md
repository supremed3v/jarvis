# Current Feature: Repository Foundation

## Working In

Foundation layer — monorepo scaffold (apps/, services/, packages/, agents/, tools/, memory/, voice/, configs/, scripts/, docs/, specs/)

## Status

In Progress

## Goals

- Create the base JARVIS monorepo directory structure per SPEC-0001
- Verify the structure matches the specification exactly
- Lay groundwork for Phase 1 specs (Dev Environment, Config, Shared Types, Logging, Error Handling) that build on this scaffold

## Dependencies

- None (SPEC-0001 is the first spec in Phase 1 Foundation; no prerequisite specs exist)

## Notes

Specification:

context/features/SPEC-0001-repository-foundation.md

Dependency resolution source: Implementation Order (docs/execution/JARVIS_IMPLEMENTATION_ORDER.md — SPEC-0001 is first entry in Phase 1 Foundation, no earlier phase exists)

Relevant ADRs: ADR-0001 (Go backend runtime), ADR-0003 (Electron desktop app) — inform the apps/desktop and services/core directories required by this spec.

Structure divergence decision (user-approved 2026-08-01): SPEC-0001's literal
tree (`packages/shared`, `packages/schemas`, top-level `tools/`, `memory/`,
`voice/`) is superseded by the already-established scaffold
(`packages/config`, `packages/logger`, `packages/shared-types`,
`services/memory`, `services/tools`, `services/voice`) — the existing
scaffold is authoritative and was not replaced or duplicated. Two genuinely
missing pieces were added as literal SPEC-0001 paths: `configs/` (no
existing equivalent) and `specs/` (kept as a placeholder pointing to
`context/features/`, which remains the real spec location).

## History

- 2026-08-01 load loaded SPEC-0001-repository-foundation
- 2026-08-01 start set status to In Progress; git init; added .gitkeep to empty scaffold dirs; created configs/ and specs/ (placeholder) to close the gap with SPEC-0001's literal tree
