// Package invoices provides the HTTP handlers for on-demand PDF invoice
// generation. Handlers are thin: parse the order id, call into
// services/invoices, and write the PDF bytes with the right headers.
package invoices

import (
	"errors"
	"net/http"
	"strconv"

	authapi "github.com/thaletto/krcrackers-go/src/apis/auth"
	"github.com/thaletto/krcrackers-go/src/server"
	svc "github.com/thaletto/krcrackers-go/src/services/invoices"
)

// Handler binds the invoice HTTP routes to a services/invoices.Service.
type Handler struct {
	svc *svc.Service
}

// NewHandler creates a new invoices HTTP handler.
func NewHandler(service *svc.Service) *Handler {
	return &Handler{svc: service}
}

// RegisterRoutes wires invoice endpoints on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /invoices/{id}", authapi.WithAuth(http.HandlerFunc(h.getCustomerInvoice)).ServeHTTP)
	mux.HandleFunc("GET /admin/invoices/{id}", authapi.WithAuth(authapi.WithAdmin(http.HandlerFunc(h.getAdminInvoice))).ServeHTTP)
}

// GetCustomerInvoice godoc
// @Summary      Get invoice PDF
// @Description  Download a PDF invoice for an order
// @Tags         invoices
// @Produce      application/pdf
// @Security     cookieAuth
// @Param        id   path      int  true  "Order ID"
// @Success      200  {file}    binary
// @Failure      401    {object}  server.ErrorResponse
// @Failure      404    {object}  server.ErrorResponse
// @Router       /invoices/{id} [get]
func (h *Handler) getCustomerInvoice(w http.ResponseWriter, r *http.Request) {
	h.serve(w, r)
}

// GetAdminInvoice godoc
// @Summary      Get invoice PDF (admin)
// @Description  Download a PDF invoice for an order (admin)
// @Tags         admin
// @Produce      application/pdf
// @Security     cookieAuth
// @Param        id   path      int  true  "Order ID"
// @Success      200  {file}    binary
// @Failure      401    {object}  server.ErrorResponse
// @Failure      403    {object}  server.ErrorResponse
// @Failure      404    {object}  server.ErrorResponse
// @Router       /admin/invoices/{id} [get]
func (h *Handler) getAdminInvoice(w http.ResponseWriter, r *http.Request) {
	h.serve(w, r)
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	res, err := h.svc.GeneratePDF(r.Context(), id)
	if err != nil {
		if errors.Is(err, svc.ErrOrderNotFound) {
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
