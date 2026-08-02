# Current Feature: Memory Storage Abstraction

## Working In

services/core — SPEC-0034 (Memory Interface) was implemented directly in
services/core (memory_interface.go), not services/memory (still a
`.gitkeep`-only scaffold), per its commit message: "SPEC-0035 (Memory
Storage Abstraction) will supply the first concrete implementation." Follow
that precedent unless the spec text or an architecture doc calls for
introducing services/memory as its own module.

## Status

Completed

## Goals

- Storage abstraction layer that backs the SPEC-0034 `Memory` interface
  (Store/Retrieve/Search/Update/Delete over `MemoryRecord`/`MemoryQuery`)
- Local (relational) database storage provider
- Vector storage provider
- Design that leaves room for future storage providers
- Hide storage implementation details from agents (agents depend only on
  the `Memory` interface, never on a concrete backend)

## Dependencies

- SPEC-0034 Memory Interface (status: Completed) — direct prerequisite;
  this spec supplies the first concrete implementation(s) of the `Memory`
  contract defined there (services/core/memory_interface.go)
- SPEC-0003 Configuration System (status: Completed) — storage backends
  will need configuration (e.g. DB path/DSN, vector store settings)
- SPEC-0006 Error Handling System (status: Completed) — storage errors
  should use `packages/errors`, consistent with memory_interface.go
- ADR-0007 Storage Architecture (Accepted) — locks the two required
  backends: relational (settings/tasks/config today, extended here to
  memory storage) and vector (semantic memory/retrieval)

## Notes

Specification:

context/features/SPEC-0035-memory-storage-abstraction.md

Index status at load time: Planned

Dependency resolution source: Requirements inference (Implementation
Order / Dependency Graph only give phase-level granularity — Phase 4
Intelligence bundles Agents/LLM/Memory/Tools together with no per-spec
detail) cross-referenced against the SPEC-0034 commit history and
ADR-0007.

Testing to satisfy (from spec): 1. storage providers can be swapped 2.
data contracts remain consistent 3. storage errors are handled.

## History

- 2026-08-02 21:03 setup_feature.ps1 loaded SPEC-0035 (SPEC-0035-memory-storage-abstraction.md)
- 2026-08-02 load (manual) resolved dependencies against SPEC-0034, ADR-0007; refined Working In and Goals
- 2026-08-02 start: completed SPEC-0034 (merged feature/memory-interface into master as an explicit merge commit); branched feature/memory-storage-abstraction off updated master; status set to In Progress
- 2026-08-02 implemented services/core/memory_storage.go (`MemoryStorageProvider` interface, `StorageMemory` router implementing `Memory` by routing per `MemoryType` and encoding the provider name into returned IDs), memory_storage_local.go (`LocalStore`), memory_storage_vector.go (`VectorStore`), and 24 tests across three test files covering all three spec testing criteria; `go build`/`vet`/`test` clean across all 5 go.work modules, `gofmt -l` clean; updated JARVIS_BUILD_TRACKER.md and regenerated FEATURE_INDEX.md; status set to Completed
