package oauth

import (
	"context"
	"strconv"

	"github.com/khanzadimahdi/testproject/domain/oauth"
)

// githubProvider signs people in with their GitHub account.
//
// GitHub speaks its own dialect rather than OpenID Connect: the person's id is
// a number, and their email is not part of who they are -- an account may show
// none at all, or show one it has never verified -- so the addresses are asked
// for separately and only a verified, primary one is taken.
type githubProvider struct {
	config    Config
	endpoints endpoints
	client    httpClient
}

var _ oauth.Provider = &githubProvider{}

func NewGithub(config Config) *githubProvider {
	return &githubProvider{
		config: config,
		endpoints: endpoints{
			authorization: "https://github.com/login/oauth/authorize",
			token:         "https://github.com/login/oauth/access_token",
			user:          "https://api.github.com/user",
			emails:        "https://api.github.com/user/emails",
		},
		client: defaultClient(),
	}
}

func (p *githubProvider) Name() string {
	return "github"
}

func (p *githubProvider) AuthorizationURL(state string) string {
	return authorizationURL(p.endpoints.authorization, p.config, "read:user user:email", state)
}

func (p *githubProvider) Identify(ctx context.Context, code string) (oauth.Identity, error) {
	accessToken, err := exchange(ctx, p.client, p.endpoints.token, p.config, code)
	if err != nil {
		return oauth.Identity{}, err
	}

	var profile struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}

	if err := get(ctx, p.client, p.endpoints.user, accessToken, &profile); err != nil {
		return oauth.Identity{}, err
	}

	if profile.ID == 0 {
		return oauth.Identity{}, ErrExchangeFailed
	}

	identity := oauth.Identity{
		Provider: p.Name(),
		ID:       strconv.FormatInt(profile.ID, 10),
		Name:     profile.Name,
		Avatar:   profile.AvatarURL,
	}

	// a name is not required of a GitHub account, and the handle is what
	// everybody knows them by anyway
	if len(identity.Name) == 0 {
		identity.Name = profile.Login
	}

	email, err := p.verifiedEmail(ctx, accessToken)
	if err != nil {
		return oauth.Identity{}, err
	}

	identity.Email = email
	identity.Verified = len(email) > 0

	return identity, nil
}

// verifiedEmail is the address GitHub has both verified and been told is the
// primary one. An account may have neither, and then this estate knows the
// person by their GitHub id alone.
func (p *githubProvider) verifiedEmail(ctx context.Context, accessToken string) (string, error) {
	var addresses []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}

	if err := get(ctx, p.client, p.endpoints.emails, accessToken, &addresses); err != nil {
		return "", err
	}

	var fallback string
	for _, address := range addresses {
		if !address.Verified {
			continue
		}

		if address.Primary {
			return address.Email, nil
		}

		if len(fallback) == 0 {
			fallback = address.Email
		}
	}

	return fallback, nil
}
