// Package orders provides the HTTP handlers for order lifecycle management.
// Handlers are thin: they parse the request, call into services/orders, and
// encode the response. Multipart parsing for /orders/checkout lives here.
package orders

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"

	authapi "github.com/thaletto/krcrackers-go/src/apis/auth"
	apperrors "github.com/thaletto/krcrackers-go/src/errors"
	"github.com/thaletto/krcrackers-go/src/server"
	"github.com/thaletto/krcrackers-go/src/services/auth"
	svc "github.com/thaletto/krcrackers-go/src/services/orders"
)

// OrderBook is what this handler needs from the order module. Declaring it
// here, at the seam that consumes it, keeps the HTTP layer testable without a
// database and keeps the module's own surface free of handler concerns. Every
// method takes the orders.Actor asking, so a handler cannot skip an ownership
// check: there is no unscoped call to make by mistake.
type OrderBook interface {
	Create(ctx context.Context, input svc.OrderInput, actor svc.Actor) (svc.Order, error)
	List(ctx context.Context, actor svc.Actor, filter svc.ListFilter) (svc.ListOrdersResponse, error)
	Get(ctx context.Context, actor svc.Actor, id int) (svc.Order, error)
	Update(ctx context.Context, actor svc.Actor, id int, input svc.OrderInput) (svc.Order, error)
	Delete(ctx context.Context, actor svc.Actor, id int) error
	Checkout(ctx context.Context, actor svc.Actor, addressID int, items []svc.CheckoutItem,
		screenshot io.Reader, filename, contentType, paymentRef string) (svc.Order, string, error)
	Cancel(ctx context.Context, actor svc.Actor, orderID int) (svc.Order, error)
	UpdateStatus(ctx context.Context, actor svc.Actor, orderID int, status svc.OrderStatus) (svc.Order, error)
	DashboardStats(ctx context.Context, actor svc.Actor) (svc.DashboardStats, error)
}

// Handler binds the order HTTP routes to the order module.
type Handler struct {
	svc OrderBook
	// accounts supplies the dashboard's new-customer figure. Optional: the
	// dashboard renders without it rather than failing.
	accounts accounts
}

// NewHandler creates a new orders HTTP handler. accountSource may be nil.
func NewHandler(service OrderBook, accountSource accounts) *Handler {
	return &Handler{svc: service, accounts: accountSource}
}

// actorFor projects the authenticated session onto an orders.Actor. A request
// with no session is a guest, and a guest can read nothing, so a route that
// forgets its middleware degrades to an empty result rather than to a leak.
//
// The handler never inspects a user id itself; it only says who is asking
// and lets the Order module decide what that actor may see.
func actorFor(r *http.Request) svc.Actor {
	user := auth.GetUser(r)
	if user == nil {
		return svc.Guest()
	}
	if user.Role == authapi.RoleAdmin {
		return svc.Administrator(user.ID)
	}
	return svc.Shopper(user.ID)
}

// RegisterRoutes wires all order endpoints on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	authMw := func(fn http.HandlerFunc) http.HandlerFunc {
		return authapi.WithAuth(fn).ServeHTTP
	}
	// Creating an order stays open to guests; a valid token just files the
	// order under that account so it shows up in their order history.
	optionalAuth := func(fn http.HandlerFunc) http.HandlerFunc {
		return authapi.WithOptionalAuth(fn).ServeHTTP
	}
	adminAuth := func(fn http.HandlerFunc) http.HandlerFunc {
		return authapi.WithAuth(authapi.WithAdmin(fn)).ServeHTTP
	}

	mux.HandleFunc("POST /orders", optionalAuth(h.create))

	// These are actor-scoped rather than open: a shopper reaches their own
	// orders, an administrator reaches any. The Order module applies the
	// ownership check, so the middleware here only establishes who is asking.
	mux.HandleFunc("GET /orders", authMw(h.list))
	mux.HandleFunc("GET /orders/{id}", authMw(h.get))
	mux.HandleFunc("PUT /orders/{id}", authMw(h.update))
	mux.HandleFunc("DELETE /orders/{id}", authMw(h.delete))
	mux.HandleFunc("GET /orders/my", authMw(h.listMyOrders))
	mux.HandleFunc("GET /orders/my/{id}", authMw(h.getMyOrder))
	mux.HandleFunc("DELETE /orders/my/{id}", authMw(h.cancelMyOrder))

	mux.HandleFunc("POST /orders/checkout", authMw(h.checkout))

	mux.HandleFunc("GET /admin/orders", adminAuth(h.adminListOrders))
	mux.HandleFunc("GET /admin/orders/{id}", adminAuth(h.adminGetOrder))
	mux.HandleFunc("PATCH /admin/orders/{id}/status", adminAuth(h.adminUpdateStatus))
	mux.HandleFunc("GET /admin/dashboard", adminAuth(h.adminDashboard))
}

