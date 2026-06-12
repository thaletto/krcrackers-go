package search

import (
	"context"
	"log"

	"github.com/thaletto/krcrackers-go/eventbus"
	"github.com/thaletto/krcrackers-go/eventbus/events"
)

type Subscriber struct {
	service Service
}

func NewSubscriber(service Service) *Subscriber {
	return &Subscriber{service: service}
}

func (s *Subscriber) RegisterHandlers(bus eventbus.Bus) {
	bus.Subscribe(events.ProductCreated, s.handleProductUpsert)
	bus.Subscribe(events.ProductUpdated, s.handleProductUpsert)
	bus.Subscribe(events.ProductDeleted, s.handleProductDelete)
}

func (s *Subscriber) handleProductUpsert(ctx context.Context, event eventbus.Event) error {
	payload, ok := event.Payload.(events.ProductEvent)
	if !ok {
		return nil
	}

	doc := ProductDocument{
		ID:           int64(payload.ID),
		Name:         payload.Name,
		Description:  payload.Description,
		Price:        payload.Price,
		Brand:        payload.Brand,
		Category:     payload.Category,
		Image:        payload.Image,
		ComparePrice: payload.ComparePrice,
	}

	if err := s.service.IndexProduct(ctx, doc); err != nil {
		log.Printf("search: failed to index product %d: %v", payload.ID, err)
		return err
	}
	return nil
}

func (s *Subscriber) handleProductDelete(ctx context.Context, event eventbus.Event) error {
	payload, ok := event.Payload.(events.ProductEvent)
	if !ok {
		return nil
	}

	if err := s.service.DeleteProduct(ctx, payload.ID); err != nil {
		log.Printf("search: failed to delete product %d: %v", payload.ID, err)
		return err
	}
	return nil
}
