// Package customers provides the HTTP handlers for customer profile and
// address management. Handlers are thin: decode the request, call into
// services/customers, and encode the response.
package customers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	authapi "github.com/thaletto/krcrackers-go/src/apis/auth"
	"github.com/thaletto/krcrackers-go/src/server"
	"github.com/thaletto/krcrackers-go/src/services/auth"
	svc "github.com/thaletto/krcrackers-go/src/services/customers"
)

// Handler binds the customer HTTP routes to a services/customers.Service.
type Handler struct {
	svc *svc.Service
}

// NewHandler creates a new customers HTTP handler.
func NewHandler(service *svc.Service) *Handler {
	return &Handler{svc: service}
}

// RegisterRoutes wires all customer endpoints on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /customers/profile", authapi.WithAuth(http.HandlerFunc(h.getProfile)).ServeHTTP)
	mux.HandleFunc("PUT /customers/profile", authapi.WithAuth(http.HandlerFunc(h.updateProfile)).ServeHTTP)
	mux.HandleFunc("GET /customers/addresses", authapi.WithAuth(http.HandlerFunc(h.listAddresses)).ServeHTTP)
	mux.HandleFunc("POST /customers/addresses", authapi.WithAuth(http.HandlerFunc(h.createAddress)).ServeHTTP)
	mux.HandleFunc("PUT /customers/addresses/{id}", authapi.WithAuth(http.HandlerFunc(h.updateAddress)).ServeHTTP)
	mux.HandleFunc("DELETE /customers/addresses/{id}", authapi.WithAuth(http.HandlerFunc(h.deleteAddress)).ServeHTTP)
	mux.HandleFunc("PUT /customers/addresses/{id}/default", authapi.WithAuth(http.HandlerFunc(h.setDefaultAddress)).ServeHTTP)
}

// GetProfile godoc
// @Summary      Get customer profile
// @Description  Return the authenticated customer's profile
// @Tags         customers
// @Produce      json
// @Security     cookieAuth
// @Success      200    {object}  auth.User
// @Failure      401    {object}  server.ErrorResponse
// @Failure      404    {object}  server.ErrorResponse
// @Router       /customers/profile [get]
func (h *Handler) getProfile(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r)
	profile, err := h.svc.GetProfile(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, svc.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, "profile not found")
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to get profile")
		return
	}
	server.WriteJSON(w, http.StatusOK, profile)
}

// UpdateProfile godoc
// @Summary      Update customer profile
// @Description  Update the authenticated customer's name and phone
// @Tags         customers
// @Accept       json
// @Produce      json
// @Security     cookieAuth
// @Param        input  body      object{name:string, phone:string}  true  "Profile update"
// @Success      200    {object}  auth.User
// @Failure      400    {object}  server.ErrorResponse
// @Failure      401    {object}  server.ErrorResponse
// @Router       /customers/profile [put]
func (h *Handler) updateProfile(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r)
	var input struct {
		Name  string `json:"name"`
		Phone string `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	profile, err := h.svc.UpdateProfile(r.Context(), user.ID, input.Name, input.Phone)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to update profile")
		return
	}
	server.WriteJSON(w, http.StatusOK, profile)
}

// ListAddresses godoc
// @Summary      List customer addresses
// @Description  Return all addresses for the authenticated customer
// @Tags         customers
// @Produce      json
// @Security     cookieAuth
// @Success      200    {object}  svc.ListAddressesResponse
// @Failure      401    {object}  server.ErrorResponse
// @Router       /customers/addresses [get]
func (h *Handler) listAddresses(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r)
	addresses, err := h.svc.ListAddresses(r.Context(), user.ID)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to list addresses")
		return
	}
	server.WriteJSON(w, http.StatusOK, svc.ListAddressesResponse{Items: addresses, Total: len(addresses)})
}

// CreateAddress godoc
// @Summary      Create a new address
// @Description  Add a new address for the authenticated customer
// @Tags         customers
// @Accept       json
// @Produce      json
// @Security     cookieAuth
// @Param        input  body      svc.AddressInput  true  "Address details"
// @Success      201    {object}  svc.Address
// @Failure      400    {object}  server.ErrorResponse
// @Failure      401    {object}  server.ErrorResponse
// @Failure      422    {object}  server.ErrorResponse
// @Router       /customers/addresses [post]
func (h *Handler) createAddress(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r)
	var input svc.AddressInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	addr, err := h.svc.CreateAddress(r.Context(), user.ID, input)
	if err != nil {
		if err.Error() == "street, city, state, and pincode are required" {
			server.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to create address")
		return
	}
	server.WriteJSON(w, http.StatusCreated, addr)
}

// UpdateAddress godoc
// @Summary      Update an address
// @Description  Update an existing address for the authenticated customer
// @Tags         customers
// @Accept       json
// @Produce      json
// @Security     cookieAuth
// @Param        id     path      int              true  "Address ID"
// @Param        input  body      svc.AddressInput true  "Address details"
// @Success      200    {object}  svc.Address
// @Failure      400    {object}  server.ErrorResponse
// @Failure      401    {object}  server.ErrorResponse
// @Router       /customers/addresses/{id} [put]
func (h *Handler) updateAddress(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r)
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid address id")
		return
	}

	var input svc.AddressInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	addr, err := h.svc.UpdateAddress(r.Context(), user.ID, id, input)
	if err != nil {
		if err.Error() == "street, city, state, and pincode are required" {
			server.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to update address")
		return
	}
	server.WriteJSON(w, http.StatusOK, addr)
}

// DeleteAddress godoc
// @Summary      Delete an address
// @Description  Remove an address for the authenticated customer
// @Tags         customers
// @Security     cookieAuth
// @Param        id   path      int  true  "Address ID"
// @Success      204
// @Failure      400    {object}  server.ErrorResponse
// @Failure      401    {object}  server.ErrorResponse
// @Failure      404    {object}  server.ErrorResponse
// @Router       /customers/addresses/{id} [delete]
func (h *Handler) deleteAddress(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r)
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid address id")
		return
	}

	if err := h.svc.DeleteAddress(r.Context(), user.ID, id); err != nil {
		if errors.Is(err, svc.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, "address not found")
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to delete address")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SetDefaultAddress godoc
// @Summary      Set default address
// @Description  Mark an address as the default for the authenticated customer
// @Tags         customers
// @Produce      json
// @Security     cookieAuth
// @Param        id   path      int  true  "Address ID"
// @Success      200    {object}  server.StatusResponse
// @Failure      400    {object}  server.ErrorResponse
// @Failure      401    {object}  server.ErrorResponse
// @Failure      404    {object}  server.ErrorResponse
// @Router       /customers/addresses/{id}/default [put]
func (h *Handler) setDefaultAddress(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r)
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid address id")
		return
	}

	if err := h.svc.SetDefaultAddress(r.Context(), user.ID, id); err != nil {
		if errors.Is(err, svc.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, "address not found")
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to set default address")
		return
	}
	server.WriteJSON(w, http.StatusOK, server.StatusResponse{Status: "ok"})
}
