# Start Action

## Purpose

Begin implementation of the loaded JARVIS feature.

## Requirements

Before starting:

Read:

- context/current-feature.md
- relevant SPEC file
- architecture documents
- ADR decisions

## Steps

1. Verify current-feature.md contains goals.
2. Set status:

```
In Progress
```

3. Create branch:

```
feature/<feature-name>
```

4. Identify affected areas:

- services/
- agents/
- apps/
- packages/

5. Create implementation plan.
6. Implement goals one by one.

## Rules

- Do not modify unrelated modules.
- Follow architecture decisions.
- Do not bypass existing services.
- Add tests for new logic.
