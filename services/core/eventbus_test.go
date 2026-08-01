package core

import (
	"context"
	stderrors "errors"
	"sync"
	"testing"
	"time"

	"jarvis-pa/packages/errors"
	"jarvis-pa/packages/logger"
	types "jarvis-pa/packages/shared-types"
)

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	if !cond() {
		t.Fatal("condition not met before timeout")
	}
}

func TestBus_PublishDeliversToSubscriber(t *testing.T) {
	b := NewBus()
	received := make(chan types.Event, 1)

	b.Subscribe(EventUserMessageReceived, func(event types.Event) {
		received <- event
	})

	want := types.Event{ID: "1", Type: EventUserMessageReceived, Source: "test"}
	b.Publish(want)

	select {
	case got := <-received:
		if got.ID != want.ID || got.Type != want.Type {
			t.Fatalf("handler received %+v, want %+v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("handler was not invoked within timeout")
	}
}

func TestBus_PublishOnlyReachesMatchingEventType(t *testing.T) {
	b := NewBus()
	received := make(chan types.Event, 1)

	b.Subscribe(EventAgentStarted, func(event types.Event) {
		received <- event
	})

	b.Publish(types.Event{ID: "1", Type: EventTaskCompleted})

	select {
	case got := <-received:
		t.Fatalf("subscriber for AGENT_STARTED should not receive %+v", got)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestBus_MultipleListenersAllReceive(t *testing.T) {
	b := NewBus()
	const n = 5
	var wg sync.WaitGroup
	wg.Add(n)

	var mu sync.Mutex
	count := 0

	for i := 0; i < n; i++ {
		b.Subscribe(EventTaskCompleted, func(event types.Event) {
			mu.Lock()
			count++
			mu.Unlock()
			wg.Done()
		})
	}

	b.Publish(types.Event{ID: "1", Type: EventTaskCompleted})

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("not all listeners were invoked within timeout")
	}

	mu.Lock()
	defer mu.Unlock()
	if count != n {
		t.Fatalf("got %d deliveries, want %d", count, n)
	}
}

func TestBus_PublishDoesNotBlockOnSlowSubscriber(t *testing.T) {
	b := NewBus(WithSubscriberBufferSize(1))
	block := make(chan struct{})

	b.Subscribe(EventAgentStarted, func(event types.Event) {
		<-block // never invoked until test closes it
	})

	done := make(chan struct{})
	go func() {
		// First event fills the subscriber's single buffer slot (and
		// starts blocking in the handler); further Publishes must still
		// return immediately rather than waiting on the slow handler.
		b.Publish(types.Event{ID: "1", Type: EventAgentStarted})
		b.Publish(types.Event{ID: "2", Type: EventAgentStarted})
		b.Publish(types.Event{ID: "3", Type: EventAgentStarted})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on a slow subscriber instead of dropping")
	}

	close(block)
}

func TestBus_UnsubscribeStopsDelivery(t *testing.T) {
	b := NewBus()
	var mu sync.Mutex
	count := 0

	unsubscribe := b.Subscribe(EventAgentStarted, func(event types.Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	b.Publish(types.Event{ID: "1", Type: EventAgentStarted})
	waitFor(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return count == 1
	})

	unsubscribe()
	// Idempotent: a second call must not panic.
	unsubscribe()

	b.Publish(types.Event{ID: "2", Type: EventAgentStarted})
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if count != 1 {
		t.Fatalf("got %d deliveries after unsubscribe, want 1", count)
	}
}

func TestBus_SubscribersAreIndependent(t *testing.T) {
	b := NewBus()
	var aCount, bCount int
	var mu sync.Mutex

	b.Subscribe(EventAgentStarted, func(event types.Event) {
		mu.Lock()
		aCount++
		mu.Unlock()
	})
	b.Subscribe(EventTaskCompleted, func(event types.Event) {
		mu.Lock()
		bCount++
		mu.Unlock()
	})

	b.Publish(types.Event{ID: "1", Type: EventAgentStarted})

	waitFor(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return aCount == 1
	})

	mu.Lock()
	defer mu.Unlock()
	if bCount != 0 {
		t.Fatalf("subscriber for TASK_COMPLETED received %d events, want 0", bCount)
	}
}

func TestBus_HandlerRunsOffPublisherGoroutine(t *testing.T) {
	b := NewBus()
	publisherGoroutine := make(chan struct{})
	handlerGoroutine := make(chan struct{})

	b.Subscribe(EventAgentStarted, func(event types.Event) {
		close(handlerGoroutine)
	})

	go func() {
		defer close(publisherGoroutine)
		b.Publish(types.Event{ID: "1", Type: EventAgentStarted})
	}()

	<-publisherGoroutine // Publish returned...
	select {
	case <-handlerGoroutine:
		// ...and the handler ran asynchronously, possibly after Publish
		// already returned. Either ordering is fine; this just proves
		// Publish did not synchronously invoke the handler itself.
	case <-time.After(time.Second):
		t.Fatal("handler was never invoked")
	}
}

func TestBus_PublishAfterCloseIsNoop(t *testing.T) {
	b := NewBus()
	var mu sync.Mutex
	count := 0

	b.Subscribe(EventAgentStarted, func(event types.Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	if err := b.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	b.Publish(types.Event{ID: "1", Type: EventAgentStarted})
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if count != 0 {
		t.Fatalf("got %d deliveries after Close, want 0", count)
	}
}

func TestBus_SubscribeAfterCloseReturnsNoopUnsubscribe(t *testing.T) {
	b := NewBus()
	if err := b.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	unsubscribe := b.Subscribe(EventAgentStarted, func(event types.Event) {})
	unsubscribe() // must not panic
}

func TestBus_CloseIsIdempotent(t *testing.T) {
	b := NewBus()
	if err := b.Close(context.Background()); err != nil {
		t.Fatalf("first Close returned error: %v", err)
	}
	if err := b.Close(context.Background()); err != nil {
		t.Fatalf("second Close returned error: %v", err)
	}
}

func TestBus_CloseWaitsForInFlightHandlersOrContext(t *testing.T) {
	b := NewBus()
	release := make(chan struct{})
	entered := make(chan struct{})

	b.Subscribe(EventAgentStarted, func(event types.Event) {
		close(entered)
		<-release
	})

	b.Publish(types.Event{ID: "1", Type: EventAgentStarted})
	<-entered // handler is now blocked inside release

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := b.Close(ctx)
	if err == nil {
		t.Fatal("Close should have returned ctx's error while the handler was still blocked")
	}
	if !errors.HasCode(err, "EVENTBUS_CLOSE_INCOMPLETE") {
		t.Fatalf("Close error = %v, want a packages/errors error coded EVENTBUS_CLOSE_INCOMPLETE", err)
	}
	if !errors.Is(err, errors.TypeTimeout) {
		t.Fatalf("Close error = %v, want Type TIMEOUT", err)
	}
	if !stderrors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close error chain does not preserve context.DeadlineExceeded: %v", err)
	}

	close(release)
}

func TestBus_HandlerPanicDoesNotStopOtherSubscribers(t *testing.T) {
	b := NewBus(WithBusLogger(logger.New("test")))
	received := make(chan types.Event, 1)

	b.Subscribe(EventAgentStarted, func(event types.Event) {
		panic("boom")
	})
	b.Subscribe(EventAgentStarted, func(event types.Event) {
		received <- event
	})

	b.Publish(types.Event{ID: "1", Type: EventAgentStarted})

	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("a panicking subscriber prevented delivery to a healthy one")
	}

	// The panicking subscriber's goroutine must still be alive afterward.
	var mu sync.Mutex
	count := 0
	unsubscribe := b.Subscribe(EventAgentStarted, func(event types.Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})
	defer unsubscribe()

	b.Publish(types.Event{ID: "2", Type: EventAgentStarted})
	waitFor(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return count == 1
	})
}

func TestBus_SubscribeConcurrentWithCloseDoesNotRace(t *testing.T) {
	// Regression test: wg.Add must happen before Subscribe releases its
	// lock, otherwise a concurrent Close can call wg.Wait before Add,
	// which sync.WaitGroup forbids. Run with -race to catch a regression.
	for i := 0; i < 200; i++ {
		b := NewBus()
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.Subscribe(EventAgentStarted, func(event types.Event) {})
		}()
		if err := b.Close(context.Background()); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
		wg.Wait()
	}
}

func TestBus_ImplementsEventBusInterface(t *testing.T) {
	var _ EventBus = NewBus()
}

func TestBus_IsRuntimeDependency(t *testing.T) {
	var _ Dependency = NewBus()
}

func TestBus_DropsEventsWhenLoggerConfigured(t *testing.T) {
	// Exercises the WithBusLogger option path without asserting on log
	// output content, which packages/logger doesn't expose a hook for.
	b := NewBus(WithSubscriberBufferSize(1), WithBusLogger(logger.New("test")))
	block := make(chan struct{})
	defer close(block)

	b.Subscribe(EventAgentStarted, func(event types.Event) {
		<-block
	})

	b.Publish(types.Event{ID: "1", Type: EventAgentStarted})
	b.Publish(types.Event{ID: "2", Type: EventAgentStarted}) // buffer full, should log+drop, not block
}
