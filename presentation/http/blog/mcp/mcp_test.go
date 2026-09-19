package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	"github.com/khanzadimahdi/testproject/presentation/http/router"
)

const userUUID = "user-uuid"

// asked records what a route was asked for, so a tool call can be held against
// the request it turned into.
type asked struct {
	calls   int
	method  string
	path    string
	escaped string
	query   string
	url     string
	host    string
	body    string
	header  http.Header
}

func (a *asked) handler(status int, contentType string, answer string) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		a.calls++
		a.method = r.Method
		a.path = r.URL.Path
		a.escaped = r.URL.EscapedPath()
		a.url = r.URL.String()
		a.host = r.Host
		a.query = r.URL.RawQuery
		a.body = string(body)
		a.header = r.Header.Clone()

		if len(contentType) > 0 {
			rw.Header().Set("Content-Type", contentType)
		}

		rw.WriteHeader(status)
		rw.Write([]byte(answer))
	})
}

// signedIn is a session, and the authenticator that reads it.
func signedIn(t *testing.T, permissions ...string) (*auth.Authenticator, string) {
	t.Helper()

	privateKey, err := ecdsa.Generate()
	require.NoError(t, err)

	j := jwt.NewJWT(privateKey, privateKey.Public())

	claims := jwt.NewClaimsBuilder()
	claims.SetSubject(userUUID)
	claims.SetAudience([]string{auth.AccessToken})
	claims.SetExpirationTime(time.Now().Add(time.Minute))
	claims.SetPermissions(permissions)

	token, err := j.Generate(context.Background(), claims.Build())
	require.NoError(t, err)

	var userRepository users.MockUsersRepository
	userRepository.On("GetOne", mock.Anything, userUUID).Return(user.User{UUID: userUUID}, nil).Maybe()

	return auth.NewAuthenticator(j, &userRepository), token
}

// call makes one MCP request and hands back what came of it.
func call(t *testing.T, handler http.Handler, token string, body string) (int, map[string]any) {
	t.Helper()

	request := httptest.NewRequest(http.MethodPost, Path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	if len(token) > 0 {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	var answer map[string]any
	if recorder.Body.Len() > 0 {
		_ = json.Unmarshal(recorder.Body.Bytes(), &answer)
	}

	return recorder.Code, answer
}

func toolCall(name string, arguments map[string]any) string {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  map[string]any{"name": name, "arguments": arguments},
	})

	return string(body)
}

// results reads the text a tool answered with, and whether it refused.
func results(t *testing.T, answer map[string]any) (string, bool) {
	t.Helper()

	result, ok := answer["result"].(map[string]any)
	require.True(t, ok, "%v", answer)

	isError, _ := result["isError"].(bool)

	var text strings.Builder
	for _, item := range result["content"].([]any) {
		content := item.(map[string]any)
		if value, ok := content["text"].(string); ok {
			text.WriteString(value)
		}

		if resource, ok := content["resource"].(map[string]any); ok {
			if value, ok := resource["text"].(string); ok {
				text.WriteString(value)
			}

			if value, ok := resource["blob"].(string); ok {
				text.WriteString("base64:")
				text.WriteString(value)
			}
		}
	}

	return text.String(), isError
}

