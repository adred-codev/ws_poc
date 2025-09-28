package auth

import (
	"context"
)

type contextKey string

const userContextKey contextKey = "user"

// SetUserContext adds user claims to the context
func SetUserContext(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, userContextKey, claims)
}

// GetUserFromContext retrieves user claims from the context
func GetUserFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(userContextKey).(*Claims)
	return claims, ok
}