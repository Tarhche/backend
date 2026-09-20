package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/oauth"
	approveauthorization "github.com/khanzadimahdi/testproject/application/oauth/approveAuthorization"
	"github.com/khanzadimahdi/testproject/application/oauth/authorize"
	describeauthorization "github.com/khanzadimahdi/testproject/application/oauth/describeAuthorization"
	exchangecode "github.com/khanzadimahdi/testproject/application/oauth/exchangeCode"
	registerclient "github.com/khanzadimahdi/testproject/application/oauth/registerClient"
	"github.com/khanzadimahdi/testproject/domain/oauth/client"
	"github.com/khanzadimahdi/testproject/domain/oauth/grant"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/role"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/argon2"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	clients "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/oauth/clients"
	grants "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/oauth/grants"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/roles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

const (
	serviceURL = "https://api.example"
	userUUID   = "user-uuid"
	verifier   = "a-verifier-long-enough-to-be-one"
	secret     = "the-grant-secret"
)

func hasher() password.Hasher {
	return argon2.NewArgon2id(1, 8*1024, 1, 32)
}

func decoded(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))

	return body
}

func TestMetadata(t *testing.T) {
	t.Parallel()

	t.Run("the resource says who issues the tokens it takes", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		NewProtectedResourceHandler(serviceURL).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, ProtectedPath, nil))

		require.Equal(t, http.StatusOK, recorder.Code)
		body := decoded(t, recorder)

		assert.Equal(t, serviceURL+"/mcp", body["resource"])
		assert.Equal(t, []any{serviceURL}, body["authorization_servers"])
		assert.Equal(t, []any{oauth.Scope}, body["scopes_supported"])
	})

	t.Run("the server says where to ask and where to collect", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		NewAuthorizationServerHandler(serviceURL).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, AuthServerPath, nil))

		require.Equal(t, http.StatusOK, recorder.Code)
		body := decoded(t, recorder)

		assert.Equal(t, serviceURL, body["issuer"])
		assert.Equal(t, serviceURL+AuthorizePath, body["authorization_endpoint"])
		assert.Equal(t, serviceURL+TokenPath, body["token_endpoint"])
		assert.Equal(t, serviceURL+RegisterPath, body["registration_endpoint"])
		assert.Equal(t, []any{grant.ChallengeMethodS256}, body["code_challenge_methods_supported"])
		assert.Equal(t, []any{client.GrantAuthorizationCode, client.GrantRefreshToken}, body["grant_types_supported"])
	})

	t.Run("a refused request is told where to look", func(t *testing.T) {
		assert.Equal(t, serviceURL+"/.well-known/oauth-protected-resource/mcp", ProtectedResourceMetadataURL(serviceURL+"/"))
	})
}

func TestRegister(t *testing.T) {
	t.Parallel()

	t.Run("an application is registered", func(t *testing.T) {
		var repository clients.MockClientsRepository
		repository.On("Save", mock.Anything, mock.Anything).Return("client-id", nil).Once()

		request := httptest.NewRequest(http.MethodPost, RegisterPath, strings.NewReader(`{"client_name":"An agent","redirect_uris":["https://agent.example/callback"]}`))
		recorder := httptest.NewRecorder()

		NewRegisterHandler(registerclient.NewUseCase(&repository, hasher())).ServeHTTP(recorder, request)

		require.Equal(t, http.StatusCreated, recorder.Code)
		assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
		assert.Equal(t, "client-id", decoded(t, recorder)["client_id"])
	})

	t.Run("a registration that cannot be honoured is refused under its own name", func(t *testing.T) {
		var repository clients.MockClientsRepository

		request := httptest.NewRequest(http.MethodPost, RegisterPath, strings.NewReader(`{"client_name":"An agent","redirect_uris":["http://agent.example/callback"]}`))
		recorder := httptest.NewRecorder()

		NewRegisterHandler(registerclient.NewUseCase(&repository, hasher())).ServeHTTP(recorder, request)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Equal(t, oauth.ErrorInvalidRedirectURI, decoded(t, recorder)["error"])
	})
}

