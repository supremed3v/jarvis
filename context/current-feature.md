# Current Feature

## Working In

Not specified — no feature currently loaded.

## Status

Not Started

## Goals

_None yet._

## Dependencies

_None yet._

## Notes

Next candidate per docs/execution/JARVIS_IMPLEMENTATION_ORDER.md: SPEC-0036
(Conversation Memory), continuing the Memory branch of Phase 4 Intelligence
now that SPEC-0035 (Memory Storage Abstraction) is Completed.

## History

- 2026-08-02 SPEC-0035 Memory Storage Abstraction — Completed. Implemented
  services/core/memory_storage.go (`MemoryStorageProvider` interface,
  `StorageMemory` routing SPEC-0034's `Memory` by MemoryType with
  provider-name-encoded IDs), memory_storage_local.go (`LocalStore`),
  memory_storage_vector.go (`VectorStore`), 24 new tests, and a
  `Container.Memory`/`WithMemory` slot (found missing during review, fixed
  before completion). `go build`/`vet`/`test` clean across all 5 go.work
  modules. Merged feature/memory-storage-abstraction into master.
