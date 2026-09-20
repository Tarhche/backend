// Package approveauthorization records somebody's answer to an application
// that asked for a session of theirs.
//
// An approval is written down as a grant: a code the application collects
// once, in the moment between the browser leaving here and arriving there.
// Nothing is issued yet — the session itself is handed over at the token
// endpoint, to whoever can show the code and the verifier the challenge was
// made from.
package approveauthorization

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/url"
	"time"

	"github.com/khanzadimahdi/testproject/application/oauth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/oauth/client"
	"github.com/khanzadimahdi/testproject/domain/oauth/grant"
	"github.com/khanzadimahdi/testproject/domain/password"
)

const (
	secretLength = 32
	saltLength   = 16
)

type UseCase struct {
	clientRepository client.Repository
	grantRepository  grant.Repository
	requests         *oauth.Requests
	hasher           password.Hasher
}

func NewUseCase(
	clientRepository client.Repository,
	grantRepository grant.Repository,
	requests *oauth.Requests,
	hasher password.Hasher,
) *UseCase {
	return &UseCase{
		clientRepository: clientRepository,
		grantRepository:  grantRepository,
		requests:         requests,
		hasher:           hasher,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	authorizationRequest, err := uc.requests.Verify(ctx, request.RequestToken)
	if err != nil {
		return nil, err
	}

	if len(request.UserUUID) == 0 {
		return nil, oauth.NewError(oauth.ErrorAccessDenied, "nobody is signed in to answer this")
	}

	// a shadow session is somebody else's to look through, not to give away:
	// an application approved from one would go on acting as them long after
	// whoever opened it has gone.
	if len(request.ImpersonatorUUID) > 0 {
		return nil, oauth.NewError(oauth.ErrorAccessDenied, "an application cannot be approved while you are seeing the dashboard as somebody else")
	}

	c, err := uc.clientRepository.GetOne(ctx, authorizationRequest.ClientID)
	if errors.Is(err, domain.ErrNotExists) {
		return nil, oauth.NewError(oauth.ErrorInvalidClient, "this application is not registered here")
	} else if err != nil {
		return nil, err
	}

	// where it registered is asked again here: what was true when the request
	// was read has to still be true when it is answered.
	if !c.AllowsRedirectURI(authorizationRequest.RedirectURI) {
		return nil, oauth.NewError(oauth.ErrorInvalidRedirectURI, "this redirect_uri is not one this application registered")
	}

	if !request.Approved {
		return &Response{
			RedirectTo: redirect(authorizationRequest.RedirectURI, url.Values{
				"error":             {oauth.ErrorAccessDenied},
				"error_description": {"the request was refused"},
			}, authorizationRequest.State),
		}, nil
	}

	// a registration is thrown away until somebody approves it, and this is
	// somebody approving it. It is kept before the code is written, so nothing
	// is handed out that leans on a client which may be gone by the time it is
	// collected.
	if err := uc.clientRepository.Keep(ctx, c.ID); err != nil {
		return nil, err
	}

	code, err := uc.grant(ctx, c.ID, request.UserUUID, authorizationRequest)
	if err != nil {
		return nil, err
	}

	return &Response{
		RedirectTo: redirect(authorizationRequest.RedirectURI, url.Values{"code": {code}}, authorizationRequest.State),
	}, nil
}

// grant writes down what was approved and returns the code that collects it.
// The code is the grant's id and a secret of its own: the id says which grant
// to look at, and only a hash of the secret is kept, so what is stored is not
// enough to collect anything.
func (uc *UseCase) grant(ctx context.Context, clientID string, userUUID string, request oauth.AuthorizationRequest) (string, error) {
	secret, err := randomString(secretLength)
	if err != nil {
		return "", err
	}

	salt, err := randomBytes(saltLength)
	if err != nil {
		return "", err
	}

	g := grant.Grant{
		Secret: password.Hash{
			Value: uc.hasher.Hash(ctx, []byte(secret), salt),
			Salt:  salt,
		},
		ClientID:            clientID,
		UserUUID:            userUUID,
		RedirectURI:         request.RedirectURI,
		Scope:               request.Scope,
		CodeChallenge:       request.CodeChallenge,
		CodeChallengeMethod: request.CodeChallengeMethod,
		ExpiredAt:           time.Now().Add(grant.Lifetime),
		CreatedAt:           time.Now(),
	}

	id, err := uc.grantRepository.Save(ctx, &g)
	if err != nil {
		return "", err
	}

	return id + "." + secret, nil
}

// redirect builds the address the browser is sent to, keeping whatever the
// application already had in its own url and putting the state back exactly as
// it was given.
func redirect(redirectURI string, values url.Values, state string) string {
	parsed, err := url.Parse(redirectURI)
	if err != nil {
		return redirectURI
	}

	query := parsed.Query()
	for name, value := range values {
		query.Set(name, value[0])
	}

	if len(state) > 0 {
		query.Set("state", state)
	}

	parsed.RawQuery = query.Encode()

	return parsed.String()
}

func randomString(length int) (string, error) {
	value, err := randomBytes(length)
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(value), nil
}

func randomBytes(length int) ([]byte, error) {
	value := make([]byte, length)
	if _, err := rand.Read(value); err != nil {
		return nil, err
	}

	return value, nil
}
