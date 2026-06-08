package orders

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

type OrderItemFields struct {
	ProductID   int     `json:"productId"`
	ProductName string  `json:"productName"`
	Price       float64 `json:"price"`
	Quantity    int     `json:"quantity"`
	Total       float64 `json:"total"`
}

type OrderItem struct {
	ID int `json:"id"`
	OrderItemFields
}

type OrderFields struct {
	UserName         string   `json:"userName"`
	Email            string   `json:"email"`
	Phone            string   `json:"phone"`
	Street           string   `json:"street"`
	TownOrCity       string   `json:"townOrCity"`
	State            string   `json:"state"`
	Pincode          string   `json:"pincode"`
	Notes            *string  `json:"notes,omitempty"`
	DeliveryRegion   string   `json:"deliveryRegion"`
	DeliveryLocation string   `json:"deliveryLocation"`
	Total            float64  `json:"total"`
}

type Order struct {
	ID int `json:"id"`
	OrderFields
	Items []OrderItem `json:"items"`
}

type OrderInput struct {
	OrderFields
	Items []OrderItemFields `json:"items"`
}

type ListOrdersResponse struct {
	Items  []Order `json:"items"`
	Total  int     `json:"total"`
	Limit  *int    `json:"limit,omitempty"`
	Offset *int    `json:"offset,omitempty"`
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /orders", s.create)
	mux.HandleFunc("GET /orders", s.list)
	mux.HandleFunc("GET /orders/{id}", s.get)
	mux.HandleFunc("PUT /orders/{id}", s.update)
	mux.HandleFunc("DELETE /orders/{id}", s.delete)
}

func (s *Service) create(w http.ResponseWriter, r *http.Request) {
	var input OrderInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateOrderInput(input); err != nil {
		server.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	order, err := s.repo.Create(r.Context(), input)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to create order")
		return
	}

	server.WriteJSON(w, http.StatusCreated, order)
}

func (s *Service) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	resp, err := s.repo.List(r.Context(), limit, offset)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to list orders")
		return
	}

	server.WriteJSON(w, http.StatusOK, resp)
}

func (s *Service) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid order id")
		return
	}

	order, err := s.repo.Get(r.Context(), id)
	if err != nil {
		if err.Error() == "order not found" {
			server.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to get order")
		return
	}

	server.WriteJSON(w, http.StatusOK, order)
}

func (s *Service) update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid order id")
		return
	}

	var input OrderInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateOrderInput(input); err != nil {
		server.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	order, err := s.repo.Update(r.Context(), id, input)
	if err != nil {
		if err.Error() == "order not found" {
			server.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to update order")
		return
	}

	server.WriteJSON(w, http.StatusOK, order)
}

func (s *Service) delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid order id")
		return
	}

	if err := s.repo.Delete(r.Context(), id); err != nil {
		if err.Error() == "order not found" {
			server.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to delete order")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func validateOrderInput(o OrderInput) error {
	if o.UserName == "" {
		return fmt.Errorf("userName is required")
	}
	if o.Email == "" {
		return fmt.Errorf("email is required")
	}
	if o.Phone == "" {
		return fmt.Errorf("phone is required")
	}
	if o.Street == "" {
		return fmt.Errorf("street is required")
	}
	if o.TownOrCity == "" {
		return fmt.Errorf("townOrCity is required")
	}
	if o.State == "" {
		return fmt.Errorf("state is required")
	}
	if o.Pincode == "" {
		return fmt.Errorf("pincode is required")
	}
	if o.DeliveryRegion == "" {
		return fmt.Errorf("deliveryRegion is required")
	}
	if o.DeliveryLocation == "" {
		return fmt.Errorf("deliveryLocation is required")
	}
	if len(o.Items) == 0 {
		return fmt.Errorf("at least one item is required")
	}
	return nil
}
