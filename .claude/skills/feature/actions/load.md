# Load Action

## Purpose

Load a JARVIS feature specification into the active implementation context,
routed through the Feature Index rather than opening the SPEC file directly.

Implements: `context/features/JARVIS_FEATURE_INDEX_INTEGRATION_SPEC.md`.

## Input

Command:

```
/feature load <spec-name>
```

Example:

```
/feature load SPEC-0007-go-runtime-bootstrap
```

If no argument is given, return:

```
Error: load requires a specification name.
```

## Steps

### Step 1 — Validate Input

Confirm a spec name (or SPEC-ID) was provided. If not, return the error above
and stop.

### Step 2 — Read Feature Index First

Before opening any SPEC file, read:

```
context/features/FEATURE_INDEX.md
```

Use it to:

- Locate the matching `SPEC-NNNN` entry and its filename
- Identify its feature layer / category (Foundation, Runtime, Task
  Execution, Agent, Intelligence, Memory, Tools, Voice, Application,
  Advanced Systems — per `FEATURE_INDEX_SPEC.md`)
- Note its Status

The current generator (`scripts/generate_feature_index.ps1`) only emits
`File` and `Status` per entry — it does not yet carry explicit
`Dependencies` / `Related` fields. If a future index entry does contain
those fields, prefer them. Otherwise, resolve dependencies in Step 4.

### Step 3 — Read the Specification

Load:

```
context/features/{spec-file}.md
```

Understand its Overview, Requirements, and Testing/acceptance criteria.

### Step 4 — Resolve Dependencies

Since per-spec dependency data isn't authoritative in the index yet,
determine dependencies by cross-referencing, in order:

1. `docs/execution/JARVIS_IMPLEMENTATION_ORDER.md` — which phase the spec
   belongs to, and which phases must already be implemented.
2. `docs/execution/JARVIS_DEPENDENCY_GRAPH.md` — phase-level dependency
   flow and any explicit "Critical Dependencies" entry matching this
   feature.
3. The spec's own Requirements text — infer concrete prerequisite specs
   (e.g. a requirement to load configuration implies the Configuration
   System spec; a requirement to initialize logging implies the Logging
   System spec).

Record the resolved list of prerequisite SPEC IDs. If a prerequisite spec's
Status (per the index) is not yet implemented, flag this explicitly — do not
silently proceed as if it's done.

### Step 5 — Load Supporting Context (only what's relevant)

Read only the documents relevant to this spec's implementation area:

- `docs/architecture/` — `JARVIS_MASTER_ARCHITECTURE.md` / `SYSTEM_OVERVIEW.md`
- `docs/execution/JARVIS_DEPENDENCY_GRAPH.md` (if not already read in Step 4)
- `docs/decisions/ADR-*.md` — any ADR whose "Concern" matches this feature

Do not bulk-load unrelated specs, architecture docs, or ADRs.

### Step 6 — Update Current Feature

Write `context/current-feature.md`, overwriting any prior contents:

```md
# Current Feature: <feature-name>

## Working In

<implementation-area>

## Status

Not Started

## Goals

- Goal 1
- Goal 2

## Dependencies

- SPEC-XXXX (status: <status from index>)

## Notes

Specification:

context/features/SPEC-XXXX.md

Dependency resolution source: <Implementation Order | Dependency Graph | Requirements inference>

## History

- <date> load loaded SPEC-XXXX
```

## Automation

This manual workflow has scripted equivalents under `scripts/` — prefer them
when you can run a shell, and use the manual steps above when reasoning
about the result:

- `scripts/setup_feature.ps1 <spec-name>` performs Steps 2–4 and 6
  mechanically: reads `FEATURE_INDEX.md`, resolves and validates
  dependencies, reads the SPEC file, and regenerates
  `context/current-feature.md`.
- `scripts/check_dependencies.ps1` validates every `Dependencies:` block in
  `FEATURE_INDEX.md` against files that actually exist in
  `context/features/` — run it whenever the index is edited.
- `scripts/validate_specs.ps1` checks SPEC filename format, required
  sections, and duplicate SPEC IDs — useful before trusting a spec's
  content during Step 3.
- `scripts/validate_structure.ps1` confirms the repository skeleton this
  workflow depends on (`context/features/`, `docs/execution/`, etc.) is
  intact.

All four are run from inside `scripts/` (they use relative `../` paths).

## Agent Rules

- Always read `FEATURE_INDEX.md` before opening a SPEC file.
- Never skip dependency discovery, even when the index itself has no
  Dependencies field — resolve via Step 4's fallback chain instead.
- Never bulk-load all specifications.
- Keep loaded context minimal — only the target spec, its resolved
  dependencies, and directly relevant architecture/ADR docs.
- Respect locked architecture decisions (`docs/decisions/ADR-*.md`); do not
  redesign without `docs/agents/ARCHITECTURE_CHANGE_PROTOCOL.md`.

## Output

Confirm:

```
Feature Loaded Successfully

Specification:
SPEC-XXXX

Dependencies:
SPEC-AAAA
SPEC-BBBB

Status:
Not Started
```

Followed by a brief summary of the feature, its dependencies (with
implementation status), and expected implementation area.
