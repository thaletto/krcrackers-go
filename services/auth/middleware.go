package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/thaletto/krcrackers-go/server"
)

type contextKey string

const UserContextKey contextKey = "user"

type ContextUser struct {
	ID   int
	Role string
}

func WithAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r)
		if token == "" {
			server.WriteError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		claims, err := ValidateJWT(token)
		if err != nil {
			server.WriteError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		ctx := context.WithValue(r.Context(), UserContextKey, &ContextUser{
			ID:   claims.UserID,
			Role: claims.Role,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func WithAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := r.Context().Value(UserContextKey).(*ContextUser)
		if !ok || user.Role != "admin" {
			server.WriteError(w, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(w, r.WithContext(r.Context()))
	})
}

func WithOptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r)
		if token != "" {
			if claims, err := ValidateJWT(token); err == nil {
				ctx := context.WithValue(r.Context(), UserContextKey, &ContextUser{
					ID:   claims.UserID,
					Role: claims.Role,
				})
				r = r.WithContext(ctx)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func GetUser(r *http.Request) *ContextUser {
	u, _ := r.Context().Value(UserContextKey).(*ContextUser)
	return u
}

func extractToken(r *http.Request) string {
	if c, err := r.Cookie("access_token"); err == nil {
		return c.Value
	}
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}
