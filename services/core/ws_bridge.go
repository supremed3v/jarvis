// ws_bridge.go implements SPEC-0065: the Core Runtime Communication Bridge -
// the transport that connects the Electron desktop app to the Go runtime.
// A Bridge runs a local WebSocket server (the SPEC-0065 "WebSocket" option;
// the same option the Container.WSBridge slot's doc comment names) that the
// desktop's main process connects to, exposing four surfaces, one per
// SPEC-0065 requirement:
//
//   - Sending tasks to core:  command.submit / command.cancel frames dispatch
//     a Command through a CommandHandler / StreamingCommandHandler seam (or,
//     when an AgentRegistry is wired, the default dispatch-to-agent handler),
//     mirroring the RequestHandler / StreamingRequestHandler seam precedent
//     the Voice Session Manager (SPEC-0060/0061) established for agent
//     communication.
//   - Receiving events:  the Bridge subscribes to the SPEC-0009 EventBus and
//     forwards the runtime's internal Events to every connected client as
//     "event" frames; Broadcast() lets the embedding process push arbitrary
//     additional events the same way.
//   - Streaming responses:  a StreamingCommandHandler's chunks are forwarded
//     as "command.stream" frames before a terminal "command.result" frame,
//     so the desktop can render a response before it is complete.
//   - Runtime status updates:  the Bridge tracks its own lifecycle state and
//     pushes a "status.changed" frame whenever it transitions, and answers
//     "get_status" requests with the current status.
//
// The wire protocol mirrors the SPEC-0064 desktop IPC surface (runtime
// ping/status, command submit/cancel, tool approval, voice/event pushes) so
// the two halves of the bridge describe the same contract - the Go side here
// owns the authoritative message shapes, and apps/desktop/src/shared/runtime.ts
// mirrors them for the Electron client. See ws_bridge_test.go for the
// end-to-end client/server round trips.
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
	types "jarvis-pa/packages/shared-types"
)

// defaultBridgeAddr is where a Bridge listens when the embedding process
// supplies no explicit address. The Electron main process assumes the same
// default (apps/desktop/src/shared/runtime.ts), so a runtime that calls
// Start with an empty address and a desktop app left at its default settings
// find each other without extra configuration.
const defaultBridgeAddr = "127.0.0.1:42321"

// bridgePath is the HTTP path a Bridge serves the WebSocket handshake on.
// Both the Go client (tests) and the Electron client dial ws://host:port + this path.
const bridgePath = "/ws"

// maxFrameBytes caps how large a single client frame may be. Client frames
// are small JSON requests; the cap mainly guards against a misbehaving
// client exhausting memory.
const maxFrameBytes = 4 << 20 // 4 MiB

// writeTimeout bounds each outbound WebSocket write, so a stuck client can
// never block the bridge's broadcast or event-forwarding goroutines forever.
const writeTimeout = 5 * time.Second

// approvalPollInterval is how often the Bridge checks the wired
// ApprovalQueue for newly pending requests to forward to clients. ApprovalQueue
// (SPEC-0048) publishes no events of its own, so polling Pending() is the
// only way to notice a new request without modifying that spec.
const approvalPollInterval = 250 * time.Millisecond

// Bridge state values carried in BridgeStatus.State (mirrors the desktop's
// RuntimeStatus.state union in apps/desktop/src/shared/ipc.ts).
const (
	BridgeStateStarting string = "starting"
	BridgeStateReady    string = "ready"
	BridgeStateStopping string = "stopping"
	BridgeStateStopped  string = "stopped"
	BridgeStateError    string = "error"
)

// Wire frame types, shared by both directions. Client-to-server frames
// (ping, get_status, command.submit, command.cancel, tool.approval_response,
// voice.start, voice.stop, agents.list, agent.start, agent.stop,
// memory.list, memory.search, memory.update, memory.delete) carry a
// client-generated id the server echoes in every frame it sends in reply.
// Server-to-client frames (pong, status, status.changed, event,
// command.stream, command.result, tool.approval_requested, voice.result,
// agents.result, agent.result, memory.result, error) are the transport half
// of the same domains SPEC-0064's renderer IPC channels already define.
const (
	framePing                 = "ping"
	frameGetStatus            = "get_status"
	frameCommandSubmit        = "command.submit"
	frameCommandCancel        = "command.cancel"
	frameToolApprovalResponse = "tool.approval_response"
	frameVoiceStart           = "voice.start"
	frameVoiceStop            = "voice.stop"
	frameAgentsList           = "agents.list"
	frameAgentStart           = "agent.start"
	frameAgentStop            = "agent.stop"
	frameMemoryList           = "memory.list"
	frameMemorySearch         = "memory.search"
	frameMemoryUpdate         = "memory.update"
	frameMemoryDelete         = "memory.delete"

	framePong                  = "pong"
	frameStatus                = "status"
	frameStatusChanged         = "status.changed"
	frameEvent                 = "event"
	frameCommandStream         = "command.stream"
	frameCommandResult         = "command.result"
	frameToolApprovalRequested = "tool.approval_requested"
	frameVoiceResult           = "voice.result"
	frameAgentsResult          = "agents.result"
	frameAgentResult           = "agent.result"
	frameMemoryResult          = "memory.result"
	frameError                 = "error"
)

