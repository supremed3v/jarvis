# JARVIS Agent Interface

## Overview

Define the base contract for all JARVIS agents.

All agents must follow a common interface so the runtime can discover,
execute, and manage them consistently.

## Requirements

Define agent contract containing:

-   Agent ID
-   Agent name
-   Description
-   Instructions
-   Available tools
-   Memory access rules
-   Permission requirements
-   Execution handler

## Agent Responsibilities

Agents are responsible for:

-   Understanding assigned tasks
-   Creating execution plans
-   Using approved tools
-   Returning structured results

## Testing

Verify: 1. Agent interface can be implemented 2. Runtime can execute a
sample agent 3. Agent metadata validates correctly
