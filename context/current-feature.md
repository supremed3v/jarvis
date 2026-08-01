# Current Feature: Configuration System

## Working In

packages/config (Foundation layer, per docs/architecture and
JARVIS_IMPLEMENTATION_ORDER.md Phase 1)

## Status

Completed

## Goals

- Application settings [done — AppConfig: Environment, LogLevel]
- Model configuration [done — ModelConfig: Provider/OllamaHost/OllamaPort,
  reserved NvidiaAPIKey/NvidiaBaseURL fields unwired per ADR-0004]
- Tool permissions [done — ToolPermissions: FilesystemEnabled/
  TerminalEnabled/BrowserEnabled]
- Feature flags [done — Config.Features map[string]bool]
- Environment variables [done — JARVIS_ENV, LOG_LEVEL, OLLAMA_HOST,
  OLLAMA_PORT, NVIDIA_API_KEY, NVIDIA_API_BASE_URL override file/defaults]

## Dependencies

- SPEC-0001 Repository Foundation (status: Completed)
- SPEC-0002 Development Environment (status: Completed)

Note: FEATURE_INDEX.md previously showed both as "Planned" due to a bug in
scripts/generate_feature_index.ps1 (hardcoded status, never read the real
tracker). Fixed as part of this work — see History.

## Notes

Specification:

context/features/SPEC-0003-configuration-system.md

Implementation: packages/config (standalone Go module, module path
jarvis-pa/packages/config, go 1.23, stdlib-only — no go.sum). Independent
of the SPEC-0007 Go Runtime Bootstrap module; SPEC-0007 requirements list
"Configuration loading" as something it wires up, implying this package
needed to exist and be buildable/testable on its own first.

Files: go.mod, config.go (types + Defaults()), load.go (Load(path) —
defaults -> optional JSON file -> env override -> validate, fails safely
with an error on malformed file / bad env value / out-of-range value,
never a partial Config), config_test.go (7 tests, all passing).

Config file format is JSON (stdlib encoding/json) to avoid pulling in a
third-party YAML dependency for the very first Go module in the repo.

## History

- 2026-08-01 06:09 setup_feature.ps1 loaded SPEC-0003 (SPEC-0003-configuration-system.md)
- 2026-08-01 load: resolved dependencies via Implementation Order (Step 4 fallback); flagged SPEC-0001/SPEC-0002 as unimplemented prerequisites
- 2026-08-01 start: discovered SPEC-0001/SPEC-0002 were actually already Completed per docs/agents/JARVIS_BUILD_TRACKER.md; FEATURE_INDEX.md was stale because generate_feature_index.ps1 hardcoded "Planned" for every spec. Fixed the generator to read real status from the Build Tracker (regex on the tracker's own Status Values enum) and regenerated FEATURE_INDEX.md.
- 2026-08-01 implemented packages/config (SPEC-0003): go.mod + config.go + load.go + config_test.go. `go build ./...`, `go vet ./...`, `go test ./... -v` all pass (7/7 tests).
