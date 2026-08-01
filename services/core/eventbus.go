// eventbus.go implements SPEC-0009: the internal publish/subscribe event
// bus. Bus lets Core Runtime components publish Events without knowing who,
// if anyone, is listening, and lets components subscribe to an EventType
// without knowing who publishes it.
package core

import (
	"context"
	"fmt"
	"sync"

	"jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
	types "jarvis-pa/packages/shared-types"
)

// Example event types a producer may publish. Concrete event names are
// producer-specific (see packages/shared-types/event.go); these three are
// the ones named directly in SPEC-0009 as illustrative examples.
const (
	EventUserMessageReceived types.EventType = "USER_MESSAGE_RECEIVED"
	EventAgentStarted        types.EventType = "AGENT_STARTED"
	EventTaskCompleted       types.EventType = "TASK_COMPLETED"
)

// defaultSubscriberBufferSize bounds how many undelivered events queue up
// for a single slow subscriber before Publish starts dropping events for
// it, so one slow listener can never block Publish or the other listeners.
const defaultSubscriberBufferSize = 64

// Handler processes a single Event delivered to a subscription. Handler
// runs on its subscription's own goroutine, never on the publisher's
// goroutine, and never concurrently with itself.
type Handler func(event types.Event)

// EventBus is the SPEC-0009 publish/subscribe contract for internal
// communication between Core Runtime components. This is the concrete
// contract that fills the Container.EventBus slot reserved by SPEC-0008.
type EventBus interface {
	// Publish delivers event to every current subscriber of event.Type.
	// Publish does not block on subscriber processing: delivery to each
	// subscriber happens asynchronously on that subscriber's own
	// goroutine.
	Publish(event types.Event)

	// Subscribe registers handler to receive every future Event published
	// with the given eventType. It returns an unsubscribe function that
	// stops delivery to handler; unsubscribe is safe to call more than
	// once and safe to call concurrently with Publish.
	Subscribe(eventType types.EventType, handler Handler) (unsubscribe func())
}

// subscription is one Subscribe registration.
type subscription struct {
	id       uint64
	handler  Handler
	ch       chan types.Event
	done     chan struct{}
	stopOnce sync.Once
}

// stop signals the subscription's delivery goroutine to exit. Safe to call
// more than once.
func (s *subscription) stop() {
	s.stopOnce.Do(func() { close(s.done) })
}

// Bus is the default EventBus implementation: an in-process, in-memory
// publish/subscribe hub. Bus is safe for concurrent use.
type Bus struct {
	mu         sync.RWMutex
	subs       map[types.EventType][]*subscription
	nextID     uint64
	bufferSize int
	log        *logger.Logger
	closed     bool
	wg         sync.WaitGroup
}

// BusOption configures a Bus created by NewBus.
type BusOption func(*Bus)

// WithSubscriberBufferSize overrides the per-subscriber queue depth. A
// subscriber whose queue is full when Publish sends to it has that event
// dropped rather than blocking the publisher.
func WithSubscriberBufferSize(n int) BusOption {
	return func(b *Bus) { b.bufferSize = n }
}

// WithBusLogger attaches a Logger used to report dropped events. Optional;
// a Bus with no logger silently drops events that overflow a subscriber's
// buffer.
func WithBusLogger(log *logger.Logger) BusOption {
	return func(b *Bus) { b.log = log }
}

// NewBus creates a ready-to-use Bus. No Init call is required before
// Publish or Subscribe.
func NewBus(opts ...BusOption) *Bus {
	b := &Bus{
		subs:       make(map[types.EventType][]*subscription),
		bufferSize: defaultSubscriberBufferSize,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Name identifies the Bus as a Runtime Dependency (see runtime.go), so it
// can be registered via WithDependencies for ordered startup/shutdown.
func (b *Bus) Name() string { return "core.eventbus" }

// Init satisfies the Runtime Dependency interface. Bus needs no I/O to
// become ready, so Init is a no-op.
func (b *Bus) Init(ctx context.Context) error { return nil }

// Publish delivers event to every subscriber currently subscribed to
// event.Type. Delivery is asynchronous and non-blocking: Publish enqueues
// the event on each matching subscriber's buffered channel and returns
// immediately. If a subscriber's buffer is full, that subscriber's copy of
// the event is dropped and logged rather than blocking Publish.
func (b *Bus) Publish(event types.Event) {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return
	}
	subs := append([]*subscription(nil), b.subs[event.Type]...)
	b.mu.RUnlock()

	for _, sub := range subs {
		select {
		case sub.ch <- event:
		default:
			if b.log != nil {
				b.log.Warn("event dropped: subscriber buffer full", map[string]any{
					"event_type":      string(event.Type),
					"subscription_id": sub.id,
				})
			}
		}
	}
}

// deliver invokes sub.handler, recovering any panic so a single faulty
// subscriber cannot take down its delivery goroutine — or, since Handlers
// commonly run on Core Runtime code paths, the process.
func (b *Bus) deliver(sub *subscription, event types.Event) {
	defer func() {
		if r := recover(); r != nil {
			if b.log != nil {
				b.log.Error("event handler panicked", map[string]any{
					"event_type":      string(event.Type),
					"subscription_id": sub.id,
					"panic":           fmt.Sprint(r),
				})
			}
		}
	}()
	sub.handler(event)
}

// Subscribe registers handler for eventType and starts its delivery
// goroutine. The returned unsubscribe function removes the subscription
// and stops that goroutine; it is idempotent and safe for concurrent use.
func (b *Bus) Subscribe(eventType types.EventType, handler Handler) (unsubscribe func()) {
	sub := &subscription{
		handler: handler,
		ch:      make(chan types.Event, b.bufferSize),
		done:    make(chan struct{}),
	}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return func() {}
	}
	b.nextID++
	sub.id = b.nextID
	b.subs[eventType] = append(b.subs[eventType], sub)
	// wg.Add must happen before Unlock: Close acquires this same lock to
	// flip closed and then calls wg.Wait, so adding after Unlock would
	// race Close's Wait on the "Add called concurrently with Wait"
	// contract sync.WaitGroup forbids.
	b.wg.Add(1)
	b.mu.Unlock()

	go func() {
		defer b.wg.Done()
		for {
			select {
			case event := <-sub.ch:
				b.deliver(sub, event)
			case <-sub.done:
				return
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			list := b.subs[eventType]
			for i, s := range list {
				if s == sub {
					b.subs[eventType] = append(list[:i:i], list[i+1:]...)
					break
				}
			}
			if len(b.subs[eventType]) == 0 {
				delete(b.subs, eventType)
			}
			b.mu.Unlock()
			sub.stop()
		})
	}
}

// Close stops every subscription's delivery goroutine and waits for them
// to exit, or for ctx to be done, whichever comes first. After Close,
// Publish is a no-op and Subscribe returns a no-op unsubscribe function.
// Close is safe to call more than once.
func (b *Bus) Close(ctx context.Context) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	var subs []*subscription
	for _, list := range b.subs {
		subs = append(subs, list...)
	}
	b.subs = make(map[types.EventType][]*subscription)
	b.mu.Unlock()

	for _, sub := range subs {
		sub.stop()
	}

	waited := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(waited)
	}()

	select {
	case <-waited:
		return nil
	case <-ctx.Done():
		errType := errors.TypeCanceled
		if ctx.Err() == context.DeadlineExceeded {
			errType = errors.TypeTimeout
		}
		return errors.Wrap(ctx.Err(), errType, "EVENTBUS_CLOSE_INCOMPLETE", "core.eventbus",
			"event bus did not finish delivering in-flight events before context was done")
	}
}
