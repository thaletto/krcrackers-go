package products

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/thaletto/krcrackers-go/server"
)

type Service struct {
	repo Repository
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

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /products", s.create)
	mux.HandleFunc("GET /products", s.list)
	mux.HandleFunc("GET /products/{id}", s.get)
	mux.HandleFunc("PUT /products/{id}", s.update)
	mux.HandleFunc("DELETE /products/{id}", s.delete)
}

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

	server.WriteJSON(w, http.StatusCreated, product)
}

func (s *Service) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	resp, err := s.repo.List(r.Context(), limit, offset)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to list products")
		return
	}

	server.WriteJSON(w, http.StatusOK, resp)
}

func (s *Service) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid product id")
		return
	}

	product, err := s.repo.Get(r.Context(), id)
	if err != nil {
		if err.Error() == "product not found" {
			server.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to get product")
		return
	}

	server.WriteJSON(w, http.StatusOK, product)
}

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
		if err.Error() == "product not found" {
			server.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to update product")
		return
	}

	server.WriteJSON(w, http.StatusOK, product)
}

func (s *Service) delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid product id")
		return
	}

	if err := s.repo.Delete(r.Context(), id); err != nil {
		if err.Error() == "product not found" {
			server.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to delete product")
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
