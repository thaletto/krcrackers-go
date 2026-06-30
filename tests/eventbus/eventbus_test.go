package eventbus_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thaletto/krcrackers-go/src/eventbus"
)

func TestSubscribeReceivesPublishedEvent(t *testing.T) {
	bus := eventbus.New()
	got := make(chan eventbus.Event, 1)
	bus.Subscribe("ping", func(_ context.Context, e eventbus.Event) error {
		got <- e
		return nil
	})

	want := eventbus.Event{Name: "ping", Payload: "hello"}
	if err := bus.Publish(context.Background(), want); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case got := <-got:
		if got.Name != want.Name || got.Payload != want.Payload {
			t.Fatalf("event mismatch: got %+v, want %+v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("handler did not run within 1s")
	}
}

func TestMultipleSubscribersAllReceiveEvent(t *testing.T) {
	bus := eventbus.New()
	var n int32
	bus.Subscribe("tick", func(_ context.Context, _ eventbus.Event) error {
		atomic.AddInt32(&n, 1)
		return nil
	})
	bus.Subscribe("tick", func(_ context.Context, _ eventbus.Event) error {
		atomic.AddInt32(&n, 1)
		return nil
	})
	bus.Subscribe("tick", func(_ context.Context, _ eventbus.Event) error {
		atomic.AddInt32(&n, 1)
		return nil
	})

	if err := bus.Publish(context.Background(), eventbus.Event{Name: "tick"}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&n) == 3 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected 3 handler invocations, got %d", atomic.LoadInt32(&n))
}

func TestPublishWithNoSubscribersIsNoop(t *testing.T) {
	bus := eventbus.New()
	if err := bus.Publish(context.Background(), eventbus.Event{Name: "nobody-home"}); err != nil {
		t.Fatalf("publish: %v", err)
	}
}

func TestPublishIsAsyncPublishDoesNotBlock(t *testing.T) {
	bus := eventbus.New()
	released := make(chan struct{})
	bus.Subscribe("slow", func(_ context.Context, _ eventbus.Event) error {
		<-released
		return nil
	})

	done := make(chan struct{})
	go func() {
		_ = bus.Publish(context.Background(), eventbus.Event{Name: "slow"})
		close(done)
	}()

	select {
	case <-done:
		close(released)
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on a slow handler")
	}
}

func TestHandlerErrorIsSwallowedByBus(t *testing.T) {
	bus := eventbus.New()
	bus.Subscribe("boom", func(_ context.Context, _ eventbus.Event) error {
		return errors.New("handler kaboom")
	})

	if err := bus.Publish(context.Background(), eventbus.Event{Name: "boom"}); err != nil {
		t.Fatalf("publish should not return handler error, got: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
}
