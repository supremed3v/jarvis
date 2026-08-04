// ws_bridge_test.go covers SPEC-0065's bridge against a real WebSocket
// client (the library's own Dial), exercising the four requirements - tasks
// transmitted to core, events received, streaming responses, runtime status
// updates - plus cancellation, tool approval forwarding, and lifecycle.
package core

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	pkgerrors "jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
	types "jarvis-pa/packages/shared-types"
)

// testClient wraps a dialed WebSocket connection with read helpers.
type testClient struct {
	t    *testing.T
	conn *websocket.Conn
	addr string
}

// dialTestBridge starts a Bridge on an ephemeral port and connects a test
// client to it.
func dialTestBridge(t *testing.T, b *Bridge) *testClient {
	t.Helper()
	if err := b.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = b.Stop() })

	addr := b.Addr()
	if addr == "" {
		t.Fatal("Bridge.Addr() returned empty after Start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws://"+addr+bridgePath, nil)
	if err != nil {
		t.Fatalf("Dial %s: %v", "ws://"+addr+bridgePath, err)
	}
	t.Cleanup(func() { conn.Close(websocket.StatusNormalClosure, "test done") })

	return &testClient{t: t, conn: conn, addr: addr}
}

// send writes a frame to the client.
func (c *testClient) send(frame bridgeFrame) {
	c.t.Helper()
	data, err := json.Marshal(frame)
	if err != nil {
		c.t.Fatalf("marshal frame: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.conn.Write(ctx, websocket.MessageText, data); err != nil {
		c.t.Fatalf("write frame: %v", err)
	}
}

// read reads the next frame with a timeout.
func (c *testClient) read() bridgeFrame {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, data, err := c.conn.Read(ctx)
	if err != nil {
		c.t.Fatalf("read frame: %v", err)
	}
	var frame bridgeFrame
	if err := json.Unmarshal(data, &frame); err != nil {
		c.t.Fatalf("unmarshal frame %s: %v", string(data), err)
	}
	return frame
}

// readType reads frames until one of the given types arrives (or times out),
// so an unrelated push like a status.changed doesn't derail an assertion.
func (c *testClient) readType(types ...string) bridgeFrame {
	c.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		frame := c.read()
		for _, want := range types {
			if frame.Type == want {
				return frame
			}
		}
	}
	c.t.Fatalf("timed out waiting for frame of type %v", types)
	return bridgeFrame{}
}

// waitConnected blocks until the bridge's on-connect status.changed push
// arrives, which accept only sends after registering the client. Tests that
// push server-side frames (events, broadcasts, approvals) must wait for this
// before acting, or the frame can be dispatched to zero clients and lost.
func waitConnected(t *testing.T, c *testClient) {
	t.Helper()
	frame := c.readType(frameStatusChanged)
	if frame.Payload["state"] != BridgeStateReady {
		t.Fatalf("on-connect status.changed state = %v, want %q", frame.Payload["state"], BridgeStateReady)
	}
}

func TestBridgePing(t *testing.T) {
	b := NewBridge(WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: framePing, ID: "p1"})
	frame := c.readType(framePong)
	if frame.ID != "p1" {
		t.Fatalf("pong id = %q, want p1", frame.ID)
	}
	if frame.Payload["pong"] != true {
		t.Fatalf("pong payload = %v, want pong:true", frame.Payload)
	}
}

func TestBridgeGetStatus(t *testing.T) {
	b := NewBridge(WithBridgeVersion("test-1.0"), WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameGetStatus, ID: "s1"})
	frame := c.readType(frameStatus)
	if frame.ID != "s1" {
		t.Fatalf("status id = %q, want s1", frame.ID)
	}
	if frame.Payload["state"] != BridgeStateReady {
		t.Fatalf("status state = %v, want %q", frame.Payload["state"], BridgeStateReady)
	}
	if frame.Payload["version"] != "test-1.0" {
		t.Fatalf("status version = %v, want test-1.0", frame.Payload["version"])
	}
}

func TestBridgeStatusChangedPushedOnStart(t *testing.T) {
	b := NewBridge(WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)
	waitConnected(t, c)
}

func TestBridgeCommandSubmitBatch(t *testing.T) {
	var mu sync.Mutex
	var got Command
	b := NewBridge(
		WithBridgeLogger(logger.New("test")),
		WithCommandHandler(func(ctx context.Context, cmd Command) (map[string]any, error) {
			mu.Lock()
			got = cmd
			mu.Unlock()
			return map[string]any{"echo": cmd.Text}, nil
		}),
	)
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameCommandSubmit, ID: "cmd-1", Payload: map[string]any{"text": "hello core"}})
	frame := c.readType(frameCommandResult)

	if frame.ID != "cmd-1" {
		t.Fatalf("command.result id = %q, want cmd-1", frame.ID)
	}
	if frame.Payload["ok"] != true {
		t.Fatalf("command.result ok = %v, want true", frame.Payload["ok"])
	}

	mu.Lock()
	if got.ID != "cmd-1" || got.Text != "hello core" {
		t.Fatalf("handler got %+v, want Command{ID:cmd-1, Text:hello core}", got)
	}
	mu.Unlock()
}

func TestBridgeCommandSubmitRejectsEmptyText(t *testing.T) {
	b := NewBridge()
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameCommandSubmit, ID: "cmd-x", Payload: map[string]any{"text": "  "}})
	frame := c.readType(frameError)
	if frame.Payload["error"].(map[string]any)["code"] != "INVALID_COMMAND" {
		t.Fatalf("error code = %v, want INVALID_COMMAND", frame.Payload["error"])
	}
}

