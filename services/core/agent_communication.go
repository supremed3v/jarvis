// agent_communication.go implements SPEC-0025: the Agent Communication
// Protocol - the message contracts and routing agents use to talk to each
// other and to Core Runtime: requests, responses, delegation, status
// updates, and error reporting. It builds on SPEC-0010's Message envelope
// (packages/shared-types/message.go, whose MessageTypeAgentCommunication
// constant already names exactly this traffic) and SPEC-0020's Agent
// Registry (agent_registry.go) for routing a request or delegation to the
// right Agent.
//
// SPEC-0025's own example (Core Agent -> Developer Agent -> Tool
// Execution) continues past the destination Agent to Tool Execution, a
// Tools-layer concern (SPEC-0043 Tool Interface onward) that is still
// Planned, not implemented. Communicator.Delegate therefore stops at the
// Agent boundary - handing a Task to another registered Agent - rather
// than assuming a concrete tool-execution backend exists.
package core

import (
	"context"
	"time"

	"jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
	types "jarvis-pa/packages/shared-types"
)

// AgentMessageKind identifies which SPEC-0025 communication pattern a
// types.Message (Type MessageTypeAgentCommunication) carries, recorded at
// Payload["kind"].
type AgentMessageKind string

const (
	AgentMessageRequest      AgentMessageKind = "request"
	AgentMessageResponse     AgentMessageKind = "response"
	AgentMessageDelegation   AgentMessageKind = "delegation"
	AgentMessageStatusUpdate AgentMessageKind = "status_update"
	AgentMessageErrorReport  AgentMessageKind = "error_report"
)

// EventAgentMessage is the EventType a Communicator publishes on the
// SPEC-0009 EventBus to broadcast a status update or error report Message -
// the two SPEC-0025 kinds with no single addressed recipient (mirrors
// types.Message.Destination's own broadcast-style doc comment).
const EventAgentMessage types.EventType = "AGENT_MESSAGE"

// NewAgentRequest builds the SPEC-0025 request Message requesterID sends to
// responderID asking it to run task.
func NewAgentRequest(requesterID, responderID string, task *types.Task) types.Message {
	return types.Message{
		ID:          task.ID,
		Timestamp:   time.Now().UTC(),
		Source:      requesterID,
		Destination: responderID,
		Type:        types.MessageTypeAgentCommunication,
		Payload: map[string]any{
			"kind": string(AgentMessageRequest),
			"task": task,
		},
	}
}

// NewDelegationMessage builds the SPEC-0025 delegation Message delegatorID
// sends handing task off to delegateID (e.g. Core Agent delegating to
// Developer Agent, SPEC-0025's own example), and records the delegation on
// task.Metadata so the receiving Agent - and anything it logs or returns -
// can see who delegated the work.
func NewDelegationMessage(delegatorID, delegateID string, task *types.Task) types.Message {
	if task.Metadata == nil {
		task.Metadata = map[string]any{}
	}
	task.Metadata["delegatedFrom"] = delegatorID
	task.Metadata["delegatedTo"] = delegateID

	return types.Message{
		ID:          task.ID,
		Timestamp:   time.Now().UTC(),
		Source:      delegatorID,
		Destination: delegateID,
		Type:        types.MessageTypeAgentCommunication,
		Payload: map[string]any{
			"kind": string(AgentMessageDelegation),
			"task": task,
		},
	}
}

// NewStatusUpdateMessage builds the SPEC-0025 status update Message agentID
// broadcasts about requestID's progress (e.g. "started", "completed",
// "failed"). Destination is left empty: a status update has no single
// addressed recipient.
func NewStatusUpdateMessage(agentID, requestID, status string) types.Message {
	return types.Message{
		ID:        requestID,
		Timestamp: time.Now().UTC(),
		Source:    agentID,
		Type:      types.MessageTypeAgentCommunication,
		Payload: map[string]any{
			"kind":      string(AgentMessageStatusUpdate),
			"requestId": requestID,
			"status":    status,
		},
	}
}

// NewErrorReportMessage builds the SPEC-0025 error report Message agentID
// broadcasts about a failure processing requestID. Destination is left
// empty, as with NewStatusUpdateMessage.
func NewErrorReportMessage(agentID, requestID string, cause error) types.Message {
	errText := ""
	if cause != nil {
		errText = cause.Error()
	}
	return types.Message{
		ID:        requestID,
		Timestamp: time.Now().UTC(),
		Source:    agentID,
		Type:      types.MessageTypeAgentCommunication,
		Payload: map[string]any{
			"kind":      string(AgentMessageErrorReport),
			"requestId": requestID,
			"error":     errText,
		},
	}
}

