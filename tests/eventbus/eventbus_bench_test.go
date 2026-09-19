package eventbus_test

import (
	"context"
	"testing"

	"github.com/thaletto/krcrackers-go/src/eventbus"
)

func BenchmarkPublishNoSubscribers(b *testing.B) {
	bus := eventbus.New()
	ctx := context.Background()
	ev := eventbus.Event{Name: "bench-none"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bus.Publish(ctx, ev)
	}
}

func BenchmarkPublishOneSubscriber(b *testing.B) {
	bus := eventbus.New()
	bus.Subscribe("bench-one", func(_ context.Context, _ eventbus.Event) error { return nil })
	ctx := context.Background()
	ev := eventbus.Event{Name: "bench-one"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bus.Publish(ctx, ev)
	}
}
