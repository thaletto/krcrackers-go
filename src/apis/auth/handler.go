// Package auth provides the HTTP handlers for authentication: register,
// login, Google login, refresh, logout, and current-user lookup. Handlers
// are thin: they decode the request, call into services/auth, and encode
// the response. Cookie attributes (HttpOnly, Secure, SameSite, MaxAge) are
// the only HTTP-specific policy the handler owns.
package auth

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/thaletto/krcrackers-go/src/server"
	svc "github.com/thaletto/krcrackers-go/src/services/auth"
)

// Handler binds the auth HTTP routes to a services/auth.Service.
type Handler struct {
	svc      *svc.Service
	isSecure bool
}

// NewHandler creates a new auth HTTP handler. isSecure controls the
// Secure flag on the access_token and refresh_token cookies; pass true
// in production.
func NewHandler(service *svc.Service, isSecure bool) *Handler {
	return &Handler{svc: service, isSecure: isSecure}
}

// RegisterRoutes wires all authentication endpoints on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/register", h.register)
	mux.HandleFunc("POST /auth/login", h.login)
	mux.HandleFunc("POST /auth/google", h.googleLogin)
	mux.HandleFunc("POST /auth/refresh", h.refresh)
	mux.HandleFunc("POST /auth/logout", h.logout)
	mux.HandleFunc("GET /auth/me", WithAuth(http.HandlerFunc(h.me)).ServeHTTP)
}

// Register godoc
// @Summary      Register a new user
// @Description  Create a new user account with email and password
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        input  body      object{email:string, password:string, name:string, phone:string}  true  "Registration details"
// @Success      201    {object}  authResponse
// @Failure      400    {object}  server.ErrorResponse
// @Failure      409    {object}  server.ErrorResponse
// @Failure      422    {object}  server.ErrorResponse
// @Router       /auth/register [post]
func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"name"`
		Phone    string `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if input.Email == "" || input.Password == "" {
		server.WriteError(w, http.StatusUnprocessableEntity, "email and password are required")
		return
	}

	res, err := h.svc.Register(r.Context(), input.Email, input.Password, input.Name, input.Phone)
	if err != nil {
		if errors.Is(err, svc.ErrEmailExists) {
			server.WriteError(w, http.StatusConflict, err.Error())
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to register")
		return
	}

	h.setTokenCookies(w, res.AccessToken, res.RefreshToken)
	server.WriteJSON(w, http.StatusCreated, authResponse{User: res.User})
}

// Login godoc
// @Summary      Log in with email and password
// @Description  Authenticate with email and password, sets JWT cookies
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        input  body      object{email:string, password:string}  true  "Login credentials"
// @Success      200    {object}  authResponse
// @Failure      400    {object}  server.ErrorResponse
// @Failure      401    {object}  server.ErrorResponse
// @Failure      422    {object}  server.ErrorResponse
// @Router       /auth/login [post]
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if input.Email == "" || input.Password == "" {
		server.WriteError(w, http.StatusUnprocessableEntity, "email and password are required")
		return
	}

	res, err := h.svc.Login(r.Context(), input.Email, input.Password)
	if err != nil {
		if errors.Is(err, svc.ErrInvalidCredentials) {
			server.WriteError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to login")
		return
	}

	h.setTokenCookies(w, res.AccessToken, res.RefreshToken)
	server.WriteJSON(w, http.StatusOK, authResponse{User: res.User})
}

// GoogleLogin godoc
// @Summary      Log in with Google
// @Description  Authenticate using a Google ID token, auto-creates account if new
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        input  body      object{idToken:string}  true  "Google ID token"
// @Success      200    {object}  authResponse
// @Failure      400    {object}  server.ErrorResponse
// @Failure      401    {object}  server.ErrorResponse
// @Failure      409    {object}  server.ErrorResponse
// @Failure      422    {object}  server.ErrorResponse
// @Failure      503    {object}  server.ErrorResponse
// @Router       /auth/google [post]
func (h *Handler) googleLogin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		IDToken string `json:"idToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if input.IDToken == "" {
		server.WriteError(w, http.StatusUnprocessableEntity, "idToken is required")
		return
	}

	res, err := h.svc.LoginWithGoogle(r.Context(), input.IDToken)
	if err != nil {
		if errors.Is(err, svc.ErrGoogleLoginUnavailable) {
			server.WriteError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		if errors.Is(err, svc.ErrGoogleAccountLinkRequired) {
			server.WriteError(w, http.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, svc.ErrInvalidGoogleToken) {
			server.WriteError(w, http.StatusUnauthorized, err.Error())
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to login with google")
		return
	}

	h.setTokenCookies(w, res.AccessToken, res.RefreshToken)
	server.WriteJSON(w, http.StatusOK, authResponse{User: res.User})
}

// Refresh godoc
// @Summary      Refresh access token
// @Description  Rotate refresh token and issue new access token
// @Tags         auth
// @Produce      json
// @Success      200    {object}  authResponse
// @Failure      401    {object}  server.ErrorResponse
// @Router       /auth/refresh [post]
func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("refresh_token")
	if err != nil || cookie.Value == "" {
		server.WriteError(w, http.StatusUnauthorized, "no refresh token")
		return
	}

	res, err := h.svc.Refresh(r.Context(), cookie.Value)
	if err != nil {
		switch {
		case errors.Is(err, svc.ErrNoRefreshToken),
			errors.Is(err, svc.ErrInvalidRefreshToken),
			errors.Is(err, svc.ErrRefreshExpired),
			errors.Is(err, svc.ErrUserNotFound):
			server.WriteError(w, http.StatusUnauthorized, err.Error())
		default:
			server.WriteError(w, http.StatusInternalServerError, "failed to refresh")
		}
		return
	}

	h.setTokenCookies(w, res.AccessToken, res.RefreshToken)
	server.WriteJSON(w, http.StatusOK, authResponse{User: res.User})
}

// Logout godoc
// @Summary      Log out
// @Description  Revoke refresh token and clear cookies
// @Tags         auth
// @Success      204
// @Router       /auth/logout [post]
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("refresh_token"); err == nil {
		_ = h.svc.Logout(r.Context(), cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{Name: "access_token", Path: "/", MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: "refresh_token", Path: "/", MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

// Me godoc
// @Summary      Get current user
// @Description  Return the authenticated user's profile
// @Tags         auth
// @Produce      json
// @Security     cookieAuth
// @Success      200    {object}  User
// @Failure      401    {object}  server.ErrorResponse
// @Failure      404    {object}  server.ErrorResponse
// @Router       /auth/me [get]
func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	user := svc.GetUser(r)
	if user == nil {
		server.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	u, err := h.svc.GetMe(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, svc.ErrUserNotFound) {
			server.WriteError(w, http.StatusNotFound, "user not found")
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to get user")
		return
	}

	server.WriteJSON(w, http.StatusOK, u)
}

type authResponse struct {
	User any `json:"user"`
}

func (h *Handler) setTokenCookies(w http.ResponseWriter, accessToken, refreshToken string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    accessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.isSecure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   15 * 60,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.isSecure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   7 * 24 * 60 * 60,
	})
}
