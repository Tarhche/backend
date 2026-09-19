package oauth

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/oauth"
)

var testConfig = Config{
	ClientID:     "client-id",
	ClientSecret: "client-secret",
	RedirectURL:  "https://tarhche.com/auth/callback",
}

// answers stands in for a provider: it hands back what each of its doors is
// told to, and keeps what it was asked so a test can read it back.
type answers struct {
	bodies   map[string]string
	statuses map[string]int
	asked    map[string]*http.Request
	forms    map[string]url.Values
}

func newAnswers() *answers {
	return &answers{
		bodies:   map[string]string{},
		statuses: map[string]int{},
		asked:    map[string]*http.Request{},
		forms:    map[string]url.Values{},
	}
}

func (a *answers) Do(request *http.Request) (*http.Response, error) {
	door := request.URL.String()

	if request.Body != nil {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}

		form, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, err
		}

		a.forms[door] = form
	}

	a.asked[door] = request

	status := a.statuses[door]
	if status == 0 {
		status = http.StatusOK
	}

	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Body:       io.NopCloser(strings.NewReader(a.bodies[door])),
		Header:     http.Header{},
	}, nil
}

func TestAuthorizationURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider oauth.Provider
		door     string
		scope    string
	}{
		{
			name:     "google",
			provider: NewGoogle(testConfig),
			door:     "https://accounts.google.com/o/oauth2/v2/auth",
			scope:    "openid email profile",
		},
		{
			name:     "github",
			provider: NewGithub(testConfig),
			door:     "https://github.com/login/oauth/authorize",
			scope:    "read:user user:email",
		},
		{
			name:     "linkedin",
			provider: NewLinkedin(testConfig),
			door:     "https://www.linkedin.com/oauth/v2/authorization",
			scope:    "openid profile email",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.name, test.provider.Name(), "a provider is asked for by the name it answers to")

			address, err := url.Parse(test.provider.AuthorizationURL("state-value"))
			require.NoError(t, err)

			assert.Equal(t, test.door, address.Scheme+"://"+address.Host+address.Path)

			query := address.Query()
			assert.Equal(t, testConfig.ClientID, query.Get("client_id"))
			assert.Equal(t, testConfig.RedirectURL, query.Get("redirect_uri"))
			assert.Equal(t, "code", query.Get("response_type"))
			assert.Equal(t, test.scope, query.Get("scope"))
			assert.Equal(t, "state-value", query.Get("state"), "the state comes back untouched, so it has to go out")

			assert.NotContains(t, address.RawQuery, testConfig.ClientSecret, "a secret does not belong in an address a browser is sent to")
		})
	}
}

func TestGoogleIdentify(t *testing.T) {
	t.Parallel()

	t.Run("who the provider says answered", func(t *testing.T) {
		t.Parallel()

		provider := NewGoogle(testConfig)
		stub := newAnswers()
		stub.bodies[provider.endpoints.token] = `{"access_token":"token-value"}`
		stub.bodies[provider.endpoints.user] = `{
			"sub": "google-subject",
			"email": "somebody@example.com",
			"email_verified": true,
			"name": "Some Body",
			"picture": "https://example.com/avatar.png"
		}`
		provider.client = stub

		identity, err := provider.Identify(context.Background(), "code-value")
		require.NoError(t, err)

		assert.Equal(t, oauth.Identity{
			Provider: "google",
			ID:       "google-subject",
			Email:    "somebody@example.com",
			Verified: true,
			Name:     "Some Body",
			Avatar:   "https://example.com/avatar.png",
		}, identity)

		// the secret is spent in the body, where no log on the way writes it down
		form := stub.forms[provider.endpoints.token]
		assert.Equal(t, "code-value", form.Get("code"))
		assert.Equal(t, "authorization_code", form.Get("grant_type"))
		assert.Equal(t, testConfig.ClientSecret, form.Get("client_secret"))
		assert.Equal(t, testConfig.RedirectURL, form.Get("redirect_uri"))

		assert.Equal(t, "Bearer token-value", stub.asked[provider.endpoints.user].Header.Get("Authorization"))
	})

	t.Run("an address the provider has not verified is not vouched for", func(t *testing.T) {
		t.Parallel()

		provider := NewGoogle(testConfig)
		stub := newAnswers()
		stub.bodies[provider.endpoints.token] = `{"access_token":"token-value"}`
		stub.bodies[provider.endpoints.user] = `{"sub":"google-subject","email":"typed@example.com","email_verified":false}`
		provider.client = stub

		identity, err := provider.Identify(context.Background(), "code-value")
		require.NoError(t, err)

		assert.False(t, identity.Verified)
		assert.Equal(t, "google-subject", identity.ID, "who they are is still known, only their address is not")
	})

	t.Run("what a refusal looks like", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name   string
			body   string
			status int
		}{
			{
				name: "a code that was already spent",
				body: `{"error":"invalid_grant","error_description":"Code was already redeemed."}`,
			},
			{
				name:   "a door that answers with a failure",
				body:   `{}`,
				status: http.StatusUnauthorized,
			},
			{
				name: "an answer with no token in it",
				body: `{"scope":"openid"}`,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				provider := NewGoogle(testConfig)
				stub := newAnswers()
				stub.bodies[provider.endpoints.token] = test.body
				stub.statuses[provider.endpoints.token] = test.status
				provider.client = stub

				_, err := provider.Identify(context.Background(), "code-value")

				assert.ErrorIs(t, err, ErrExchangeFailed)
				assert.NotContains(t, stub.asked, provider.endpoints.user, "nothing is read as somebody the provider never vouched for")
			})
		}
	})

	t.Run("an answer that names nobody", func(t *testing.T) {
		t.Parallel()

		provider := NewGoogle(testConfig)
		stub := newAnswers()
		stub.bodies[provider.endpoints.token] = `{"access_token":"token-value"}`
		stub.bodies[provider.endpoints.user] = `{"email":"somebody@example.com","email_verified":true}`
		provider.client = stub

		_, err := provider.Identify(context.Background(), "code-value")

		assert.ErrorIs(t, err, ErrExchangeFailed)
	})
}

