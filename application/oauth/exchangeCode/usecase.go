// Package exchangecode hands an application the session it was approved for.
//
// The code is worth nothing on its own: whoever presents it also has to answer
// the challenge the request was made with, be the application the grant names,
// and come back to the address the grant was given for. It is then destroyed,
// so it is worth nothing twice either.
package exchangecode

import (
	"context"
	"errors"
	"strings"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/oauth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/oauth/grant"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type UseCase struct {
	grantRepository    grant.Repository
	clients            *oauth.Clients
	userRepository     user.Repository
	hasher             password.Hasher
	authTokenGenerator *auth.AuthTokenGenerator
}

func NewUseCase(
	grantRepository grant.Repository,
	clients *oauth.Clients,
	userRepository user.Repository,
	hasher password.Hasher,
	authTokenGenerator *auth.AuthTokenGenerator,
) *UseCase {
	return &UseCase{
		grantRepository:    grantRepository,
		clients:            clients,
		userRepository:     userRepository,
		hasher:             hasher,
		authTokenGenerator: authTokenGenerator,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	id, secret, ok := strings.Cut(request.Code, ".")
	if !ok || len(id) == 0 || len(secret) == 0 {
		return nil, oauth.NewError(oauth.ErrorInvalidGrant, "the code is not valid")
	}

	// who is asking is settled before the code is touched, so a wrong secret
	// does not spend somebody's code.
	c, err := uc.clients.Authenticate(ctx, request.ClientID, request.ClientSecret)
	if err != nil {
		return nil, err
	}

	// reading the grant destroys it, whether or not the rest of this holds.
	g, err := uc.grantRepository.Consume(ctx, id)
	if errors.Is(err, domain.ErrNotExists) {
		return nil, oauth.NewError(oauth.ErrorInvalidGrant, "the code is not valid")
	} else if err != nil {
		return nil, err
	}

	if g.IsExpired() {
		return nil, oauth.NewError(oauth.ErrorInvalidGrant, "the code has expired")
	}

	if !uc.hasher.Equal(ctx, []byte(secret), g.Secret.Value, g.Secret.Salt) {
		return nil, oauth.NewError(oauth.ErrorInvalidGrant, "the code is not valid")
	}

	if g.ClientID != c.ID {
		return nil, oauth.NewError(oauth.ErrorInvalidGrant, "the code was not given to this application")
	}

	if len(request.RedirectURI) > 0 && request.RedirectURI != g.RedirectURI {
		return nil, oauth.NewError(oauth.ErrorInvalidGrant, "the redirect_uri is not the one the code was given for")
	}

	if !oauth.AnswersChallenge(g.CodeChallenge, request.CodeVerifier) {
		return nil, oauth.NewError(oauth.ErrorInvalidGrant, "the code_verifier does not answer the challenge this request was made with")
	}

	u, err := uc.userRepository.GetOne(ctx, g.UserUUID)
	if errors.Is(err, domain.ErrNotExists) {
		return nil, oauth.NewError(oauth.ErrorInvalidGrant, "the code is not valid")
	} else if err != nil {
		return nil, err
	}

	if u.IsBanned() {
		return nil, oauth.NewError(oauth.ErrorInvalidGrant, "the code is not valid")
	}

	accessToken, err := uc.authTokenGenerator.GenerateAccessToken(ctx, &u)
	if err != nil {
		return nil, err
	}

	refreshToken, err := uc.authTokenGenerator.GenerateRefreshToken(ctx, u.UUID)
	if err != nil {
		return nil, err
	}

	return &Response{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(auth.AccessTokenExpirationTime.Seconds()),
		RefreshToken: refreshToken,
		Scope:        g.Scope,
	}, nil
}
