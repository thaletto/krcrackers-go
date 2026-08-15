// Package auth provides authentication and authorization services including
// email/password registration, Google ID token validation, JWT-based access
// tokens, refresh token rotation, and HTTP middleware for protected routes.
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Domain sentinels returned by the auth Service. The HTTP layer maps these
// to status codes via errors.Is.
var (
	ErrEmailExists               = errors.New("email already registered")
	ErrInvalidCredentials        = errors.New("invalid credentials")
	ErrInvalidGoogleToken        = errors.New("invalid google id token")
	ErrGoogleLoginUnavailable    = errors.New("google login is temporarily unavailable")
	ErrGoogleAccountLinkRequired = errors.New("this email already has a password account; sign in with your password")
	ErrNoRefreshToken            = errors.New("no refresh token")
	ErrInvalidRefreshToken       = errors.New("invalid refresh token")
	ErrRefreshExpired            = errors.New("refresh token expired")
	ErrUserNotFound              = errors.New("user not found")
)

// AuthResult bundles a user with the access and refresh tokens issued for
// a successful authentication.
type AuthResult struct {
	User         User
	AccessToken  string
	RefreshToken string
}

// Service owns the authentication flow: registration, login, Google login,
// refresh token rotation, logout, and current-user lookup. It depends on a
// Repository for persistence and is otherwise free of HTTP concerns.
type Service struct {
	repo           Repository
	googleVerifier GoogleTokenVerifier
}

// GoogleTokenVerifier verifies a Google ID token and returns its trusted identity.
type GoogleTokenVerifier func(idToken string) (GoogleIdentity, error)

// NewService creates a new auth service. Panics if jwtSecret is empty; the
// secret is registered with the package's JWT helpers so the HTTP middleware
// in apis/auth can validate tokens.
func NewService(repo Repository, jwtSecret, googleClientID string) *Service {
	return NewServiceWithGoogleVerifier(repo, jwtSecret, func(idToken string) (GoogleIdentity, error) {
		return VerifyGoogleIDToken(idToken, googleClientID)
	})
}

// NewServiceWithGoogleVerifier creates an auth service with an injectable Google
// verifier for external tests.
func NewServiceWithGoogleVerifier(repo Repository, jwtSecret string, verifier GoogleTokenVerifier) *Service {
	if jwtSecret == "" {
		panic("auth: JWT_SECRET is required")
	}
	SetJWTSecret(jwtSecret)
	return &Service{repo: repo, googleVerifier: verifier}
}

// Register creates a new user with email and password, then issues tokens.
func (s *Service) Register(ctx context.Context, email, password, name, phone string) (AuthResult, error) {
	existing, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		return AuthResult{}, err
	}
	if existing.ID != 0 {
		return AuthResult{}, ErrEmailExists
	}

	hash, err := HashPassword(password)
	if err != nil {
		return AuthResult{}, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.repo.Create(ctx, email, name, phone, "email", "", hash, "customer")
	if err != nil {
		return AuthResult{}, fmt.Errorf("create user: %w", err)
	}

	return s.issueTokens(ctx, user)
}

// Login authenticates with email and password, then issues tokens.
func (s *Service) Login(ctx context.Context, email, password string) (AuthResult, error) {
	user, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		return AuthResult{}, ErrInvalidCredentials
	}
	if user.ID == 0 {
		return AuthResult{}, ErrInvalidCredentials
	}

	if !CheckPassword(password, user.PasswordHash) {
		return AuthResult{}, ErrInvalidCredentials
	}

	return s.issueTokens(ctx, user)
}

// LoginWithGoogle authenticates with a Google ID token, auto-creating the
// user if the email is new.
func (s *Service) LoginWithGoogle(ctx context.Context, idToken string) (AuthResult, error) {
	if s.googleVerifier == nil {
		return AuthResult{}, ErrGoogleLoginUnavailable
	}
	identity, err := s.googleVerifier(idToken)
	if err != nil {
		if errors.Is(err, ErrGoogleLoginUnavailable) {
			return AuthResult{}, err
		}
		return AuthResult{}, fmt.Errorf("%w: %s", ErrInvalidGoogleToken, err.Error())
	}

	user, err := s.repo.GetByProviderID(ctx, "google", identity.Subject)
	if err != nil {
		return AuthResult{}, fmt.Errorf("get user by Google subject: %w", err)
	}
	if user.ID != 0 {
		return s.issueTokens(ctx, user)
	}

	user, err = s.repo.GetByEmail(ctx, identity.Email)
	if err != nil {
		return AuthResult{}, fmt.Errorf("get user: %w", err)
	}
	if user.ID != 0 {
		return AuthResult{}, ErrGoogleAccountLinkRequired
	}

	user, err = s.repo.Create(ctx, identity.Email, identity.Name, "", "google", identity.Subject, "", "customer")
	if err != nil {
		return AuthResult{}, fmt.Errorf("create user: %w", err)
	}

	return s.issueTokens(ctx, user)
}

// Refresh rotates a refresh token: the old token is revoked, a new pair is
// issued, and the user is returned.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (AuthResult, error) {
	if refreshToken == "" {
		return AuthResult{}, ErrNoRefreshToken
	}

	userID, expiresAt, err := s.repo.GetRefreshToken(ctx, refreshToken)
	if err != nil {
		return AuthResult{}, err
	}
	if userID == 0 {
		return AuthResult{}, ErrInvalidRefreshToken
	}
	if time.Now().After(expiresAt) {
		_ = s.repo.DeleteRefreshToken(ctx, refreshToken)
		return AuthResult{}, ErrRefreshExpired
	}

	user, err := s.repo.GetByID(ctx, userID)
	if err != nil || user.ID == 0 {
		return AuthResult{}, ErrUserNotFound
	}

	if err := s.repo.DeleteRefreshToken(ctx, refreshToken); err != nil {
		return AuthResult{}, fmt.Errorf("revoke old refresh token: %w", err)
	}
	return s.issueTokens(ctx, user)
}

// Logout revokes a refresh token. A nil/empty token is a no-op.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}
	return s.repo.DeleteRefreshToken(ctx, refreshToken)
}

// GetMe returns the full user record for the authenticated user.
func (s *Service) GetMe(ctx context.Context, userID int) (User, error) {
	u, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return User{}, err
	}
	if u.ID == 0 {
		return User{}, ErrUserNotFound
	}
	return u, nil
}

func (s *Service) issueTokens(ctx context.Context, user User) (AuthResult, error) {
	access, err := GenerateAccessToken(user.ID, user.Role)
	if err != nil {
		return AuthResult{}, fmt.Errorf("generate access token: %w", err)
	}
	refresh, err := GenerateRefreshToken()
	if err != nil {
		return AuthResult{}, fmt.Errorf("generate refresh token: %w", err)
	}
	if err := s.repo.CreateRefreshToken(ctx, user.ID, refresh, time.Now().Add(7*24*time.Hour)); err != nil {
		return AuthResult{}, fmt.Errorf("store refresh token: %w", err)
	}
	return AuthResult{User: user, AccessToken: access, RefreshToken: refresh}, nil
}