// Create godoc
// @Summary      Create an order
// @Description  Create a new order. Guest checkout needs no session; a signed-in shopper's order is filed under their account
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
	if err := server.DecodeJSON(w, r, &input); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Guests stay anonymous, but a signed-in shopper's order belongs to that
	// account, which is what makes it visible in their order history.
	order, err := h.svc.Create(r.Context(), input, actorFor(r))
	if err != nil {
		h.writeIntakeError(w, err)
		return
	}
	server.WriteJSON(w, http.StatusCreated, order)
}

// writeIntakeError maps the failures order intake can report. Prices come from
// the catalogue, so a basket naming a product the shop does not stock is a
// client error rather than a server fault.
func (h *Handler) writeIntakeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, svc.ErrNoItems),
		errors.Is(err, svc.ErrUnknownProduct),
		errors.Is(err, svc.ErrInvalidQuantity),
		errors.Is(err, svc.ErrTooManyItems),
		errors.Is(err, svc.ErrImmutableFields):
		server.WriteError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, svc.ErrIntakeUnavailable):
		server.WriteError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, svc.ErrNotPermitted):
		server.WriteError(w, http.StatusForbidden, err.Error())
	case isValidationError(err):
		// ValidateOrderInput's messages are written for a client to read.
		server.WriteError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		// Anything else is ours: a driver, a network or a storage failure whose
		// text has been observed carrying the Cloudflare account id and the D1
		// database id. Log it, do not hand it over.
		server.WriteInternalError(w, err)
	}
}

// isValidationError recognises the messages ValidateOrderInput builds, which
// exist to be read by a client. Everything else is treated as internal, so a
// new failure cannot leak by default.
func isValidationError(err error) bool {
	msg := err.Error()
	for _, field := range []string{
		"userName", "email", "phone", "street", "townOrCity",
		"state", "pincode", "deliveryRegion", "deliveryLocation",
	} {
		if msg == field+" is required" {
			return true
		}
	}
	return false
}

// List godoc
// @Summary      List orders
// @Description  List the orders the caller may see: their own, or every order for an administrator
// @Tags         orders
// @Produce      json
// @Security     cookieAuth
// @Param        limit   query     int  false  "Page limit"
// @Param        offset  query     int  false  "Page offset"
// @Success      200    {object}  svc.ListOrdersResponse
// @Failure      401    {object}  server.ErrorResponse
// @Failure      500    {object}  server.ErrorResponse
// @Router       /orders [get]
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	h.writeList(w, r, svc.ListFilter{})
}

func (h *Handler) writeList(w http.ResponseWriter, r *http.Request, filter svc.ListFilter) {
	filter.Limit, filter.Offset = server.ParsePage(r)

	resp, err := h.svc.List(r.Context(), actorFor(r), filter)
	if err != nil {
		server.WriteInternalError(w, err)
		return
	}
	server.WriteJSON(w, http.StatusOK, resp)
}

// Get godoc
// @Summary      Get an order
// @Description  Get a single order by ID
// @Tags         orders
// @Produce      json
// @Security     cookieAuth
// @Param        id   path      int  true  "Order ID"
// @Success      200    {object}  svc.Order
// @Failure      400    {object}  server.ErrorResponse
// @Failure      401    {object}  server.ErrorResponse
// @Failure      404    {object}  server.ErrorResponse
// @Router       /orders/{id} [get]
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := orderID(r)
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	order, err := h.svc.Get(r.Context(), actorFor(r), id)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		server.WriteInternalError(w, err)
		return
	}
	server.WriteJSON(w, http.StatusOK, order)
}

// orderID reads the {id} path value. Every order route parses it the same
// way, so the parse and its 400 live in one place.
func orderID(r *http.Request) (int, error) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		return 0, errors.New("invalid order id")
	}
	return id, nil
}

// Update godoc
// @Summary      Update an order
// @Description  Update an order the caller may edit
// @Tags         orders
// @Accept       json
// @Produce      json
// @Security     cookieAuth
// @Param        id     path      int             true  "Order ID"
// @Param        input  body      svc.OrderInput true  "Order details"
// @Success      200    {object}  svc.Order
// @Failure      400    {object}  server.ErrorResponse
// @Failure      401    {object}  server.ErrorResponse
// @Failure      404    {object}  server.ErrorResponse
// @Failure      422    {object}  server.ErrorResponse
// @Router       /orders/{id} [put]
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := orderID(r)
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	var input svc.OrderInput
	if err := server.DecodeJSON(w, r, &input); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	order, err := h.svc.Update(r.Context(), actorFor(r), id, input)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		h.writeIntakeError(w, err)
		return
	}
	server.WriteJSON(w, http.StatusOK, order)
}

