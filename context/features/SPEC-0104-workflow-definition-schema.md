# JARVIS Workflow Definition Schema

## Overview

Define the standard format for describing JARVIS workflows.

## Requirements

Workflow definitions must include:

-   Workflow ID
-   Name
-   Description
-   Triggers
-   Steps
-   Required permissions
-   Variables

Example:

``` yaml
workflow:
  name: daily_briefing
  steps:
    - collect_calendar
    - summarize_email
    - generate_report
```

## Testing

Verify: 1. Workflow definitions validate 2. Invalid schemas fail 3.
Variables resolve correctly
