package middleware

import (
	"context"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
)

// Authorize refuses a request from somebody who does not hold the permission
// its subject is served under.
type Authorize struct {
	next       domain.MessageHandler
	authorizer domain.Authorizer
	permission string
	replyer    domain.Replyer
}

var _ domain.MessageHandler = &Authorize{}

func NewAuthorizeMiddleware(
	next domain.MessageHandler,
	authorizer domain.Authorizer,
	permission string,
	replyer domain.Replyer,
) *Authorize {
	return &Authorize{
		next:       next,
		authorizer: authorizer,
		permission: permission,
		replyer:    replyer,
	}
}

func (a *Authorize) Handle(ctx context.Context, data []byte) error {
	var r request
	if err := json.Unmarshal(data, &r); err != nil {
		return nil
	}

	ok, err := a.authorizer.Authorize(ctx, auth.UUIDFromContext(ctx), a.permission)
	if err != nil {
		return err
	}

	if !ok {
		return refuse(ctx, a.replyer, r.ID, "forbidden")
	}

	return a.next.Handle(ctx, data)
}