// bridgeFrame is the JSON envelope every message on the wire uses. Type names
// the frame; ID is the request id a server reply echoes (empty for server
// pushes); Payload holds frame-specific fields.
type bridgeFrame struct {
	Type    string         `json:"type"`
	ID      string         `json:"id,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
}

// bridgeError is the structured error carried by error and command.result
// frames. It mirrors the desktop's IpcError shape (code + message).
type bridgeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Command is one task submitted to core by a desktop client (SPEC-0065's
// "Sending tasks to core"). ID is the client-generated request id; Text is
// the user's command text.
type Command struct {
	ID   string
	Text string
}

// CommandChunk is one incremental piece of a streamed command response,
// delivered to the callback StreamingCommandHandler invokes - the
// streaming-response counterpart of core.StreamChunk, whose Text/Done shape
// it mirrors (same precedent as voice.ResponseChunk).
type CommandChunk struct {
	Text string
	Done bool
}

// AgentView is one registered agent as the SPEC-0070 Agent Management
// Dashboard renders it: the identity fields AgentMetadata exposes, its
// declared capabilities (the tools it may use), its permission-gated tools,
// its declared memory access, and its current lifecycle status. Status is the
// SPEC-0021 AgentStatus value ("registered" when no LifecycleManager is wired,
// so every registered agent is still visible), matching the desktop's
// AgentStatus union in apps/desktop/src/shared/agents.ts.
type AgentView struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Permissions  []string `json:"permissions,omitempty"`
	MemoryAccess []string `json:"memoryAccess,omitempty"`
	Status       string   `json:"status"`
}

// MemoryEntry is one memory record as the SPEC-0071 Memory Viewer renders it:
// the SPEC-0034 MemoryRecord's fields plus the decoded metadata the UI shows.
// MemoryRecord itself has no JSON tags, so this view mirrors its shape for the
// wire (matching the AgentView precedent).
type MemoryEntry struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Content   string         `json:"content"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

// CommandHandler processes a single Command and returns its complete result
// payload. This is the batch seam to the Agent layer - e.g. a closure over
// an AgentRegistry lookup + Execute, or over Communicator.Request - so the
// Bridge stays agnostic of which agent or dispatch mechanism produced the
// response, exactly like SPEC-0060's RequestHandler. CommandHandler must
// respect ctx cancellation.
type CommandHandler func(ctx context.Context, cmd Command) (map[string]any, error)

// StreamingCommandHandler is CommandHandler's streaming counterpart
// (SPEC-0065's "Streaming responses" requirement): instead of returning the
// complete result in one call, it invokes onChunk for each incremental piece
// as it becomes available, so the desktop can render a response before the
// runtime has finished producing all of it - e.g. a closure over
// core.Provider.Stream or core.StreamHandler.Stream. StreamingCommandHandler
// must respect ctx cancellation, must invoke onChunk with a final Done chunk
// once the response is complete, and must return promptly if onChunk returns
// an error.
type StreamingCommandHandler func(ctx context.Context, cmd Command, onChunk func(CommandChunk) error) error

// BridgeStatus is the runtime status the Bridge reports and pushes (SPEC-0065's
// "Runtime status updates"). It mirrors the desktop's RuntimeStatus contract
// (apps/desktop/src/shared/ipc.ts).
type BridgeStatus struct {
	State     string `json:"state"`
	Version   string `json:"version"`
	LastError string `json:"lastError,omitempty"`
}

// defaultForwardedEvents is the closed set of EventBus event types a Bridge
// forwards to connected clients as "event" frames. It covers the task
// lifecycle and agent communication events the desktop most plausibly
// renders; any additional events the embedding process wants pushed can be
// sent via Broadcast. The VOICE_* names are string literals mirroring the
// constants in core/voice (session_manager.go): core cannot import that
// subpackage (it imports core), so the mirror is spelled out here, with the
// desktop main process mapping them onto its voice:event channel.
var defaultForwardedEvents = []types.EventType{
	EventUserMessageReceived, // eventbus.go
	EventAgentStarted,        // eventbus.go
	EventTaskCompleted,       // eventbus.go
	EventTaskStarted,         // task_worker.go
	EventTaskFailed,          // task_worker.go
	EventTaskRetryScheduled,  // task_retry.go
	EventTaskScheduled,       // task_scheduler.go
	EventAgentMessage,        // agent_communication.go

	// Mirrors core/voice/session_manager.go's session lifecycle events.
	"VOICE_SESSION_STARTED",
	"VOICE_SESSION_PROCESSING",
	"VOICE_SESSION_SPEAKING",
	"VOICE_SESSION_COMPLETED",
	"VOICE_SESSION_FAILED",
	"VOICE_SESSION_INTERRUPTED",
}

// inflightCommand tracks a command submitted over one client connection so a
// later command.cancel frame can stop it (and so disconnecting the client
// cancels its commands rather than letting them run on unseen).
type inflightCommand struct {
	cancel   context.CancelFunc
	clientID uint64
}

// wsClient is one connected desktop client. All writes to conn are serialized
// through writeMu: the Bridge's broadcast/event-forwarding goroutines, a
// command's streaming goroutine, and the read loop's own replies can all fire
// concurrently, and coder/websocket forbids concurrent writes to one Conn.
type wsClient struct {
	bridge  *Bridge
	conn    *websocket.Conn
	id      uint64
	writeMu sync.Mutex
}

// Bridge implements SPEC-0065: it serves the WebSocket bridge the desktop app
// connects to, dispatching commands, forwarding events, streaming responses,
// and reporting runtime status. Bridge is safe for concurrent use.
type Bridge struct {
	bus     EventBus
	log     *logger.Logger
	version string
	addr    string

	commandHandler   CommandHandler
	streamingHandler StreamingCommandHandler
	agents           AgentRegistry
	lifecycle        *LifecycleManager
	defaultAgent     string
	approvals        *ApprovalQueue
	voice            VoiceSessionManager
	memory           MemoryViewer

	mu          sync.Mutex
	state       string
	lastError   string
	listener    net.Listener
	server      *http.Server
	clients     map[uint64]*wsClient
	nextClient  uint64
	inflight    map[string]inflightCommand
	unsubs      []func()
	announced   map[string]bool
	approvalCtx context.CancelFunc
	started     bool
}

// BridgeOption configures a Bridge created by NewBridge.
type BridgeOption func(*Bridge)

// WithBridgeEventBus attaches the EventBus whose Events the Bridge forwards
// to connected clients as "event" frames (SPEC-0065's "Receiving events").
// Optional; a Bridge with none configured serves commands and status but
// forwards no events. Broadcast still works either way.
func WithBridgeEventBus(bus EventBus) BridgeOption {
	return func(b *Bridge) { b.bus = bus }
}

// WithBridgeLogger attaches a Logger used to record connections, command
// outcomes, and transport errors. Optional; a Bridge with no logger runs
// silently.
func WithBridgeLogger(log *logger.Logger) BridgeOption {
	return func(b *Bridge) { b.log = log }
}

// WithBridgeVersion sets the version string reported in BridgeStatus and in
// every status frame. Optional; defaults to "0.1.0".
func WithBridgeVersion(version string) BridgeOption {
	return func(b *Bridge) { b.version = version }
}

// WithBridgeListenAddr sets the address Start uses when it is called with an
// empty address, and the address Runtime dependency Init binds. Optional;
// defaults to defaultBridgeAddr.
func WithBridgeListenAddr(addr string) BridgeOption {
	return func(b *Bridge) { b.addr = addr }
}

// WithCommandHandler sets the batch CommandHandler used for command.submit
// frames. A Bridge without a command handler and without an AgentRegistry
// rejects command submissions with a descriptive error. Optional.
func WithCommandHandler(h CommandHandler) BridgeOption {
	return func(b *Bridge) { b.commandHandler = h }
}

// WithStreamingCommandHandler sets the StreamingCommandHandler used for
// command.submit frames (SPEC-0065's "Streaming responses"). When set, it
// takes precedence over WithCommandHandler and the default agent dispatch.
// Optional.
func WithStreamingCommandHandler(h StreamingCommandHandler) BridgeOption {
	return func(b *Bridge) { b.streamingHandler = h }
}

// WithBridgeAgentRegistry enables the Bridge's default command dispatch:
// when no explicit command handler is set, command.submit looks up
// defaultAgentID in registry and hands it a desktop-sourced Task, publishing
// task lifecycle events on the configured EventBus around the Execute call.
// Optional.
func WithBridgeAgentRegistry(registry AgentRegistry, defaultAgentID string) BridgeOption {
	return func(b *Bridge) {
		b.agents = registry
		b.defaultAgent = defaultAgentID
	}
}

// WithApprovalQueue wires the SPEC-0048 ApprovalQueue so the Bridge forwards
// newly pending approval requests to clients as "tool.approval_requested"
// frames and resolves them from "tool.approval_response" frames. Optional.
func WithApprovalQueue(q *ApprovalQueue) BridgeOption {
	return func(b *Bridge) { b.approvals = q }
}

// WithBridgeVoiceSessionManager wires the SPEC-0060 SessionManager so
// "voice.start" and "voice.stop" frames (SPEC-0068's tray "Start voice mode"
// control) start and stop the voice session lifecycle. Optional; a Bridge
// without one answers those frames with a VOICE_DISABLED error.
func WithBridgeVoiceSessionManager(m VoiceSessionManager) BridgeOption {
	return func(b *Bridge) { b.voice = m }
}

// WithBridgeLifecycleManager wires the SPEC-0021 LifecycleManager so
// "agents.list" reports each registered agent's real lifecycle status and
// "agent.start" / "agent.stop" frames (SPEC-0070's enable/disable controls)
// drive actual lifecycle transitions rather than failing. Optional; a Bridge
// without one reports every registered agent as "registered" and answers
// agent.start / agent.stop with an AGENT_LIFECYCLE_DISABLED error.
func WithBridgeLifecycleManager(m *LifecycleManager) BridgeOption {
	return func(b *Bridge) { b.lifecycle = m }
}

// WithBridgeMemory wires the SPEC-0071 MemoryViewer so "memory.list" /
// "memory.search" / "memory.update" / "memory.delete" frames (the Memory
// Viewer UI's data source) read and mutate the runtime's memory. Optional; a
// Bridge without one answers memory.list with an empty list (a runtime with
// no memory viewer is a valid state, mirroring agents.list) and the other
// memory frames with a MEMORY_DISABLED error.
func WithBridgeMemory(m MemoryViewer) BridgeOption {
	return func(b *Bridge) { b.memory = m }
}

// NewBridge creates a ready-to-use Bridge. All slots are optional and set
// via options; a Bridge with no handlers still serves ping, get_status, and
// tool approval frames.
func NewBridge(opts ...BridgeOption) *Bridge {
	b := &Bridge{
		version:   "0.1.0",
		addr:      defaultBridgeAddr,
		state:     BridgeStateStopped,
		clients:   make(map[uint64]*wsClient),
		inflight:  make(map[string]inflightCommand),
		announced: make(map[string]bool),
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Name identifies the Bridge as a Runtime Dependency (see runtime.go), so it
// can be registered via WithDependencies for ordered startup/shutdown.
func (b *Bridge) Name() string { return "core.wsbridge" }

// Init satisfies the Runtime Dependency interface: it starts the Bridge on
// b.addr (the WithBridgeListenAddr value, or defaultBridgeAddr).
func (b *Bridge) Init(ctx context.Context) error { return b.Start(b.addr) }

// Close satisfies the Runtime Dependency interface: it stops the Bridge and
// releases every connection.
func (b *Bridge) Close(ctx context.Context) error { return b.Stop() }

// State reports the Bridge's current lifecycle state.
func (b *Bridge) State() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// Status returns a snapshot of the Bridge's current runtime status.
func (b *Bridge) Status() BridgeStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	return BridgeStatus{State: b.state, Version: b.version, LastError: b.lastError}
}

// Addr reports the address the Bridge is currently listening on. It is
// intended for callers that started the Bridge on an ephemeral port
// ("127.0.0.1:0") and need to discover the real address; Addr returns "" if
// the Bridge is not running.
func (b *Bridge) Addr() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.listener == nil {
		return ""
	}
	return b.listener.Addr().String()
}

// Start brings the Bridge up on addr (an empty addr uses the WithBridgeListenAddr
// value, or defaultBridgeAddr): it opens the listener, serves the WebSocket
// handshake on /ws, subscribes to the configured EventBus, and starts the
// approval-polling goroutine. It returns a packages/errors error typed
// TypeInternal if the Bridge is already running, or TypeUnavailable if the
// address is already in use. Calling Start twice is an error.
func (b *Bridge) Start(addr string) error {
	if addr == "" {
		addr = b.addr
	}

	b.mu.Lock()
	if b.started {
		b.mu.Unlock()
		return errors.New(errors.TypeInternal, "BRIDGE_ALREADY_STARTED", "core.wsbridge",
			"bridge is already running")
	}
	b.state = BridgeStateStarting
	b.mu.Unlock()

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		b.fail("listen failed: " + err.Error())
		return errors.Wrap(err, errors.TypeUnavailable, "BRIDGE_LISTEN_FAILED", "core.wsbridge",
			fmt.Sprintf("cannot listen on %s", addr))
	}

	mux := http.NewServeMux()
	mux.HandleFunc(bridgePath, b.accept)
	srv := &http.Server{Handler: mux}

	b.mu.Lock()
	b.listener = ln
	b.server = srv
	b.started = true
	b.mu.Unlock()

	go func() {
		if err := srv.Serve(ln); err != nil && !isServerClosed(err) && b.State() != BridgeStateStopped {
			b.logf("bridge server stopped unexpectedly", map[string]any{"error": err.Error()})
			b.fail("serve failed: " + err.Error())
		}
	}()

	if b.bus != nil {
		b.mu.Lock()
		b.unsubs = nil
		b.mu.Unlock()
		for _, eventType := range defaultForwardedEvents {
			unsub := b.bus.Subscribe(eventType, b.forwardEvent)
			b.mu.Lock()
			b.unsubs = append(b.unsubs, unsub)
			b.mu.Unlock()
		}
	}

	if b.approvals != nil {
		ctx, cancel := context.WithCancel(context.Background())
		b.mu.Lock()
		b.approvalCtx = cancel
		b.mu.Unlock()
		go b.pollApprovals(ctx)
	}

	b.mu.Lock()
	b.state = BridgeStateReady
	b.mu.Unlock()

	b.logf("bridge started", map[string]any{"addr": ln.Addr().String(), "version": b.version})
	b.pushStatusChanged()
	return nil
}

