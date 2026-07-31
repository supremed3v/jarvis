# JARVIS LLM Provider Interface

## Overview

Create an abstraction layer for connecting JARVIS with different AI
model providers.

The core runtime must not depend directly on a single model
implementation.

## Requirements

Define provider interface supporting:

-   Generate text responses
-   Streaming responses
-   Model information
-   Health checks
-   Provider configuration

## Supported Providers

Initial provider:

-   Ollama

Future providers may include:

-   llama.cpp
-   Remote APIs
-   Other local runtimes

## Testing

Verify: 1. Provider interface can be implemented 2. Provider responses
follow contract 3. Provider failures are handled correctly