func TestBridgeStreamingCommand(t *testing.T) {
	b := NewBridge(
		WithBridgeLogger(logger.New("test")),
		WithStreamingCommandHandler(func(ctx context.Context, cmd Command, onChunk func(CommandChunk) error) error {
			for _, piece := range []string{"hel", "lo ", "world"} {
				if err := onChunk(CommandChunk{Text: piece}); err != nil {
					return err
				}
			}
			return onChunk(CommandChunk{Text: "", Done: true})
		}),
	)
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameCommandSubmit, ID: "stream-1", Payload: map[string]any{"text": "say hello"}})

	var texts []string
	sawDone := false
	for {
		frame := c.readType(frameCommandStream, frameCommandResult)
		switch frame.Type {
		case frameCommandStream:
			texts = append(texts, frame.Payload["text"].(string))
			if frame.Payload["done"] == true {
				sawDone = true
			}
		case frameCommandResult:
			if frame.Payload["ok"] != true {
				t.Fatalf("command.result ok = %v, want true", frame.Payload["ok"])
			}
			partial := frame.Payload["result"].(map[string]any)["text"].(string)
			if partial != "hello world" {
				t.Fatalf("result text = %q, want %q", partial, "hello world")
			}
			if got := strings.Join(texts, ""); got != "hello world" {
				t.Fatalf("streamed texts joined = %q, want %q", got, "hello world")
			}
			if !sawDone {
				t.Fatal("no done=true stream frame was forwarded")
			}
			return
		}
	}
}

func TestBridgeCommandCancel(t *testing.T) {
	b := NewBridge(
		WithBridgeLogger(logger.New("test")),
		WithCommandHandler(func(ctx context.Context, cmd Command) (map[string]any, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}),
	)
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameCommandSubmit, ID: "cancel-1", Payload: map[string]any{"text": "long task"}})
	time.Sleep(50 * time.Millisecond) // let the server register it as in-flight
	c.send(bridgeFrame{Type: frameCommandCancel, ID: "cancel-cmd", Payload: map[string]any{"id": "cancel-1"}})

	frame := c.readType(frameCommandResult)
	if frame.Payload["ok"] != false {
		t.Fatalf("cancelled command.result ok = %v, want false", frame.Payload["ok"])
	}
	if frame.Payload["cancelled"] != true {
		t.Fatalf("cancelled command.result cancelled = %v, want true", frame.Payload["cancelled"])
	}
}

func TestBridgeCommandCancelUnknown(t *testing.T) {
	b := NewBridge()
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameCommandCancel, ID: "cc", Payload: map[string]any{"id": "nope"}})
	frame := c.readType(frameError)
	if frame.Payload["error"].(map[string]any)["code"] != "COMMAND_NOT_FOUND" {
		t.Fatalf("error code = %v, want COMMAND_NOT_FOUND", frame.Payload["error"])
	}
}

// fakeAgent is a minimal SPEC-0018 Agent returning a fixed result. tools,
// perms, and memory optionally fill the Metadata the AgentView exposes.
type fakeAgent struct {
	id     string
	delay  time.Duration
	tools  []string
	perms  []string
	memory []string
}

func (f *fakeAgent) Metadata() AgentMetadata {
	return AgentMetadata{
		ID:           f.id,
		Name:         f.id,
		Description:  "test agent",
		Tools:        f.tools,
		Permissions:  f.perms,
		MemoryAccess: f.memory,
	}
}

func (f *fakeAgent) Execute(ctx context.Context, task *types.Task) (map[string]any, error) {
	if f.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(f.delay):
		}
	}
	return map[string]any{"agent": f.id, "taskId": task.ID, "text": task.Input["text"]}, nil
}

func TestBridgeDefaultAgentDispatch(t *testing.T) {
	bus := NewBus()
	registry := NewRegistry()
	if err := registry.Register(&fakeAgent{id: "core-agent"}); err != nil {
		t.Fatalf("register agent: %v", err)
	}

	var eventMu sync.Mutex
	var events []types.Event
	bus.Subscribe(EventTaskCompleted, func(e types.Event) {
		eventMu.Lock()
		events = append(events, e)
		eventMu.Unlock()
	})

	b := NewBridge(
		WithBridgeEventBus(bus),
		WithBridgeAgentRegistry(registry, "core-agent"),
		WithBridgeLogger(logger.New("test")),
	)
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameCommandSubmit, ID: "agent-1", Payload: map[string]any{"text": "do the thing"}})
	frame := c.readType(frameCommandResult)

	if frame.Payload["ok"] != true {
		t.Fatalf("command.result ok = %v, want true (result: %v)", frame.Payload["ok"], frame.Payload)
	}
	result := frame.Payload["result"].(map[string]any)
	if result["agent"] != "core-agent" {
		t.Fatalf("result agent = %v, want core-agent", result["agent"])
	}
	if result["taskId"] != "agent-1" {
		t.Fatalf("result taskId = %v, want agent-1", result["taskId"])
	}

	// The task lifecycle events must have been published on the bus.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		eventMu.Lock()
		n := len(events)
		eventMu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	eventMu.Lock()
	defer eventMu.Unlock()
	if len(events) == 0 {
		t.Fatal("no EventTaskCompleted was published during agent dispatch")
	}
}