// Stop tears the Bridge down: it cancels every in-flight command, unsubscribes
// from the EventBus, stops the approval poller, closes every client
// connection, and closes the listener. Calling Stop before Start, or twice,
// is a no-op.
func (b *Bridge) Stop() error {
	b.mu.Lock()
	if !b.started {
		b.mu.Unlock()
		return nil
	}
	b.started = false
	b.state = BridgeStateStopping
	server := b.server
	approvalCancel := b.approvalCtx
	clients := make([]*wsClient, 0, len(b.clients))
	for _, c := range b.clients {
		clients = append(clients, c)
	}
	unsubs := append([]func(){}, b.unsubs...)
	inflight := make([]context.CancelFunc, 0, len(b.inflight))
	for _, ic := range b.inflight {
		inflight = append(inflight, ic.cancel)
	}
	b.mu.Unlock()

	for _, unsub := range unsubs {
		unsub()
	}
	if approvalCancel != nil {
		approvalCancel()
	}
	for _, cancel := range inflight {
		cancel()
	}
	for _, c := range clients {
		c.conn.Close(websocket.StatusGoingAway, "bridge stopping")
		c.conn.CloseNow()
	}

	var closeErr error
	if server != nil {
		if err := server.Close(); err != nil {
			closeErr = errors.Wrap(err, errors.TypeInternal, "BRIDGE_CLOSE_FAILED", "core.wsbridge",
				"bridge http server failed to close cleanly")
		}
	}

	b.mu.Lock()
	b.state = BridgeStateStopped
	b.clients = make(map[uint64]*wsClient)
	b.inflight = make(map[string]inflightCommand)
	b.announced = make(map[string]bool)
	b.unsubs = nil
	b.approvalCtx = nil
	b.listener = nil
	b.server = nil
	b.mu.Unlock()

	b.logf("bridge stopped", nil)
	return closeErr
}

