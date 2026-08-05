# SPEC-0200: Runtime Entrypoint

## Status: In Progress

## Overview

Create the `cmd/jarvis/main.go` binary that boots the Go runtime, wires every completed subsystem into a live Container, starts the WebSocket bridge, and blocks until interrupted — the missing piece that turns the library-only codebase into a runnable server the Electron desktop app (and voice pipeline) can connect to.

## Dependencies

- SPEC-0007 (Runtime Bootstrap) — `Runtime.Run` lifecycle
- SPEC-0008 (Dependency Container) — `Container` + all `With*` options
- SPEC-0009 (Event Bus) — `Bus`
- SPEC-0020 (Agent Registry) — `Registry`
- SPEC-0021 (Agent Lifecycle) — `LifecycleManager`
- SPEC-0022 (Execution Loop) — `ExecutionLoop`
- SPEC-0026/0027 (LLM Provider / Ollama) — `OllamaProvider`
- SPEC-0029 (Model Router) — `ModelRouter`
- SPEC-0030 (Prompt Templates) — `PromptRegistry`
- SPEC-0031 (Stream Handler) — `StreamHandler`
- SPEC-0032 (Context Window Manager) — `WindowManager`
- SPEC-0033 (Token Budget Manager) — `BudgetManager`
- SPEC-0034/0035 (Memory Interface / Storage) — `StorageMemory`, `LocalStore`
- SPEC-0036 (Conversation Memory) — `ConversationMemory`
- SPEC-0037 (User Profile Memory) — `UserProfileMemory`
- SPEC-0041 (Memory Retrieval) — `MemoryRetriever`
- SPEC-0042 (Consolidation Engine) — `ConsolidationEngine`
- SPEC-0048 (Tool Approval) — `ApprovalQueue`
- SPEC-0065 (WS Bridge) — `Bridge`
- SPEC-0060 (Voice Session Manager) — `SessionManager` (optional, depends on Python/Piper availability)

## Requirements

1. **Binary entrypoint** — `services/core/cmd/jarvis/main.go` (inside the existing `services/core` module, no new `go.mod`).
2. **Full wiring** — Constructs a `Container` with every completed subsystem: config, logger, event bus, agent registry + lifecycle manager, LLM provider (Ollama, configured from config), model router, stream handler, prompt registry, window manager, budget manager, memory (StorageMemory over LocalStore), conversation memory, user profile memory, memory retriever, consolidation engine, approval queue, memory viewer, and WS bridge.
3. **Default agent** — Registers a "jarvis" agent (via `NewAgentFromManifest`) that uses the `ExecutionLoop` with a simple Ollama-backed planner (sends user text to the LLM, returns the response). This is intentionally minimal — just enough to prove the pipeline works end-to-end.
4. **Signal handling** — Uses `signal.NotifyContext` for SIGINT/SIGTERM, passes to `Runtime.Run`, as SPEC-0007's tracker entry anticipated.
5. **Voice wiring (best-effort)** — If voice dependencies (Python, Piper, wake word model) are available, wires the voice pipeline. If not, logs a warning and runs without voice.
6. **Startup banner** — Logs the listen address and which subsystems are active.

## Testing Criteria

1. `go build ./cmd/jarvis/` succeeds from `services/core`.
2. Running the binary with Ollama available starts the WebSocket server on `:42321`.
3. The Electron desktop app connects and displays "ready" status.
4. Submitting a command through the desktop app reaches Ollama and returns a response.
