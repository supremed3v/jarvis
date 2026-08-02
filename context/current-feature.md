# Current Feature: Vector Memory Engine

## Working In

Phase 4 Intelligence — Memory branch (services/core). Upgrades the
placeholder `VectorStore` (services/core/memory_storage_vector.go, SPEC-0035)
from naive word-overlap scoring to real embedding-based similarity search,
behind the existing SPEC-0034 `Memory` / `MemoryStorageProvider` contracts.

## Status

In Progress

## Goals

- Embedding storage: persist an embedding vector alongside each MemoryRecord
- Similarity search: replace VectorStore.Query's word-overlap scoring with
  vector similarity (e.g. cosine)
- Metadata filtering: support filtering results via MemoryQuery.Filters /
  MemoryQuery.Type
- Ranking results: order search results by similarity score

## Dependencies

- SPEC-0034 Memory Interface (status: Completed) — Memory, MemoryRecord,
  MemoryQuery contracts this spec must keep satisfying
- SPEC-0035 Memory Storage Abstraction (status: Completed) — defines
  MemoryStorageProvider and the current VectorStore placeholder that this
  spec replaces the internals of; its own doc comment explicitly names
  SPEC-0038 as owning "real embedding-based similarity"
- SPEC-0036 Conversation Memory (status: Completed) — existing consumer of
  the Memory interface, must keep working unchanged
- SPEC-0037 User Profile Memory (status: Completed) — existing consumer of
  the Memory interface, must keep working unchanged

Note: SPEC-0039 Embedding Pipeline (status: Planned) is the *next* spec
after this one and is not a prerequisite — SPEC-0038's own Requirements
("Embedding storage") imply this spec needs some embedding source of its
own (or a stub) rather than depending on SPEC-0039.

## Notes

Specification:

context/features/SPEC-0038-vector-memory-engine.md

Dependency resolution source: Implementation Order (Phase 4 Memory branch)
+ Requirements inference, corroborated by the explicit forward-reference in
services/core/memory_storage_vector.go's doc comment ("Real embedding-based
similarity is SPEC-0038 ... this spec only needs a backend whose retrieval
behaves differently ... a naive word-overlap score - a placeholder relevance
measure, not a real vector similarity metric").

Relevant lock-in: ADR-0007 Storage Architecture — vector storage is the
locked choice for semantic memory/retrieval.

Existing code to build on:
- services/core/memory_interface.go (SPEC-0034)
- services/core/memory_storage.go, memory_storage_local.go,
  memory_storage_vector.go (SPEC-0035)

## History

- 2026-08-02 load loaded SPEC-0038
- 2026-08-02 start — branch feature/vector-memory-engine created. Implemented
  all four goals in services/core:
  - New services/core/memory_embedding.go: `Embedder` interface, `HashEmbedder`
    (deterministic hashing-trick term-frequency embedding, dependency-free —
    SPEC-0039 Embedding Pipeline can supply a model-backed Embedder behind the
    same interface later), and `cosineSimilarity`.
  - Rewrote services/core/memory_storage_vector.go: `VectorStore` now embeds
    each record's Content on `Put`/`Replace` (via a `VectorStoreOption`-
    configurable `Embedder`, defaulting to `HashEmbedder`), and `Query` scores
    candidates by cosine similarity against the query's embedding instead of
    raw word-overlap count, applies `q.Filters` as an equal-match filter over
    `rec.Metadata` (`matchesFilters`, via `reflect.DeepEqual`), then ranks
    descending by score (ties broken by `CreatedAt` ascending) and caps at
    `q.Limit` — same as before.
  - Extended services/core/memory_storage_vector_test.go: renamed the
    overlap-ranking test to reflect similarity-based ranking, added a test
    proving cosine similarity (not raw overlap count) drives ranking (exact
    match outranks a noisy superset match despite equal shared-word count),
    a metadata filtering test, and a Replace-recomputes-embedding test.
  - New services/core/memory_embedding_test.go: unit tests for
    `HashEmbedder.Embed` (deterministic, respects configured `Dims`, empty
    text -> zero vector, different content -> different vectors) and
    `cosineSimilarity` (identical -> ~1, orthogonal -> 0, zero vector -> 0
    not NaN).
  - `go build`/`go vet` clean; all new/changed tests pass
  (`TestVectorStore_*`, `TestHashEmbedder_*`, `TestCosineSimilarity_*`,
  `TestStorageMemory_*`). One pre-existing, unrelated flaky test
  (`TestWorker_FailsTerminallyAfterMaxAttempts` in task_worker_test.go)
  intermittently fails on master too (verified via `git stash`) — not caused
  by this change.
  - No changes to SPEC-0034's `Memory` contract, `MemoryRecord`/`MemoryQuery`
    shapes, `StorageMemory`, `LocalStore`, or any Memory consumer
    (Conversation/User Profile memory) — `NewVectorStore()` remains callable
    with no arguments.
- 2026-08-02 test — filled coverage gaps found while auditing against
  happy-path/error-handling/edge-case/integration-boundary requirements:
  added `TestVectorStore_RemoveMissingIsNotFound` (parity with LocalStore's
  existing equivalent), `TestVectorStore_QueryFiltersExcludeRecordsWithNilMetadata`
  (nil-Metadata edge case), `TestHashEmbedder_ZeroDimsDefaultsToDefaultEmbeddingDims`
  (zero-value HashEmbedder edge case), and
  `TestStorageMemory_SearchPropagatesFiltersToProvider` in
  memory_storage_test.go (integration boundary: q.Filters reaches VectorStore
  correctly when driven through the Memory/StorageMemory interface, not just
  via a direct provider.Query() call). `-race` unavailable in this
  environment (CGO_ENABLED=0, no cgo toolchain) so not run. Ran
  `scripts/go_all.ps1 all`: all 5 workspace modules clean (build/vet/test),
  including the previously-flaky TestWorker_FailsTerminallyAfterMaxAttempts
  passing this run. All 25 memory/vector-related tests individually verified
  passing via `go test -run "TestVectorStore|TestHashEmbedder|TestCosineSimilarity|TestStorageMemory" -v`.
- 2026-08-02 review — reviewed against docs/agents/CODE_REVIEW_PROTOCOL.md
  (Architecture/Code Quality/Security/Testing). No issues found: all four
  SPEC-0038 goals completed; SPEC-0034 `Memory` contract and SPEC-0035
  `MemoryStorageProvider` contract untouched; no new third-party dependency
  (go.mod/go.sum unchanged); changes confined to services/core memory files;
  embeddings map mutated only under VectorStore's existing single mutex, no
  new race risk; no sensitive data logged; ADR-0007 and ADR-0008 respected;
  VectorStore correctly stays in-memory, consistent with LocalStore's own
  "no real DB dependency this phase" precedent. `scripts/go_all.ps1 all`
  re-run clean (5/5 modules). Verdict: Ready to complete.
