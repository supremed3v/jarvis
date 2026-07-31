# JARVIS Model Configuration System

## Overview

Create centralized model configuration management.

## Requirements

Support:

-   Default model selection
-   Agent-specific models
-   Temperature settings
-   Token limits
-   Runtime parameters

Example:

``` yaml
models:
  general:
    provider: ollama
    name: qwen

  coding:
    provider: ollama
    name: qwen-coder
```

## Testing

Verify: 1. Configuration loads 2. Agent model overrides work 3. Invalid
configuration is rejected