func TestBridgeDefaultAgentDispatchMissingAgent(t *testing.T) {
	b := NewBridge(WithBridgeAgentRegistry(NewRegistry(), "nobody"))
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameCommandSubmit, ID: "m", Payload: map[string]any{"text": "x"}})
	frame := c.readType(frameCommandResult)
	if frame.Payload["ok"] != false {
		t.Fatalf("command.result ok = %v, want false", frame.Payload["ok"])
	}
	if frame.Payload["error"].(map[string]any)["code"] != "AGENT_REGISTRY_AGENT_NOT_FOUND" {
		t.Fatalf("error code = %v, want AGENT_REGISTRY_AGENT_NOT_FOUND", frame.Payload["error"])
	}
}

func TestBridgeCommandWithoutHandler(t *testing.T) {
	b := NewBridge()
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameCommandSubmit, ID: "h", Payload: map[string]any{"text": "x"}})
	frame := c.readType(frameCommandResult)
	if frame.Payload["ok"] != false {
		t.Fatalf("command.result ok = %v, want false", frame.Payload["ok"])
	}
	if frame.Payload["error"].(map[string]any)["code"] != "BRIDGE_NO_COMMAND_HANDLER" {
		t.Fatalf("error code = %v, want BRIDGE_NO_COMMAND_HANDLER", frame.Payload["error"])
	}
}

func TestBridgeForwardsEventBusEvents(t *testing.T) {
	bus := NewBus()
	b := NewBridge(WithBridgeEventBus(bus), WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)
	waitConnected(t, c)

	bus.Publish(types.Event{
		Type:      EventTaskStarted,
		Source:    "core.taskworker",
		Timestamp: time.Now().UTC(),
		Payload:   map[string]any{"taskId": "t-1"},
	})

	frame := c.readType(frameEvent)
	if frame.Payload["eventType"] != string(EventTaskStarted) {
		t.Fatalf("event eventType = %v, want %v", frame.Payload["eventType"], EventTaskStarted)
	}
	if frame.Payload["payload"].(map[string]any)["taskId"] != "t-1" {
		t.Fatalf("event payload = %v, want taskId t-1", frame.Payload["payload"])
	}
}

func TestBridgeBroadcast(t *testing.T) {
	b := NewBridge(WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)
	waitConnected(t, c)

	if err := b.Broadcast("CUSTOM_EVENT", map[string]any{"note": "hi"}); err != nil {
		t.Fatalf("Broadcast: %v", err)
	}
	frame := c.readType(frameEvent)
	if frame.Payload["eventType"] != "CUSTOM_EVENT" {
		t.Fatalf("event eventType = %v, want CUSTOM_EVENT", frame.Payload["eventType"])
	}
}

func TestBridgeToolApprovalFlow(t *testing.T) {
	queue := NewApprovalQueue(WithApprovalQueueLogger(logger.New("test")))
	b := NewBridge(WithApprovalQueue(queue), WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)
	waitConnected(t, c)

	// A pending request blocks until the client approves it.
	type approvalOutcome struct {
		approved bool
		err      error
	}
	outcomeCh := make(chan approvalOutcome, 1)
	go func() {
		approved, err := queue.Request(context.Background(), "core-agent", "terminal")
		outcomeCh <- approvalOutcome{approved: approved, err: err}
	}()

	// The poller must forward the new request.
	frame := c.readType(frameToolApprovalRequested)
	id, _ := frame.Payload["id"].(string)
	if frame.Payload["agentId"] != "core-agent" || frame.Payload["category"] != "terminal" {
		t.Fatalf("approval frame = %v, want agentId core-agent, category terminal", frame.Payload)
	}

	c.send(bridgeFrame{Type: frameToolApprovalResponse, ID: "resp-1", Payload: map[string]any{
		"id": id, "approved": true,
	}})

	select {
	case outcome := <-outcomeCh:
		if outcome.err != nil {
			t.Fatalf("approval request returned error: %v", outcome.err)
		}
		if !outcome.approved {
			t.Fatal("approval request was not approved")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("approval request never resolved")
	}
}

func TestBridgeUnknownFrameType(t *testing.T) {
	b := NewBridge()
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: "bogus.type", ID: "z"})
	frame := c.readType(frameError)
	if frame.Payload["error"].(map[string]any)["code"] != "UNKNOWN_FRAME_TYPE" {
		t.Fatalf("error code = %v, want UNKNOWN_FRAME_TYPE", frame.Payload["error"])
	}
}

func TestBridgeRuntimeDependencyLifecycle(t *testing.T) {
	b := NewBridge(
		WithBridgeListenAddr("127.0.0.1:0"),
		WithBridgeLogger(logger.New("test")),
	)
	if b.Name() != "core.wsbridge" {
		t.Fatalf("Name = %q, want core.wsbridge", b.Name())
	}
	if err := b.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if b.State() != BridgeStateReady {
		t.Fatalf("state after Init = %q, want %q", b.State(), BridgeStateReady)
	}
	if err := b.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if b.State() != BridgeStateStopped {
		t.Fatalf("state after Close = %q, want %q", b.State(), BridgeStateStopped)
	}
	// Close is idempotent.
	if err := b.Close(context.Background()); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestBridgeStopClosesClients(t *testing.T) {
	b := NewBridge(WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)

	if err := b.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if b.State() != BridgeStateStopped {
		t.Fatalf("state after Stop = %q, want %q", b.State(), BridgeStateStopped)
	}
	// The server closing the connection makes the client's next Read fail.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, _, err := c.conn.Read(ctx); err == nil {
		t.Fatal("client read still succeeds after bridge Stop")
	}
}