func TestAuthorize(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.Generate()
	require.NoError(t, err)

	requests := oauth.NewRequests(jwt.NewJWT(privateKey, privateKey.Public()))

	registered := client.Client{
		ID:            "client-id",
		Name:          "An agent",
		RedirectURIs:  []string{"https://agent.example/callback"},
		GrantTypes:    []string{client.GrantAuthorizationCode},
		ResponseTypes: []string{client.ResponseTypeCode},
	}

	query := url.Values{
		"client_id":             {"client-id"},
		"redirect_uri":          {"https://agent.example/callback"},
		"response_type":         {"code"},
		"code_challenge":        {"a-challenge"},
		"code_challenge_method": {grant.ChallengeMethodS256},
		"state":                 {"state-from-the-client"},
	}

	t.Run("somebody is sent to the page that asks them", func(t *testing.T) {
		var repository clients.MockClientsRepository
		repository.On("GetOne", mock.Anything, "client-id").Return(registered, nil).Once()

		recorder := httptest.NewRecorder()
		NewAuthorizeHandler(authorize.NewUseCase(&repository, requests), "https://web.example"+ConsentPath).
			ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, AuthorizePath+"?"+query.Encode(), nil))

		require.Equal(t, http.StatusFound, recorder.Code)

		sent, err := url.Parse(recorder.Header().Get("Location"))
		require.NoError(t, err)
		assert.Equal(t, "web.example", sent.Host)
		assert.Equal(t, ConsentPath, sent.Path)

		carried, err := requests.Verify(context.Background(), sent.Query().Get("request"))
		require.NoError(t, err)
		assert.Equal(t, "client-id", carried.ClientID)
	})

	t.Run("what the application got wrong goes back to the application", func(t *testing.T) {
		var repository clients.MockClientsRepository
		repository.On("GetOne", mock.Anything, "client-id").Return(registered, nil).Once()

		asked := url.Values{}
		maps.Copy(asked, query)
		asked.Set("response_type", "token")

		recorder := httptest.NewRecorder()
		NewAuthorizeHandler(authorize.NewUseCase(&repository, requests), "https://web.example"+ConsentPath).
			ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, AuthorizePath+"?"+asked.Encode(), nil))

		require.Equal(t, http.StatusFound, recorder.Code)

		sent, err := url.Parse(recorder.Header().Get("Location"))
		require.NoError(t, err)
		assert.Equal(t, "agent.example", sent.Host)
		assert.Equal(t, oauth.ErrorUnsupportedResponseType, sent.Query().Get("error"))
		assert.Equal(t, "state-from-the-client", sent.Query().Get("state"))
	})

	t.Run("an address nobody registered is not redirected to", func(t *testing.T) {
		var repository clients.MockClientsRepository
		repository.On("GetOne", mock.Anything, "client-id").Return(registered, nil).Once()

		asked := url.Values{}
		maps.Copy(asked, query)
		asked.Set("redirect_uri", "https://somewhere.else/callback")

		recorder := httptest.NewRecorder()
		NewAuthorizeHandler(authorize.NewUseCase(&repository, requests), "https://web.example"+ConsentPath).
			ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, AuthorizePath+"?"+asked.Encode(), nil))

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Empty(t, recorder.Header().Get("Location"))
		assert.Equal(t, oauth.ErrorInvalidRedirectURI, decoded(t, recorder)["error"])
	})
}