// Broadcast pushes an arbitrary event to every connected client as an "event"
// frame (SPEC-0065's "Receiving events"), letting the embedding process
// publish Events the Bridge's own EventBus subscription does not cover.
// Broadcast never fails because no client is connected; it returns an error
// only if the frame cannot be marshaled.
func (b *Bridge) Broadcast(event string, payload any) error {
	if b.log != nil {
		b.log.Debug("bridge broadcast", map[string]any{"event": event})
	}
	return b.pushEvent(bridgeFrame{
		Type:    frameEvent,
		Payload: eventToPayload(event, time.Now().UTC(), "", payload),
	})
}

// accept upgrades an HTTP request to a WebSocket connection and starts the
// client's read loop.
func (b *Bridge) accept(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		b.logf("bridge rejected websocket upgrade", map[string]any{"error": err.Error()})
		return
	}

	b.mu.Lock()
	if !b.started {
		b.mu.Unlock()
		conn.Close(websocket.StatusGoingAway, "bridge is stopping")
		conn.CloseNow()
		return
	}
	b.nextClient++
	client := &wsClient{bridge: b, conn: conn, id: b.nextClient}
	b.clients[client.id] = client
	b.mu.Unlock()

	b.logf("bridge client connected", map[string]any{"clientId": client.id})
	status, _ := b.statusPayload()
	client.send(bridgeFrame{Type: frameStatusChanged, Payload: status})
	go client.readLoop()
}

// removeClient drops a client whose read loop has ended and cancels any
// commands it submitted, since there is no one left to deliver their output
// to.
func (b *Bridge) removeClient(c *wsClient) {
	b.mu.Lock()
	delete(b.clients, c.id)
	var cancels []context.CancelFunc
	for id, ic := range b.inflight {
		if ic.clientID == c.id {
			cancels = append(cancels, ic.cancel)
			delete(b.inflight, id)
		}
	}
	b.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
	b.logf("bridge client disconnected", map[string]any{"clientId": c.id})
}

// readLoop is a client's sole reader goroutine: it decodes inbound frames and
// dispatches them, replying with error frames to malformed or unknown input.
// It exits when the connection closes (client disconnect or Stop), at which
// point removeClient cleans the client up.
func (c *wsClient) readLoop() {
	defer c.bridge.removeClient(c)

	c.conn.SetReadLimit(maxFrameBytes)
	for {
		_, data, err := c.conn.Read(context.Background())
		if err != nil {
			return
		}

		var frame bridgeFrame
		if err := json.Unmarshal(data, &frame); err != nil {
			c.send(bridgeFrame{Type: frameError, Payload: map[string]any{
				"error": map[string]any{"code": "INVALID_FRAME", "message": "frame is not valid JSON"},
			}})
			continue
		}
		c.bridge.handleFrame(c, frame)
	}
}

// handleFrame routes one client frame to its handler.
func (b *Bridge) handleFrame(c *wsClient, frame bridgeFrame) {
	switch frame.Type {
	case framePing:
		c.send(bridgeFrame{Type: framePong, ID: frame.ID, Payload: map[string]any{"pong": true}})
	case frameGetStatus:
		status, _ := b.statusPayload()
		c.send(bridgeFrame{Type: frameStatus, ID: frame.ID, Payload: status})
	case frameCommandSubmit:
		b.handleCommandSubmit(c, frame)
	case frameCommandCancel:
		b.handleCommandCancel(c, frame)
	case frameToolApprovalResponse:
		b.handleToolApprovalResponse(c, frame)
	case frameVoiceStart:
		b.handleVoiceControl(c, frame, true)
	case frameVoiceStop:
		b.handleVoiceControl(c, frame, false)
	case frameAgentsList:
		b.handleAgentsList(c, frame)
	case frameAgentStart:
		b.handleAgentControl(c, frame, true)
	case frameAgentStop:
		b.handleAgentControl(c, frame, false)
	case frameMemoryList:
		b.handleMemoryList(c, frame)
	case frameMemorySearch:
		b.handleMemorySearch(c, frame)
	case frameMemoryUpdate:
		b.handleMemoryUpdate(c, frame)
	case frameMemoryDelete:
		b.handleMemoryDelete(c, frame)
	default:
		c.send(bridgeFrame{Type: frameError, ID: frame.ID, Payload: map[string]any{
			"error": map[string]any{"code": "UNKNOWN_FRAME_TYPE", "message": "unknown frame type: " + frame.Type},
		}})
	}
}

// handleCommandSubmit processes a command.submit frame: it validates the
// payload, picks the command handler (streaming handler, then batch handler,
// then default agent dispatch), and runs it on its own goroutine so the read
// loop stays free to receive a later command.cancel for it.
func (b *Bridge) handleCommandSubmit(c *wsClient, frame bridgeFrame) {
	text, _ := frame.Payload["text"].(string)
	if strings.TrimSpace(text) == "" {
		c.send(bridgeFrame{Type: frameError, ID: frame.ID, Payload: map[string]any{
			"error": map[string]any{"code": "INVALID_COMMAND", "message": "command text must be a non-empty string"},
		}})
		return
	}

	cmd := Command{ID: frame.ID, Text: text}
	ctx, cancel := context.WithCancel(context.Background())

	b.mu.Lock()
	b.inflight[cmd.ID] = inflightCommand{cancel: cancel, clientID: c.id}
	b.mu.Unlock()

	go b.runCommand(c, cmd, ctx, cancel)
}

// runCommand executes one command and delivers its terminal command.result
// frame, streaming command.stream frames first when a StreamingCommandHandler
// is configured. The single exit path guarantees every command - success,
// failure, or cancellation - produces exactly one command.result.
func (b *Bridge) runCommand(c *wsClient, cmd Command, ctx context.Context, cancel context.CancelFunc) {
	defer func() {
		cancel()
		b.mu.Lock()
		delete(b.inflight, cmd.ID)
		b.mu.Unlock()
	}()

	if b.streamingHandler != nil {
		b.runStreamingCommand(c, cmd, ctx)
		return
	}

	result, err := b.executeCommand(ctx, cmd)
	b.finishCommand(c, cmd.ID, result, err, ctx)
}

