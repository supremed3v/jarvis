# Complete Action

## Purpose

Finalize a completed JARVIS feature.

## Steps

1. Run final review.
2. Run build checks.

Backend:

```
go build ./...
```

Tests:

```
go test ./...
```

3. Ask before committing.
4. Create conventional commit.

Example:

```
feat(runtime): implement service lifecycle manager
```

5. Push branch.
6. Merge after approval.
7. Return to main branch.
8. Reset:

```
context/current-feature.md
```

Set:

- Status: Not Started
- Clear active feature data

9. Append feature summary to history.

## Never

- Auto commit
- Skip tests
- Merge without approval
