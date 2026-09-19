// Package authorize reads what an application is asking for before anybody is
// asked about it.
//
// What it settles is the part a person cannot: that the application is one we
// know, that the address it wants to be answered at is one it registered, and
// that it brought a proof-key. Only then is there something worth putting to
// somebody, and the request is carried on to the page that asks them.
package authorize

import (
	"context"
	"errors"
	"strings"

	"github.com/khanzadimahdi/testproject/application/oauth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/oauth/client"
	"github.com/khanzadimahdi/testproject/domain/oauth/grant"
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
	if len(request.ClientID) == 0 {
		return nil, oauth.NewError(oauth.ErrorInvalidRequest, "client_id is required")
	}

	c, err := uc.clientRepository.GetOne(ctx, request.ClientID)
	if errors.Is(err, domain.ErrNotExists) {
		return nil, oauth.NewError(oauth.ErrorInvalidClient, "this application is not registered here")
	} else if err != nil {
		return nil, err
	}

	// where the application is answered is settled before anything else is,
	// because everything after this is answered there.
	redirectURI := request.RedirectURI
	if len(redirectURI) == 0 && len(c.RedirectURIs) == 1 {
		redirectURI = c.RedirectURIs[0]
	}

	if len(redirectURI) == 0 {
		return nil, oauth.NewError(oauth.ErrorInvalidRequest, "redirect_uri is required")
	}

	if !c.AllowsRedirectURI(redirectURI) {
		return nil, oauth.NewError(oauth.ErrorInvalidRedirectURI, "this redirect_uri is not one this application registered")
	}

	refuse := func(code string, description string) (*Response, error) {
		return &Response{
			RedirectURI: redirectURI,
			State:       request.State,
			Error:       oauth.NewError(code, description),
		}, nil
	}

	if request.ResponseType != client.ResponseTypeCode {
		return refuse(oauth.ErrorUnsupportedResponseType, "only the code response type is supported")
	}

	if !c.AllowsGrant(client.GrantAuthorizationCode) {
		return refuse(oauth.ErrorUnauthorizedClient, "this application did not register for the authorization_code grant")
	}

	if len(request.CodeChallenge) == 0 {
		return refuse(oauth.ErrorInvalidRequest, "code_challenge is required")
	}

	if request.CodeChallengeMethod != grant.ChallengeMethodS256 {
		return refuse(oauth.ErrorInvalidRequest, "code_challenge_method has to be "+grant.ChallengeMethodS256)
	}

	for asked := range strings.FieldsSeq(request.Scope) {
		if asked != oauth.Scope {
			return refuse(oauth.ErrorInvalidScope, "the only scope this server offers is "+oauth.Scope)
		}
	}

	requestToken, err := uc.requests.Generate(ctx, oauth.AuthorizationRequest{
		ClientID:            c.ID,
		ClientName:          c.Name,
		ClientURI:           c.URI,
		RedirectURI:         redirectURI,
		State:               request.State,
		Scope:               oauth.Scope,
		CodeChallenge:       request.CodeChallenge,
		CodeChallengeMethod: request.CodeChallengeMethod,
		Resource:            request.Resource,
	})
	if err != nil {
		return nil, err
	}

	return &Response{
		RequestToken: requestToken,
		RedirectURI:  redirectURI,
		State:        request.State,
	}, nil
}