func TestTheTableAndTheRoutesAgree(t *testing.T) {
	t.Parallel()

	table := tools()

	t.Run("every tool names a route, once", func(t *testing.T) {
		routes := make(map[string]string, len(table))
		names := make(map[string]string, len(table))

		for _, tool := range table {
			assert.Regexp(t, `^[a-z][a-z0-9_]*$`, tool.name)
			assert.NotEmpty(t, tool.description, tool.name)

			method, path, found := strings.Cut(tool.route, " ")
			require.True(t, found, tool.name)
			assert.Contains(t, []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete}, method)
			assert.True(t, strings.HasPrefix(path, "/"), tool.route)

			if previous, taken := names[tool.name]; taken {
				t.Fatalf("%q names both %q and %q", tool.name, previous, tool.route)
			}
			names[tool.name] = tool.route

			if previous, taken := routes[tool.route]; taken {
				t.Fatalf("%q is reached by both %q and %q", tool.route, previous, tool.name)
			}
			routes[tool.route] = tool.name
		}
	})

	t.Run("every tool describes what its path carries", func(t *testing.T) {
		for _, tool := range table {
			schema, err := inputSchema(tool)
			require.NoError(t, err, tool.name)

			for _, name := range tool.pathParams() {
				assert.Contains(t, schema.Properties, name, tool.name)
				assert.Contains(t, schema.Required, name, tool.name)
			}

			resolved, err := schema.Resolve(nil)
			require.NoError(t, err, tool.name)
			require.NotNil(t, resolved)
		}
	})
}

func TestARouteWithoutAToolStopsTheServerBeingBuilt(t *testing.T) {
	t.Parallel()

	routes := router.New()
	routes.Handle("GET /api/one", http.NotFoundHandler())
	routes.Handle("GET /api/two", http.NotFoundHandler())

	authenticator, _ := signedIn(t)

	_, err := newHandler(routes, []tool{{
		name:        "one",
		description: "the only tool there is",
		route:       "GET /api/one",
	}}, authenticator, "https://api.example/metadata", nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "GET /api/two")
}

func TestAToolWithoutARouteStopsTheServerBeingBuilt(t *testing.T) {
	t.Parallel()

	routes := router.New()
	routes.Handle("GET /api/one", http.NotFoundHandler())

	authenticator, _ := signedIn(t)

	_, err := newHandler(routes, []tool{{
		name:        "two",
		description: "a tool for a route nobody registered",
		route:       "GET /api/two",
	}}, authenticator, "https://api.example/metadata", nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no route answers")
}

func TestAnUnsignedRequestIsToldWhereToAskForASession(t *testing.T) {
	t.Parallel()

	routes := router.New()
	routes.Handle("GET /api/one", http.NotFoundHandler())

	authenticator, _ := signedIn(t)

	handler, err := newHandler(routes, []tool{{
		name:        "one",
		description: "the only tool there is",
		route:       "GET /api/one",
		readOnly:    true,
	}}, authenticator, "https://api.example/.well-known/oauth-protected-resource/mcp", nil)
	require.NoError(t, err)

	request := httptest.NewRequest(http.MethodPost, Path, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, recorder.Header().Get("WWW-Authenticate"), `resource_metadata="https://api.example/.well-known/oauth-protected-resource/mcp"`)
}

func TestAToolCallBecomesTheRequestTheRouteReads(t *testing.T) {
	t.Parallel()

	var route asked

	routes := router.New()
	routes.Handle("PUT /api/things/{thingUUID}/{language_code}", route.handler(http.StatusOK, "application/json", `{"done":true}`))

	authenticator, token := signedIn(t)

	handler, err := newHandler(routes, []tool{{
		name:        "thing_update",
		description: "changes a thing",
		route:       "PUT /api/things/{thingUUID}/{language_code}",
		params:      []Parameter{text("thing_uuid", "which thing"), languageCode(), page()},
		body: object(map[string]*jsonschema.Schema{
			"title": {Type: "string"},
		}, "title"),
	}}, authenticator, "https://api.example/metadata", nil)
	require.NoError(t, err)

	status, answer := call(t, handler, token, toolCall("thing_update", map[string]any{
		"thing_uuid":    "abc def",
		"language_code": "fa",
		"page":          2,
		"title":         "a new title",
	}))

	require.Equal(t, http.StatusOK, status)

	text, isError := results(t, answer)
	assert.False(t, isError)
	assert.Equal(t, `{"done":true}`, text)

	require.Equal(t, 1, route.calls)
	assert.Equal(t, http.MethodPut, route.method)
	assert.Equal(t, "/api/things/abc def/fa", route.path)
	assert.Equal(t, "/api/things/abc%20def/fa", route.escaped)
	assert.Equal(t, "page=2", route.query)
	assert.JSONEq(t, `{"title":"a new title"}`, route.body)
	assert.Equal(t, "Bearer "+token, route.header.Get("Authorization"))
	assert.Equal(t, "application/json", route.header.Get("Content-Type"))

	// it reaches the route looking like a request that arrived, so what the
	// route caches its answer under is the same either way
	assert.Equal(t, "/api/things/abc%20def/fa?page=2", route.url)
	assert.Equal(t, internalHost, route.host)
}

func TestWhatARouteAnswersIsWhatTheToolAnswers(t *testing.T) {
	t.Parallel()

	for name, answered := range map[string]struct {
		status      int
		contentType string
		body        string
		text        string
		isError     bool
	}{
		"json comes back as it is": {
			status: http.StatusOK, contentType: "application/json", body: `{"a":1}`, text: `{"a":1}`,
		},
		"an empty answer says what it was": {
			status: http.StatusNoContent, text: `{"status":204}`,
		},
		"a session that expired is said to have": {
			status: http.StatusUnauthorized, text: "expired", isError: true,
		},
		"a refusal is a refusal, not a crash": {
			status: http.StatusForbidden, text: "may not", isError: true,
		},
		"nothing there is said plainly": {
			status: http.StatusNotFound, text: "no such thing", isError: true,
		},
		"a validation error is handed back whole": {
			status: http.StatusBadRequest, contentType: "application/json", body: `{"errors":{"title":"required_field"}}`, text: "required_field", isError: true,
		},
		"a failure is a failure": {
			status: http.StatusInternalServerError, text: "500", isError: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var route asked

			routes := router.New()
			routes.Handle("GET /api/things", route.handler(answered.status, answered.contentType, answered.body))

			authenticator, token := signedIn(t)

			handler, err := newHandler(routes, []tool{{
				name:        "things_list",
				description: "lists things",
				route:       "GET /api/things",
				readOnly:    true,
			}}, authenticator, "https://api.example/metadata", nil)
			require.NoError(t, err)

			status, answer := call(t, handler, token, toolCall("things_list", map[string]any{}))
			require.Equal(t, http.StatusOK, status)

			text, isError := results(t, answer)
			assert.Equal(t, answered.isError, isError)
			assert.Contains(t, text, answered.text)
		})
	}
}

