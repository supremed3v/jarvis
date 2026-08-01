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
  SPEC-0001   Completed           Structure created and validated (scripts/validate_structure.ps1: 14/14). Existing scaffold (packages/config, packages/logger, packages/shared-types, services/memory, services/tools, services/voice) kept authoritative over SPEC-0001's literal tree; configs/ and specs/ added to close the remaining gap — divergence approved by user, recorded in context/current-feature.md history. Build/test steps (go build/go test) not applicable at the repository-root level yet — no root-level go.mod/go.work exists until SPEC-0007 Go Runtime Bootstrap ties the modules together. This does not block individual Foundation-layer packages from having their own standalone go.mod: see SPEC-0003 below, which added one for packages/config since SPEC-0007 itself depends on that package already existing and being buildable/testable in isolation.
  SPEC-0002   Completed         Toolchain setup only (.go-version=1.23, .nvmrc=20, .env.example, scripts/verify_dev_environment.ps1, docs/DEVELOPMENT.md) — no go.mod/package.json/app code, per JARVIS_IMPLEMENTATION_ORDER.md (those arrive in SPEC-0007/SPEC-0063). Testing criteria 2 ("desktop launches") and 3 ("core runtime starts") rescoped to toolchain smoke-checks (node/npm present; go version present) since no Electron app or Go runtime exists yet — full criteria deferred to SPEC-0007/SPEC-0063. verify_dev_environment.ps1 run locally: Go 1.26.2, Node v24.15.0, npm 11.12.1 detected (all newer than pins — warning only, not failure); Ollama not installed locally (warning only). .env.example includes an optional NVIDIA_API_KEY/NVIDIA_API_BASE_URL pair reserved for a future hybrid local+cloud LLM path (not wired to code; ADR-0004 unchanged, Ollama remains the only active runtime).
  SPEC-0003   Completed         packages/config: standalone Go module (module jarvis-pa/packages/config, go 1.23, stdlib-only, no go.sum) — independent of the SPEC-0007 Go Runtime Bootstrap module, since SPEC-0007 lists "Configuration loading" as something it wires up (implying this package must exist and be buildable/testable on its own first). Implements Config{App, Model, Tools, Features} and Load(path) layering defaults -> optional JSON file -> env var overrides (JARVIS_ENV, LOG_LEVEL, OLLAMA_HOST, OLLAMA_PORT, NVIDIA_API_KEY, NVIDIA_API_BASE_URL) -> validation; fails safely (returns error, never a partial Config, never panics) on malformed file, bad env value, or out-of-range value. JSON chosen over YAML to avoid a third-party dependency for the repo's first Go module. `go build ./...`, `go vet ./...`, `go test ./... -v` all pass (7/7 tests). Also fixed scripts/generate_feature_index.ps1, which had hardcoded `Status: Planned` for every spec unconditionally — it now reads real status from this tracker (regex on the Status Values enum below), so FEATURE_INDEX.md no longer falsely shows completed specs as Planned.
  SPEC-0004   Completed         packages/shared-types: standalone Go module (module jarvis-pa/packages/shared-types, go 1.23, stdlib-only, no go.sum), following the packages/config precedent for Foundation-layer packages. Defines four framework-independent, serializable data contracts with no business logic: Event (id/type/source/timestamp/payload), Task (id/type/status via TaskStatus enum/input/result/error/timestamps), Agent (id/name/type/status via AgentStatus enum/capabilities), Tool (name/description/parameters/permissions). EventType intentionally has no hardcoded constants — concrete event names are producer-specific and belong to future services (SPEC-0009 Event Bus onward), not to this shared contract. `go build ./...`, `go vet ./...`, `go test ./... -v`, `gofmt -l .` all clean (12/12 tests pass); coverage reports "no statements" as expected for a types-only package. Tests cover JSON round-trips for all four types, omitempty/zero-value edge cases, malformed-JSON error handling (fails safely, never panics), and cross-service wire-compatibility (unknown fields ignored for forward-compat, enums encode as plain JSON strings).

## Rules

Every completed specification must include:

-   Implementation status
-   Test status
-   Review status
