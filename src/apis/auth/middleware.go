// Package auth provides HTTP middleware for authenticating and authorizing
// requests via JWT cookies or Authorization headers. The middleware injects
// the authenticated user into the request context under the context key
// defined in the services/auth package.
package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/thaletto/krcrackers-go/src/server"
	"github.com/thaletto/krcrackers-go/src/services/auth"
)

// WithAuth returns middleware that validates the JWT access token from
// cookies or Authorization header and injects the user into context.
func WithAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r)
		if token == "" {
			server.WriteError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		claims, err := auth.ValidateJWT(token)
		if err != nil {
			server.WriteError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		ctx := context.WithValue(r.Context(), auth.UserContextKey, &auth.ContextUser{
			ID:   claims.UserID,
			Role: claims.Role,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// WithAdmin returns middleware that checks the authenticated user has the
// "admin" role. Must be chained after WithAuth.
func WithAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := r.Context().Value(auth.UserContextKey).(*auth.ContextUser)
		if !ok || user.Role != "admin" {
			server.WriteError(w, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(w, r.WithContext(r.Context()))
	})
}

// WithOptionalAuth returns middleware that optionally authenticates the
// request. If a valid token is present, the user is injected into context;
// otherwise the request proceeds unauthenticated.
func WithOptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r)
		if token != "" {
			if claims, err := auth.ValidateJWT(token); err == nil {
				ctx := context.WithValue(r.Context(), auth.UserContextKey, &auth.ContextUser{
					ID:   claims.UserID,
					Role: claims.Role,
				})
				r = r.WithContext(ctx)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func extractToken(r *http.Request) string {
	if c, err := r.Cookie("access_token"); err == nil {
		return c.Value
	}
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	return ""
}
