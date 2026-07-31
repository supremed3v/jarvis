# JARVIS FEATURE INDEX INTEGRATION SPEC

## Document Type

Claude Feature Workflow Enhancement Specification

## Purpose

Integrate FEATURE_INDEX.md into the JARVIS feature execution workflow.

The Feature Index becomes the navigation layer between feature requests
and detailed specification files.

------------------------------------------------------------------------

# Problem

Currently:

    /feature load SPEC-XXXX
            |
            v
    Directly open SPEC file

This works but does not provide:

-   Dependency awareness
-   Feature discovery
-   Context routing
-   Related specification loading

------------------------------------------------------------------------

# Goal

Modify the feature loading workflow so that agents use:

    FEATURE_INDEX.md
            |
            v
    SPEC file
            |
            v
    Dependencies
            |
            v
    Architecture context
            |
            v
    current-feature.md

------------------------------------------------------------------------

# Files To Modify

Target:

    .claude/skills/feature/actions/load.md

------------------------------------------------------------------------

# New Load Workflow

## Step 1 --- Validate Input

Command:

    /feature load <spec-name>

Example:

    /feature load SPEC-0007-go-runtime-bootstrap

If no argument exists:

Return:

    Error: load requires a specification name.

------------------------------------------------------------------------

## Step 2 --- Read Feature Index

Before loading any specification:

Read:

    context/features/FEATURE_INDEX.md

Purpose:

-   Locate specification
-   Identify category
-   Identify dependencies
-   Identify implementation area
-   Identify related features

------------------------------------------------------------------------

## Step 3 --- Locate Specification

Search FEATURE_INDEX.md for:

    SPEC-XXXX

Extract:

-   Filename
-   Feature name
-   Dependencies
-   Related features
-   Implementation area

------------------------------------------------------------------------

## Step 4 --- Read Specification

Load:

    context/features/{spec-file}.md

Understand:

-   Purpose
-   Requirements
-   Acceptance criteria
-   Constraints

------------------------------------------------------------------------

## Step 5 --- Load Supporting Context

Read relevant:

Architecture:

    docs/architecture/

Execution:

    docs/execution/DEPENDENCY_GRAPH.md

Decisions:

    docs/decisions/

Only load relevant documents.

Avoid loading unrelated context.

------------------------------------------------------------------------

## Step 6 --- Update Current Feature

Update:

    context/current-feature.md

Template:

``` md
# Current Feature: <feature-name>

## Working In

<implementation-area>

## Status

Not Started

## Goals

- Goal 1
- Goal 2

## Dependencies

- SPEC-XXXX

## Notes

Specification:

context/features/SPEC-XXXX.md
```

------------------------------------------------------------------------

# Agent Rules

The agent must:

-   Always check FEATURE_INDEX.md first
-   Never skip dependency discovery
-   Never load all specifications
-   Keep context minimal
-   Respect architecture decisions

------------------------------------------------------------------------

# Expected Result

After implementation:

Running:

    /feature load SPEC-0007-go-runtime-bootstrap

should produce:

    Feature Loaded Successfully

    Specification:
    SPEC-0007

    Dependencies:
    SPEC-0001
    SPEC-0003
    SPEC-0005

    Status:
    Not Started

------------------------------------------------------------------------

# Acceptance Criteria

-   FEATURE_INDEX.md is read before SPEC files
-   Dependencies are identified
-   current-feature.md contains dependency information
-   Agents load only required context
-   Existing feature workflow remains compatible

------------------------------------------------------------------------

# Completion

The Feature Index becomes the routing layer for all JARVIS feature
execution.
