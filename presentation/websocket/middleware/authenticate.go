package middleware

import (
	"context"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
)

// Authenticate establishes who a request is from, and refuses one whose token
// identifies nobody.
type Authenticate struct {
	next          domain.MessageHandler
	authenticator *auth.Authenticator
	replyer       domain.Replyer
}

var _ domain.MessageHandler = &Authenticate{}

func NewAuthenticateMiddleware(
	next domain.MessageHandler,
	authenticator *auth.Authenticator,
	replyer domain.Replyer,
) *Authenticate {
	return &Authenticate{
		next:          next,
		authenticator: authenticator,
		replyer:       replyer,
	}
}

func (a *Authenticate) Handle(ctx context.Context, data []byte) error {
	var r request
	if err := json.Unmarshal(data, &r); err != nil {
		return nil
	}

	identity, err := a.authenticator.Authenticate(ctx, r.AccessToken)
	if err != nil {
		return refuse(ctx, a.replyer, r.ID, "unauthenticated")
	}

	return a.next.Handle(auth.IdentityToContext(ctx, identity), data)
}