func TestAuthorization(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.Generate()
	require.NoError(t, err)

	requests := oauth.NewRequests(jwt.NewJWT(privateKey, privateKey.Public()))

	token, err := requests.Generate(context.Background(), oauth.AuthorizationRequest{
		ClientID:            "client-id",
		ClientName:          "An agent",
		RedirectURI:         "https://agent.example/callback",
		State:               "state-from-the-client",
		Scope:               oauth.Scope,
		CodeChallenge:       "a-challenge",
		CodeChallengeMethod: grant.ChallengeMethodS256,
	})
	require.NoError(t, err)

	registered := client.Client{
		ID:           "client-id",
		Name:         "An agent",
		URI:          "https://agent.example",
		RedirectURIs: []string{"https://agent.example/callback"},
		GrantTypes:   []string{client.GrantAuthorizationCode},
	}

	t.Run("the page is told what it is asking about", func(t *testing.T) {
		var repository clients.MockClientsRepository
		repository.On("GetOne", mock.Anything, "client-id").Return(registered, nil).Once()

		recorder := httptest.NewRecorder()
		NewDescribeAuthorizationHandler(describeauthorization.NewUseCase(&repository, requests)).
			ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/oauth/authorization?request="+url.QueryEscape(token), nil))

		require.Equal(t, http.StatusOK, recorder.Code)
		body := decoded(t, recorder)
		assert.Equal(t, "An agent", body["client_name"])
		assert.Equal(t, "https://agent.example", body["client_uri"])
		assert.Equal(t, []any{oauth.Scope}, body["scopes"])
	})

	t.Run("who approves is who the request's own token is for", func(t *testing.T) {
		var clientRepository clients.MockClientsRepository
		clientRepository.On("GetOne", mock.Anything, "client-id").Return(registered, nil).Once()
		clientRepository.On("Keep", mock.Anything, "client-id").Return(nil).Once()

		var grantRepository grants.MockGrantsRepository
		grantRepository.On("Save", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			args.Get(1).(*grant.Grant).ID = "grant-id"
		}).Return("grant-id", nil).Once()

		body, err := json.Marshal(map[string]any{"request": token, "approved": true, "uuid": "somebody-else"})
		require.NoError(t, err)

		request := httptest.NewRequest(http.MethodPost, "/api/oauth/authorization", strings.NewReader(string(body)))
		request = request.WithContext(auth.ToContext(request.Context(), &user.User{UUID: userUUID}))

		recorder := httptest.NewRecorder()
		NewApproveAuthorizationHandler(approveauthorization.NewUseCase(&clientRepository, &grantRepository, requests, hasher())).
			ServeHTTP(recorder, request)

		require.Equal(t, http.StatusOK, recorder.Code)

		sent, err := url.Parse(decoded(t, recorder)["redirect_to"].(string))
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(sent.Query().Get("code"), "grant-id."))
	})
}

