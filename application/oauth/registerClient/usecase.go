// Package registerclient takes an application at its word about what it is and
// where it may be answered, and gives it a name to ask under.
//
// Anybody may register: that is what makes an MCP client able to connect
// without somebody being asked to issue it credentials first. Registering
// establishes nothing but where a session may be sent back to — a registered
// client holds no permission of its own, and is worth nothing until somebody
// approves it.
//
// Unlike the rest of the application's use cases, what is wrong with a request
// comes back as an error rather than in the response: these failures travel to
// software rather than to a person, and the protocol has its own names and its
// own shape for them.
package registerclient

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/application/oauth"
	"github.com/khanzadimahdi/testproject/domain/oauth/client"
	"github.com/khanzadimahdi/testproject/domain/password"
)

const (
	// maxRedirectURIs and maxNameLength keep one registration small. Anybody
	// may register, so nothing a registration carries is unbounded.
	maxRedirectURIs = 10
	maxNameLength   = 256

	secretLength = 32
	saltLength   = 16
)

type UseCase struct {
	clientRepository client.Repository
	hasher           password.Hasher
}

func NewUseCase(clientRepository client.Repository, hasher password.Hasher) *UseCase {
	return &UseCase{
		clientRepository: clientRepository,
		hasher:           hasher,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	registered, err := describe(request)
	if err != nil {
		return nil, err
	}

	var secret string
	if !registered.IsPublic() {
		secret, err = randomString(secretLength)
		if err != nil {
			return nil, err
		}

		salt, err := randomBytes(saltLength)
		if err != nil {
			return nil, err
		}

		registered.Secret = password.Hash{
			Value: uc.hasher.Hash(ctx, []byte(secret), salt),
			Salt:  salt,
		}
	}

	registered.CreatedAt = time.Now()
	registered.ExpiresAt = registered.CreatedAt.Add(client.UnapprovedLifetime)

	id, err := uc.clientRepository.Save(ctx, registered)
	if err != nil {
		return nil, err
	}

	response := &Response{
		ClientID:                id,
		ClientIDIssuedAt:        registered.CreatedAt.Unix(),
		ClientSecret:            secret,
		ClientName:              registered.Name,
		ClientURI:               registered.URI,
		RedirectURIs:            registered.RedirectURIs,
		GrantTypes:              registered.GrantTypes,
		ResponseTypes:           registered.ResponseTypes,
		TokenEndpointAuthMethod: registered.TokenEndpointAuthMethod,
		Scope:                   registered.Scope,
	}

	if len(secret) > 0 {
		var never int64
		response.ClientSecretExpiresAt = &never
	}

	return response, nil
}

// describe reads what the application said about itself, fills in what it left
// out, and refuses what cannot be honoured.
func describe(request *Request) (*client.Client, error) {
	if len(request.RedirectURIs) == 0 {
		return nil, oauth.NewError(oauth.ErrorInvalidRedirectURI, "at least one redirect_uri is required")
	}

	if len(request.RedirectURIs) > maxRedirectURIs {
		return nil, oauth.NewError(oauth.ErrorInvalidRedirectURI, "too many redirect_uris")
	}

	for _, uri := range request.RedirectURIs {
		if err := validateRedirectURI(uri); err != nil {
			return nil, err
		}
	}

	if len(request.ClientName) > maxNameLength {
		return nil, oauth.NewError(oauth.ErrorInvalidClientMetadata, "client_name is too long")
	}

	grantTypes := request.GrantTypes
	if len(grantTypes) == 0 {
		grantTypes = []string{client.GrantAuthorizationCode, client.GrantRefreshToken}
	}

	for _, grantType := range grantTypes {
		if grantType != client.GrantAuthorizationCode && grantType != client.GrantRefreshToken {
			return nil, oauth.NewError(oauth.ErrorInvalidClientMetadata, "only the authorization_code and refresh_token grants are supported")
		}
	}

	if !slices.Contains(grantTypes, client.GrantAuthorizationCode) {
		return nil, oauth.NewError(oauth.ErrorInvalidClientMetadata, "the authorization_code grant is required")
	}

	responseTypes := request.ResponseTypes
	if len(responseTypes) == 0 {
		responseTypes = []string{client.ResponseTypeCode}
	}

	for _, responseType := range responseTypes {
		if responseType != client.ResponseTypeCode {
			return nil, oauth.NewError(oauth.ErrorInvalidClientMetadata, "only the code response type is supported")
		}
	}

	authMethod := request.TokenEndpointAuthMethod
	if len(authMethod) == 0 {
		authMethod = client.AuthMethodNone
	}

	switch authMethod {
	case client.AuthMethodNone, client.AuthMethodSecretPost, client.AuthMethodSecretBasic:
	default:
		return nil, oauth.NewError(oauth.ErrorInvalidClientMetadata, "unsupported token_endpoint_auth_method")
	}

	scope := strings.TrimSpace(request.Scope)
	if len(scope) > 0 {
		for asked := range strings.FieldsSeq(scope) {
			if asked != oauth.Scope {
				return nil, oauth.NewError(oauth.ErrorInvalidClientMetadata, "the only scope this server offers is "+oauth.Scope)
			}
		}
	}

	return &client.Client{
		Name:                    request.ClientName,
		URI:                     request.ClientURI,
		RedirectURIs:            request.RedirectURIs,
		GrantTypes:              grantTypes,
		ResponseTypes:           responseTypes,
		TokenEndpointAuthMethod: authMethod,
		Scope:                   oauth.Scope,
	}, nil
}

// validateRedirectURI refuses an address a session must never be sent to.
//
// An application that runs on somebody's own machine is answered either on the
// loopback interface or on a scheme of its own, which is what [RFC 8252] asks
// for; everything else has to be reached over TLS. A fragment is refused
// outright, because the code is put there.
//
// [RFC 8252]: https://www.rfc-editor.org/rfc/rfc8252
func validateRedirectURI(uri string) error {
	parsed, err := url.Parse(uri)
	if err != nil || !parsed.IsAbs() {
		return oauth.NewError(oauth.ErrorInvalidRedirectURI, "redirect_uri has to be an absolute url")
	}

	if len(parsed.Fragment) > 0 || strings.Contains(uri, "#") {
		return oauth.NewError(oauth.ErrorInvalidRedirectURI, "redirect_uri may not carry a fragment")
	}

	switch parsed.Scheme {
	case "https":
		return nil
	case "http":
		if isLoopback(parsed.Hostname()) {
			return nil
		}

		return oauth.NewError(oauth.ErrorInvalidRedirectURI, "an http redirect_uri is only allowed on the loopback interface")
	default:
		// a scheme of the application's own, which the operating system hands
		// back to it. It has nowhere else to go by definition.
		return nil
	}
}

func isLoopback(host string) bool {
	switch host {
	case "127.0.0.1", "::1", "[::1]", "localhost":
		return true
	default:
		return false
	}
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
