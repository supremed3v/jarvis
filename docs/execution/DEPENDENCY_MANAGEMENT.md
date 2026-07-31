# JARVIS Dependency Management

## Purpose

Ensure components are implemented in a valid order.

## Rules

Features must not be implemented before dependencies exist.

Example:

Agent system requires:

-   Runtime
-   Event system
-   LLM interface
-   Memory interface

## Dependency Direction

    Applications
        |
    Agents
        |
    Core Runtime
        |
    Infrastructure
