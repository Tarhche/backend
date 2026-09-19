// Package describeauthorization says what an application is asking for, so the
// person being asked is told by us rather than by whoever sent them to the
// page.
package describeauthorization

import (
	"context"
	"errors"
	"strings"

	"github.com/khanzadimahdi/testproject/application/oauth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/oauth/client"
)

type UseCase struct {
	clientRepository client.Repository
	requests         *oauth.Requests
}

func NewUseCase(clientRepository client.Repository, requests *oauth.Requests) *UseCase {
	return &UseCase{
		clientRepository: clientRepository,
		requests:         requests,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	authorizationRequest, err := uc.requests.Verify(ctx, request.RequestToken)
	if err != nil {
		return nil, err
	}

	// the name shown is the one the application is registered under now, not
	// the one carried in the request, so a client that was removed is not
	// still being vouched for.
	c, err := uc.clientRepository.GetOne(ctx, authorizationRequest.ClientID)
	if errors.Is(err, domain.ErrNotExists) {
		return nil, oauth.NewError(oauth.ErrorInvalidClient, "this application is not registered here")
	} else if err != nil {
		return nil, err
	}

	return &Response{
		ClientID:    c.ID,
		ClientName:  c.Name,
		ClientURI:   c.URI,
		RedirectURI: authorizationRequest.RedirectURI,
		Scopes:      strings.Fields(authorizationRequest.Scope),
	}, nil
}
