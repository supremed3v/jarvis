# JARVIS Agent Context Builder

## Overview

Create the system responsible for building context before agent
execution.

## Requirements

Context may include:

-   User message
-   Conversation history
-   Memories
-   Task information
-   Available tools
-   Previous execution results

## Requirements

The builder must:

-   Avoid unnecessary context
-   Maintain ordering
-   Support token limits

## Testing

Verify: 1. Context is generated correctly 2. Required information is
included 3. Large contexts are handled
