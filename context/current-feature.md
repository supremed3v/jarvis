# Current Feature: Memory Interface

## Working In

`services/core` (matches SPEC-0026 LLM Provider Interface's precedent of an
interface-only spec living alongside the runtime it serves, rather than a
new module) — first spec of the Memory layer, defining the storage-agnostic
interface the runtime and future memory providers (SPEC-0035 Memory Storage
Abstraction, SPEC-0036 Conversation Memory, SPEC-0037 User Profile Memory)
will implement.

## Status

Completed

## Goals

- Define memory operations: Store, Retrieve, Search, Update, Delete
- Support memory types: Conversation, User Profile, Knowledge, Experience
- Memory providers implement the interface via contract, not concrete storage

## Dependencies

- SPEC-0018..0025 Agent layer (status: Completed)
- SPEC-0026..0033 LLM layer (status: Completed)
- No specific spec is declared as a hard prerequisite in FEATURE_INDEX.md
  (it has no Dependencies field); resolved via
  docs/execution/JARVIS_DEPENDENCY_GRAPH.md, which places Memory after
  Agents/LLM/Tools in the core flow, and via
  docs/execution/JARVIS_IMPLEMENTATION_ORDER.md, where SPEC-0034 is the next
  spec after SPEC-0033 (Token Budget Manager, the last LLM-layer spec).

## Notes

Specification:

context/features/SPEC-0034-memory-interface.md

Index status at load time: Planned

Dependency resolution source: Implementation Order + Dependency Graph (Step 4
fallback chain — FEATURE_INDEX.md itself carries no per-spec Dependencies
field yet).

Related specs: SPEC-0035 Memory Storage Abstraction, SPEC-0036 Conversation
Memory, SPEC-0037 User Profile Memory, SPEC-0038 Vector Memory Engine (all
build on this interface; none are prerequisites).

## History

- 2026-08-02 05:17 setup_feature.ps1 loaded SPEC-0034 (SPEC-0034-memory-interface.md)
- 2026-08-02 load resolved dependencies (Step 4) and confirmed all prerequisite layers (Agent, LLM) are Completed per JARVIS_BUILD_TRACKER.md
- 2026-08-02 start: branched feature/memory-interface off master; implemented services/core/memory_interface.go (Memory interface, MemoryRecord, MemoryQuery, MemoryType) and memory_interface_test.go (11 tests); go build/vet/test clean across all 5 go.work modules; updated JARVIS_BUILD_TRACKER.md and regenerated FEATURE_INDEX.md; status set to Completed