func TestBridgeStartTwiceFails(t *testing.T) {
	b := NewBridge(WithBridgeListenAddr("127.0.0.1:0"))
	if err := b.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	defer func() { _ = b.Stop() }()
	if err := b.Start(""); err == nil {
		t.Fatal("second Start succeeded, want error")
	}
}

func TestBridgeListenConflictFails(t *testing.T) {
	b := NewBridge(WithBridgeListenAddr("127.0.0.1:0"))
	c := dialTestBridge(t, b)

	// A second bridge cannot bind the same address.
	conflict := NewBridge(WithBridgeListenAddr(c.addr))
	if err := conflict.Start(""); err == nil {
		t.Fatal("conflicting Start succeeded, want error")
	} else if !pkgerrors.Is(err, pkgerrors.TypeUnavailable) {
		t.Fatalf("conflicting Start error type = %v, want TypeUnavailable", err)
	}
}

// fakeVoiceSessionManager is a test double for the SPEC-0060 SessionManager:
// it records whether Start/Stop were called and can be made to fail.
type fakeVoiceSessionManager struct {
	mu       sync.Mutex
	calls    []string
	startErr error
	stopErr  error
}

func (f *fakeVoiceSessionManager) Start() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "start")
	return f.startErr
}

func (f *fakeVoiceSessionManager) Stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "stop")
	return f.stopErr
}

func (f *fakeVoiceSessionManager) callsSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// readVoiceResult reads a voice.result frame, asserts its echoed id and ok
// flag, and returns the frame so the caller can inspect the error.
func (c *testClient) readVoiceResult(id string, wantOK bool) bridgeFrame {
	c.t.Helper()
	frame := c.readType(frameVoiceResult)
	if frame.ID != id {
		c.t.Fatalf("voice.result id = %q, want %q", frame.ID, id)
	}
	if frame.Payload["ok"] != wantOK {
		c.t.Fatalf("voice.result ok = %v, want %v (payload %v)", frame.Payload["ok"], wantOK, frame.Payload)
	}
	return frame
}

func TestBridgeVoiceControlStartAndStop(t *testing.T) {
	sm := &fakeVoiceSessionManager{}
	b := NewBridge(WithBridgeVoiceSessionManager(sm), WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)
	waitConnected(t, c)

	c.send(bridgeFrame{Type: frameVoiceStart, ID: "v1"})
	c.readVoiceResult("v1", true)

	c.send(bridgeFrame{Type: frameVoiceStop, ID: "v2"})
	c.readVoiceResult("v2", true)

	if got, want := sm.callsSnapshot(), []string{"start", "stop"}; !slicesEqual(got, want) {
		t.Fatalf("session manager calls = %v, want %v", got, want)
	}
}

func TestBridgeVoiceControlDisabled(t *testing.T) {
	b := NewBridge(WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)
	waitConnected(t, c)

	c.send(bridgeFrame{Type: frameVoiceStart, ID: "v1"})
	frame := c.readVoiceResult("v1", false)
	if code := frame.Payload["error"].(map[string]any)["code"]; code != "VOICE_DISABLED" {
		t.Fatalf("voice.result error code = %v, want VOICE_DISABLED", code)
	}
}

func TestBridgeVoiceControlFailure(t *testing.T) {
	sm := &fakeVoiceSessionManager{startErr: pkgerrors.New(pkgerrors.TypeInternal, "VOICE_START_FAILED", "core.voice", "boom")}
	b := NewBridge(WithBridgeVoiceSessionManager(sm), WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)
	waitConnected(t, c)

	c.send(bridgeFrame{Type: frameVoiceStart, ID: "v1"})
	frame := c.readVoiceResult("v1", false)
	errorPayload, _ := frame.Payload["error"].(map[string]any)
	if errorPayload["code"] != "VOICE_START_FAILED" {
		t.Fatalf("voice.result error code = %v, want VOICE_START_FAILED", errorPayload["code"])
	}
}

// readAgentResult reads an agent.result frame, asserts its echoed id and ok
// flag, and returns the frame so the caller can inspect the error.
func (c *testClient) readAgentResult(id string, wantOK bool) bridgeFrame {
	c.t.Helper()
	frame := c.readType(frameAgentResult)
	if id != "" && frame.ID != id {
		c.t.Fatalf("agent.result id = %q, want %q", frame.ID, id)
	}
	if frame.Payload["ok"] != wantOK {
		c.t.Fatalf("agent.result ok = %v, want %v (payload %v)", frame.Payload["ok"], wantOK, frame.Payload)
	}
	return frame
}

// readMemoryControlResult reads a memory.result frame carrying an ok field
// (update/delete acks), asserts its echoed id and ok flag, and returns the
// frame so the caller can inspect the error.
func (c *testClient) readMemoryControlResult(id string, wantOK bool) bridgeFrame {
	c.t.Helper()
	frame := c.readType(frameMemoryResult)
	if frame.ID != id {
		c.t.Fatalf("memory.result id = %q, want %q", frame.ID, id)
	}
	if frame.Payload["ok"] != wantOK {
		c.t.Fatalf("memory.result ok = %v, want %v (payload %v)", frame.Payload["ok"], wantOK, frame.Payload)
	}
	return frame
}

