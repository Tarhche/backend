package oauth

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/oauth"
)

// openIDProvider is any provider that speaks OpenID Connect, where the answer
// to "who just signed in" is the same document whoever is asked: a subject that
// never changes, a name, a picture, and an email the provider says whether it
// has verified.
//
// Google and LinkedIn are both this. What differs between them is their doors
// and the words they want their scopes in, which is what the constructors are
// for.
type openIDProvider struct {
	name      string
	scope     string
	config    Config
	endpoints endpoints
	client    httpClient
}

var _ oauth.Provider = &openIDProvider{}

// NewGoogle signs people in with their Google account.
func NewGoogle(config Config) *openIDProvider {
	return &openIDProvider{
		name:   "google",
		scope:  "openid email profile",
		config: config,
		endpoints: endpoints{
			authorization: "https://accounts.google.com/o/oauth2/v2/auth",
			token:         "https://oauth2.googleapis.com/token",
			user:          "https://openidconnect.googleapis.com/v1/userinfo",
		},
		client: defaultClient(),
	}
}

// NewLinkedin signs people in with their LinkedIn account.
func NewLinkedin(config Config) *openIDProvider {
	return &openIDProvider{
		name:   "linkedin",
		scope:  "openid profile email",
		config: config,
		endpoints: endpoints{
			authorization: "https://www.linkedin.com/oauth/v2/authorization",
			token:         "https://www.linkedin.com/oauth/v2/accessToken",
			user:          "https://api.linkedin.com/v2/userinfo",
		},
		client: defaultClient(),
	}
}

func (p *openIDProvider) Name() string {
	return p.name
}

func (p *openIDProvider) AuthorizationURL(state string) string {
	return authorizationURL(p.endpoints.authorization, p.config, p.scope, state)
}

func (p *openIDProvider) Identify(ctx context.Context, code string) (oauth.Identity, error) {
	accessToken, err := exchange(ctx, p.client, p.endpoints.token, p.config, code)
	if err != nil {
		return oauth.Identity{}, err
	}

	// what OpenID Connect calls a userinfo response. `sub` is the provider's
	// own name for the person and the only part that is theirs for good.
	var profile struct {
		Subject  string `json:"sub"`
		Email    string `json:"email"`
		Verified bool   `json:"email_verified"`
		Name     string `json:"name"`
		Picture  string `json:"picture"`
	}

	if err := get(ctx, p.client, p.endpoints.user, accessToken, &profile); err != nil {
		return oauth.Identity{}, err
	}

	if len(profile.Subject) == 0 {
		return oauth.Identity{}, ErrExchangeFailed
	}

	return oauth.Identity{
		Provider: p.name,
		ID:       profile.Subject,
		Email:    profile.Email,
		Verified: profile.Verified && len(profile.Email) > 0,
		Name:     profile.Name,
		Avatar:   profile.Picture,
	}, nil
}
