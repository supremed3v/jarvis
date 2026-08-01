# JARVIS Development Environment

Implements: `context/features/SPEC-0002-development-environment.md`.

## Overview

This document covers the local toolchain setup required before any JARVIS
implementation work begins. It sets up tooling only — no application code
exists yet. See:

- ADR-0001 — Go backend runtime
- ADR-0003 — Electron desktop application (Node.js ecosystem)
- ADR-0004 — Ollama as the local AI model runtime (local-only, no
  mandatory API dependency)

## Prerequisites

| Tool   | Version        | Pin file      |
| ------ | -------------- | ------------- |
| Go     | 1.23           | `.go-version` |
| Node.js| 20 (LTS)       | `.nvmrc`      |
| npm    | bundled with Node | —          |
| Ollama | latest stable  | none (manual install) |
| Git    | any recent     | —             |

## Setup Steps

1. Install Go 1.23 (matching `.go-version`): https://go.dev/dl/
2. Install Node.js 20 LTS (matching `.nvmrc`), e.g. via `nvm install` /
   `nvm use` if you use a Node version manager.
3. Install Ollama: https://ollama.com/download
4. Copy the environment template and adjust as needed:
   ```
   cp .env.example .env
   ```
5. Run the verification script (see below) to confirm your toolchain is
   correctly set up.

## Hybrid LLM (optional)

`.env.example` reserves `NVIDIA_API_KEY` and `NVIDIA_API_BASE_URL` for a
future hybrid local+cloud LLM setup. These are **not wired to any code
yet** — they exist so a later spec (SPEC-0026 LLM Provider Interface /
SPEC-0029 Model Router) can pick them up without revisiting this spec.
Leave them blank to stay fully local. Ollama is the only active LLM
runtime today, per ADR-0004.

## Verifying Your Environment

From inside `scripts/`:

```
./verify_dev_environment.ps1
```

This checks that Go, Node/npm, and Ollama are present and reports their
versions against the pinned versions above. It also confirms
`.env.example` exists and is well-formed. It does not install anything.

## What This Does NOT Include Yet

- No `go.mod` — arrives with SPEC-0007 Go Runtime Bootstrap.
- No `package.json` or Electron app scaffold — arrives with SPEC-0063
  Electron Application Bootstrap.
- No way to actually "launch the desktop app" or "start the core
  runtime" — those verification steps are deferred to the specs above.
  See SPEC-0002's Build Tracker notes for how its testing criteria were
  rescoped in the meantime.
