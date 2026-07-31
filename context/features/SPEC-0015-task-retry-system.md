# JARVIS Task Retry System

## Overview

Implement retry handling for failed tasks.

## Requirements

Support:

-   Retry count
-   Retry delay
-   Maximum attempts
-   Failure reasons

## Rules

Retries must avoid infinite loops.

## Testing

Verify: 1. Failed tasks retry 2. Retry limits work 3. Final failures are
recorded
