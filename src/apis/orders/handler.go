// Package orders provides the HTTP handlers for order lifecycle management.
// Handlers are thin: they parse the request, call into services/orders, and
// encode the response. Multipart parsing for /orders/checkout lives here.
package orders

import (
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"strconv"

	authapi "github.com/thaletto/krcrackers-go/src/apis/auth"
	apperrors "github.com/thaletto/krcrackers-go/src/errors"
	"github.com/thaletto/krcrackers-go/src/server"
	"github.com/thaletto/krcrackers-go/src/services/auth"
	svc "github.com/thaletto/krcrackers-go/src/services/orders"
)

// Handler binds the order HTTP routes to a services/orders.Service.
type Handler struct {
	svc *svc.Service
}

// NewHandler creates a new orders HTTP handler.
func NewHandler(service *svc.Service) *Handler {
	return &Handler{svc: service}
}

// RegisterRoutes wires all order endpoints on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /orders", h.create)
	mux.HandleFunc("GET /orders", h.list)
	mux.HandleFunc("GET /orders/{id}", h.get)
	mux.HandleFunc("PUT /orders/{id}", h.update)
	mux.HandleFunc("DELETE /orders/{id}", h.delete)

	authMw := func(fn http.HandlerFunc) http.HandlerFunc {
		return authapi.WithAuth(fn).ServeHTTP
	}
	adminAuth := func(fn http.HandlerFunc) http.HandlerFunc {
		return authapi.WithAuth(authapi.WithAdmin(fn)).ServeHTTP
	}

	mux.HandleFunc("POST /orders/checkout", authMw(h.checkout))
	mux.HandleFunc("GET /orders/my", authMw(h.listMyOrders))
	mux.HandleFunc("GET /orders/my/{id}", authMw(h.getMyOrder))
	mux.HandleFunc("DELETE /orders/my/{id}", authMw(h.cancelMyOrder))

	mux.HandleFunc("GET /admin/orders", adminAuth(h.adminListOrders))
	mux.HandleFunc("GET /admin/orders/{id}", adminAuth(h.adminGetOrder))
	mux.HandleFunc("PATCH /admin/orders/{id}/status", adminAuth(h.adminUpdateStatus))
	mux.HandleFunc("GET /admin/dashboard", adminAuth(h.adminDashboard))
}

// Create godoc
// @Summary      Create an order
// @Description  Create a new order (admin/direct)
// @Tags         orders
// @Accept       json
// @Produce      json
// @Param        input  body      svc.OrderInput  true  "Order details"
// @Success      201    {object}  svc.Order
// @Failure      400    {object}  server.ErrorResponse
// @Failure      422    {object}  server.ErrorResponse
// @Router       /orders [post]
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var input svc.OrderInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	order, err := h.svc.Create(r.Context(), input)
	if err != nil {
		server.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	server.WriteJSON(w, http.StatusCreated, order)
}

// List godoc
// @Summary      List orders
// @Description  List all orders with pagination (admin)
// @Tags         orders
// @Produce      json
// @Param        limit   query     int  false  "Page limit"
// @Param        offset  query     int  false  "Page offset"
// @Success      200    {object}  svc.ListOrdersResponse
// @Failure      500    {object}  server.ErrorResponse
// @Router       /orders [get]
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	resp, err := h.svc.List(r.Context(), limit, offset)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to list orders")
		return
	}
	server.WriteJSON(w, http.StatusOK, resp)
}

// Get godoc
// @Summary      Get an order
// @Description  Get a single order by ID
// @Tags         orders
// @Produce      json
// @Param        id   path      int  true  "Order ID"
// @Success      200    {object}  svc.Order
// @Failure      400    {object}  server.ErrorResponse
// @Failure      404    {object}  server.ErrorResponse
// @Router       /orders/{id} [get]
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid order id")
		return
	}

	order, err := h.svc.Get(r.Context(), id)
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

// Update godoc
// @Summary      Update an order
// @Description  Update an existing order by ID
// @Tags         orders
// @Accept       json
// @Produce      json
// @Param        id     path      int             true  "Order ID"
// @Param        input  body      svc.OrderInput true  "Order details"
// @Success      200    {object}  svc.Order
// @Failure      400    {object}  server.ErrorResponse
// @Failure      404    {object}  server.ErrorResponse
// @Failure      422    {object}  server.ErrorResponse
// @Router       /orders/{id} [put]
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid order id")
		return
	}

	var input svc.OrderInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	order, err := h.svc.Update(r.Context(), id, input)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		server.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	server.WriteJSON(w, http.StatusOK, order)
}

