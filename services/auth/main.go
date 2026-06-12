// Package auth provides authentication and authorization services including
// email/password registration, Google ID token validation, JWT-based access
// tokens, refresh token rotation, and HTTP middleware for protected routes.
package auth

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/thaletto/krcrackers-go/server"
)

// Service handles authentication HTTP endpoints and token management.
type Service struct {
	repo     Repository
	isSecure bool
}

// NewService creates a new auth service. Panics if jwtSecret is empty.
func NewService(repo Repository, jwtSecret string, isSecure bool) *Service {
	if jwtSecret == "" {
		panic("auth: JWT_SECRET is required")
	}
	SetJWTSecret(jwtSecret)
	return &Service{repo: repo, isSecure: isSecure}
}

// RegisterRoutes registers all authentication endpoints on the given mux.
func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/register", s.register)
	mux.HandleFunc("POST /auth/login", s.login)
	mux.HandleFunc("POST /auth/google", s.googleLogin)
	mux.HandleFunc("POST /auth/refresh", s.refresh)
	mux.HandleFunc("POST /auth/logout", s.logout)
	mux.HandleFunc("GET /auth/me", WithAuth(http.HandlerFunc(s.me)).ServeHTTP)
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
func (s *Service) register(w http.ResponseWriter, r *http.Request) {
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

	existing, _ := s.repo.GetByEmail(r.Context(), input.Email)
	if existing.ID != 0 {
		server.WriteError(w, http.StatusConflict, "email already registered")
		return
	}

	hash, err := HashPassword(input.Password)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	user, err := s.repo.Create(r.Context(), input.Email, input.Name, input.Phone, "email", "", hash, "customer")
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	accessToken, err := GenerateAccessToken(user.ID, user.Role)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	refreshToken, err := GenerateRefreshToken()
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to generate refresh token")
		return
	}

	if err := s.repo.CreateRefreshToken(r.Context(), user.ID, refreshToken, time.Now().Add(7*24*time.Hour)); err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to store refresh token")
		return
	}

	s.setTokenCookies(w, accessToken, refreshToken)
	server.WriteJSON(w, http.StatusCreated, authResponse{User: user})
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
func (s *Service) login(w http.ResponseWriter, r *http.Request) {
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

	user, err := s.repo.GetByEmail(r.Context(), input.Email)
	if err != nil || user.ID == 0 {
		server.WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if !CheckPassword(input.Password, user.PasswordHash) {
		server.WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	accessToken, err := GenerateAccessToken(user.ID, user.Role)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	refreshToken, err := GenerateRefreshToken()
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to generate refresh token")
		return
	}

	if err := s.repo.CreateRefreshToken(r.Context(), user.ID, refreshToken, time.Now().Add(7*24*time.Hour)); err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to store refresh token")
		return
	}

	s.setTokenCookies(w, accessToken, refreshToken)
	server.WriteJSON(w, http.StatusOK, authResponse{User: user})
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
// @Failure      422    {object}  server.ErrorResponse
// @Router       /auth/google [post]
func (s *Service) googleLogin(w http.ResponseWriter, r *http.Request) {
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

	claims, err := VerifyGoogleIDToken(input.IDToken)
	if err != nil {
		server.WriteError(w, http.StatusUnauthorized, "invalid google id token: "+err.Error())
		return
	}

	user, err := s.repo.GetByEmail(r.Context(), claims.Email)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to get user")
		return
	}

	if user.ID == 0 {
		user, err = s.repo.Create(r.Context(), claims.Email, claims.Name, "", "google", claims.Sub, "", "customer")
		if err != nil {
			server.WriteError(w, http.StatusInternalServerError, "failed to create user")
			return
		}
	}

	accessToken, err := GenerateAccessToken(user.ID, user.Role)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	refreshToken, err := GenerateRefreshToken()
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to generate refresh token")
		return
	}

	if err := s.repo.CreateRefreshToken(r.Context(), user.ID, refreshToken, time.Now().Add(7*24*time.Hour)); err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to store refresh token")
		return
	}

	s.setTokenCookies(w, accessToken, refreshToken)
	server.WriteJSON(w, http.StatusOK, authResponse{User: user})
}

// Refresh godoc
// @Summary      Refresh access token
// @Description  Rotate refresh token and issue new access token
// @Tags         auth
// @Produce      json
// @Success      200    {object}  authResponse
// @Failure      401    {object}  server.ErrorResponse
// @Router       /auth/refresh [post]
func (s *Service) refresh(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := r.Cookie("refresh_token")
	if err != nil || refreshToken.Value == "" {
		server.WriteError(w, http.StatusUnauthorized, "no refresh token")
		return
	}

	userID, expiresAt, err := s.repo.GetRefreshToken(r.Context(), refreshToken.Value)
	if err != nil || userID == 0 {
		server.WriteError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	if time.Now().After(expiresAt) {
		s.repo.DeleteRefreshToken(r.Context(), refreshToken.Value)
		server.WriteError(w, http.StatusUnauthorized, "refresh token expired")
		return
	}

	user, err := s.repo.GetByID(r.Context(), userID)
	if err != nil || user.ID == 0 {
		server.WriteError(w, http.StatusUnauthorized, "user not found")
		return
	}

	newAccessToken, err := GenerateAccessToken(user.ID, user.Role)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	newRefreshToken, err := GenerateRefreshToken()
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to generate refresh token")
		return
	}

	if err := s.repo.DeleteRefreshToken(r.Context(), refreshToken.Value); err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to revoke old refresh token")
		return
	}
	if err := s.repo.CreateRefreshToken(r.Context(), user.ID, newRefreshToken, time.Now().Add(7*24*time.Hour)); err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to store new refresh token")
		return
	}

	s.setTokenCookies(w, newAccessToken, newRefreshToken)
	server.WriteJSON(w, http.StatusOK, authResponse{User: user})
}

// Logout godoc
// @Summary      Log out
// @Description  Revoke refresh token and clear cookies
// @Tags         auth
// @Success      204
// @Router       /auth/logout [post]
func (s *Service) logout(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := r.Cookie("refresh_token")
	if err == nil && refreshToken.Value != "" {
		s.repo.DeleteRefreshToken(r.Context(), refreshToken.Value)
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
func (s *Service) me(w http.ResponseWriter, r *http.Request) {
	user := GetUser(r)
	if user == nil {
		server.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	u, err := s.repo.GetByID(r.Context(), user.ID)
	if err != nil || u.ID == 0 {
		server.WriteError(w, http.StatusNotFound, "user not found")
		return
	}

	server.WriteJSON(w, http.StatusOK, u)
}

type authResponse struct {
	User interface{} `json:"user"`
}

func (s *Service) setTokenCookies(w http.ResponseWriter, accessToken, refreshToken string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    accessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.isSecure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   15 * 60,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.isSecure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   7 * 24 * 60 * 60,
	})
}