func TestLinkedinIdentify(t *testing.T) {
	t.Parallel()

	provider := NewLinkedin(testConfig)
	stub := newAnswers()
	stub.bodies[provider.endpoints.token] = `{"access_token":"token-value"}`
	stub.bodies[provider.endpoints.user] = `{
		"sub": "linkedin-subject",
		"email": "somebody@example.com",
		"email_verified": true,
		"name": "Some Body",
		"picture": "https://example.com/avatar.png"
	}`
	provider.client = stub

	identity, err := provider.Identify(context.Background(), "code-value")
	require.NoError(t, err)

	// LinkedIn speaks the same protocol as Google, which is why one
	// implementation answers for both: only the doors differ
	assert.Equal(t, oauth.Identity{
		Provider: "linkedin",
		ID:       "linkedin-subject",
		Email:    "somebody@example.com",
		Verified: true,
		Name:     "Some Body",
		Avatar:   "https://example.com/avatar.png",
	}, identity)
}

func TestGithubIdentify(t *testing.T) {
	t.Parallel()

	t.Run("the primary verified address is the one taken", func(t *testing.T) {
		t.Parallel()

		provider := NewGithub(testConfig)
		stub := newAnswers()
		stub.bodies[provider.endpoints.token] = `{"access_token":"token-value"}`
		stub.bodies[provider.endpoints.user] = `{
			"id": 4711,
			"login": "somebody",
			"name": "Some Body",
			"avatar_url": "https://example.com/avatar.png"
		}`
		stub.bodies[provider.endpoints.emails] = `[
			{"email":"unverified@example.com","primary":false,"verified":false},
			{"email":"second@example.com","primary":false,"verified":true},
			{"email":"primary@example.com","primary":true,"verified":true}
		]`
		provider.client = stub

		identity, err := provider.Identify(context.Background(), "code-value")
		require.NoError(t, err)

		assert.Equal(t, oauth.Identity{
			Provider: "github",
			ID:       "4711",
			Email:    "primary@example.com",
			Verified: true,
			Name:     "Some Body",
			Avatar:   "https://example.com/avatar.png",
		}, identity)
	})

	t.Run("a verified address that is not primary will do", func(t *testing.T) {
		t.Parallel()

		provider := NewGithub(testConfig)
		stub := newAnswers()
		stub.bodies[provider.endpoints.token] = `{"access_token":"token-value"}`
		stub.bodies[provider.endpoints.user] = `{"id":4711,"login":"somebody"}`
		stub.bodies[provider.endpoints.emails] = `[
			{"email":"unverified@example.com","primary":true,"verified":false},
			{"email":"second@example.com","primary":false,"verified":true}
		]`
		provider.client = stub

		identity, err := provider.Identify(context.Background(), "code-value")
		require.NoError(t, err)

		assert.Equal(t, "second@example.com", identity.Email)
		assert.True(t, identity.Verified)
		assert.Equal(t, "somebody", identity.Name, "an account with no name is known by its handle")
	})

	t.Run("an account with nothing verified is vouched for by id alone", func(t *testing.T) {
		t.Parallel()

		provider := NewGithub(testConfig)
		stub := newAnswers()
		stub.bodies[provider.endpoints.token] = `{"access_token":"token-value"}`
		stub.bodies[provider.endpoints.user] = `{"id":4711,"login":"somebody"}`
		stub.bodies[provider.endpoints.emails] = `[{"email":"typed@example.com","primary":true,"verified":false}]`
		provider.client = stub

		identity, err := provider.Identify(context.Background(), "code-value")
		require.NoError(t, err)

		assert.Empty(t, identity.Email)
		assert.False(t, identity.Verified)
		assert.Equal(t, "4711", identity.ID)
	})
}
