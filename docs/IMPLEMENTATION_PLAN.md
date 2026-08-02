# JARVIS Voice-First "Iron Man" Implementation Plan

## Vision & Goals

**What we're building:** A local-first, voice-activated AI assistant that feels like Iron Man's JARVIS — always listening, understands context, and can act on your behalf across your development environment.

**Target user:** Senior software developer (6+ years) building SaaS projects who wants:
- **Voice-first interaction** — "Jarvis, check my GitHub notifications and summarize the failing CI"
- **Claude Code integration** — JARVIS spawns/manages `claude-code` sessions, learns from them, acts on your behalf
- **Full dev environment control** — Terminal, filesystem, Git, GitHub, AWS, email — all voice-controllable
- **Local-first privacy** — No mandatory cloud services; self-hosted sync optional
- **Proactive assistance** — Daily briefings, habit coaching, context-aware suggestions

**End-state capabilities:**
```
"Jarvis" → [wake word]
    ↓
[Streaming STT] → "Check my GitHub notifications and summarize the failing CI on api-service"
    ↓
[GitHub Tool] → fetches notifications + Actions runs
    ↓
[Developer Agent] → analyzes failure, suggests fix
    ↓
"Want me to open the failing test and propose a fix?"
    ↓
"Yes, and create a branch"
    ↓
[Terminal Tool] → git checkout -b fix/ci-failure
[Filesystem Tool] → opens test file
[Developer Agent] → writes fix, runs test
    ↓
"Tests pass. Push and open PR?"
    ↓
"Yes"
    ↓
[GitHub Tool] → push + create PR with description
    ↓
"PR #247 opened. CI running now." → [Streaming TTS]
```

---

## Overview
Voice-first personal AI assistant for a senior software developer (6+ years) building SaaS projects. Integrates with Gmail, Outlook, GitHub, AWS, and Claude Code CLI.

## Architecture Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Wake word engine | openWakeWord via Python subprocess | No CGO, works on Windows |
| STT | faster-whisper (Python) | Fastest, easy model swap |
| TTS | Piper binary (en_US-amy-medium) | Natural voice, prebuilt binaries |
| Audio I/O | Python subprocess (sounddevice) | Avoids CGO/miniaudio issues on Windows |
| Terminal | PTY via creack/pty | Native claude-code integration |
| Desktop | Electron + React + xterm.js | Cross-platform, rich UI |
| Bridge | WebSocket (JSON) | Low latency, bidirectional |
| Sync | Encrypted (Age) + CRDT | Self-hosted, privacy-first |

---

## Phase 1: Foundation Extensions (COMPLETED)

### Modified Specs
- **SPEC-0003 Config**: Added `VoiceConfig` struct + env overrides
- **SPEC-0004 Shared Types**: Added `VoiceCapabilities` pointer to `Agent`
- **SPEC-0008 Container**: Added 7 new interface slots

### New Specs Implemented
| Spec | File | Status |
|------|------|--------|
| SPEC-0053 | `services/core/voice/audio_engine.go` | ✅ |
| SPEC-0054 | `services/core/voice/microphone.go` | ✅ |
| SPEC-0055 | `services/core/voice/wake_word.go` | ✅ |

### Files Created/Modified
```
packages/config/config.go          # + VoiceConfig
packages/config/load.go            # + JARVIS_VOICE_* env vars
packages/shared-types/agent.go     # + VoiceCapabilities ptr
packages/shared-types/types_test.go # + JSON tests
services/core/container.go         # + 7 interfaces + slots + WithXxx
services/core/agent.go             # + VoiceCapabilities in AgentMetadata
services/core/voice/audio_engine.go
services/core/voice/microphone.go
services/core/voice/wake_word.go
services/core/voice/voice_test.go
context/features/SPEC-0053-*.md
context/features/SPEC-0054-*.md
context/features/SPEC-0055-*.md
```

---

## Phase 2: STT + TTS (NEXT)

| Spec | Implementation | Key Files |
|------|----------------|-----------|
| SPEC-0057 | faster-whisper Python subprocess | `services/core/voice/stt_whisper.go` |
| SPEC-0059 | Piper binary subprocess | `services/core/voice/tts_piper.go` |

### Integration Points
- Microphone fans out to both WakeWordDetector + STTProvider
- STT transcripts → EventBus → Agent execution
- Agent responses → TTSProvider → AudioEngine.Playback()
- WebSocket bridge streams transcripts + TTS audio to Electron

---

## Phase 3: Tools (Terminal + Filesystem)

| Spec | Implementation | Key Files |
|------|----------------|-----------|
| SPEC-0050 | PTY terminal + claude-code sessions | `services/tools/terminal.go` |
| SPEC-0049 | Sandboxed filesystem access | `services/tools/filesystem.go` |

### Terminal Tool Features
- `Execute()` - one-shot commands with allowlist
- `StartSession()` - persistent PTY
- `StartClaudeCode()` - spawns `claude-code --session <id>`
- ANSI capture → stream to Electron xterm.js
- Session recording → Memory (vector store)

---

## Phase 4: Desktop Shell