// decodeMemoryEntries decodes the memories array of a memory.result frame
// (the list/search replies, which carry no ok field).
func decodeMemoryEntries(t *testing.T, frame bridgeFrame) []MemoryEntry {
	t.Helper()
	raw, ok := frame.Payload["memories"].([]any)
	if !ok {
		t.Fatalf("memory.result payload missing memories array: %v", frame.Payload)
	}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal memories: %v", err)
	}
	var entries []MemoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("unmarshal memories: %v", err)
	}
	return entries
}

// fakeMemoryViewer is a minimal SPEC-0071 MemoryViewer backed by an in-memory
// record map, recording which records are stored so tests can assert what the
// bridge actually read or changed.
type fakeMemoryViewer struct {
	mu        sync.Mutex
	records   map[string]MemoryRecord
	order     []string
	listErr   error
	searchErr error
	updateErr error
	deleteErr error
}

func newFakeMemoryViewer() *fakeMemoryViewer {
	return &fakeMemoryViewer{records: make(map[string]MemoryRecord)}
}

// add stores a record under its ID in insertion order.
func (f *fakeMemoryViewer) add(rec MemoryRecord) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records[rec.ID] = rec
	f.order = append(f.order, rec.ID)
}

func (f *fakeMemoryViewer) get(id string) (MemoryRecord, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec, ok := f.records[id]
	return rec, ok
}

func (f *fakeMemoryViewer) List(ctx context.Context, t MemoryType) ([]MemoryRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []MemoryRecord
	for _, id := range f.order {
		rec := f.records[id]
		if t == "" || rec.Type == t {
			out = append(out, rec)
		}
	}
	return out, nil
}

func (f *fakeMemoryViewer) Search(ctx context.Context, q MemoryQuery) ([]MemoryRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	var out []MemoryRecord
	for _, id := range f.order {
		rec := f.records[id]
		if q.Type != "" && rec.Type != q.Type {
			continue
		}
		if strings.Contains(strings.ToLower(rec.Content), strings.ToLower(q.Query)) {
			out = append(out, rec)
		}
	}
	return out, nil
}

func (f *fakeMemoryViewer) Update(ctx context.Context, rec MemoryRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updateErr != nil {
		return f.updateErr
	}
	stored, ok := f.records[rec.ID]
	if !ok {
		return pkgerrors.New(pkgerrors.TypeNotFound, "MEMORY_NOT_FOUND", "core.wsbridge_test",
			"no memory record with id "+rec.ID)
	}
	stored.Content = rec.Content
	f.records[rec.ID] = stored
	return nil
}

func (f *fakeMemoryViewer) Delete(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if _, ok := f.records[id]; !ok {
		return pkgerrors.New(pkgerrors.TypeNotFound, "MEMORY_NOT_FOUND", "core.wsbridge_test",
			"no memory record with id "+id)
	}
	delete(f.records, id)
	return nil
}

func TestBridgeMemoryListEmpty(t *testing.T) {
	b := NewBridge(WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameMemoryList, ID: "m1"})
	frame := c.readType(frameMemoryResult)
	if frame.ID != "m1" {
		t.Fatalf("memory.result id = %q, want m1", frame.ID)
	}
	if entries := decodeMemoryEntries(t, frame); len(entries) != 0 {
		t.Fatalf("memories = %v, want empty list", entries)
	}
}

func TestBridgeMemoryListAllAndFiltered(t *testing.T) {
	viewer := newFakeMemoryViewer()
	viewer.add(MemoryRecord{ID: "local::1", Type: MemoryTypeUserProfile, Content: "prefers Go over Python"})
	viewer.add(MemoryRecord{ID: "local::2", Type: MemoryTypeKnowledge, Content: "JARVIS architecture notes"})
	b := NewBridge(WithBridgeMemory(viewer), WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameMemoryList, ID: "m1"})
	entries := decodeMemoryEntries(t, c.readType(frameMemoryResult))
	if len(entries) != 2 {
		t.Fatalf("memories = %v, want 2 entries", entries)
	}
	if entries[0].ID != "local::1" || entries[0].Type != string(MemoryTypeUserProfile) ||
		entries[0].Content != "prefers Go over Python" {
		t.Fatalf("entry[0] = %+v, want the user-profile record", entries[0])
	}

	c.send(bridgeFrame{Type: frameMemoryList, ID: "m2", Payload: map[string]any{"type": string(MemoryTypeKnowledge)}})
	filtered := decodeMemoryEntries(t, c.readType(frameMemoryResult))
	if len(filtered) != 1 || filtered[0].ID != "local::2" {
		t.Fatalf("filtered = %v, want only the knowledge record", filtered)
	}
}

func TestBridgeMemoryListInvalidType(t *testing.T) {
	viewer := newFakeMemoryViewer()
	b := NewBridge(WithBridgeMemory(viewer), WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameMemoryList, ID: "m1", Payload: map[string]any{"type": "telepathy"}})
	frame := c.readType(frameMemoryResult)
	if frame.Payload["ok"] != false {
		t.Fatalf("memory.result ok = %v, want false", frame.Payload["ok"])
	}
	if code := frame.Payload["error"].(map[string]any)["code"]; code != "INVALID_MEMORY_TYPE" {
		t.Fatalf("error code = %v, want INVALID_MEMORY_TYPE", code)
	}
}

