// Package customers provides customer profile and address management endpoints.
// All routes require authentication via the auth middleware.
package customers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/thaletto/krcrackers-go/server"
	"github.com/thaletto/krcrackers-go/services/auth"
)

// Service handles customer profile and address HTTP endpoints.
type Service struct {
	repo Repository
}

// ListAddressesResponse is the response for listing customer addresses.
type ListAddressesResponse struct {
	Items  []Address `json:"items"`
	Total  int       `json:"total"`
}

// NewService creates a new customers service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// RegisterRoutes registers all customer endpoints on the given mux.
func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /customers/profile", auth.WithAuth(http.HandlerFunc(s.getProfile)).ServeHTTP)
	mux.HandleFunc("PUT /customers/profile", auth.WithAuth(http.HandlerFunc(s.updateProfile)).ServeHTTP)
	mux.HandleFunc("GET /customers/addresses", auth.WithAuth(http.HandlerFunc(s.listAddresses)).ServeHTTP)
	mux.HandleFunc("POST /customers/addresses", auth.WithAuth(http.HandlerFunc(s.createAddress)).ServeHTTP)
	mux.HandleFunc("PUT /customers/addresses/{id}", auth.WithAuth(http.HandlerFunc(s.updateAddress)).ServeHTTP)
	mux.HandleFunc("DELETE /customers/addresses/{id}", auth.WithAuth(http.HandlerFunc(s.deleteAddress)).ServeHTTP)
	mux.HandleFunc("PUT /customers/addresses/{id}/default", auth.WithAuth(http.HandlerFunc(s.setDefaultAddress)).ServeHTTP)
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
func (s *Service) getProfile(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r)
	profile, err := s.repo.GetProfile(r.Context(), user.ID)
	if err != nil || profile.ID == 0 {
		server.WriteError(w, http.StatusNotFound, "profile not found")
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
func (s *Service) updateProfile(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r)
	var input struct {
		Name  string `json:"name"`
		Phone string `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	profile, err := s.repo.UpdateProfile(r.Context(), user.ID, input.Name, input.Phone)
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
// @Success      200    {object}  ListAddressesResponse
// @Failure      401    {object}  server.ErrorResponse
// @Router       /customers/addresses [get]
func (s *Service) listAddresses(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r)
	addresses, err := s.repo.ListAddresses(r.Context(), user.ID)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to list addresses")
		return
	}
	server.WriteJSON(w, http.StatusOK, map[string]any{"items": addresses, "total": len(addresses)})
}

// CreateAddress godoc
// @Summary      Create a new address
// @Description  Add a new address for the authenticated customer
// @Tags         customers
// @Accept       json
// @Produce      json
// @Security     cookieAuth
// @Param        input  body      AddressInput  true  "Address details"
// @Success      201    {object}  Address
// @Failure      400    {object}  server.ErrorResponse
// @Failure      401    {object}  server.ErrorResponse
// @Failure      422    {object}  server.ErrorResponse
// @Router       /customers/addresses [post]
func (s *Service) createAddress(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r)
	var input AddressInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if input.Street == "" || input.City == "" || input.State == "" || input.Pincode == "" {
		server.WriteError(w, http.StatusUnprocessableEntity, "street, city, state, and pincode are required")
		return
	}

	addr, err := s.repo.CreateAddress(r.Context(), user.ID, input)
	if err != nil {
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
// @Param        id     path      int           true  "Address ID"
// @Param        input  body      AddressInput  true  "Address details"
// @Success      200    {object}  Address
// @Failure      400    {object}  server.ErrorResponse
// @Failure      401    {object}  server.ErrorResponse
// @Router       /customers/addresses/{id} [put]
func (s *Service) updateAddress(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r)
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid address id")
		return
	}

	var input AddressInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	addr, err := s.repo.UpdateAddress(r.Context(), user.ID, id, input)
	if err != nil {
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
func (s *Service) deleteAddress(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r)
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid address id")
		return
	}

	if err := s.repo.DeleteAddress(r.Context(), user.ID, id); err != nil {
		if errors.Is(err, errNotFound) {
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
func (s *Service) setDefaultAddress(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r)
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid address id")
		return
	}

	if err := s.repo.SetDefaultAddress(r.Context(), user.ID, id); err != nil {
		if errors.Is(err, errNotFound) {
			server.WriteError(w, http.StatusNotFound, "address not found")
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to set default address")
		return
	}
	server.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
