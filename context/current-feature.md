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

Both SPEC-0040 (Knowledge Ingestion Pipeline) and SPEC-0053/0054/0055
(Audio Engine Interface / Microphone Capture System / Wake Word Detection)
are now `Completed` and merged to master — see History below and their
entries in `docs/agents/JARVIS_BUILD_TRACKER.md` for the full record.

Next candidate: continuing the Voice branch of Phase 4 Intelligence per
the voice-first MVP prioritization recorded in
`docs/execution/JARVIS_MVP_SCOPE.md` — SPEC-0056 (Speech To Text Provider
interface) and SPEC-0057 (Whisper Integration), matching
`docs/IMPLEMENTATION_PLAN.md`'s "Phase 2: STT + TTS" roadmap. SPEC-0041/0042
(Memory Retrieval/Consolidation) and the Tools layer (SPEC-0043-0052) remain
Planned and are also unblocked, per `JARVIS_IMPLEMENTATION_ORDER.md`'s
parallel Phase 4 branches - which to pick up next is a product-priority call
for whoever loads the next feature.

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
- 2026-08-03 fixed SPEC-0053/0054/0055 (Audio Engine Interface / Microphone
  Capture System / Wake Word Detection, the first three specs of the Voice
  branch of Phase 4 Intelligence) alongside the above. Another agent's
  concurrent draft in this same working tree was non-functional: the
  `core.WakeWordDetector` interface had no way to receive audio at all
  (`Microphone` fanned captured audio into a channel nothing read), PCM audio
  was read via `bufio.Scanner` (newline-unsafe for binary data, 64KB cap),
  two real concurrency bugs (send-on-closed-channel and an unsynchronized
  nil-deref race), neither Python helper script existed anywhere in the
  repo, and `voice_test.go`'s three tests could never fail (real errors were
  `t.Logf`+`return`, not `t.Fail`). Fixed all of the above — see the
  SPEC-0053 entry in `docs/agents/JARVIS_BUILD_TRACKER.md` for the full
  write-up (interface change, new `framing.go` length-prefixed protocol,
  `//go:embed`-ed Python scripts, self-owned cancellation in
  `WakeWordDetectorImpl.Stop`, rewritten tests with real assertions/honest
  `t.Skip`). Also recorded a product-scope decision:
  `docs/execution/JARVIS_MVP_SCOPE.md`/`CLAUDE.md` now state voice (not the
  desktop chat window) is the primary MVP interaction surface, per user
  direction. `go build`/`vet`/`test` clean for the `voice` subpackage and for
  package `core` non-recursively. One incident: found and fixed (with
  explicit user approval, since it required a raw-byte file truncation) a
  UTF-16 corruption in `docs/agents/JARVIS_BUILD_TRACKER.md` caused by
  another agent's own tracker-update script. Not yet merged.
- 2026-08-03 complete: split into two commits (SPEC-0040 on
  feature/knowledge-ingestion-pipeline; the voice fix moved to a new
  feature/voice-pipeline-fix branch, matching this repo's one-feature-
  per-branch merge convention) and merged both into master
  (`Merge feature/knowledge-ingestion-pipeline: SPEC-0040 Knowledge
  Ingestion Pipeline`, `Merge feature/voice-pipeline-fix: Fix
  SPEC-0053/0054/0055 Voice Pipeline`), both clean no-conflict merges.
  Re-verified `go build`/`vet`/`test` clean on master for package `core`,
  the `voice` subpackage, and `packages/config`/`errors`/`logger`; `gofmt`
  diffs on merged files confirmed to be the pre-existing CRLF/LF checkout
  artifact only. Pushed master to origin
  (`https://github.com/supremed3v/jarvis.git`, 450deb9..c6bd936).
