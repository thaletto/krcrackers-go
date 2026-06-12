package eventbus

import (
	"context"
	"log"
	"sync"
)

type Event struct {
	Name    string
	Payload any
}

type Handler func(ctx context.Context, event Event) error

type Bus interface {
	Publish(ctx context.Context, event Event) error
	Subscribe(eventName string, handler Handler)
}

type memoryBus struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
}

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
