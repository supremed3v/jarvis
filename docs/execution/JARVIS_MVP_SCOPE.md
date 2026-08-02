# JARVIS MVP Scope

## Goal

Build the first usable personal JARVIS before implementing the entire
architecture.

## MVP Features

## Core

Required:

-   Go runtime
-   Configuration
-   Logging
-   Event system

## AI

Required:

-   Ollama integration
-   Basic agent
-   Prompt system

## Memory

Required:

-   Conversation memory
-   User profile memory

## Tools

Required:

-   Filesystem
-   Terminal
-   Browser

## Interface

Voice is the primary MVP interaction surface, not the desktop chat window.
JARVIS is a voice-first assistant ("Jarvis" wake word -> speak -> hear a
response); a full chat interface (SPEC-0066) is secondary and deferrable
past MVP. The Application-phase target for MVP is a minimal visual
presence only: System Tray (SPEC-0068) for status/control, plus Voice
Interface UI (SPEC-0067, an orb/status view showing idle/listening/speaking
state) - not a message-history chat window. Streaming responses still
apply, but as streaming transcription/speech, not streaming chat text.

Required:

-   System tray presence
-   Voice interface UI (status/orb view)
-   Streaming responses (transcription + speech)

Deferred past MVP:

-   Full desktop chat interface (SPEC-0066)

## Voice

Required (core feature, not deferrable, and the primary interaction
surface per the Interface section above):

-   Whisper
-   Piper
-   Wake word

## Research

Required (core feature, not deferrable):

-   SearXNG
-   Browser research agent

## MVP Success Criteria

JARVIS can:

1.  Receive requests
2.  Reason with local models
3.  Use tools safely
4.  Remember context
5.  Execute useful tasks