// AgentResponse is the SPEC-0025 payload behind a response Message: the
// outcome of one agent request or delegation.
type AgentResponse struct {
	RequestID string
	AgentID   string
	Success   bool
	Result    map[string]any
	Error     string
}

// NewAgentResponseMessage builds the SPEC-0025 response Message responderID
// sends back to requesterID. It does not itself validate resp - callers
// that need SPEC-0025's "responses are validated" guarantee should call
// ValidateAgentResponse(resp) first; Communicator.Request/Delegate always
// do.
func NewAgentResponseMessage(responderID, requesterID string, resp AgentResponse) types.Message {
	return types.Message{
		ID:          resp.RequestID,
		Timestamp:   time.Now().UTC(),
		Source:      responderID,
		Destination: requesterID,
		Type:        types.MessageTypeAgentCommunication,
		Payload: map[string]any{
			"kind":    string(AgentMessageResponse),
			"success": resp.Success,
			"result":  resp.Result,
			"error":   resp.Error,
		},
	}
}

// ValidateAgentResponse reports whether resp is well-formed (SPEC-0025's
// "responses are validated" testing criterion): it must name the request
// it answers and the agent that produced it, and must carry exactly one of
// a successful outcome or a failure Error, never both and never neither.
// It returns a packages/errors error typed TypeInvalidInput naming the
// first problem found, or nil if resp is valid.
func ValidateAgentResponse(resp AgentResponse) error {
	if resp.RequestID == "" {
		return errors.New(errors.TypeInvalidInput, "AGENT_RESPONSE_MISSING_REQUEST_ID", "core.agentcommunication",
			"agent response is missing a RequestID")
	}
	if resp.AgentID == "" {
		return errors.New(errors.TypeInvalidInput, "AGENT_RESPONSE_MISSING_AGENT_ID", "core.agentcommunication",
			"agent response is missing an AgentID").With("requestId", resp.RequestID)
	}
	if resp.Success && resp.Error != "" {
		return errors.New(errors.TypeInvalidInput, "AGENT_RESPONSE_INVALID_STATE", "core.agentcommunication",
			"agent response cannot report success while also carrying an error").
			With("requestId", resp.RequestID).With("agentId", resp.AgentID)
	}
	if !resp.Success && resp.Error == "" {
		return errors.New(errors.TypeInvalidInput, "AGENT_RESPONSE_INVALID_STATE", "core.agentcommunication",
			"agent response reports failure but carries no Error message").
			With("requestId", resp.RequestID).With("agentId", resp.AgentID)
	}
	return nil
}

// Communicator routes SPEC-0025 agent-to-agent communication: Request and
// Delegate dispatch a Task to a registered Agent (via the SPEC-0020
// AgentRegistry) and validate the resulting response before returning it;
// status updates and error reports are broadcast on the SPEC-0009 EventBus
// if one is configured. Communicator is safe for concurrent use - it holds
// no per-call state beyond its (read-only after construction) registry,
// bus, validator, and logger.
type Communicator struct {
	registry AgentRegistry
	bus      EventBus
	validate func(AgentResponse) error
	log      *logger.Logger
}

// CommunicatorOption configures a Communicator created by NewCommunicator.
type CommunicatorOption func(*Communicator)

// WithCommunicatorEventBus attaches the EventBus status updates and error
// reports are broadcast on. Optional; a Communicator with none configured
// still performs the direct request/response call, without broadcasting.
func WithCommunicatorEventBus(bus EventBus) CommunicatorOption {
	return func(c *Communicator) { c.bus = bus }
}

// WithCommunicatorValidator overrides the function used to validate every
// response before it is returned. Optional; defaults to
// ValidateAgentResponse.
func WithCommunicatorValidator(v func(AgentResponse) error) CommunicatorOption {
	return func(c *Communicator) { c.validate = v }
}

// WithCommunicatorLogger attaches a Logger used to record every
// communication error. Optional; a Communicator with none configured still
// broadcasts error reports (if an EventBus is configured) but does not log
// them.
func WithCommunicatorLogger(log *logger.Logger) CommunicatorOption {
	return func(c *Communicator) { c.log = log }
}

