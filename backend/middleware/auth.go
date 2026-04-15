package middleware

import "context"

type contextKey string

const userContextKey contextKey = "auth_user"

type AuthUser struct {
	ID       int64
	Username string
	IsAdmin  bool
	IsGuest  bool
}

func WithUser(ctx context.Context, user AuthUser) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

func UserFromContext(ctx context.Context) (AuthUser, bool) {
	user, ok := ctx.Value(userContextKey).(AuthUser)
	return user, ok
}