func TestToken(t *testing.T) {
	t.Parallel()

	h := hasher()
	salt := []byte("salt")
	sum := sha256.Sum256([]byte(verifier))

	granted := grant.Grant{
		ID:                  "grant-id",
		Secret:              password.Hash{Value: h.Hash(context.Background(), []byte(secret), salt), Salt: salt},
		ClientID:            "client-id",
		UserUUID:            userUUID,
		RedirectURI:         "https://agent.example/callback",
		Scope:               oauth.Scope,
		CodeChallenge:       base64.RawURLEncoding.EncodeToString(sum[:]),
		CodeChallengeMethod: grant.ChallengeMethodS256,
		ExpiresAt:           time.Now().Add(time.Minute),
	}

	privateKey, err := ecdsa.Generate()
	require.NoError(t, err)

	var roleRepository roles.MockRolesRepository
	roleRepository.On("GetByUserUUID", mock.Anything, userUUID).Return([]role.Role{}, nil).Maybe()
	generator := auth.NewTokenGenerator(jwt.NewJWT(privateKey, privateKey.Public()), &roleRepository)

	exchange := func(registered client.Client) (*exchangecode.UseCase, *clients.MockClientsRepository) {
		var clientRepository clients.MockClientsRepository
		clientRepository.On("GetOne", mock.Anything, "client-id").Return(registered, nil).Maybe()

		var grantRepository grants.MockGrantsRepository
		grantRepository.On("Consume", mock.Anything, "grant-id").Return(granted, nil).Maybe()

		var userRepository users.MockUsersRepository
		userRepository.On("GetOne", mock.Anything, userUUID).Return(user.User{UUID: userUUID}, nil).Maybe()

		return exchangecode.NewUseCase(&grantRepository, oauth.NewClients(&clientRepository, h), &userRepository, h, generator), &clientRepository
	}

	public := client.Client{
		ID:                      "client-id",
		RedirectURIs:            []string{"https://agent.example/callback"},
		GrantTypes:              []string{client.GrantAuthorizationCode, client.GrantRefreshToken},
		TokenEndpointAuthMethod: client.AuthMethodNone,
	}

	t.Run("a code is traded for a session", func(t *testing.T) {
		useCase, _ := exchange(public)

		form := url.Values{
			"grant_type":    {client.GrantAuthorizationCode},
			"code":          {"grant-id." + secret},
			"client_id":     {"client-id"},
			"redirect_uri":  {"https://agent.example/callback"},
			"code_verifier": {verifier},
		}

		request := httptest.NewRequest(http.MethodPost, TokenPath, strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		recorder := httptest.NewRecorder()
		NewTokenHandler(useCase, nil).ServeHTTP(recorder, request)

		require.Equal(t, http.StatusOK, recorder.Code)
		body := decoded(t, recorder)
		assert.NotEmpty(t, body["access_token"])
		assert.NotEmpty(t, body["refresh_token"])
		assert.Equal(t, "Bearer", body["token_type"])
	})

	t.Run("an application may prove itself the way a header does it", func(t *testing.T) {
		confidential := public
		confidential.TokenEndpointAuthMethod = client.AuthMethodSecretPost
		confidential.Secret = password.Hash{Value: h.Hash(context.Background(), []byte("the-client-secret"), salt), Salt: salt}

		useCase, _ := exchange(confidential)

		form := url.Values{
			"grant_type":    {client.GrantAuthorizationCode},
			"code":          {"grant-id." + secret},
			"code_verifier": {verifier},
		}

		request := httptest.NewRequest(http.MethodPost, TokenPath, strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.SetBasicAuth("client-id", "the-client-secret")

		recorder := httptest.NewRecorder()
		NewTokenHandler(useCase, nil).ServeHTTP(recorder, request)

		require.Equal(t, http.StatusOK, recorder.Code)
		assert.NotEmpty(t, decoded(t, recorder)["access_token"])
	})

	t.Run("an application that cannot prove itself is told so, and not with a 500", func(t *testing.T) {
		confidential := public
		confidential.TokenEndpointAuthMethod = client.AuthMethodSecretPost
		confidential.Secret = password.Hash{Value: h.Hash(context.Background(), []byte("the-client-secret"), salt), Salt: salt}

		useCase, _ := exchange(confidential)

		form := url.Values{
			"grant_type":    {client.GrantAuthorizationCode},
			"code":          {"grant-id." + secret},
			"client_id":     {"client-id"},
			"client_secret": {"not-the-secret"},
			"code_verifier": {verifier},
		}

		request := httptest.NewRequest(http.MethodPost, TokenPath, strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		recorder := httptest.NewRecorder()
		NewTokenHandler(useCase, nil).ServeHTTP(recorder, request)

		require.Equal(t, http.StatusUnauthorized, recorder.Code)
		assert.Equal(t, oauth.ErrorInvalidClient, decoded(t, recorder)["error"])
	})

	t.Run("a grant this server does not do is refused by name", func(t *testing.T) {
		useCase, _ := exchange(public)

		form := url.Values{"grant_type": {"password"}}

		request := httptest.NewRequest(http.MethodPost, TokenPath, strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		recorder := httptest.NewRecorder()
		NewTokenHandler(useCase, nil).ServeHTTP(recorder, request)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Equal(t, oauth.ErrorUnsupportedGrantType, decoded(t, recorder)["error"])
	})
}
