// Package refreshsession renews the session an application holds.
//
// What a refresh means is decided in one place — a ban handed down or a
// permission taken away ends a session at its next refresh, and a shadow
// session is asked again whether it may still be one — so this asks that one
// place rather than answering it again. What it adds is the part that is
// OAuth's: which application is asking, and the shape of the answer.
package refreshsession

import (
	"context"
	"strings"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/auth/refresh"
	"github.com/khanzadimahdi/testproject/application/oauth"
	"github.com/khanzadimahdi/testproject/domain/oauth/client"
)

type UseCase struct {
	clients *oauth.Clients
	refresh *refresh.UseCase
}

func NewUseCase(clients *oauth.Clients, refresh *refresh.UseCase) *UseCase {
	return &UseCase{
		clients: clients,
		refresh: refresh,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	c, err := uc.clients.Authenticate(ctx, request.ClientID, request.ClientSecret)
	if err != nil {
		return nil, err
	}

	if !c.AllowsGrant(client.GrantRefreshToken) {
		return nil, oauth.NewError(oauth.ErrorUnauthorizedClient, "this application did not register for the refresh_token grant")
	}

	for asked := range strings.FieldsSeq(request.Scope) {
		if asked != oauth.Scope {
			return nil, oauth.NewError(oauth.ErrorInvalidScope, "the only scope this server offers is "+oauth.Scope)
		}
	}

	if len(request.RefreshToken) == 0 {
		return nil, oauth.NewError(oauth.ErrorInvalidRequest, "refresh_token is required")
	}

	response, err := uc.refresh.Execute(ctx, &refresh.Request{Token: request.RefreshToken})
	if err != nil {
		return nil, err
	}

	// what the session refused to be renewed for is the application's answer
	// too, under the name OAuth gives it.
	if len(response.ValidationErrors) > 0 {
		return nil, oauth.NewError(oauth.ErrorInvalidGrant, "the refresh token is not valid any more")
	}

	return &Response{
		AccessToken:  response.AccessToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(auth.AccessTokenExpirationTime.Seconds()),
		RefreshToken: response.RefreshToken,
		Scope:        oauth.Scope,
	}, nil
}