func TestBridgeMemorySearchMatchesAndTypeFilters(t *testing.T) {
	viewer := newFakeMemoryViewer()
	viewer.add(MemoryRecord{ID: "local::1", Type: MemoryTypeUserProfile, Content: "prefers Go over Python"})
	viewer.add(MemoryRecord{ID: "local::2", Type: MemoryTypeKnowledge, Content: "JARVIS architecture notes"})
	b := NewBridge(WithBridgeMemory(viewer), WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameMemorySearch, ID: "m1", Payload: map[string]any{"query": "go"}})
	entries := decodeMemoryEntries(t, c.readType(frameMemoryResult))
	if len(entries) != 1 || entries[0].ID != "local::1" {
		t.Fatalf("search 'go' = %v, want the Go preference record", entries)
	}

	c.send(bridgeFrame{
		Type:    frameMemorySearch,
		ID:      "m2",
		Payload: map[string]any{"query": "architecture", "type": string(MemoryTypeKnowledge), "limit": 5},
	})
	scoped := decodeMemoryEntries(t, c.readType(frameMemoryResult))
	if len(scoped) != 1 || scoped[0].ID != "local::2" {
		t.Fatalf("scoped search = %v, want the knowledge record", scoped)
	}
}

func TestBridgeMemorySearchInvalidQuery(t *testing.T) {
	viewer := newFakeMemoryViewer()
	b := NewBridge(WithBridgeMemory(viewer), WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameMemorySearch, ID: "m1", Payload: map[string]any{"query": "  "}})
	frame := c.readType(frameMemoryResult)
	if code := frame.Payload["error"].(map[string]any)["code"]; code != "INVALID_MEMORY_QUERY" {
		t.Fatalf("error code = %v, want INVALID_MEMORY_QUERY", code)
	}
}

func TestBridgeMemoryControlDisabled(t *testing.T) {
	b := NewBridge(WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)

	for _, frameType := range []string{frameMemorySearch, frameMemoryUpdate, frameMemoryDelete} {
		c.send(bridgeFrame{Type: frameType, ID: "m1", Payload: map[string]any{"query": "x", "id": "a", "content": "b"}})
		frame := c.readMemoryControlResult("m1", false)
		if code := frame.Payload["error"].(map[string]any)["code"]; code != "MEMORY_DISABLED" {
			t.Fatalf("%s error code = %v, want MEMORY_DISABLED", frameType, code)
		}
	}
}

func TestBridgeMemoryUpdateReplacesContent(t *testing.T) {
	viewer := newFakeMemoryViewer()
	viewer.add(MemoryRecord{ID: "local::1", Type: MemoryTypeUserProfile, Content: "old fact"})
	b := NewBridge(WithBridgeMemory(viewer), WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameMemoryUpdate, ID: "m1", Payload: map[string]any{"id": "local::1", "content": "new fact"}})
	c.readMemoryControlResult("m1", true)

	stored, ok := viewer.get("local::1")
	if !ok || stored.Content != "new fact" {
		t.Fatalf("stored record = %+v (ok %v), want content replaced", stored, ok)
	}
	if stored.Type != MemoryTypeUserProfile {
		t.Fatalf("stored type = %q, want unchanged user_profile", stored.Type)
	}
}

func TestBridgeMemoryUpdateInvalidPayload(t *testing.T) {
	viewer := newFakeMemoryViewer()
	b := NewBridge(WithBridgeMemory(viewer), WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameMemoryUpdate, ID: "m1", Payload: map[string]any{"content": "x"}})
	frame := c.readMemoryControlResult("m1", false)
	if code := frame.Payload["error"].(map[string]any)["code"]; code != "INVALID_MEMORY_ID" {
		t.Fatalf("error code = %v, want INVALID_MEMORY_ID", code)
	}

	c.send(bridgeFrame{Type: frameMemoryUpdate, ID: "m2", Payload: map[string]any{"id": "local::1", "content": "  "}})
	frame = c.readMemoryControlResult("m2", false)
	if code := frame.Payload["error"].(map[string]any)["code"]; code != "INVALID_MEMORY_CONTENT" {
		t.Fatalf("error code = %v, want INVALID_MEMORY_CONTENT", code)
	}
}

func TestBridgeMemoryUpdateUnknownRecord(t *testing.T) {
	viewer := newFakeMemoryViewer()
	b := NewBridge(WithBridgeMemory(viewer), WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameMemoryUpdate, ID: "m1", Payload: map[string]any{"id": "ghost", "content": "x"}})
	frame := c.readMemoryControlResult("m1", false)
	if code := frame.Payload["error"].(map[string]any)["code"]; code != "MEMORY_NOT_FOUND" {
		t.Fatalf("error code = %v, want the viewer's typed MEMORY_NOT_FOUND", code)
	}
}

func TestBridgeMemoryDeleteRemovesRecord(t *testing.T) {
	viewer := newFakeMemoryViewer()
	viewer.add(MemoryRecord{ID: "local::1", Type: MemoryTypeKnowledge, Content: "notes"})
	b := NewBridge(WithBridgeMemory(viewer), WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameMemoryDelete, ID: "m1", Payload: map[string]any{"id": "local::1"}})
	c.readMemoryControlResult("m1", true)

	if _, ok := viewer.get("local::1"); ok {
		t.Fatal("record still present after delete")
	}
}

