package orders

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
	"github.com/thaletto/krcrackers-go/services/uploads"
)

type Service struct {
	repo         Repository
	userProvider UserProvider
	addrProvider AddressProvider
	uploads      UploadsService
	bus          eventbus.Bus
}

func NewService(
	repo Repository,
	userProvider UserProvider,
	addrProvider AddressProvider,
	uploadsSvc UploadsService,
	bus eventbus.Bus,
) *Service {
	return &Service{
		repo:         repo,
		userProvider: userProvider,
		addrProvider: addrProvider,
		uploads:      uploadsSvc,
		bus:          bus,
	}
}

func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /orders", s.create)
	mux.HandleFunc("GET /orders", s.list)
	mux.HandleFunc("GET /orders/{id}", s.get)
	mux.HandleFunc("PUT /orders/{id}", s.update)
	mux.HandleFunc("DELETE /orders/{id}", s.delete)

	authMw := func(h http.HandlerFunc) http.HandlerFunc {
		return auth.WithAuth(h).ServeHTTP
	}
	adminAuth := func(h http.HandlerFunc) http.HandlerFunc {
		return auth.WithAuth(auth.WithAdmin(h)).ServeHTTP
	}

	mux.HandleFunc("POST /orders/checkout", authMw(s.checkout))
	mux.HandleFunc("GET /orders/my", authMw(s.listMyOrders))
	mux.HandleFunc("GET /orders/my/{id}", authMw(s.getMyOrder))
	mux.HandleFunc("DELETE /orders/my/{id}", authMw(s.cancelMyOrder))

	mux.HandleFunc("GET /admin/orders", adminAuth(s.adminListOrders))
	mux.HandleFunc("GET /admin/orders/{id}", adminAuth(s.adminGetOrder))
	mux.HandleFunc("PATCH /admin/orders/{id}/status", adminAuth(s.adminUpdateStatus))
	mux.HandleFunc("GET /admin/dashboard", adminAuth(s.adminDashboard))
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
		if errors.Is(err, apperrors.ErrNotFound) {
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
		if errors.Is(err, apperrors.ErrNotFound) {
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
		if errors.Is(err, apperrors.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to delete order")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) checkout(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	user := auth.GetUser(r)

	addressIDStr := r.FormValue("address_id")
	addressID, err := strconv.Atoi(addressIDStr)
	if err != nil || addressID == 0 {
		server.WriteError(w, http.StatusUnprocessableEntity, "address_id is required")
		return
	}

	itemsJSON := r.FormValue("items")
	if itemsJSON == "" {
		server.WriteError(w, http.StatusUnprocessableEntity, "items is required")
		return
	}

	var items []CheckoutItem
	if err := json.Unmarshal([]byte(itemsJSON), &items); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid items JSON")
		return
	}
	if len(items) == 0 {
		server.WriteError(w, http.StatusUnprocessableEntity, "at least one item is required")
		return
	}

	var screenshotURL string
	file, header, err := r.FormFile("payment_screenshot")
	if err == nil {
		defer file.Close()
		if s.uploads == nil {
			server.WriteError(w, http.StatusServiceUnavailable, "file uploads not configured")
			return
		}
		key := uploads.GenerateKey("screenshots", header.Filename)
		url, err := s.uploads.Put(r.Context(), key, file, header.Header.Get("Content-Type"))
		if err != nil {
			server.WriteError(w, http.StatusInternalServerError, "failed to upload screenshot")
			return
		}
		screenshotURL = url
	}

	paymentRef := r.FormValue("payment_reference")

	userInfo, err := s.userProvider.GetUser(r.Context(), user.ID)
	if err != nil || userInfo.ID == 0 {
		server.WriteError(w, http.StatusInternalServerError, "failed to get user")
		return
	}

	addr, err := s.addrProvider.GetAddress(r.Context(), addressID)
	if err != nil || addr.ID == 0 {
		server.WriteError(w, http.StatusBadRequest, "address not found")
		return
	}

	orderItems := make([]OrderItemFields, len(items))
	var total float64
	for i, item := range items {
		lineTotal := item.Price * float64(item.Quantity)
		total += lineTotal
		orderItems[i] = OrderItemFields{
			ProductID:   item.ProductID,
			ProductName: item.ProductName,
			Price:       item.Price,
			Quantity:    item.Quantity,
			Total:       lineTotal,
		}
	}

	input := OrderInput{
		OrderFields: OrderFields{
			Status:               StatusPending,
			UserID:               &user.ID,
			UserName:             userInfo.Name,
			Email:                userInfo.Email,
			Phone:                userInfo.Phone,
			Street:               addr.Street,
			TownOrCity:           addr.City,
			State:                addr.State,
			Pincode:              addr.Pincode,
			DeliveryRegion:       addr.State,
			DeliveryLocation:     addr.City,
			Total:                total,
			PaymentScreenshotURL: screenshotURL,
			PaymentReference:     paymentRef,
		},
		Items: orderItems,
	}

	order, err := s.repo.Checkout(r.Context(), input)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to create order")
		return
	}

	s.publishOrderEvent(r.Context(), events.OrderPlaced, order.ID, userInfo.Phone)

	server.WriteJSON(w, http.StatusCreated, order)
}

