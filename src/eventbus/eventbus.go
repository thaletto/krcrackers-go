// Package eventbus provides an in-memory publish-subscribe event system
// for decoupled inter-service communication. Handlers run on a bounded
// worker pool so a burst of publishes cannot spawn unbounded goroutines.
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

// Handler is a function that processes an event.
type Handler func(ctx context.Context, event Event) error

// Bus defines the interface for publishing and subscribing to events.
type Bus interface {
	Publish(ctx context.Context, event Event) error
	Subscribe(eventName string, handler Handler)
}

type job struct {
	handler Handler
	ctx     context.Context
	event   Event
}

type memoryBus struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
	queue    chan job
}

const (
	defaultWorkers   = 16
	defaultQueueSize = 1024
)

// New returns a new in-memory event bus backed by a bounded worker pool.
func New() Bus {
	return NewWithOptions(defaultWorkers, defaultQueueSize)
}

// NewWithOptions returns a bus with the given worker/queue sizes. It is
// primarily for tests and tuning.
func NewWithOptions(workers, queueSize int) Bus {
	if workers <= 0 {
		workers = defaultWorkers
	}
	if queueSize <= 0 {
		queueSize = defaultQueueSize
	}
	b := &memoryBus{
		handlers: make(map[string][]Handler),
		queue:    make(chan job, queueSize),
	}
	for i := 0; i < workers; i++ {
		go b.run()
	}
	return b
}

func (b *memoryBus) run() {
	for j := range b.queue {
		if err := j.handler(j.ctx, j.event); err != nil {
			log.Printf("eventbus: handler error for %s: %v", j.event.Name, err)
		}
	}
}

func (b *memoryBus) Publish(ctx context.Context, event Event) error {
	if ctx == nil {
		ctx = context.Background()
	}
	b.mu.RLock()
	handlers := b.handlers[event.Name]
	b.mu.RUnlock()

	for _, h := range handlers {
		j := job{handler: h, ctx: ctx, event: event}
		select {
		case b.queue <- j:
		default:
			log.Printf("eventbus: queue full, running handler inline for %s", event.Name)
			go func(j job) {
				if err := j.handler(j.ctx, j.event); err != nil {
					log.Printf("eventbus: handler error for %s: %v", j.event.Name, err)
				}
			}(j)
		}
	}
	return nil
}

func (b *memoryBus) Subscribe(eventName string, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventName] = append(b.handlers[eventName], handler)
}