func TestAFileComesBackAsAFile(t *testing.T) {
	t.Parallel()

	var route asked

	routes := router.New()
	routes.Handle("GET /files/{uuid}", route.handler(http.StatusOK, "image/png", "\x89PNG\r\n\x1a\n"))

	authenticator, token := signedIn(t)

	handler, err := newHandler(routes, []tool{{
		name:        "file_download",
		description: "downloads a file",
		route:       "GET /files/{uuid}",
		params:      []Parameter{uuidOf("file")},
		download:    true,
		readOnly:    true,
	}}, authenticator, "https://api.example/metadata", nil)
	require.NoError(t, err)

	status, answer := call(t, handler, token, toolCall("file_download", map[string]any{"uuid": "file-uuid"}))
	require.Equal(t, http.StatusOK, status)

	text, isError := results(t, answer)
	assert.False(t, isError)
	assert.Equal(t, "base64:"+base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\n")), text)
	assert.Equal(t, "/files/file-uuid", route.path)
}

func TestAFileIsUploadedAsAForm(t *testing.T) {
	t.Parallel()

	var route asked

	routes := router.New()
	routes.Handle("POST /api/files", route.handler(http.StatusCreated, "application/json", `{"uuid":"new"}`))

	authenticator, token := signedIn(t)

	handler, err := newHandler(routes, []tool{{
		name:        "file_upload",
		description: "uploads a file",
		route:       "POST /api/files",
		upload:      true,
		body: object(map[string]*jsonschema.Schema{
			"name":    {Type: "string"},
			"content": {Type: "string"},
		}, "name", "content"),
	}}, authenticator, "https://api.example/metadata", nil)
	require.NoError(t, err)

	status, answer := call(t, handler, token, toolCall("file_upload", map[string]any{
		"name":    "notes.txt",
		"content": base64.StdEncoding.EncodeToString([]byte("hello")),
	}))
	require.Equal(t, http.StatusOK, status)

	text, isError := results(t, answer)
	assert.False(t, isError)
	assert.Equal(t, `{"uuid":"new"}`, text)

	require.Equal(t, 1, route.calls)
	assert.Contains(t, route.header.Get("Content-Type"), "multipart/form-data")
	assert.Contains(t, route.body, `name="file"; filename="notes.txt"`)
	assert.Contains(t, route.body, "hello")
}

func TestASessionIsOfferedOnlyWhatItHolds(t *testing.T) {
	t.Parallel()

	authorizer := domain.MockAuthorizer{}
	authorizer.On("Authorize", mock.Anything, userUUID, permission.ArticlesIndex).Return(true, nil).Maybe()
	authorizer.On("Authorize", mock.Anything, userUUID, permission.UsersIndex).Return(false, nil).Maybe()

	authenticator, token := signedIn(t, permission.ArticlesIndex)

	var articles, users asked

	routes := router.New()
	routes.Handle("GET /api/public", articles.handler(http.StatusOK, "application/json", `[]`))
	routes.Handle("GET /api/articles", middleware.NewAuthorizeMiddleware(articles.handler(http.StatusOK, "application/json", `[]`), &authorizer, permission.ArticlesIndex))
	routes.Handle("GET /api/users", middleware.NewAuthorizeMiddleware(users.handler(http.StatusOK, "application/json", `[]`), &authorizer, permission.UsersIndex))

	handler, err := newHandler(routes, []tool{
		{name: "public_list", description: "anybody", route: "GET /api/public", readOnly: true},
		{name: "articles_list", description: "articles", route: "GET /api/articles", readOnly: true},
		{name: "users_list", description: "users", route: "GET /api/users", readOnly: true},
	}, authenticator, "https://api.example/metadata", nil)
	require.NoError(t, err)

	status, answer := call(t, handler, token, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	require.Equal(t, http.StatusOK, status)

	result := answer["result"].(map[string]any)
	var offered []string
	for _, listed := range result["tools"].([]any) {
		offered = append(offered, listed.(map[string]any)["name"].(string))
	}

	assert.ElementsMatch(t, []string{"public_list", "articles_list"}, offered)
}

func TestWhatIsNotOfferedIsStillRefusedByTheRoute(t *testing.T) {
	t.Parallel()

	authorizer := domain.MockAuthorizer{}
	authorizer.On("Authorize", mock.Anything, userUUID, permission.UsersIndex).Return(false, nil)

	authenticator, token := signedIn(t, permission.UsersIndex)

	var route asked

	routes := router.New()
	routes.Handle("GET /api/users", middleware.NewAuthorizeMiddleware(route.handler(http.StatusOK, "application/json", `[]`), &authorizer, permission.UsersIndex))

	handler, err := newHandler(routes, []tool{
		{name: "users_list", description: "users", route: "GET /api/users", readOnly: true},
	}, authenticator, "https://api.example/metadata", nil)
	require.NoError(t, err)

	// the token says it may, so the tool is offered; the roles say otherwise,
	// and it is the roles that are asked when the call is made.
	status, answer := call(t, handler, token, toolCall("users_list", map[string]any{}))
	require.Equal(t, http.StatusOK, status)

	text, isError := results(t, answer)
	assert.True(t, isError)
	assert.Contains(t, text, "may not")
	assert.Zero(t, route.calls)
	authorizer.AssertExpectations(t)
}
