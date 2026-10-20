// Package products provides the product catalogue: creation, listing,
// full-text search, filtering and admin maintenance. Changes are published to
// the event bus.
//
// The repository owns the SQL; this file holds the product rules, the response
// types and the event publishing.
package products

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/thaletto/krcrackers-go/src/eventbus"
	"github.com/thaletto/krcrackers-go/src/eventbus/events"
)

// ErrInvalidSort is returned when a filter asks for an ordering this module
// does not offer. An unrecognised sort is a client mistake, not a request for
// the default order.
var ErrInvalidSort = errors.New("unsupported sort")

type ProductFields struct {
	Price        float64  `json:"price"`
	ComparePrice float64  `json:"comparePrice"`
	Name         string   `json:"name"`
	Category     string   `json:"category"`
	Brand        *string  `json:"brand,omitempty"`
	Description  *string  `json:"description,omitempty"`
	Image        *string  `json:"image,omitempty"`
	Rating       *float64 `json:"rating,omitempty"`
	Delivery     *string  `json:"delivery,omitempty"`
}

type Product struct {
	ID int `json:"id"`
	ProductFields
}

type ProductInput struct {
	ProductFields
}

type ListProductsResponse struct {
	Items  []Product `json:"items"`
	Total  int       `json:"total"`
	Limit  *int      `json:"limit,omitempty"`
	Offset *int      `json:"offset,omitempty"`
}

// Filter narrows and orders a product search. A zero Limit means "omitted":
// every matching product is returned.
type Filter struct {
	Query    string
	Category string
	Brand    string
	MinPrice float64
	MaxPrice float64
	Sort     string
	Limit    int
	Offset   int
}

// sortOrders is the closed set of orderings a search may ask for. It is a map
// rather than a switch with a silent default so that adding one is the only way
// to accept a new value.
var sortOrders = map[string]string{
	"":           "p.id DESC",
	"price_asc":  "p.price ASC",
	"price_desc": "p.price DESC",
}

func orderByClause(sort string) (string, error) {
	clause, ok := sortOrders[sort]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrInvalidSort, sort)
	}
	return clause, nil
}

// Service owns the product rules: validation, the search filter, and the
// events a catalogue change publishes. The HTTP layer (apis/products) decodes
// requests and maps errors to status codes.
type Service struct {
	repo Repository
	bus  eventbus.Bus
}

// NewService creates a new products service over a repository and an event bus.
// bus may be nil when nothing subscribes.
func NewService(repo Repository, bus eventbus.Bus) *Service {
	return &Service{repo: repo, bus: bus}
}

func (s *Service) Create(ctx context.Context, input ProductInput) (Product, error) {
	input = normalizeProductInput(input)
	if err := validateProductInput(input); err != nil {
		return Product{}, err
	}
	product, err := s.repo.Create(ctx, input)
	if err != nil {
		return Product{}, err
	}
	s.publishProductEvent(ctx, events.ProductCreated, product)
	return product, nil
}

func (s *Service) List(ctx context.Context, limit, offset int) (ListProductsResponse, error) {
	return s.repo.List(ctx, limit, offset)
}

// Search filters the catalogue. An unrecognised sort is rejected rather than
// quietly returning a different order than the caller asked for.
func (s *Service) Search(ctx context.Context, filter Filter) (ListProductsResponse, error) {
	if _, err := orderByClause(filter.Sort); err != nil {
		return ListProductsResponse{}, err
	}
	return s.repo.Search(ctx, filter)
}

func (s *Service) Get(ctx context.Context, id int) (Product, error) {
	return s.repo.Get(ctx, id)
}

// GetByIDs returns the catalogue entries for the given ids. It is how the Order
// module prices a basket without reading the products table.
func (s *Service) GetByIDs(ctx context.Context, ids []int) ([]Product, error) {
	return s.repo.GetByIDs(ctx, ids)
}

func (s *Service) Update(ctx context.Context, id int, input ProductInput) (Product, error) {
	input = normalizeProductInput(input)
	if err := validateProductInput(input); err != nil {
		return Product{}, err
	}
	product, err := s.repo.Update(ctx, id, input)
	if err != nil {
		return Product{}, err
	}
	s.publishProductEvent(ctx, events.ProductUpdated, product)
	return product, nil
}

func (s *Service) Delete(ctx context.Context, id int) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.publishProductDeleteEvent(ctx, id)
	return nil
}

func (s *Service) publishProductEvent(ctx context.Context, eventName string, p Product) {
	if s.bus == nil {
		return
	}
	brand := ""
	if p.Brand != nil {
		brand = *p.Brand
	}
	desc := ""
	if p.Description != nil {
		desc = *p.Description
	}
	img := ""
	if p.Image != nil {
		img = *p.Image
	}
	s.bus.Publish(ctx, eventbus.Event{
		Name: eventName,
		Payload: events.ProductEvent{
			ID:           p.ID,
			Name:         p.Name,
			Description:  desc,
			Price:        p.Price,
			Brand:        brand,
			Category:     p.Category,
			Image:        img,
			ComparePrice: p.ComparePrice,
			Rating:       p.Rating,
			Delivery:     p.Delivery,
		},
	})
}

func (s *Service) publishProductDeleteEvent(ctx context.Context, id int) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, eventbus.Event{
		Name:    events.ProductDeleted,
		Payload: events.ProductEvent{ID: id},
	})
}

func validateProductInput(p ProductInput) error {
	if p.Name == "" {
		return fmt.Errorf("name is required")
	}
	if p.Price < 0 {
		return fmt.Errorf("price must be >= 0")
	}
	if p.Category == "" {
		return fmt.Errorf("category is required")
	}
	if p.Rating != nil && (*p.Rating < 0 || *p.Rating > 5) {
		return fmt.Errorf("rating must be between 0 and 5")
	}
	return nil
}

func normalizeProductInput(input ProductInput) ProductInput {
	if input.Delivery != nil {
		delivery := strings.TrimSpace(*input.Delivery)
		if delivery == "" {
			input.Delivery = nil
		} else {
			input.Delivery = &delivery
		}
	}
	return input
}
