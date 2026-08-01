# Current Feature

## Working In

Not Started

## Status

Not Started

## Goals

-

## Dependencies

-

## Notes

-

## History

- 2026-08-01 SPEC-0025 Agent Communication Protocol — Completed. Added `services/core/agent_communication.go`: `AgentMessageKind`/`Communicator` filling the "communication between agents and the core runtime" requirements of SPEC-0025, the seventh and final spec of Phase 4 Intelligence's Agent layer - the first real consumer of `packages/shared-types/message.go`'s `Message` envelope (SPEC-0010), whose `MessageTypeAgentCommunication` constant had named this exact traffic since SPEC-0010 but had no builder or dispatcher until now. `AgentMessageKind` is a closed 5-value enum (`request`/`response`/`delegation`/`status_update`/`error_report`) recorded at `Message.Payload["kind"]`, covering exactly SPEC-0025's five listed message categories; `NewAgentRequest`/`NewDelegationMessage`/`NewStatusUpdateMessage`/`NewErrorReportMessage`/`NewAgentResponseMessage` are pure builders producing the envelope for each kind (`Destination` left empty for the two broadcast-style kinds, matching `Message.Destination`'s own SPEC-0010 doc comment). `Communicator` (wraps the SPEC-0020 `AgentRegistry`, and optionally the SPEC-0009 `EventBus` under a new `EventAgentMessage` type) is the routing/orchestration layer: `Request`/`Delegate` share one `dispatch` implementation that builds the outgoing Message, broadcasts a "started" status update, looks up and runs the destination Agent's `Execute` (SPEC-0018), validates the resulting `AgentResponse` via `ValidateAgentResponse` (must name its request/agent and carry exactly one of a successful outcome or a failure `Error`, satisfying "responses are validated"), broadcasts a "completed"/"failed" status update plus an error report on failure, and returns the response `Message` alongside the Agent's own execution error. `Delegate` additionally tags `task.Metadata["delegatedFrom"/"delegatedTo"]` before dispatch (SPEC-0025's own Core Agent -> Developer Agent example) but otherwise reuses `Request`'s exact code path - the destination Agent's `Execute` is the same seam either way. Deliberately stops at the Agent boundary rather than continuing to the "Tool Execution" step in SPEC-0025's own example diagram: the Tools layer (SPEC-0043 Tool Interface onward) is still `Planned`, not implemented, so `Delegate` does not assume a concrete tool-execution backend exists. No `Container` slot was added (same reasoning as SPEC-0021-0024: no pre-reserved placeholder exists for one); `AgentRegistry`, `EventBus`, `Agent`, and `PermissionChecker` (SPEC-0009/0018/0020/0024) were all left completely untouched, this spec is purely additive. `go build ./...`, `go vet ./...`, `go test ./... -v`, `gofmt -l` all clean across all 5 go.work modules (15 new tests: SPEC-0025's three explicit testing criteria - agents can communicate, responses are validated, delegation works correctly - plus registry-lookup failure, Execute failure surfaced alongside a validated failure response, an always-invalid custom validator override, EventBus broadcast of status-update and error-report events, and table-driven `ValidateAgentResponse` coverage for every malformed/well-formed case). `go test -race` unavailable in this environment (no cgo toolchain), same constraint noted since SPEC-0005. Reviewed against `docs/agents/CODE_REVIEW_PROTOCOL.md` (Architecture/Code Quality/Security/Testing): found and fixed one Code Quality gap before approval - `WithCommunicatorValidator(nil)` left `c.validate` nil, causing a nil-pointer panic in `dispatch` on the next `Request`/`Delegate` call, the same bug class SPEC-0023's `ContextBuilder` hit with `WithSizeEstimator(nil)`; fixed with the identical nil-fallback-at-point-of-use pattern, plus a regression test (`TestCommunicator_NilValidatorFallsBackToDefault`). Accepted one known, out-of-scope limitation: delegation is not gated by SPEC-0024's `PermissionChecker`, since that model governs an agent's tool-category access (filesystem/terminal/browser), not agent-to-agent delegation, and SPEC-0025's own Requirements don't ask for it. Approved after fix; re-verified `go build`/`go vet`/`go test ./... -v` clean and `gofmt -l` clean. Built on feature/agent-communication-protocol (off master, post SPEC-0024 merge). Full rationale: see docs/agents/JARVIS_BUILD_TRACKER.md (SPEC-0025 row).

Earlier entries (SPEC-0001 through SPEC-0024): see docs/agents/JARVIS_BUILD_TRACKER.md and `git log` — this file's History section is reset on each feature completion rather than accumulated indefinitely.