// runStreamingCommand drives a StreamingCommandHandler, forwarding each chunk
// as a command.stream frame and accumulating the partial text for the final
// command.result frame.
func (b *Bridge) runStreamingCommand(c *wsClient, cmd Command, ctx context.Context) {
	var partial strings.Builder
	var handlerErr error
	handlerErr = b.streamingHandler(ctx, cmd, func(chunk CommandChunk) error {
		partial.WriteString(chunk.Text)
		return c.send(bridgeFrame{Type: frameCommandStream, ID: cmd.ID, Payload: map[string]any{
			"id":      cmd.ID,
			"text":    chunk.Text,
			"partial": partial.String(),
			"done":    chunk.Done,
		}})
	})

	result := map[string]any{"text": partial.String()}
	if handlerErr != nil && ctx.Err() == nil {
		// The handler failed for a real reason, not cancellation.
		c.send(bridgeFrame{Type: frameCommandResult, ID: cmd.ID, Payload: map[string]any{
			"id": cmd.ID, "ok": false,
			"error": bridgeError{Code: "COMMAND_FAILED", Message: handlerErr.Error()},
		}})
		return
	}
	if ctx.Err() != nil {
		c.send(bridgeFrame{Type: frameCommandResult, ID: cmd.ID, Payload: map[string]any{
			"id": cmd.ID, "ok": false, "cancelled": true,
		}})
		return
	}
	c.send(bridgeFrame{Type: frameCommandResult, ID: cmd.ID, Payload: map[string]any{
		"id": cmd.ID, "ok": true, "result": result,
	}})
}

// executeCommand picks the command execution path: the batch CommandHandler
// if configured, otherwise the default dispatch-to-agent handler.
func (b *Bridge) executeCommand(ctx context.Context, cmd Command) (map[string]any, error) {
	if b.commandHandler != nil {
		return b.commandHandler(ctx, cmd)
	}
	return b.dispatchToAgent(ctx, cmd)
}

