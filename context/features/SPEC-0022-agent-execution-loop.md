# JARVIS Agent Execution Loop

## Overview

Implement the execution cycle used by agents.

## Execution Flow

    Receive Task
          |
    Analyze Context
          |
    Create Plan
          |
    Select Tools
          |
    Execute Actions
          |
    Evaluate Result
          |
    Return Response

## Requirements

Support:

-   Multi-step execution
-   Tool calls
-   Intermediate results
-   Failure handling

## Testing

Verify: 1. Agent completes simple tasks 2. Tool execution works 3.
Failures return useful results
