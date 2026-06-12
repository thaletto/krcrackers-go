// Package products provides product catalog management endpoints with
// search, filtering, and sorting. Admin routes require admin role.
// Product changes are published to the event bus for search index sync.
package products

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	apperrors "github.com/thaletto/krcrackers-go/errors"
	"github.com/thaletto/krcrackers-go/eventbus"
	"github.com/thaletto/krcrackers-go/eventbus/events"
	"github.com/thaletto/krcrackers-go/server"
	"github.com/thaletto/krcrackers-go/services/auth"
)

// Service handles product HTTP endpoints and event publishing.
type Service struct {
	repo Repository
	bus  eventbus.Bus
}

type ProductFields struct {
	Name         string   `json:"name"`
	Price        float64  `json:"price"`
	Brand        *string  `json:"brand,omitempty"`
	Description  *string  `json:"description,omitempty"`
	Category     string   `json:"category"`
	Image        *string  `json:"image,omitempty"`
	ComparePrice float64  `json:"comparePrice"`
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

// NewService creates a new products service with event bus integration.
func NewService(repo Repository, bus eventbus.Bus) *Service {
	return &Service{repo: repo, bus: bus}
}

// RegisterRoutes registers product endpoints. Public routes are accessible
// to all; admin routes require admin role authentication.
func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /products", s.list)
	mux.HandleFunc("GET /products/{id}", s.get)

	adminAuth := func(h http.HandlerFunc) http.HandlerFunc {
		return auth.WithAuth(auth.WithAdmin(h)).ServeHTTP
	}
	mux.HandleFunc("POST /admin/products", adminAuth(s.create))
	mux.HandleFunc("PUT /admin/products/{id}", adminAuth(s.update))
	mux.HandleFunc("DELETE /admin/products/{id}", adminAuth(s.delete))
}

// Create godoc
// @Summary      Create a product
// @Description  Create a new product (admin only)
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     cookieAuth
// @Param        input  body      ProductInput  true  "Product details"
// @Success      201    {object}  Product
// @Failure      400    {object}  server.ErrorResponse
// @Failure      401    {object}  server.ErrorResponse
// @Failure      403    {object}  server.ErrorResponse
// @Failure      422    {object}  server.ErrorResponse
// @Router       /admin/products [post]
func (s *Service) create(w http.ResponseWriter, r *http.Request) {
	var input ProductInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateProductInput(input); err != nil {
		server.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	product, err := s.repo.Create(r.Context(), input)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to create product")
		return
	}

	s.publishProductEvent(r.Context(), events.ProductCreated, product)

	server.WriteJSON(w, http.StatusCreated, product)
}

// List godoc
// @Summary      List products
// @Description  List products with optional search, filter, and sort
// @Tags         products
// @Produce      json
// @Param        q          query     string  false  "Search query"
// @Param        category   query     string  false  "Filter by category"
// @Param        brand      query     string  false  "Filter by brand"
// @Param        min_price  query     number  false  "Minimum price"
// @Param        max_price  query     number  false  "Maximum price"
// @Param        sort       query     string  false  "Sort order"
// @Param        limit      query     int     false  "Page limit"
// @Param        offset     query     int     false  "Page offset"
// @Success      200    {object}  ListProductsResponse
// @Failure      500    {object}  server.ErrorResponse
// @Router       /products [get]
func (s *Service) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")
	category := q.Get("category")
	brand := q.Get("brand")
	minPrice, _ := strconv.ParseFloat(q.Get("min_price"), 64)
	maxPrice, _ := strconv.ParseFloat(q.Get("max_price"), 64)
	sort := q.Get("sort")
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	hasFilters := query != "" || category != "" || brand != "" || minPrice > 0 || maxPrice > 0 || sort != ""

	if hasFilters {
		resp, err := s.repo.Search(r.Context(), Filter{
			Query:    query,
			Category: category,
			Brand:    brand,
			MinPrice: minPrice,
			MaxPrice: maxPrice,
			Sort:     sort,
			Limit:    limit,
			Offset:   offset,
		})
		if err != nil {
			server.WriteError(w, http.StatusInternalServerError, "failed to search products")
			return
		}
		server.WriteJSON(w, http.StatusOK, resp)
		return
	}

	resp, err := s.repo.List(r.Context(), limit, offset)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to list products")
		return
	}
	server.WriteJSON(w, http.StatusOK, resp)
}

// Get godoc
// @Summary      Get a product
// @Description  Get a single product by ID
// @Tags         products
// @Produce      json
// @Param        id   path      int  true  "Product ID"
// @Success      200    {object}  Product
// @Failure      400    {object}  server.ErrorResponse
// @Failure      404    {object}  server.ErrorResponse
// @Router       /products/{id} [get]
func (s *Service) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid product id")
		return
	}

	product, err := s.repo.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to get product")
		return
	}

	server.WriteJSON(w, http.StatusOK, product)
}

// Update godoc
// @Summary      Update a product
// @Description  Update an existing product (admin only)
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     cookieAuth
// @Param        id     path      int           true  "Product ID"
// @Param        input  body      ProductInput  true  "Product details"
// @Success      200    {object}  Product
// @Failure      400    {object}  server.ErrorResponse
// @Failure      401    {object}  server.ErrorResponse
// @Failure      403    {object}  server.ErrorResponse
// @Failure      404    {object}  server.ErrorResponse
// @Failure      422    {object}  server.ErrorResponse
// @Router       /admin/products/{id} [put]
func (s *Service) update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid product id")
		return
	}

	var input ProductInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateProductInput(input); err != nil {
		server.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	product, err := s.repo.Update(r.Context(), id, input)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to update product")
		return
	}

	s.publishProductEvent(r.Context(), events.ProductUpdated, product)

	server.WriteJSON(w, http.StatusOK, product)
}

// Delete godoc
// @Summary      Delete a product
// @Description  Delete a product by ID (admin only)
// @Tags         admin
// @Security     cookieAuth
// @Param        id   path      int  true  "Product ID"
// @Success      204
// @Failure      400    {object}  server.ErrorResponse
// @Failure      401    {object}  server.ErrorResponse
// @Failure      403    {object}  server.ErrorResponse
// @Failure      404    {object}  server.ErrorResponse
// @Router       /admin/products/{id} [delete]
func (s *Service) delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid product id")
		return
	}

	if err := s.repo.Delete(r.Context(), id); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to delete product")
		return
	}

	s.publishProductDeleteEvent(r.Context(), id)

	w.WriteHeader(http.StatusNoContent)
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
	return nil
}