// dispatchToAgent is the Bridge's default command dispatch (SPEC-0065's
// "Sending tasks to core" with no custom handler): it builds a desktop-sourced
// Task from the Command, publishes its lifecycle events on the configured
// EventBus, hands it to the default agent, and returns the agent's result
// payload. It returns a packages/errors error typed TypeInternal if no agent
// registry is wired, or TypeNotFound if the default agent is not registered.
func (b *Bridge) dispatchToAgent(ctx context.Context, cmd Command) (map[string]any, error) {
	if b.agents == nil {
		return nil, errors.New(errors.TypeInternal, "BRIDGE_NO_COMMAND_HANDLER", "core.wsbridge",
			"no command handler or agent registry is configured on the bridge")
	}
	if b.defaultAgent == "" {
		return nil, errors.New(errors.TypeInternal, "BRIDGE_NO_DEFAULT_AGENT", "core.wsbridge",
			"no default agent is configured on the bridge")
	}

	task := &types.Task{
		ID:        cmd.ID,
		Title:     cmd.Text,
		Source:    types.TaskSourceDesktop,
		Type:      "agent",
		Status:    types.TaskStatusCreated,
		Input:     map[string]any{"text": cmd.Text},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	b.publish(EventUserMessageReceived, map[string]any{"taskId": task.ID, "text": cmd.Text})
	b.publish(EventTaskStarted, map[string]any{"taskId": task.ID, "agentId": b.defaultAgent})

	agent, err := b.agents.Lookup(b.defaultAgent)
	if err != nil {
		b.publish(EventTaskFailed, map[string]any{"taskId": task.ID, "agentId": b.defaultAgent, "error": err.Error()})
		return nil, err
	}
	result, err := agent.Execute(ctx, task)
	if err != nil {
		b.publish(EventTaskFailed, map[string]any{"taskId": task.ID, "agentId": b.defaultAgent, "error": err.Error()})
		return nil, err
	}
	b.publish(EventTaskCompleted, map[string]any{"taskId": task.ID, "agentId": b.defaultAgent, "result": result})
	return result, nil
}

// finishCommand sends the single terminal command.result frame for a batch
// command: ok + result on success, a cancelled frame when ctx was cancelled,
// or an error frame carrying the failure's code and message.
func (b *Bridge) finishCommand(c *wsClient, id string, result map[string]any, err error, ctx context.Context) {
	if err != nil {
		if ctx.Err() != nil {
			c.send(bridgeFrame{Type: frameCommandResult, ID: id, Payload: map[string]any{
				"id": id, "ok": false, "cancelled": true,
			}})
			return
		}
		fe := bridgeError{Code: "COMMAND_FAILED", Message: err.Error()}
		if e, ok := err.(*errors.Error); ok {
			fe.Code = e.Code
			fe.Message = e.Message
		}
		c.send(bridgeFrame{Type: frameCommandResult, ID: id, Payload: map[string]any{
			"id": id, "ok": false, "error": fe,
		}})
		return
	}
	c.send(bridgeFrame{Type: frameCommandResult, ID: id, Payload: map[string]any{
		"id": id, "ok": true, "result": result,
	}})
}

// handleCommandCancel processes a command.cancel frame: it cancels the
// in-flight command with the matching id (its runCommand goroutine then
// delivers a cancelled command.result) and replies with an error frame if no
// such command is running.
func (b *Bridge) handleCommandCancel(c *wsClient, frame bridgeFrame) {
	id, _ := frame.Payload["id"].(string)
	if id == "" {
		c.send(bridgeFrame{Type: frameError, ID: frame.ID, Payload: map[string]any{
			"error": map[string]any{"code": "INVALID_COMMAND_ID", "message": "cancel payload requires a non-empty id"},
		}})
		return
	}

	b.mu.Lock()
	ic, ok := b.inflight[id]
	b.mu.Unlock()

	if !ok {
		c.send(bridgeFrame{Type: frameError, ID: frame.ID, Payload: map[string]any{
			"error": map[string]any{"code": "COMMAND_NOT_FOUND", "message": "no in-flight command with id " + id},
		}})
		return
	}

	ic.cancel()
	b.logf("bridge command cancelled", map[string]any{"commandId": id})
}

// handleToolApprovalResponse processes a tool.approval_response frame by
// resolving the wired ApprovalQueue request, and replies with an error frame
// if no ApprovalQueue is configured or no pending request matches.
func (b *Bridge) handleToolApprovalResponse(c *wsClient, frame bridgeFrame) {
	if b.approvals == nil {
		c.send(bridgeFrame{Type: frameError, ID: frame.ID, Payload: map[string]any{
			"error": map[string]any{"code": "APPROVAL_DISABLED", "message": "no approval queue is configured on the bridge"},
		}})
		return
	}

	id, _ := frame.Payload["id"].(string)
	approved, _ := frame.Payload["approved"].(bool)
	if id == "" {
		c.send(bridgeFrame{Type: frameError, ID: frame.ID, Payload: map[string]any{
			"error": map[string]any{"code": "INVALID_APPROVAL_ID", "message": "approval response requires a non-empty id"},
		}})
		return
	}

	if err := b.approvals.Resolve(id, approved); err != nil {
		fe := bridgeError{Code: "APPROVAL_NOT_FOUND", Message: err.Error()}
		if e, ok := err.(*errors.Error); ok {
			fe.Code = e.Code
			fe.Message = e.Message
		}
		c.send(bridgeFrame{Type: frameError, ID: frame.ID, Payload: map[string]any{"error": fe}})
		return
	}
	b.logf("bridge resolved tool approval", map[string]any{"approvalId": id, "approved": approved})
}

// handleVoiceControl processes a voice.start / voice.stop frame by starting
// or stopping the wired VoiceSessionManager (SPEC-0060), enabling SPEC-0068's
// tray "Start voice mode" control over the same bridge the desktop uses for
// everything else. It always replies with a synchronous "voice.result" frame
// echoing the request id, so the desktop knows whether the transition took
// effect; without a wired session manager it replies ok:false with a
// VOICE_DISABLED error.
func (b *Bridge) handleVoiceControl(c *wsClient, frame bridgeFrame, start bool) {
	if b.voice == nil {
		c.send(bridgeFrame{Type: frameVoiceResult, ID: frame.ID, Payload: map[string]any{
			"ok": false,
			"error": map[string]any{
				"code":    "VOICE_DISABLED",
				"message": "no voice session manager is configured on the bridge",
			},
		}})
		return
	}

	var err error
	if start {
		err = b.voice.Start()
	} else {
		err = b.voice.Stop()
	}
	if err != nil {
		fe := bridgeError{Code: "VOICE_CONTROL_FAILED", Message: err.Error()}
		if e, ok := err.(*errors.Error); ok {
			fe.Code = e.Code
			fe.Message = e.Message
		}
		c.send(bridgeFrame{Type: frameVoiceResult, ID: frame.ID, Payload: map[string]any{
			"ok": false, "error": fe,
		}})
		return
	}
	action := "stop"
	if start {
		action = "start"
	}
	b.logf("bridge voice control", map[string]any{"action": action})
	c.send(bridgeFrame{Type: frameVoiceResult, ID: frame.ID, Payload: map[string]any{"ok": true}})
}

// handleAgentsList processes an agents.list frame by replying with an
// agents.result frame carrying the AgentView of every registered agent
// (SPEC-0070's "available agents / status / capabilities / permissions"
// display data). A Bridge with no AgentRegistry wired replies with an empty
// list - a runtime with no agents is a valid state, not an error.
func (b *Bridge) handleAgentsList(c *wsClient, frame bridgeFrame) {
	c.send(bridgeFrame{Type: frameAgentsResult, ID: frame.ID, Payload: map[string]any{
		"agents": b.agentViews(),
	}})
}

// agentViews builds the AgentView for every registered agent, in the
// registry's deterministic (ID-sorted) order. Status comes from the wired
// SPEC-0021 LifecycleManager when one is present, and defaults to "registered"
// otherwise - an agent registered directly on the registry (not through the
// lifecycle manager) is visible with the state Register itself records.
func (b *Bridge) agentViews() []AgentView {
	if b.agents == nil {
		return []AgentView{}
	}

	registered := b.agents.List()
	views := make([]AgentView, 0, len(registered))
	for _, agent := range registered {
		m := agent.Metadata()
		view := AgentView{
			ID:           m.ID,
			Name:         m.Name,
			Description:  m.Description,
			Capabilities: m.Tools,
			Permissions:  m.Permissions,
			MemoryAccess: m.MemoryAccess,
			Status:       string(types.AgentStatusRegistered),
		}
		if b.lifecycle != nil {
			if state, err := b.lifecycle.State(m.ID); err == nil {
				view.Status = string(state)
			}
		}
		views = append(views, view)
	}
	return views
}

// handleAgentControl processes an agent.start / agent.stop frame (SPEC-0070's
// enable/disable controls): start enables the agent by driving the wired
// SPEC-0021 LifecycleManager to a usable state, stop disables it by driving
// it to STOPPED. It always replies with a synchronous "agent.result" frame
// echoing the request id, so the desktop knows whether the transition took
// effect; without a wired lifecycle manager it replies ok:false with an
// AGENT_LIFECYCLE_DISABLED error.
func (b *Bridge) handleAgentControl(c *wsClient, frame bridgeFrame, start bool) {
	if b.lifecycle == nil {
		c.send(bridgeFrame{Type: frameAgentResult, ID: frame.ID, Payload: map[string]any{
			"ok": false,
			"error": map[string]any{
				"code":    "AGENT_LIFECYCLE_DISABLED",
				"message": "no agent lifecycle manager is configured on the bridge",
			},
		}})
		return
	}

	id, _ := frame.Payload["id"].(string)
	if id == "" {
		c.send(bridgeFrame{Type: frameAgentResult, ID: frame.ID, Payload: map[string]any{
			"ok": false,
			"error": map[string]any{
				"code":    "INVALID_AGENT_ID",
				"message": "agent control requires a non-empty agent id",
			},
		}})
		return
	}

	var err error
	if start {
		err = b.enableAgent(id)
	} else {
		err = b.disableAgent(id)
	}
	if err != nil {
		fe := bridgeError{Code: "AGENT_CONTROL_FAILED", Message: err.Error()}
		if e, ok := err.(*errors.Error); ok {
			fe.Code = e.Code
			fe.Message = e.Message
		}
		c.send(bridgeFrame{Type: frameAgentResult, ID: frame.ID, Payload: map[string]any{
			"id": id, "ok": false, "error": fe,
		}})
		return
	}
	action := "stop"
	if start {
		action = "start"
	}
	b.logf("bridge agent control", map[string]any{"agentId": id, "action": action})
	c.send(bridgeFrame{Type: frameAgentResult, ID: frame.ID, Payload: map[string]any{
		"id": id, "ok": true,
	}})
}

// enableAgent transitions id to a usable state via the wired LifecycleManager:
// REGISTERED agents are initialized (-> READY) and READY agents are started
// (-> RUNNING). RUNNING is already enabled (idempotent no-op); every other
// state (INITIALIZING in progress, STOPPED/FAILED terminal) is rejected with
// the lifecycle's own transition validation error.
func (b *Bridge) enableAgent(id string) error {
	state, err := b.lifecycle.State(id)
	if err != nil {
		return err
	}
	switch state {
	case types.AgentStatusRegistered:
		return b.lifecycle.Initialize(context.Background(), id)
	case types.AgentStatusReady:
		return b.lifecycle.Start(id)
	case types.AgentStatusRunning:
		return nil
	default:
		return errors.New(errors.TypeInvalidInput, "AGENT_LIFECYCLE_INVALID_TRANSITION", "core.wsbridge",
			fmt.Sprintf("agent %q cannot be enabled from state %q", id, state)).With("agentId", id).With("state", string(state))
	}
}

// disableAgent transitions id to STOPPED via the wired LifecycleManager:
// READY and RUNNING agents are stopped (their Cleanup hook runs first, per
// SPEC-0021). REGISTERED/STOPPED/FAILED agents are already not operational,
// so disable is an idempotent no-op; INITIALIZING/STOPPING in-progress states
// fall through to the lifecycle's own transition validation error.
func (b *Bridge) disableAgent(id string) error {
	state, err := b.lifecycle.State(id)
	if err != nil {
		return err
	}
	switch state {
	case types.AgentStatusReady, types.AgentStatusRunning:
		return b.lifecycle.Stop(context.Background(), id)
	default:
		return nil
	}
}

// handleMemoryList processes a memory.list frame by replying with a
// memory.result frame carrying the MemoryEntry of every record the wired
// MemoryViewer lists (SPEC-0071's "memories load correctly" data source),
// optionally scoped to one MemoryType via the "type" payload field. A Bridge
// with no MemoryViewer wired replies with an empty list - a runtime with no
// memory viewer is a valid state, not an error.
func (b *Bridge) handleMemoryList(c *wsClient, frame bridgeFrame) {
	typ, ok := memoryTypeField(frame.Payload)
	if !ok {
		b.sendMemoryInvalidType(c, frame)
		return
	}
	entries, err := b.listMemories(typ)
	if err != nil {
		b.sendMemoryError(c, frame, "MEMORY_LIST_FAILED", err)
		return
	}
	c.send(bridgeFrame{Type: frameMemoryResult, ID: frame.ID, Payload: map[string]any{
		"memories": entries,
	}})
}

// listMemories asks the wired MemoryViewer for every record of type typ (all
// types when typ is empty), returning an empty list when no viewer is wired.
func (b *Bridge) listMemories(typ MemoryType) ([]MemoryEntry, error) {
	if b.memory == nil {
		return []MemoryEntry{}, nil
	}
	records, err := b.memory.List(context.Background(), typ)
	if err != nil {
		return nil, err
	}
	return memoryEntries(records), nil
}

// handleMemorySearch processes a memory.search frame: it validates the query
// payload, asks the wired MemoryViewer to search, and replies with a
// memory.result frame carrying the matching MemoryEntries. Without a wired
// viewer it replies ok:false with a MEMORY_DISABLED error.
func (b *Bridge) handleMemorySearch(c *wsClient, frame bridgeFrame) {
	if b.memory == nil {
		b.sendMemoryDisabled(c, frame)
		return
	}
	query, _ := frame.Payload["query"].(string)
	if strings.TrimSpace(query) == "" {
		c.send(bridgeFrame{Type: frameMemoryResult, ID: frame.ID, Payload: map[string]any{
			"ok": false,
			"error": map[string]any{
				"code":    "INVALID_MEMORY_QUERY",
				"message": "memory search requires a non-empty query",
			},
		}})
		return
	}
	q := MemoryQuery{Query: strings.TrimSpace(query)}
	if typ, ok := memoryTypeField(frame.Payload); ok {
		if typ != "" {
			q.Type = typ
		}
	} else {
		c.send(bridgeFrame{Type: frameMemoryResult, ID: frame.ID, Payload: map[string]any{
			"ok": false,
			"error": map[string]any{
				"code":    "INVALID_MEMORY_TYPE",
				"message": "memory search requires a known memory type when one is given",
			},
		}})
		return
	}
	if limit, ok := intField(frame.Payload, "limit"); ok && limit > 0 {
		q.Limit = limit
	}
	records, err := b.memory.Search(context.Background(), q)
	if err != nil {
		b.sendMemoryError(c, frame, "MEMORY_SEARCH_FAILED", err)
		return
	}
	c.send(bridgeFrame{Type: frameMemoryResult, ID: frame.ID, Payload: map[string]any{
		"memories": memoryEntries(records),
	}})
}

// handleMemoryUpdate processes a memory.update frame (SPEC-0071's "editing
// where allowed"): it validates the id/content payload, asks the wired
// MemoryViewer to replace the record's content, and replies with a
// synchronous memory.result ack. Without a wired viewer it replies ok:false
// with a MEMORY_DISABLED error.
func (b *Bridge) handleMemoryUpdate(c *wsClient, frame bridgeFrame) {
	if b.memory == nil {
		b.sendMemoryDisabled(c, frame)
		return
	}
	id, _ := frame.Payload["id"].(string)
	if id == "" {
		c.send(bridgeFrame{Type: frameMemoryResult, ID: frame.ID, Payload: map[string]any{
			"ok": false,
			"error": map[string]any{
				"code":    "INVALID_MEMORY_ID",
				"message": "memory update requires a non-empty id",
			},
		}})
		return
	}
	content, _ := frame.Payload["content"].(string)
	if strings.TrimSpace(content) == "" {
		c.send(bridgeFrame{Type: frameMemoryResult, ID: frame.ID, Payload: map[string]any{
			"ok": false,
			"error": map[string]any{
				"code":    "INVALID_MEMORY_CONTENT",
				"message": "memory update requires non-empty content",
			},
		}})
		return
	}
	if err := b.memory.Update(context.Background(), MemoryRecord{ID: id, Content: content}); err != nil {
		b.sendMemoryError(c, frame, "MEMORY_UPDATE_FAILED", err)
		return
	}
	b.logf("bridge memory update", map[string]any{"memoryId": id})
	c.send(bridgeFrame{Type: frameMemoryResult, ID: frame.ID, Payload: map[string]any{
		"id": id, "ok": true,
	}})
}

// handleMemoryDelete processes a memory.delete frame (SPEC-0071's deletion
// support): it validates the id payload, asks the wired MemoryViewer to
// remove the record, and replies with a synchronous memory.result ack. Without
// a wired viewer it replies ok:false with a MEMORY_DISABLED error.
func (b *Bridge) handleMemoryDelete(c *wsClient, frame bridgeFrame) {
	if b.memory == nil {
		b.sendMemoryDisabled(c, frame)
		return
	}
	id, _ := frame.Payload["id"].(string)
	if id == "" {
		c.send(bridgeFrame{Type: frameMemoryResult, ID: frame.ID, Payload: map[string]any{
			"ok": false,
			"error": map[string]any{
				"code":    "INVALID_MEMORY_ID",
				"message": "memory delete requires a non-empty id",
			},
		}})
		return
	}
	if err := b.memory.Delete(context.Background(), id); err != nil {
		b.sendMemoryError(c, frame, "MEMORY_DELETE_FAILED", err)
		return
	}
	b.logf("bridge memory delete", map[string]any{"memoryId": id})
	c.send(bridgeFrame{Type: frameMemoryResult, ID: frame.ID, Payload: map[string]any{
		"id": id, "ok": true,
	}})
}

// sendMemoryDisabled replies to a memory frame with the MEMORY_DISABLED error
// a Bridge emits when no MemoryViewer is wired.
func (b *Bridge) sendMemoryDisabled(c *wsClient, frame bridgeFrame) {
	c.send(bridgeFrame{Type: frameMemoryResult, ID: frame.ID, Payload: map[string]any{
		"ok": false,
		"error": map[string]any{
			"code":    "MEMORY_DISABLED",
			"message": "no memory viewer is configured on the bridge",
		},
	}})
}

// sendMemoryInvalidType replies to a memory frame whose "type" payload is not
// a known MemoryType.
func (b *Bridge) sendMemoryInvalidType(c *wsClient, frame bridgeFrame) {
	c.send(bridgeFrame{Type: frameMemoryResult, ID: frame.ID, Payload: map[string]any{
		"ok": false,
		"error": map[string]any{
			"code":    "INVALID_MEMORY_TYPE",
			"message": "memory frame requires a known memory type when one is given",
		},
	}})
}

// sendMemoryError replies to a memory frame whose MemoryViewer call failed,
// propagating the failure's typed code (packages/errors) when there is one.
func (b *Bridge) sendMemoryError(c *wsClient, frame bridgeFrame, defaultCode string, err error) {
	fe := bridgeError{Code: defaultCode, Message: err.Error()}
	if e, ok := err.(*errors.Error); ok {
		fe.Code = e.Code
		fe.Message = e.Message
	}
	c.send(bridgeFrame{Type: frameMemoryResult, ID: frame.ID, Payload: map[string]any{
		"ok": false, "error": fe,
	}})
}

// memoryEntries maps SPEC-0034 MemoryRecords onto the wire's MemoryEntry view.
func memoryEntries(records []MemoryRecord) []MemoryEntry {
	entries := make([]MemoryEntry, 0, len(records))
	for _, rec := range records {
		entries = append(entries, MemoryEntry{
			ID:        rec.ID,
			Type:      string(rec.Type),
			Content:   rec.Content,
			Metadata:  rec.Metadata,
			CreatedAt: rec.CreatedAt,
			UpdatedAt: rec.UpdatedAt,
		})
	}
	return entries
}

// memoryTypeField reads the optional "type" payload field: an absent or empty
// value means "all types" (returned as ("", true)); a present value must be a
// known MemoryType or the field is invalid ((MemoryType, false)).
func memoryTypeField(payload map[string]any) (MemoryType, bool) {
	raw, ok := payload["type"].(string)
	if !ok || raw == "" {
		return "", true
	}
	t := MemoryType(raw)
	return t, t.IsValid()
}

// intField reads an integer-typed payload field as JSON numbers unmarshal into
// float64.
func intField(payload map[string]any, key string) (int, bool) {
	switch v := payload[key].(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	default:
		return 0, false
	}
}

// pollApprovals watches the wired ApprovalQueue for newly pending requests
// and forwards each one to every client as a tool.approval_requested frame,
// until ctx is done.
func (b *Bridge) pollApprovals(ctx context.Context) {
	ticker := time.NewTicker(approvalPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, req := range b.approvals.Pending() {
				b.mu.Lock()
				if b.announced[req.ID] {
					b.mu.Unlock()
					continue
				}
				b.announced[req.ID] = true
				b.mu.Unlock()

				b.sendToAll(bridgeFrame{Type: frameToolApprovalRequested, Payload: map[string]any{
					"id":       req.ID,
					"agentId":  req.AgentID,
					"category": req.Category,
				}})
			}
		}
	}
}

