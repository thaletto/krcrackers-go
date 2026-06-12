package customers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/thaletto/krcrackers-go/server"
	"github.com/thaletto/krcrackers-go/services/auth"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /customers/profile", auth.WithAuth(http.HandlerFunc(s.getProfile)).ServeHTTP)
	mux.HandleFunc("PUT /customers/profile", auth.WithAuth(http.HandlerFunc(s.updateProfile)).ServeHTTP)
	mux.HandleFunc("GET /customers/addresses", auth.WithAuth(http.HandlerFunc(s.listAddresses)).ServeHTTP)
	mux.HandleFunc("POST /customers/addresses", auth.WithAuth(http.HandlerFunc(s.createAddress)).ServeHTTP)
	mux.HandleFunc("PUT /customers/addresses/{id}", auth.WithAuth(http.HandlerFunc(s.updateAddress)).ServeHTTP)
	mux.HandleFunc("DELETE /customers/addresses/{id}", auth.WithAuth(http.HandlerFunc(s.deleteAddress)).ServeHTTP)
	mux.HandleFunc("PUT /customers/addresses/{id}/default", auth.WithAuth(http.HandlerFunc(s.setDefaultAddress)).ServeHTTP)
}

func (s *Service) getProfile(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r)
	profile, err := s.repo.GetProfile(r.Context(), user.ID)
	if err != nil || profile.ID == 0 {
		server.WriteError(w, http.StatusNotFound, "profile not found")
		return
	}
	server.WriteJSON(w, http.StatusOK, profile)
}

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

func (s *Service) listAddresses(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r)
	addresses, err := s.repo.ListAddresses(r.Context(), user.ID)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to list addresses")
		return
	}
	server.WriteJSON(w, http.StatusOK, map[string]any{"items": addresses, "total": len(addresses)})
}

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
