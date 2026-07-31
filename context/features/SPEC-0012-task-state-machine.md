# JARVIS Task State Machine

## Overview

Define lifecycle states for task execution.

## Requirements

Support states:

    CREATED
    PLANNING
    QUEUED
    EXECUTING
    WAITING
    FAILED
    COMPLETED
    CANCELLED

## Rules

Define valid state transitions.

Example:

CREATED -\> PLANNING -\> QUEUED -\> EXECUTING -\> COMPLETED

Invalid transitions must be rejected.

## Testing

Verify: 1. Valid transitions succeed 2. Invalid transitions fail 3.
State history is tracked