// forwardEvent is the EventBus Handler wired for every defaultForwardedEvents
// type: it pushes the Event to every connected client as an "event" frame.
func (b *Bridge) forwardEvent(event types.Event) {
	b.pushEvent(bridgeFrame{
		Type:    frameEvent,
		ID:      event.ID,
		Payload: eventToPayload(string(event.Type), event.Timestamp, event.Source, event.Payload),
	})
}

// pushEvent sends an already-built event frame to every client.
func (b *Bridge) pushEvent(frame bridgeFrame) error {
	return b.sendToAll(frame)
}

// sendToAll delivers frame to every connected client, tolerating and logging
// per-client write failures (a dead client is removed rather than allowed to
// block the sender).
func (b *Bridge) sendToAll(frame bridgeFrame) error {
	b.mu.Lock()
	clients := make([]*wsClient, 0, len(b.clients))
	for _, c := range b.clients {
		clients = append(clients, c)
	}
	b.mu.Unlock()

	for _, c := range clients {
		if err := c.send(frame); err != nil {
			b.logf("bridge send failed", map[string]any{"clientId": c.id, "error": err.Error()})
			b.removeClient(c)
		}
	}
	return nil
}

// send writes frame to this client's connection, serialized through writeMu.
// A context deadline bounds the write so a stalled client cannot block the
// bridge.
func (c *wsClient) send(frame bridgeFrame) error {
	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.Write(ctx, websocket.MessageText, data)
}

