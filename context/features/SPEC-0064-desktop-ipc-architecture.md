# JARVIS Desktop IPC Architecture

## Overview

Create communication between Electron processes.

## Requirements

Support:

-   Main process communication
-   Renderer communication
-   Secure IPC channels
-   Typed messages

## IPC Responsibilities

Support communication for:

-   User commands
-   Runtime status
-   Tool approvals
-   Voice events

## Testing

Verify: 1. Messages travel correctly 2. Unauthorized channels are
blocked 3. IPC errors are handled
