# JARVIS Build Tracker

## Purpose

Track implementation progress across all specifications.

## Status Values

-   Planned
-   In Progress
-   Blocked
-   Completed
-   Verified

## Tracking Structure

  Spec        Status      Owner   Notes
  ----------- ----------- ------- -------
  SPEC-0001   Completed           Structure created and validated (scripts/validate_structure.ps1: 14/14). Existing scaffold (packages/config, packages/logger, packages/shared-types, services/memory, services/tools, services/voice) kept authoritative over SPEC-0001's literal tree; configs/ and specs/ added to close the remaining gap — divergence approved by user, recorded in context/current-feature.md history. Build/test steps (go build/go test) not applicable yet — no go.mod exists until SPEC-0007.
  SPEC-0002   Completed         Toolchain setup only (.go-version=1.23, .nvmrc=20, .env.example, scripts/verify_dev_environment.ps1, docs/DEVELOPMENT.md) — no go.mod/package.json/app code, per JARVIS_IMPLEMENTATION_ORDER.md (those arrive in SPEC-0007/SPEC-0063). Testing criteria 2 ("desktop launches") and 3 ("core runtime starts") rescoped to toolchain smoke-checks (node/npm present; go version present) since no Electron app or Go runtime exists yet — full criteria deferred to SPEC-0007/SPEC-0063. verify_dev_environment.ps1 run locally: Go 1.26.2, Node v24.15.0, npm 11.12.1 detected (all newer than pins — warning only, not failure); Ollama not installed locally (warning only). .env.example includes an optional NVIDIA_API_KEY/NVIDIA_API_BASE_URL pair reserved for a future hybrid local+cloud LLM path (not wired to code; ADR-0004 unchanged, Ollama remains the only active runtime).

## Rules

Every completed specification must include:

-   Implementation status
-   Test status
-   Review status