func TestBridgeMemoryDeleteInvalidAndUnknown(t *testing.T) {
	viewer := newFakeMemoryViewer()
	b := NewBridge(WithBridgeMemory(viewer), WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameMemoryDelete, ID: "m1", Payload: map[string]any{}})
	frame := c.readMemoryControlResult("m1", false)
	if code := frame.Payload["error"].(map[string]any)["code"]; code != "INVALID_MEMORY_ID" {
		t.Fatalf("error code = %v, want INVALID_MEMORY_ID", code)
	}

	c.send(bridgeFrame{Type: frameMemoryDelete, ID: "m2", Payload: map[string]any{"id": "ghost"}})
	frame = c.readMemoryControlResult("m2", false)
	if code := frame.Payload["error"].(map[string]any)["code"]; code != "MEMORY_NOT_FOUND" {
		t.Fatalf("error code = %v, want the viewer's typed MEMORY_NOT_FOUND", code)
	}
}

func TestBridgeMemoryViewerErrorPropagation(t *testing.T) {
	viewer := newFakeMemoryViewer()
	viewer.listErr = pkgerrors.New(pkgerrors.TypeInternal, "MEMORY_INTERNAL", "core.wsbridge_test", "boom")
	b := NewBridge(WithBridgeMemory(viewer), WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameMemoryList, ID: "m1"})
	frame := c.readType(frameMemoryResult)
	if code := frame.Payload["error"].(map[string]any)["code"]; code != "MEMORY_INTERNAL" {
		t.Fatalf("error code = %v, want the viewer's typed MEMORY_INTERNAL", code)
	}
}

// decodeAgentViews decodes the agents array of an agents.result frame.
func decodeAgentViews(t *testing.T, frame bridgeFrame) []AgentView {
	t.Helper()
	raw, ok := frame.Payload["agents"].([]any)
	if !ok {
		t.Fatalf("agents.result payload missing agents array: %v", frame.Payload)
	}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal agents: %v", err)
	}
	var views []AgentView
	if err := json.Unmarshal(data, &views); err != nil {
		t.Fatalf("unmarshal agents: %v", err)
	}
	return views
}

func TestBridgeAgentsListEmpty(t *testing.T) {
	b := NewBridge(WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameAgentsList, ID: "a1"})
	frame := c.readType(frameAgentsResult)
	if frame.ID != "a1" {
		t.Fatalf("agents.result id = %q, want a1", frame.ID)
	}
	if views := decodeAgentViews(t, frame); len(views) != 0 {
		t.Fatalf("agents = %v, want empty list", views)
	}
}

func TestBridgeAgentsListWithRegistry(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(&fakeAgent{
		id: "core-agent", tools: []string{"filesystem.read", "terminal.execute"}, perms: []string{"terminal.execute"}, memory: []string{"conversation"},
	}); err != nil {
		t.Fatalf("register agent: %v", err)
	}

	b := NewBridge(WithBridgeAgentRegistry(registry, "core-agent"), WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameAgentsList, ID: "a1"})
	views := decodeAgentViews(t, c.readType(frameAgentsResult))

	if len(views) != 1 {
		t.Fatalf("agents = %v, want 1 view", views)
	}
	v := views[0]
	if v.ID != "core-agent" || v.Name != "core-agent" || v.Description != "test agent" {
		t.Fatalf("view = %+v, want id/name core-agent with description", v)
	}
	if !slicesEqual(v.Capabilities, []string{"filesystem.read", "terminal.execute"}) {
		t.Fatalf("capabilities = %v, want filesystem.read + terminal.execute", v.Capabilities)
	}
	if !slicesEqual(v.Permissions, []string{"terminal.execute"}) {
		t.Fatalf("permissions = %v, want terminal.execute", v.Permissions)
	}
	if !slicesEqual(v.MemoryAccess, []string{"conversation"}) {
		t.Fatalf("memoryAccess = %v, want conversation", v.MemoryAccess)
	}
	if v.Status != string(types.AgentStatusRegistered) {
		t.Fatalf("status = %q, want %q (no lifecycle wired)", v.Status, types.AgentStatusRegistered)
	}
}

func TestBridgeAgentsListWithLifecycleStatus(t *testing.T) {
	registry := NewRegistry()
	lifecycle := NewLifecycleManager(registry)
	agent := &fakeAgent{id: "core-agent", tools: []string{"filesystem.read"}}
	if err := lifecycle.Register(agent); err != nil {
		t.Fatalf("lifecycle register: %v", err)
	}
	if err := lifecycle.Initialize(context.Background(), "core-agent"); err != nil {
		t.Fatalf("lifecycle initialize: %v", err)
	}

	b := NewBridge(
		WithBridgeAgentRegistry(registry, "core-agent"),
		WithBridgeLifecycleManager(lifecycle),
		WithBridgeLogger(logger.New("test")),
	)
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameAgentsList, ID: "a1"})
	views := decodeAgentViews(t, c.readType(frameAgentsResult))

	if len(views) != 1 {
		t.Fatalf("agents = %v, want 1 view", views)
	}
	if views[0].Status != string(types.AgentStatusReady) {
		t.Fatalf("status = %q, want %q", views[0].Status, types.AgentStatusReady)
	}
}

