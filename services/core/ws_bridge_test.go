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

// fakeAgent is a minimal SPEC-0018 Agent returning a fixed result.
type fakeAgent struct {
	id    string
	delay time.Duration
}

func (f *fakeAgent) Metadata() AgentMetadata {
	return AgentMetadata{ID: f.id, Name: f.id, Description: "test agent"}
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