// Delete godoc
// @Summary      Delete an order
// @Description  Delete an order by ID
// @Tags         orders
// @Param        id   path      int  true  "Order ID"
// @Success      204
// @Failure      400    {object}  server.ErrorResponse
// @Failure      404    {object}  server.ErrorResponse
// @Router       /orders/{id} [delete]
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid order id")
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to delete order")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Checkout godoc
// @Summary      Checkout
// @Description  Place an order with address, items, and optional payment screenshot
// @Tags         orders
// @Accept       multipart/form-data
// @Produce      json
// @Security     cookieAuth
// @Param        address_id          formData  int     true   "Address ID"
// @Param        items               formData  string  true   "JSON array of checkout items"
// @Param        payment_screenshot   formData  file    false  "Payment screenshot"
// @Param        payment_reference   formData  string  false  "Payment reference"
// @Success      201    {object}  svc.Order
// @Failure      400    {object}  server.ErrorResponse
// @Failure      401    {object}  server.ErrorResponse
// @Failure      422    {object}  server.ErrorResponse
// @Router       /orders/checkout [post]
func (h *Handler) checkout(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	user := auth.GetUser(r)

	addressID, err := strconv.Atoi(r.FormValue("address_id"))
	if err != nil || addressID == 0 {
		server.WriteError(w, http.StatusUnprocessableEntity, "address_id is required")
		return
	}

	itemsJSON := r.FormValue("items")
	if itemsJSON == "" {
		server.WriteError(w, http.StatusUnprocessableEntity, "items is required")
		return
	}
	var items []svc.CheckoutItem
	if err := json.Unmarshal([]byte(itemsJSON), &items); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid items JSON")
		return
	}
	if len(items) == 0 {
		server.WriteError(w, http.StatusUnprocessableEntity, "at least one item is required")
		return
	}

	var (
		screenshotFile     = (multipart.File)(nil)
		screenshotFilename string
		screenshotCT       string
	)
	if file, header, err := r.FormFile("payment_screenshot"); err == nil {
		defer file.Close()
		screenshotFile = file
		screenshotFilename = header.Filename
		screenshotCT = header.Header.Get("Content-Type")
	}

	order, _, err := h.svc.Checkout(r.Context(), user.ID, addressID, items, screenshotFile, screenshotFilename, screenshotCT, r.FormValue("payment_reference"))
	if err != nil {
		h.writeCheckoutError(w, err)
		return
	}
	server.WriteJSON(w, http.StatusCreated, order)
}

// writeCheckoutError maps service errors from Checkout to HTTP statuses.
func (h *Handler) writeCheckoutError(w http.ResponseWriter, err error) {
	msg := err.Error()
	switch {
	case msg == "at least one item is required",
		msg == "address not found",
		msg == "user not found",
		msg == "get address: address not found":
		server.WriteError(w, http.StatusUnprocessableEntity, msg)
	case msg == "file uploads not configured":
		server.WriteError(w, http.StatusServiceUnavailable, msg)
	default:
		server.WriteError(w, http.StatusInternalServerError, msg)
	}
}

// ListMyOrders godoc
// @Summary      List my orders
// @Description  List orders for the authenticated customer
// @Tags         orders
// @Produce      json
// @Security     cookieAuth
// @Param        limit   query     int  false  "Page limit"
// @Param        offset  query     int  false  "Page offset"
// @Success      200    {object}  svc.ListOrdersResponse
// @Failure      401    {object}  server.ErrorResponse
// @Router       /orders/my [get]
func (h *Handler) listMyOrders(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r)
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	resp, err := h.svc.ListForUser(r.Context(), user.ID, limit, offset)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to list orders")
		return
	}
	server.WriteJSON(w, http.StatusOK, resp)
}