func TestBridgeAgentEnableDisableLifecycle(t *testing.T) {
	registry := NewRegistry()
	lifecycle := NewLifecycleManager(registry)
	agent := &fakeAgent{id: "core-agent"}
	if err := lifecycle.Register(agent); err != nil {
		t.Fatalf("lifecycle register: %v", err)
	}

	b := NewBridge(
		WithBridgeAgentRegistry(registry, "core-agent"),
		WithBridgeLifecycleManager(lifecycle),
		WithBridgeLogger(logger.New("test")),
	)
	c := dialTestBridge(t, b)

	// Enable from REGISTERED initializes the agent (-> READY).
	c.send(bridgeFrame{Type: frameAgentStart, ID: "e1", Payload: map[string]any{"id": "core-agent"}})
	c.readAgentResult("e1", true)
	if state, _ := lifecycle.State("core-agent"); state != types.AgentStatusReady {
		t.Fatalf("state after first start = %q, want ready", state)
	}

	// Enable again from READY starts it (-> RUNNING).
	c.send(bridgeFrame{Type: frameAgentStart, ID: "e2", Payload: map[string]any{"id": "core-agent"}})
	c.readAgentResult("e2", true)
	if state, _ := lifecycle.State("core-agent"); state != types.AgentStatusRunning {
		t.Fatalf("state after second start = %q, want running", state)
	}

	// Disable stops it (-> STOPPED).
	c.send(bridgeFrame{Type: frameAgentStop, ID: "d1", Payload: map[string]any{"id": "core-agent"}})
	c.readAgentResult("d1", true)
	if state, _ := lifecycle.State("core-agent"); state != types.AgentStatusStopped {
		t.Fatalf("state after stop = %q, want stopped", state)
	}

	// A stopped agent is terminal in SPEC-0021: enabling again is rejected.
	c.send(bridgeFrame{Type: frameAgentStart, ID: "e3", Payload: map[string]any{"id": "core-agent"}})
	frame := c.readAgentResult("e3", false)
	if code := frame.Payload["error"].(map[string]any)["code"]; code != "AGENT_LIFECYCLE_INVALID_TRANSITION" {
		t.Fatalf("re-enable error code = %v, want AGENT_LIFECYCLE_INVALID_TRANSITION", code)
	}
}

func TestBridgeAgentDisableIsIdempotent(t *testing.T) {
	registry := NewRegistry()
	lifecycle := NewLifecycleManager(registry)
	if err := lifecycle.Register(&fakeAgent{id: "core-agent"}); err != nil {
		t.Fatalf("lifecycle register: %v", err)
	}

	b := NewBridge(
		WithBridgeAgentRegistry(registry, "core-agent"),
		WithBridgeLifecycleManager(lifecycle),
		WithBridgeLogger(logger.New("test")),
	)
	c := dialTestBridge(t, b)

	// A REGISTERED (never-enabled) agent disables as a no-op success.
	c.send(bridgeFrame{Type: frameAgentStop, ID: "d0", Payload: map[string]any{"id": "core-agent"}})
	c.readAgentResult("d0", true)
	if state, _ := lifecycle.State("core-agent"); state != types.AgentStatusRegistered {
		t.Fatalf("state = %q, want registered (unchanged by no-op disable)", state)
	}

	// Disabling twice is also a success (already stopped).
	c.send(bridgeFrame{Type: frameAgentStart, ID: "e1", Payload: map[string]any{"id": "core-agent"}})
	c.readAgentResult("e1", true)
	c.send(bridgeFrame{Type: frameAgentStop, ID: "d1", Payload: map[string]any{"id": "core-agent"}})
	c.readAgentResult("d1", true)
	c.send(bridgeFrame{Type: frameAgentStop, ID: "d2", Payload: map[string]any{"id": "core-agent"}})
	c.readAgentResult("d2", true)
}

func TestBridgeAgentControlLifecycleDisabled(t *testing.T) {
	b := NewBridge(WithBridgeLogger(logger.New("test")))
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameAgentStart, ID: "e1", Payload: map[string]any{"id": "core-agent"}})
	frame := c.readAgentResult("e1", false)
	if code := frame.Payload["error"].(map[string]any)["code"]; code != "AGENT_LIFECYCLE_DISABLED" {
		t.Fatalf("error code = %v, want AGENT_LIFECYCLE_DISABLED", code)
	}
}

func TestBridgeAgentControlInvalidId(t *testing.T) {
	registry := NewRegistry()
	lifecycle := NewLifecycleManager(registry)
	b := NewBridge(
		WithBridgeAgentRegistry(registry, "core-agent"),
		WithBridgeLifecycleManager(lifecycle),
		WithBridgeLogger(logger.New("test")),
	)
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameAgentStart, ID: "e1", Payload: map[string]any{}})
	frame := c.readAgentResult("e1", false)
	if code := frame.Payload["error"].(map[string]any)["code"]; code != "INVALID_AGENT_ID" {
		t.Fatalf("error code = %v, want INVALID_AGENT_ID", code)
	}
}

func TestBridgeAgentControlUnknownAgent(t *testing.T) {
	registry := NewRegistry()
	lifecycle := NewLifecycleManager(registry)
	b := NewBridge(
		WithBridgeAgentRegistry(registry, "core-agent"),
		WithBridgeLifecycleManager(lifecycle),
		WithBridgeLogger(logger.New("test")),
	)
	c := dialTestBridge(t, b)

	c.send(bridgeFrame{Type: frameAgentStart, ID: "e1", Payload: map[string]any{"id": "ghost"}})
	frame := c.readAgentResult("e1", false)
	if code := frame.Payload["error"].(map[string]any)["code"]; code != "AGENT_LIFECYCLE_NOT_REGISTERED" {
		t.Fatalf("error code = %v, want AGENT_LIFECYCLE_NOT_REGISTERED", code)
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
