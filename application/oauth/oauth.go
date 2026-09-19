// Package oauth is this estate acting as an authorization server: the part of
// it that hands an application a session of somebody's, once that somebody has
// said it may have one.
//
// It is the other side of signing in with an account somebody else keeps.
// There, we are the application and Google or GitHub says who somebody is;
// here, an application — an MCP client — asks us, and what it is given is an
// ordinary session of ours. The tokens are the same tokens the dashboard
// carries, so an application acts with the permissions of whoever approved it
// and with no others, and everything that refuses a request goes on refusing
// it in exactly the same way.
package oauth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/oauth/client"
	"github.com/khanzadimahdi/testproject/domain/password"
	infraJWT "github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

const (
	// Scope is the only scope there is: an application acts as the person who
	// approved it, with the permissions they hold at the moment it asks for
	// something. There is nothing narrower to offer, because what may be done
	// is decided per request against the roles as they are then.
	Scope = "mcp"

	// AuthorizationRequestToken is the audience of the token that carries an
	// authorization request from the authorization endpoint to the page that
	// asks the person about it, and back again. Nothing is stored for a
	// request nobody has answered yet: the token is the request.
	AuthorizationRequestToken = "oauth-authorization-request"

	// AuthorizationRequestLifetime is how long somebody has to make up their
	// mind, including signing in first.
	AuthorizationRequestLifetime = 15 * time.Minute
)

// the failures OAuth has names for. They travel to the application that asked,
// which is software rather than a person, so they are not translated.
const (
	ErrorInvalidRequest          = "invalid_request"
	ErrorInvalidClient           = "invalid_client"
	ErrorInvalidGrant            = "invalid_grant"
	ErrorUnauthorizedClient      = "unauthorized_client"
	ErrorUnsupportedGrantType    = "unsupported_grant_type"
	ErrorUnsupportedResponseType = "unsupported_response_type"
	ErrorInvalidScope            = "invalid_scope"
	ErrorAccessDenied            = "access_denied"
	ErrorServerError             = "server_error"
	ErrorInvalidClientMetadata   = "invalid_client_metadata"
	ErrorInvalidRedirectURI      = "invalid_redirect_uri"
)

// Error is a failure the protocol has a name for, which is what the
// application that asked is told rather than what went wrong inside.
type Error struct {
	Code        string
	Description string
}

var _ error = &Error{}

func NewError(code string, description string) *Error {
	return &Error{
		Code:        code,
		Description: description,
	}
}

func (e *Error) Error() string {
	return e.Code + ": " + e.Description
}

// AsError reports whether err is one of the protocol's own failures, so a
// handler can answer with it rather than with a 500.
func AsError(err error) (*Error, bool) {
	var oauthError *Error
	ok := errors.As(err, &oauthError)

	return oauthError, ok
}

// AuthorizationRequest is what an application asked for, as it stands until
// somebody answers it.
type AuthorizationRequest struct {
	ClientID   string
	ClientName string
	ClientURI  string

	RedirectURI string
	State       string
	Scope       string

	CodeChallenge       string
	CodeChallengeMethod string

	// Resource is what the application named as the thing it wants to reach,
	// and is carried so the token endpoint answers the same request the
	// authorization endpoint read.
	Resource string
}

// Requests carries an authorization request to the page that asks the person
// about it and back, signed, so the page cannot put words in the
// application's mouth and nothing about a request has to be kept anywhere
// until there is something to keep.
type Requests struct {
	jwt *infraJWT.JWT
}

func NewRequests(j *infraJWT.JWT) *Requests {
	return &Requests{jwt: j}
}