func (s *Service) listMyOrders(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r)
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	resp, err := s.repo.ListForUser(r.Context(), user.ID, limit, offset)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to list orders")
		return
	}

	server.WriteJSON(w, http.StatusOK, resp)
}

func (s *Service) getMyOrder(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r)
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid order id")
		return
	}

	order, err := s.repo.GetForUser(r.Context(), id, user.ID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to get order")
		return
	}

	server.WriteJSON(w, http.StatusOK, order)
}

func (s *Service) cancelMyOrder(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r)
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid order id")
		return
	}

	order, err := s.repo.GetForUser(r.Context(), id, user.ID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to get order")
		return
	}

	if order.Status != StatusPending {
		server.WriteError(w, http.StatusUnprocessableEntity, "only pending orders can be cancelled")
		return
	}

	updated, err := s.repo.UpdateStatus(r.Context(), id, StatusCancelled)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to cancel order")
		return
	}

	s.publishOrderEvent(r.Context(), events.OrderCancelled, order.ID, order.Phone)

	server.WriteJSON(w, http.StatusOK, updated)
}

func (s *Service) adminListOrders(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status")
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	resp, err := s.repo.ListAllFilter(r.Context(), status, limit, offset)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to list orders")
		return
	}

	server.WriteJSON(w, http.StatusOK, resp)
}

func (s *Service) adminGetOrder(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid order id")
		return
	}

	order, err := s.repo.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to get order")
		return
	}

	server.WriteJSON(w, http.StatusOK, order)
}

func (s *Service) adminUpdateStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid order id")
		return
	}

	var input struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	newStatus := OrderStatus(input.Status)
	order, err := s.repo.UpdateStatus(r.Context(), id, newStatus)
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	var eventName string
	switch newStatus {
	case StatusConfirmed:
		eventName = events.OrderConfirmed
	case StatusShipped:
		eventName = events.OrderShipped
	case StatusDelivered:
		eventName = events.OrderDelivered
	case StatusCancelled:
		eventName = events.OrderCancelled
	}
	if eventName != "" {
		s.publishOrderEvent(r.Context(), eventName, order.ID, order.Phone)
	}

	server.WriteJSON(w, http.StatusOK, order)
}

func (s *Service) adminDashboard(w http.ResponseWriter, r *http.Request) {
	stats, err := s.repo.GetDashboardStats(r.Context())
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to get dashboard stats")
		return
	}

	server.WriteJSON(w, http.StatusOK, stats)
}

type CheckoutItem struct {
	ProductID   int     `json:"productId"`
	ProductName string  `json:"productName"`
	Price       float64 `json:"price"`
	Quantity    int     `json:"quantity"`
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

func (s *Service) publishOrderEvent(ctx context.Context, eventName string, orderID int, phone string) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, eventbus.Event{
		Name: eventName,
		Payload: events.OrderEvent{
			OrderID: orderID,
			Phone:   phone,
			Status:  eventName,
		},
	})
}