// Delete godoc
// @Summary      Delete an order
// @Description  Delete an order the caller may delete
// @Tags         orders
// @Security     cookieAuth
// @Param        id   path      int  true  "Order ID"
// @Success      204
// @Failure      400    {object}  server.ErrorResponse
// @Failure      401    {object}  server.ErrorResponse
// @Failure      404    {object}  server.ErrorResponse
// @Router       /orders/{id} [delete]
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := orderID(r)
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.svc.Delete(r.Context(), actorFor(r), id); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		server.WriteInternalError(w, err)
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
	r.Body = http.MaxBytesReader(w, r.Body, server.MaxUploadBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

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
		server.WriteError(w, http.StatusUnprocessableEntity, svc.ErrNoItems.Error())
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

	order, _, err := h.svc.Checkout(r.Context(), actorFor(r), addressID, items, screenshotFile, screenshotFilename, screenshotCT, r.FormValue("payment_reference"))
	if err != nil {
		writeCheckoutError(w, err)
		return
	}
	server.WriteJSON(w, http.StatusCreated, order)
}

// writeCheckoutError maps service errors from Checkout to HTTP statuses. It
// matches on sentinels, not on message text, so rewording an error inside
// the Order module cannot silently turn a 422 into a 500.
func writeCheckoutError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, svc.ErrNoItems),
		errors.Is(err, svc.ErrAddressNotFound),
		errors.Is(err, svc.ErrShopperNotFound),
		errors.Is(err, svc.ErrUnknownProduct),
		errors.Is(err, svc.ErrInvalidQuantity),
		errors.Is(err, svc.ErrTooManyItems),
		errors.Is(err, svc.ErrNotPermitted):
		server.WriteError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, svc.ErrUploadsUnavailable):
		server.WriteError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, svc.ErrIntakeUnavailable):
		server.WriteError(w, http.StatusServiceUnavailable, err.Error())
	default:
		server.WriteInternalError(w, err)
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
//
// Deprecated: identical to GET /orders now that both resolve against the
// calling actor. Kept so existing storefront links keep working.
func (h *Handler) listMyOrders(w http.ResponseWriter, r *http.Request) {
	h.writeList(w, r, svc.ListFilter{})
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
	id, err := orderID(r)
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	order, err := h.svc.Get(r.Context(), actorFor(r), id)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		server.WriteInternalError(w, err)
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
	id, err := orderID(r)
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	order, err := h.svc.Cancel(r.Context(), actorFor(r), id)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrNotFound):
			server.WriteError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, svc.ErrOnlyPendingCancellable):
			server.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		default:
			server.WriteInternalError(w, err)
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
	h.writeList(w, r, svc.ListFilter{Status: r.URL.Query().Get("status")})
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
	id, err := orderID(r)
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	order, err := h.svc.Get(r.Context(), actorFor(r), id)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		server.WriteInternalError(w, err)
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
	id, err := orderID(r)
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	var input struct {
		Status string `json:"status"`
	}
	if err := server.DecodeJSON(w, r, &input); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	order, err := h.svc.UpdateStatus(r.Context(), actorFor(r), id, svc.OrderStatus(input.Status))
	if err != nil {
		switch {
		case errors.Is(err, svc.ErrNotPermitted):
			server.WriteError(w, http.StatusForbidden, err.Error())
		case errors.Is(err, svc.ErrInvalidStatusTransition), errors.Is(err, apperrors.ErrNotFound):
			server.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		default:
			server.WriteInternalError(w, err)
		}
		return
	}
	server.WriteJSON(w, http.StatusOK, order)
}

// DashboardResponse is the admin dashboard figure set. It composes the Order
// module's numbers with the Account module's: neither module reads the other's
// table to produce a cross-module total.
type DashboardResponse struct {
	svc.DashboardStats
	NewCustomers int `json:"newCustomers"`
}

// accounts is the seam onto the Account module, which owns the users table and
// counts its own rows.
type accounts interface {
	NewCustomersThisMonth(ctx context.Context) (int, error)
}

// AdminDashboard godoc
// @Summary      Dashboard stats
// @Description  Get dashboard statistics (admin)
// @Tags         admin
// @Produce      json
// @Security     cookieAuth
// @Success      200    {object}  DashboardResponse
// @Failure      401    {object}  server.ErrorResponse
// @Failure      403    {object}  server.ErrorResponse
// @Router       /admin/dashboard [get]
func (h *Handler) adminDashboard(w http.ResponseWriter, r *http.Request) {
	actor := actorFor(r)
	stats, err := h.svc.DashboardStats(r.Context(), actor)
	if err != nil {
		if errors.Is(err, svc.ErrNotPermitted) {
			server.WriteError(w, http.StatusForbidden, err.Error())
			return
		}
		server.WriteInternalError(w, err)
		return
	}

	resp := DashboardResponse{DashboardStats: stats}
	if h.accounts != nil {
		newCustomers, err := h.accounts.NewCustomersThisMonth(r.Context())
		if err != nil {
			server.WriteInternalError(w, err)
			return
		}
		resp.NewCustomers = newCustomers
	}
	server.WriteJSON(w, http.StatusOK, resp)
}
