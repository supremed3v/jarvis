# Current Feature

## Overview
_None active._

## Status
Not Started

## Goals
_None._

## Files Modified
_None._

## Notes
_None._

## History
- 2026-08-02 SPEC-0027 Ollama Integration — Completed. Added OllamaProvider implementing the SPEC-0026 Provider interface for connecting to local Ollama servers. Created services/core/ollama_provider.go with HTTP client for Ollama's REST API (/api/generate, /api/tags), supporting Generate (non-streaming), Stream (NDJSON streaming), ListModels, and HealthCheck. Added Provider slot to Container with WithProvider option. All tests pass including mocked server tests via httptest. Review pass found and fixed a real gofmt issue (struct-field misalignment, missing trailing newlines) in the two new files; re-verified clean after fix.
