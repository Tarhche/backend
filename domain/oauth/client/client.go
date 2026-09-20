// Package client holds the applications that may ask somebody for a session of
// theirs, and what each of them said about itself when it registered.
//
// A client is not a user and holds nothing of its own: what it may do is
// whatever the person who approved it may do. It is registered by whoever
// wants to connect one — an MCP client registers itself — so the only thing
// registration establishes is where a session of ours may be sent back to.
package client

import (
	"context"
	"slices"
	"time"

	"github.com/khanzadimahdi/testproject/domain/password"
)

// the ways a client proves it is itself at the token endpoint. A public client
// proves nothing and leans on PKCE instead, which is what an application that
// runs on somebody's own machine has to do: a secret shipped to every copy of
// it is not a secret.
const (
	AuthMethodNone        = "none"
	AuthMethodSecretPost  = "client_secret_post"
	AuthMethodSecretBasic = "client_secret_basic"
)

// the grants a client may be registered for.
const (
	GrantAuthorizationCode = "authorization_code"
	GrantRefreshToken      = "refresh_token"
)

// ResponseTypeCode is the only response type there is: OAuth 2.1 has no others.
const ResponseTypeCode = "code"

type Client struct {
	ID           string
	Name         string
	URI          string
	RedirectURIs []string

	GrantTypes              []string
	ResponseTypes           []string
	TokenEndpointAuthMethod string
	Scope                   string

	// Secret is what a confidential client proves itself with, and is empty for
	// a public one.
	Secret password.Hash

	// ExpiresAt is when a registration nobody ever approved is thrown away.
	// Anybody may register, so a registration is worth nothing until somebody
	// says it is: until then it is kept on sufferance, and the moment one is
	// approved it is kept for good.
	//
	// It is zero for a client that has been approved, and for every client
	// registered before this was so.
	ExpiresAt time.Time

	CreatedAt time.Time
}

// UnapprovedLifetime is how long a registration waits to be approved. An
// application registers itself moments before it sends somebody here, so what
// is waiting a day later was never going anywhere.
const UnapprovedLifetime = 24 * time.Hour

// IsPublic reports a client that has no secret to prove itself with.
func (c Client) IsPublic() bool {
	return c.TokenEndpointAuthMethod == AuthMethodNone
}

// AllowsRedirectURI reports whether a session may be sent back to the given
// address. The comparison is exact: a client is answered at an address it
// registered or at none at all, because anything looser is how a session ends
// up somewhere else.
func (c Client) AllowsRedirectURI(uri string) bool {
	return slices.Contains(c.RedirectURIs, uri)
}

// AllowsGrant reports whether the client registered for this way of obtaining
// a session.
func (c Client) AllowsGrant(grantType string) bool {
	return slices.Contains(c.GrantTypes, grantType)
}

type Repository interface {
	Save(ctx context.Context, c *Client) (id string, err error)
	GetOne(ctx context.Context, id string) (Client, error)

	// Keep stops a client from being thrown away, and is what somebody
	// approving it comes to.
	Keep(ctx context.Context, id string) error
}
