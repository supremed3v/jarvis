# Current Feature: SPEC-0040 Knowledge Ingestion Pipeline

## Working In

Memory layer, Phase 4 Intelligence (`services/core`)

## Status

In Progress

## Goals

- Ingest external knowledge into JARVIS memory from multiple source types:
  Markdown files, PDFs, text files, code repositories, documentation
- Implement the pipeline: Input -> Parser -> Chunking -> Embedding -> Storage
- Verify: documents ingest successfully, content is searchable, metadata
  remains attached

## Dependencies

- SPEC-0034 Memory Interface (status: Completed)
- SPEC-0035 Memory Storage Abstraction (status: Completed)
- SPEC-0038 Vector Memory Engine (status: Completed)
- SPEC-0039 Embedding Pipeline (status: Completed) — SPEC-0040 sits directly
  on top of this: 0039 already provides `Chunker`/`FixedSizeChunker`,
  `EmbeddingPipeline`, and an `Embedder` (`OllamaEmbedder`) taking
  `EmbeddingInput` (source type + content + metadata) through to storage.
  0040's job is the missing front half — Input and Parser stages for
  Markdown/PDF/text/code-repo/documentation sources — that feed into 0039's
  existing chunk/embed/store stages.

## Notes

Specification:

context/features/SPEC-0040-knowledge-ingestion-pipeline.md

Dependency resolution source: Requirements inference (cross-referenced
against JARVIS_IMPLEMENTATION_ORDER.md Phase 4 "Memory" grouping and
JARVIS_DEPENDENCY_GRAPH.md's "Research requires: ... Knowledge, Memory"
line). The Feature Index does not yet carry explicit Dependencies/Related
fields for this entry, so Step 4's fallback chain was used; the spec's own
pipeline diagram (Input -> Parser -> Chunking -> Embedding -> Storage) makes
the Chunking/Embedding/Storage prerequisite on SPEC-0039/0038/0035/0034
directly inferable.

## History

- 2026-08-03 load loaded SPEC-0040 (Knowledge Ingestion Pipeline)
- 2026-08-03 start implemented SPEC-0040 on branch
  feature/knowledge-ingestion-pipeline. New services/core/knowledge_ingestion.go:
  `KnowledgeIngestionPipeline` supplies the Input and Parser stages ahead of
  SPEC-0039's existing Chunking/Embedding/Storage stages. `DetectFormat`
  routes by file extension to one of four `IngestFormat`s (Markdown, PDF,
  Text, Code); `PlainTextParser` handles Markdown/Text/Code as-is,
  `PDFParser` extracts text via the new pure-Go `github.com/ledongthuc/pdf`
  dependency (no cgo, local-first per ADR-0008). `IngestFile` parses one
  file and hands it to `EmbeddingPipeline.Process`; `IngestDirectory` walks
  a tree (code repositories / documentation sets), skipping unrecognized
  files and `.git`/`node_modules`/`vendor`/etc. Both attach
  path/filename/format metadata (plus parser-derived fields like a PDF's
  page count) alongside SPEC-0039's own source/chunk metadata.
  `services/core/container.go` gained a `KnowledgeIngestionPipeline` slot +
  `WithKnowledgeIngestionPipeline` option, matching SPEC-0039's precedent.
  No changes to SPEC-0034/0035/0038/0039 contracts. 20 tests added in
  knowledge_ingestion_test.go (including a hand-built minimal in-memory PDF
  fixture, avoiding a binary testdata file) plus container_test.go extended
  for the new slot; covers SPEC-0040's three testing criteria (documents
  ingest successfully, content is searchable, metadata remains attached)
  across all five required source types. `go build`/`vet`/`test` clean for
  package `core` (scoped non-recursively — `services/core/voice` is a
  separate, unrelated in-progress subpackage of the user's own that
  currently fails to build on its own dependencies; left untouched per
  explicit instruction). One pre-existing flaky test unrelated to this
  change: `TestWorker_FailsTerminallyAfterMaxAttempts` (task_worker_test.go,
  timing-based, fails intermittently even on its own in isolation). Not yet
  merged; `/feature test`/`/feature review`/`/feature complete` still
  pending.