| Spec | Implementation |
|------|----------------|
| SPEC-0063 | Electron + Vite + React |
| SPEC-0065 | WebSocket bridge + typed IPC |

### Electron UI Components
```
apps/desktop/
├── src/main.ts          # Main process, WS bridge, tray
├── src/preload.ts       # contextBridge APIs
└── renderer/
    ├── App.tsx          # Layout: VoiceOrb + Transcript + Terminal
    ├── VoiceOrb.tsx     # Animated orb (idle/listening/speaking)
    ├── Transcript.tsx   # Streaming transcript
    └── Terminal.tsx     # xterm.js + fit addon
```

---

## Phase 5: Integrations (Deferred)

Only Email has an existing spec today (SPEC-0090, `Planned`). GitHub, AWS,
Encrypted Sync, and a SaaS Project Agent aren't in the 182-spec library
(`context/features/` runs SPEC-0001 through SPEC-0182) - no SPEC-0183+ files
exist yet, so this table intentionally doesn't cite spec numbers for them.
Per ADR-0009 (Specification -> Planning -> Implementation), those specs
should be written before this phase starts, not invented here.

| Integration | Spec | Priority |
|-------------|------|----------|
| GitHub API | not yet specified | High |
| AWS SDK | not yet specified | High |
| Email (Gmail/Outlook) | SPEC-0090 | Medium |
| Encrypted Sync | not yet specified | Medium |
| SaaS Project Agent | not yet specified | Low |

---

## Dependencies

### Go (services/core/go.mod)
```go
github.com/creack/pty v1.1.21               # terminal PTY (Phase 3)
github.com/gorilla/websocket v1.5.1         # WS bridge (Phase 4)
```
`github.com/gen2brain/malgo` and `github.com/yalue/onnxruntime_go` were
added during an earlier, abandoned cgo-based attempt at audio/wake-word and
removed once the design moved to Python subprocesses - neither is a current
or planned dependency.

### Python (user machine)
```
faster-whisper
openwakeword
sounddevice
piper-tts (or binary)
```

### Binaries (downloaded by setup script)
```
models/
├── hey_jarvis.onnx              # openWakeWord
├── en_US-amy-medium.onnx        # Piper voice
├── en_US-amy-medium.onnx.json   # Piper config
```

---

## Windows Setup Script
`scripts/setup_windows.ps1`:
1. Creates `~/jarvis/models/`
2. Downloads Piper + Amy voice
3. Downloads openWakeWord model
4. Installs Python + faster-whisper
5. Generates default config.json

---

## Implementation Order

| Week | Specs | Deliverable |
|------|-------|-------------|
| 1 | 0053, 0054, 0055 | ✅ Wake word "Jarvis" works |
| 2 | 0057, 0059 | Voice ↔ Text loop works |
| 3 | 0050, 0049 | Terminal + FS tools work |
| 4 | 0063, 0065 | Electron app connects, shows orb + transcript |
| 5 | Integration | Full: "Jarvis" → voice → claude-code → voice response |

---

## Testing Criteria (Phase 1)

The authoritative record of Phase 1's actual status is
`docs/agents/JARVIS_BUILD_TRACKER.md`'s SPEC-0053 entry, not this checklist -
consult it for the real story, including defects found in the initial draft
(wake word audio wiring, binary-safe framing, two concurrency bugs, missing
Python scripts) and how they were fixed.

- [x] `go build .` / `go vet .` / `go test .` clean for the `voice`
      subpackage and for package `core` (scoped non-recursively - there are
      5 go.work modules, not 6, and `voice` is a subpackage of `core`, not
      its own module)
- [x] Voice tests: deterministic tests (Capture/Playback-before-Initialize,
      Shutdown/Stop no-ops, a wiring proof using test doubles) genuinely
      pass or fail; subprocess-dependent tests `t.Skip` honestly when
      Python/required packages/an audio device aren't available, rather
      than silently passing regardless of outcome
- [x] `generate_feature_index.ps1` updates FEATURE_INDEX.md (SPEC-0053/54/55
      now show `Completed` for real)
- [x] JARVIS_BUILD_TRACKER.md updated with SPEC-0053/54/55 entries
- [ ] Real hardware verification (actual audio capture/playback/wake-word
      detection against real devices, Python, and a trained model) -
      unverified in the environment this was fixed in; next owner's job

---

## Key Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Python env issues | Bundle python-embed or use whisper.cpp CGO fallback |
| WebSocket audio latency | Binary frames, 20ms chunks, Opus encoding (future) |
| claude-code session mgmt | Persistent PTY per project, session ID in metadata |
| Audio device permissions | Fallback to "default", user config override |
| CGO build failures | Primary path uses Python subprocesses |

---

## Next Immediate Actions

1. **Implement SPEC-0057** (Whisper STT via faster-whisper subprocess)
2. **Implement SPEC-0059** (Piper TTS subprocess)
3. **Wire voice pipeline** in `runtime.go` startup
4. **Add EventBus events** for wake word, transcript, TTS
5. **Create Python scripts** for audio_engine.py, stt_whisper.py, tts_piper.py, wake_word_detector.py