// GetMyOrder godoc
// @Summary      Get my order
// @Description  Get a single order for the authenticated customer
// @Tags         orders
// @Produce      json
// @Security     cookieAuth
// @Param        id   path      int  true  "Order ID"
// @Success      200    {object}  svc.Order
// @Failure      400    {object}  server.ErrorResponse
// @Failure      401    {object}  server.ErrorResponse
// @Failure      404    {object}  server.ErrorResponse
// @Router       /orders/my/{id} [get]
func (h *Handler) getMyOrder(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r)
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid order id")
		return
	}

	order, err := h.svc.GetForUser(r.Context(), id, user.ID)
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

// CancelMyOrder godoc
// @Summary      Cancel my order
// @Description  Cancel a pending order for the authenticated customer
// @Tags         orders
// @Produce      json
// @Security     cookieAuth
// @Param        id   path      int  true  "Order ID"
// @Success      200    {object}  svc.Order
// @Failure      400    {object}  server.ErrorResponse
// @Failure      401    {object}  server.ErrorResponse
// @Failure      404    {object}  server.ErrorResponse
// @Failure      422    {object}  server.ErrorResponse
// @Router       /orders/my/{id} [delete]
func (h *Handler) cancelMyOrder(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r)
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid order id")
		return
	}

	order, err := h.svc.CancelMyOrder(r.Context(), user.ID, id)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrNotFound):
			server.WriteError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, svc.ErrOnlyPendingCancellable):
			server.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		default:
			server.WriteError(w, http.StatusInternalServerError, "failed to cancel order")
		}
		return
	}
	server.WriteJSON(w, http.StatusOK, order)
}

// AdminListOrders godoc
// @Summary      List all orders
// @Description  List all orders with optional status filter (admin)
// @Tags         admin
// @Produce      json
// @Security     cookieAuth
// @Param        status  query     string  false  "Filter by status"
// @Param        limit   query     int     false  "Page limit"
// @Param        offset  query     int     false  "Page offset"
// @Success      200    {object}  svc.ListOrdersResponse
// @Failure      401    {object}  server.ErrorResponse
// @Failure      403    {object}  server.ErrorResponse
// @Router       /admin/orders [get]
func (h *Handler) adminListOrders(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status")
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	resp, err := h.svc.ListAllFilter(r.Context(), status, limit, offset)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to list orders")
		return
	}
	server.WriteJSON(w, http.StatusOK, resp)
}

// AdminGetOrder godoc
// @Summary      Get order details
// @Description  Get a single order by ID (admin)
// @Tags         admin
// @Produce      json
// @Security     cookieAuth
// @Param        id   path      int  true  "Order ID"
// @Success      200    {object}  svc.Order
// @Failure      400    {object}  server.ErrorResponse
// @Failure      401    {object}  server.ErrorResponse
// @Failure      403    {object}  server.ErrorResponse
// @Failure      404    {object}  server.ErrorResponse
// @Router       /admin/orders/{id} [get]
func (h *Handler) adminGetOrder(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid order id")
		return
	}

	order, err := h.svc.Get(r.Context(), id)
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

// AdminUpdateStatus godoc
// @Summary      Update order status
// @Description  Update the status of an order (admin)
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     cookieAuth
// @Param        id     path      int                    true  "Order ID"
// @Param        input  body      object{status:string}  true  "New status"
// @Success      200    {object}  svc.Order
// @Failure      400    {object}  server.ErrorResponse
// @Failure      401    {object}  server.ErrorResponse
// @Failure      403    {object}  server.ErrorResponse
// @Router       /admin/orders/{id}/status [patch]
func (h *Handler) adminUpdateStatus(w http.ResponseWriter, r *http.Request) {
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

	order, err := h.svc.UpdateStatus(r.Context(), id, svc.OrderStatus(input.Status))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	server.WriteJSON(w, http.StatusOK, order)
}

// AdminDashboard godoc
// @Summary      Dashboard stats
// @Description  Get dashboard statistics (admin)
// @Tags         admin
// @Produce      json
// @Security     cookieAuth
// @Success      200    {object}  svc.DashboardStats
// @Failure      401    {object}  server.ErrorResponse
// @Failure      403    {object}  server.ErrorResponse
// @Router       /admin/dashboard [get]
func (h *Handler) adminDashboard(w http.ResponseWriter, r *http.Request) {
	stats, err := h.svc.GetDashboardStats(r.Context())
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to get dashboard stats")
		return
	}
	server.WriteJSON(w, http.StatusOK, stats)
}