func (r *Requests) Generate(ctx context.Context, request AuthorizationRequest) (string, error) {
	b := infraJWT.NewClaimsBuilder()
	b.SetSubject(request.ClientID)
	b.SetAudience([]string{AuthorizationRequestToken})
	b.SetNotBefore(time.Now())
	b.SetIssuedAt(time.Now())
	b.SetExpirationTime(time.Now().Add(AuthorizationRequestLifetime))
	b.Set("client_name", request.ClientName)
	b.Set("client_uri", request.ClientURI)
	b.Set("redirect_uri", request.RedirectURI)
	b.Set("state", request.State)
	b.Set("scope", request.Scope)
	b.Set("code_challenge", request.CodeChallenge)
	b.Set("code_challenge_method", request.CodeChallengeMethod)
	b.Set("resource", request.Resource)

	return r.jwt.Generate(ctx, b.Build())
}

// Verify reads back a request this estate signed. A request that expired, was
// signed by nobody, or was issued for another purpose identifies nothing.
func (r *Requests) Verify(ctx context.Context, token string) (AuthorizationRequest, error) {
	claims, err := r.jwt.Verify(ctx, token)
	if err != nil {
		return AuthorizationRequest{}, NewError(ErrorInvalidRequest, "the authorization request is not valid any more")
	}

	if audiences, err := claims.GetAudience(); err != nil || len(audiences) == 0 || audiences[0] != AuthorizationRequestToken {
		return AuthorizationRequest{}, NewError(ErrorInvalidRequest, "the authorization request is not valid any more")
	}

	mapClaims, ok := claims.(jwt.MapClaims)
	if !ok {
		return AuthorizationRequest{}, NewError(ErrorInvalidRequest, "the authorization request is not valid any more")
	}

	clientID, err := claims.GetSubject()
	if err != nil {
		return AuthorizationRequest{}, NewError(ErrorInvalidRequest, "the authorization request is not valid any more")
	}

	text := func(name string) string {
		value, _ := mapClaims[name].(string)

		return value
	}

	return AuthorizationRequest{
		ClientID:            clientID,
		ClientName:          text("client_name"),
		ClientURI:           text("client_uri"),
		RedirectURI:         text("redirect_uri"),
		State:               text("state"),
		Scope:               text("scope"),
		CodeChallenge:       text("code_challenge"),
		CodeChallengeMethod: text("code_challenge_method"),
		Resource:            text("resource"),
	}, nil
}

// AnswersChallenge reports whether the verifier the application presented is
// the one the challenge was made from. It is what ties a code to the
// application that asked for it: whoever intercepts the code does not have the
// verifier, and the challenge cannot be worked back into one.
func AnswersChallenge(challenge string, verifier string) bool {
	if len(challenge) == 0 || len(verifier) == 0 {
		return false
	}

	sum := sha256.Sum256([]byte(verifier))
	expected := base64.RawURLEncoding.EncodeToString(sum[:])

	return subtle.ConstantTimeCompare([]byte(expected), []byte(challenge)) == 1
}

// Clients says which application is asking.
//
// A public client says who it is and is taken at its word: the proof-key is
// what makes that safe, and a secret shipped inside an application that runs
// on somebody's own machine would not be one. A confidential client has a
// secret, and shows it.
type Clients struct {
	repository client.Repository
	hasher     password.Hasher
}

func NewClients(repository client.Repository, hasher password.Hasher) *Clients {
	return &Clients{
		repository: repository,
		hasher:     hasher,
	}
}

func (c *Clients) Authenticate(ctx context.Context, id string, secret string) (client.Client, error) {
	if len(id) == 0 {
		return client.Client{}, NewError(ErrorInvalidClient, "client_id is required")
	}

	registered, err := c.repository.GetOne(ctx, id)
	if errors.Is(err, domain.ErrNotExists) {
		return client.Client{}, NewError(ErrorInvalidClient, "this application is not registered here")
	} else if err != nil {
		return client.Client{}, err
	}

	if registered.IsPublic() {
		return registered, nil
	}

	if len(secret) == 0 || !c.hasher.Equal(ctx, []byte(secret), registered.Secret.Value, registered.Secret.Salt) {
		return client.Client{}, NewError(ErrorInvalidClient, "the client secret is not valid")
	}

	return registered, nil
}
