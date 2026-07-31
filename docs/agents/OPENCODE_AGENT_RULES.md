# OpenCode Agent Rules

## Purpose

Define autonomous coding rules for OpenCode agents.

## Execution Model

Agents must:

1.  Read specification
2.  Understand dependencies
3.  Plan implementation
4.  Execute changes
5.  Validate output

## Restrictions

Do not:

-   Rewrite unrelated code
-   Skip tests
-   Change architecture without approval
-   Ignore existing conventions

## Output

Every completed task must provide:

-   Summary
-   Files changed
-   Tests executed
-   Known limitations