// NewCommunicator creates a ready-to-use Communicator routing requests and
// delegation through registry. It returns a packages/errors error typed
// TypeInvalidInput if registry is nil.
func NewCommunicator(registry AgentRegistry, opts ...CommunicatorOption) (*Communicator, error) {
	if registry == nil {
		return nil, errors.New(errors.TypeInvalidInput, "AGENT_COMMUNICATOR_MISSING_REGISTRY", "core.agentcommunication",
			"cannot create a communicator without an agent registry")
	}

	c := &Communicator{registry: registry, validate: ValidateAgentResponse}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// Request sends task to the agent registered as toAgentID on behalf of
// fromAgentID (SPEC-0025's agent requests/responses): it looks the agent
// up, runs its Execute, validates the resulting AgentResponse, and returns
// the corresponding response Message. Status updates are broadcast before
// and after execution, and an error report is broadcast (and logged) if
// the agent lookup, Execute, or response validation fails.
func (c *Communicator) Request(ctx context.Context, fromAgentID, toAgentID string, task *types.Task) (types.Message, error) {
	return c.dispatch(ctx, fromAgentID, toAgentID, task, false)
}

// Delegate hands task off from fromAgentID to toAgentID (SPEC-0025's
// delegation - e.g. Core Agent delegating to Developer Agent), tagging
// task.Metadata with the delegation chain before dispatch. Delegate
// otherwise behaves exactly like Request: the destination Agent's Execute
// is the same seam a direct request uses.
func (c *Communicator) Delegate(ctx context.Context, fromAgentID, toAgentID string, task *types.Task) (types.Message, error) {
	return c.dispatch(ctx, fromAgentID, toAgentID, task, true)
}

// dispatch is the shared Request/Delegate implementation: build the
// outgoing Message, broadcast a "started" status update, look up and
// execute the destination Agent, validate the resulting response, then
// broadcast a "completed"/"failed" status update and return the response
// Message.
func (c *Communicator) dispatch(ctx context.Context, fromAgentID, toAgentID string, task *types.Task, delegated bool) (types.Message, error) {
	if task == nil {
		return types.Message{}, errors.New(errors.TypeInvalidInput, "AGENT_COMMUNICATOR_NIL_TASK", "core.agentcommunication",
			"cannot dispatch a nil task")
	}

	var reqMsg types.Message
	if delegated {
		reqMsg = NewDelegationMessage(fromAgentID, toAgentID, task)
	} else {
		reqMsg = NewAgentRequest(fromAgentID, toAgentID, task)
	}
	c.publish(NewStatusUpdateMessage(fromAgentID, reqMsg.ID, "started"))

	agent, err := c.registry.Lookup(toAgentID)
	if err != nil {
		c.reportError(toAgentID, reqMsg.ID, err)
		return types.Message{}, err
	}

	result, execErr := agent.Execute(ctx, task)
	resp := AgentResponse{RequestID: reqMsg.ID, AgentID: toAgentID, Result: result, Success: execErr == nil}
	if execErr != nil {
		resp.Error = execErr.Error()
	}

	validate := c.validate
	if validate == nil {
		validate = ValidateAgentResponse
	}
	if err := validate(resp); err != nil {
		c.reportError(toAgentID, reqMsg.ID, err)
		return types.Message{}, err
	}

	status := "completed"
	if execErr != nil {
		status = "failed"
		c.reportError(toAgentID, reqMsg.ID, execErr)
	}
	c.publish(NewStatusUpdateMessage(toAgentID, reqMsg.ID, status))

	return NewAgentResponseMessage(toAgentID, fromAgentID, resp), execErr
}

// reportError logs (if a Logger is configured) and broadcasts (if an
// EventBus is configured) an error report Message for a failure processing
// requestID.
func (c *Communicator) reportError(agentID, requestID string, cause error) {
	if c.log != nil {
		c.log.Error("agent communication error", map[string]any{
			"agentId":   agentID,
			"requestId": requestID,
			"error":     cause.Error(),
		})
	}
	c.publish(NewErrorReportMessage(agentID, requestID, cause))
}

// publish broadcasts msg on the configured EventBus, wrapped in an Event of
// type EventAgentMessage. A no-op if no EventBus is configured.
func (c *Communicator) publish(msg types.Message) {
	if c.bus == nil {
		return
	}
	c.bus.Publish(types.Event{
		ID:        msg.ID,
		Type:      EventAgentMessage,
		Source:    msg.Source,
		Timestamp: msg.Timestamp,
		Payload:   flattenMessagePayload(msg),
	})
}

// flattenMessagePayload merges msg's envelope fields not already carried by
// Event (Type, Destination) into its Payload, so a Communicator's EventBus
// subscribers can inspect a broadcast Message's full shape from
// Event.Payload alone.
func flattenMessagePayload(msg types.Message) map[string]any {
	payload := map[string]any{
		"messageType": string(msg.Type),
		"destination": msg.Destination,
	}
	for k, v := range msg.Payload {
		payload[k] = v
	}
	return payload
}
