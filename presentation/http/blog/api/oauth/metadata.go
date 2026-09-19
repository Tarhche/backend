package oauth

import (
	"net/http"
	"strings"

	"github.com/khanzadimahdi/testproject/application/oauth"
	"github.com/khanzadimahdi/testproject/domain/oauth/client"
	"github.com/khanzadimahdi/testproject/domain/oauth/grant"
)

// ConsentPath is where the frontend puts an application's request to the
// person it is asking. The authorization endpoint settles what can be settled
// without anybody and sends the browser there; a page is the one part of this
// that is not ours.
const ConsentPath = "/auth/authorize"

// the paths the endpoints this describes are served at.
const (
	AuthorizePath    = "/oauth/authorize"
	TokenPath        = "/oauth/token"
	RegisterPath     = "/oauth/register"
	ResourcePath     = "/mcp"
	ProtectedPath    = "/.well-known/oauth-protected-resource"
	AuthServerPath   = "/.well-known/oauth-authorization-server"
	protectedForMCP  = ProtectedPath + ResourcePath
	authServerForMCP = AuthServerPath + ResourcePath
)

// ProtectedResourceMetadataURL is where an application is told to look when it
// is refused for want of a token, as [RFC 9728] describes.
//
// [RFC 9728]: https://www.rfc-editor.org/rfc/rfc9728
func ProtectedResourceMetadataURL(serviceURL string) string {
	return strings.TrimSuffix(serviceURL, "/") + protectedForMCP
}

type protectedResourceMetadata struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	ScopesSupported        []string `json:"scopes_supported"`
	BearerMethodsSupported []string `json:"bearer_methods_supported"`
	ResourceDocumentation  string   `json:"resource_documentation,omitempty"`
}

type protectedResourceHandler struct {
	metadata protectedResourceMetadata
}

// NewProtectedResourceHandler describes what sits behind the MCP endpoint and
// who issues the tokens it takes.
func NewProtectedResourceHandler(serviceURL string) *protectedResourceHandler {
	base := strings.TrimSuffix(serviceURL, "/")

	return &protectedResourceHandler{
		metadata: protectedResourceMetadata{
			Resource:               base + ResourcePath,
			AuthorizationServers:   []string{base},
			ScopesSupported:        []string{oauth.Scope},
			BearerMethodsSupported: []string{"header"},
			ResourceDocumentation:  base + "/openapi/",
		},
	}
}

// @Summary		OAuth protected resource metadata
// @Description	says which authorization server issues the tokens the MCP endpoint takes
// @Tags		oauth
// @Produce		json
// @Success		200	{object}	map[string]interface{}
// @Router		/.well-known/oauth-protected-resource [get]
func (h *protectedResourceHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	write(rw, http.StatusOK, h.metadata)
}

type authorizationServerMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint"`
	ScopesSupported                   []string `json:"scopes_supported"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
}

type authorizationServerHandler struct {
	metadata authorizationServerMetadata
}

// NewAuthorizationServerHandler describes this estate as an authorization
// server, as [RFC 8414] asks.
//
// [RFC 8414]: https://www.rfc-editor.org/rfc/rfc8414
func NewAuthorizationServerHandler(serviceURL string) *authorizationServerHandler {
	base := strings.TrimSuffix(serviceURL, "/")

	return &authorizationServerHandler{
		metadata: authorizationServerMetadata{
			Issuer:                 base,
			AuthorizationEndpoint:  base + AuthorizePath,
			TokenEndpoint:          base + TokenPath,
			RegistrationEndpoint:   base + RegisterPath,
			ScopesSupported:        []string{oauth.Scope},
			ResponseTypesSupported: []string{client.ResponseTypeCode},
			GrantTypesSupported:    []string{client.GrantAuthorizationCode, client.GrantRefreshToken},
			TokenEndpointAuthMethodsSupported: []string{
				client.AuthMethodNone,
				client.AuthMethodSecretPost,
				client.AuthMethodSecretBasic,
			},
			CodeChallengeMethodsSupported: []string{grant.ChallengeMethodS256},
		},
	}
}

// @Summary		OAuth authorization server metadata
// @Description	where to register, where to send somebody to approve an application, and where to collect a session
// @Tags		oauth
// @Produce		json
// @Success		200	{object}	map[string]interface{}
// @Router		/.well-known/oauth-authorization-server [get]
func (h *authorizationServerHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	write(rw, http.StatusOK, h.metadata)
}
