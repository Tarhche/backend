package auth

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/user"
)

const (
	AccessToken        = "access"
	RefreshToken       = "refresh"
	ResetPasswordToken = "reset-password"
	RegistrationToken  = "registration"
)

type authKey struct{}

// AuthKey is the request context key under which URL params are stored.
var AuthKey = authKey{}

func FromContext(ctx context.Context) *user.User {
	u, _ := ctx.Value(AuthKey).(*user.User)

	return u
}

func ToContext(ctx context.Context, user *user.User) context.Context {
	return context.WithValue(ctx, AuthKey, user)
}
