package auth

import (
	"context"
	"net/http"
)

type contextKey string

// UserContextKey is the request-context key under which the authenticated
// user is stored. HTTP middleware in the apis/auth package injects a
// *ContextUser at this key after validating the JWT.
const UserContextKey contextKey = "user"

// ContextUser carries the minimal authenticated-user identity injected
// into the request context by the auth middleware.
type ContextUser struct {
	ID   int
	Role string
}

// GetUser extracts the authenticated user from the request context.
// Returns nil if the request is not authenticated.
func GetUser(r *http.Request) *ContextUser {
	u, _ := r.Context().Value(UserContextKey).(*ContextUser)
	return u
}

// contextWithUser is a small helper for tests and middleware that need to
// inject a user into a context. It is unexported because the only public
// way to authenticate a request is via the HTTP middleware.
func contextWithUser(ctx context.Context, u *ContextUser) context.Context {
	return context.WithValue(ctx, UserContextKey, u)
}
