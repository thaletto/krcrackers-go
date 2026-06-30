// Package eventbus provides an in-memory publish-subscribe event system
// for decoupled inter-service communication. Handlers are invoked
// asynchronously via goroutines using context.Background().
package eventbus

import (
	"context"
	"log"
	"sync"
)

// Event represents a named event with an arbitrary payload.
type Event struct {
	Name    string
	Payload any
}

// Handler is a function that processes an event. It receives a context
// (always context.Background()) and the event to handle.
type Handler func(ctx context.Context, event Event) error

// Bus defines the interface for publishing and subscribing to events.
type Bus interface {
	Publish(ctx context.Context, event Event) error
	Subscribe(eventName string, handler Handler)
}

type memoryBus struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
}

// New returns a new in-memory event bus.
func New() Bus {
	return &memoryBus{
		handlers: make(map[string][]Handler),
	}
}

func (b *memoryBus) Publish(_ context.Context, event Event) error {
	b.mu.RLock()
	handlers := b.handlers[event.Name]
	b.mu.RUnlock()

	for _, h := range handlers {
		go func(handler Handler) {
			if err := handler(context.Background(), event); err != nil {
				log.Printf("eventbus: handler error for %s: %v", event.Name, err)
			}
		}(h)
	}
	return nil
}

func (b *memoryBus) Subscribe(eventName string, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventName] = append(b.handlers[eventName], handler)
}
