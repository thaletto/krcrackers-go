// Package invoices provides the HTTP handlers for on-demand PDF invoice
// generation. Handlers are thin: parse the order id, call into
// services/invoices, and write the PDF bytes with the right headers.
package invoices

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	authapi "github.com/thaletto/krcrackers-go/src/apis/auth"
	apperrors "github.com/thaletto/krcrackers-go/src/errors"
	"github.com/thaletto/krcrackers-go/src/server"
	"github.com/thaletto/krcrackers-go/src/services/auth"
	svc "github.com/thaletto/krcrackers-go/src/services/invoices"
	"github.com/thaletto/krcrackers-go/src/services/orders"
)

// InvoiceRenderer is what this handler needs from the invoice module.
// Declaring it here, at the seam that consumes it, keeps the HTTP layer
// testable without a database and keeps the module's own surface free of
// handler concerns.
type InvoiceRenderer interface {
	GeneratePDF(ctx context.Context, actor orders.Actor, orderID int) (svc.GenerateResult, error)
}

// Handler binds the invoice HTTP routes to the invoice module.
type Handler struct {
	svc InvoiceRenderer
}

// NewHandler creates a new invoices HTTP handler.
func NewHandler(service InvoiceRenderer) *Handler {
	return &Handler{svc: service}
}

// RegisterRoutes wires invoice endpoints on the given mux.
//
// Both routes end up in serve, which resolves the order through the Order
// module against the calling actor. The customer route is therefore scoped
// to that actor's own orders even though its handler is identical to the
// admin one.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /invoices/{id}", authapi.WithAuth(http.HandlerFunc(h.getCustomerInvoice)).ServeHTTP)
	mux.HandleFunc("GET /admin/invoices/{id}", authapi.WithAuth(authapi.WithAdmin(http.HandlerFunc(h.getAdminInvoice))).ServeHTTP)
}

// GetCustomerInvoice godoc
// @Summary      Get invoice PDF
// @Description  Download a PDF invoice for one of the caller's own orders
// @Tags         invoices
// @Produce      application/pdf
// @Security     cookieAuth
// @Param        id   path      int  true  "Order ID"
// @Success      200  {file}    binary
// @Failure      400    {object}  server.ErrorResponse
// @Failure      401    {object}  server.ErrorResponse
// @Failure      404    {object}  server.ErrorResponse
// @Router       /invoices/{id} [get]
func (h *Handler) getCustomerInvoice(w http.ResponseWriter, r *http.Request) {
	h.serve(w, r)
}

// GetAdminInvoice godoc
// @Summary      Get invoice PDF (admin)
// @Description  Download a PDF invoice for any order (admin)
// @Tags         admin
// @Produce      application/pdf
// @Security     cookieAuth
// @Param        id   path      int  true  "Order ID"
// @Success      200  {file}    binary
// @Failure      400    {object}  server.ErrorResponse
// @Failure      401    {object}  server.ErrorResponse
// @Failure      403    {object}  server.ErrorResponse
// @Failure      404    {object}  server.ErrorResponse
// @Router       /admin/invoices/{id} [get]
func (h *Handler) getAdminInvoice(w http.ResponseWriter, r *http.Request) {
	h.serve(w, r)
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		server.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	res, err := h.svc.GeneratePDF(r.Context(), actorFor(r), id)
	if err != nil {
		// The Order module answers "not yours" the same way it answers
		// "does not exist", so this route cannot be used to probe for the
		// existence of another account's orders.
		if errors.Is(err, apperrors.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, "order not found")
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to generate invoice")
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename="+res.Filename)
	_, _ = w.Write(res.PDF)
}

// actorFor projects the authenticated session onto an orders.Actor. A request
// with no session is a guest, who can see no orders at all.
func actorFor(r *http.Request) orders.Actor {
	user := auth.GetUser(r)
	if user == nil {
		return orders.Guest()
	}
	if user.Role == authapi.RoleAdmin {
		return orders.Administrator(user.ID)
	}
	return orders.Shopper(user.ID)
}
