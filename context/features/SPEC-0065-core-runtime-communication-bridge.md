# JARVIS Core Runtime Communication Bridge

## Overview

Create the bridge between Electron and the Go runtime.

## Requirements

Support:

-   Sending tasks to core
-   Receiving events
-   Streaming responses
-   Runtime status updates

## Communication Options

Initial implementation may use:

-   Local HTTP
-   WebSocket
-   IPC bridge

## Testing

Verify: 1. Desktop connects to runtime 2. Tasks are transmitted 3.
Responses return correctly
