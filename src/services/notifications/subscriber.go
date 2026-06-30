package notifications

import (
	"context"

	"github.com/thaletto/krcrackers-go/src/eventbus"
	"github.com/thaletto/krcrackers-go/src/eventbus/events"
)

// Subscriber listens to order lifecycle events and sends WhatsApp notifications.
type Subscriber struct {
	service Service
}

// NewSubscriber creates a new notification event subscriber.
func NewSubscriber(service Service) *Subscriber {
	return &Subscriber{service: service}
}

// RegisterHandlers subscribes to all order lifecycle events.
func (s *Subscriber) RegisterHandlers(bus eventbus.Bus) {
	bus.Subscribe(events.OrderPlaced, s.handleOrderPlaced)
	bus.Subscribe(events.OrderConfirmed, s.handleOrderConfirmed)
	bus.Subscribe(events.OrderShipped, s.handleOrderShipped)
	bus.Subscribe(events.OrderDelivered, s.handleOrderDelivered)
	bus.Subscribe(events.OrderCancelled, s.handleOrderCancelled)
}

func (s *Subscriber) handleOrderPlaced(ctx context.Context, event eventbus.Event) error {
	payload, ok := event.Payload.(events.OrderEvent)
	if !ok {
		return nil
	}
	s.service.SendOrderPlaced(ctx, payload.Phone, payload.OrderID)
	return nil
}

func (s *Subscriber) handleOrderConfirmed(ctx context.Context, event eventbus.Event) error {
	payload, ok := event.Payload.(events.OrderEvent)
	if !ok {
		return nil
	}
	s.service.SendPaymentConfirmed(ctx, payload.Phone, payload.OrderID)
	return nil
}

func (s *Subscriber) handleOrderShipped(ctx context.Context, event eventbus.Event) error {
	payload, ok := event.Payload.(events.OrderEvent)
	if !ok {
		return nil
	}
	s.service.SendOrderShipped(ctx, payload.Phone, payload.OrderID)
	return nil
}

func (s *Subscriber) handleOrderDelivered(ctx context.Context, event eventbus.Event) error {
	payload, ok := event.Payload.(events.OrderEvent)
	if !ok {
		return nil
	}
	s.service.SendOrderDelivered(ctx, payload.Phone, payload.OrderID)
	return nil
}

func (s *Subscriber) handleOrderCancelled(ctx context.Context, event eventbus.Event) error {
	payload, ok := event.Payload.(events.OrderEvent)
	if !ok {
		return nil
	}
	s.service.SendOrderCancelled(ctx, payload.Phone, payload.OrderID)
	return nil
}