// pushStatusChanged pushes a status.changed frame to every client carrying the
// Bridge's current status.
func (b *Bridge) pushStatusChanged() {
	payload, _ := b.statusPayload()
	b.sendToAll(bridgeFrame{Type: frameStatusChanged, Payload: payload})
}

// statusPayload builds the payload map for a status or status.changed frame.
func (b *Bridge) statusPayload() (map[string]any, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	status := BridgeStatus{State: b.state, Version: b.version}
	if b.lastError != "" {
		status.LastError = b.lastError
	}
	payload, err := json.Marshal(status)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// fail records a startup/serve failure: it transitions the Bridge to
// BridgeStateError, records lastError, and pushes the status change.
func (b *Bridge) fail(reason string) {
	b.mu.Lock()
	b.state = BridgeStateError
	b.lastError = reason
	b.mu.Unlock()
	b.logf("bridge failed", map[string]any{"error": reason})
	b.pushStatusChanged()
}

// publish emits an Event of eventType on the Bridge's EventBus, if one is
// configured.
func (b *Bridge) publish(eventType types.EventType, payload map[string]any) {
	if b.bus == nil {
		return
	}
	b.bus.Publish(types.Event{
		Type:      eventType,
		Source:    "core.wsbridge",
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	})
}

// logf records a log entry if a Logger is configured.
func (b *Bridge) logf(msg string, fields map[string]any) {
	if b.log != nil {
		b.log.Info(msg, fields)
	}
}

// eventToPayload builds the payload map for an "event" frame from an Event's
// fields (or from Broadcast's arguments).
func eventToPayload(eventType string, timestamp time.Time, source string, payload any) map[string]any {
	out := map[string]any{
		"eventType": eventType,
		"timestamp": timestamp,
	}
	if source != "" {
		out["source"] = source
	}
	if payload != nil {
		out["payload"] = payload
	}
	return out
}

// isServerClosed reports whether err is the (expected) error http.Server.Serve
// returns when Close is called.
func isServerClosed(err error) bool {
	return err == http.ErrServerClosed
}

// Ensure Bridge implements the Container.WSBridge slot (SPEC-0065).
var _ WSBridge = (*Bridge)(nil)

// Ensure Bridge can be registered as a Runtime Dependency.
var _ Dependency = (*Bridge)(nil)
