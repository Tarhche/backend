package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	infraJWT "github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

// signer mints tokens the way the blog does, and verifier is what an orchestrator is
// given: the public half, and no way to sign anything.
func keys(t *testing.T) (*infraJWT.JWT, *infraJWT.JWT) {
	t.Helper()

	private, err := ecdsa.Generate()
	require.NoError(t, err)

	return infraJWT.NewJWT(private, private.Public()), infraJWT.NewJWT(nil, private.Public())
}

func token(t *testing.T, signer *infraJWT.JWT, subject string, audience string, expires time.Time) string {
	t.Helper()

	signed, err := signer.Generate(t.Context(), jwt.RegisteredClaims{
		Subject:   subject,
		Audience:  jwt.ClaimStrings{audience},
		ExpiresAt: jwt.NewNumericDate(expires),
	})
	require.NoError(t, err)

	return signed
}

func TestToken(t *testing.T) {
	signer, verifier := keys(t)

	var seen string
	next := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		seen = Subject(r.Context())
		rw.WriteHeader(http.StatusOK)
	})

	call := func(t *testing.T, header string) *httptest.ResponseRecorder {
		t.Helper()

		seen = ""

		request := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
		if len(header) > 0 {
			request.Header.Set("authorization", header)
		}

		recorder := httptest.NewRecorder()
		NewTokenMiddleware(next, verifier).ServeHTTP(recorder, request)

		return recorder
	}

	t.Run("a token this estate signed carries its subject through", func(t *testing.T) {
		valid := token(t, signer, "owner-uuid", auth.AccessToken, time.Now().Add(time.Minute))

		assert.Equal(t, http.StatusOK, call(t, "bearer "+valid).Code)
		assert.Equal(t, "owner-uuid", seen)
	})

	t.Run("what is refused", func(t *testing.T) {
		other, _ := keys(t)

		tests := []struct {
			name   string
			header string
		}{
			{name: "no header at all", header: ""},
			{name: "no token in it", header: "bearer "},
			{name: "not a token", header: "bearer nonsense"},
			{
				name:   "signed by somebody else",
				header: "bearer " + token(t, other, "owner-uuid", auth.AccessToken, time.Now().Add(time.Minute)),
			},
			{
				name:   "expired",
				header: "bearer " + token(t, signer, "owner-uuid", auth.AccessToken, time.Now().Add(-time.Minute)),
			},
			{
				name:   "a refresh token, which is permission to ask rather than to do",
				header: "bearer " + token(t, signer, "owner-uuid", auth.RefreshToken, time.Now().Add(time.Minute)),
			},
			{
				name:   "nobody's token",
				header: "bearer " + token(t, signer, "", auth.AccessToken, time.Now().Add(time.Minute)),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				assert.Equal(t, http.StatusUnauthorized, call(t, test.header).Code)
				assert.Empty(t, seen, "nothing should have reached the handler")
			})
		}
	})

	t.Run("it verifies rather than trusts: no key, nothing passes", func(t *testing.T) {
		_, stranger := keys(t)

		request := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
		request.Header.Set("authorization", "bearer "+token(t, signer, "owner-uuid", auth.AccessToken, time.Now().Add(time.Minute)))

		recorder := httptest.NewRecorder()
		NewTokenMiddleware(next, stranger).ServeHTTP(recorder, request)

		assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	})
}

func TestOptionalToken(t *testing.T) {
	signer, verifier := keys(t)

	var (
		reached bool
		seen    string
	)

	next := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		reached, seen = true, Subject(r.Context())
		rw.WriteHeader(http.StatusOK)
	})

	call := func(t *testing.T, header string) *httptest.ResponseRecorder {
		t.Helper()

		reached, seen = false, ""

		request := httptest.NewRequest(http.MethodGet, "/api/tasks/x/attach", nil)
		if len(header) > 0 {
			request.Header.Set("authorization", header)
		}

		recorder := httptest.NewRecorder()
		NewOptionalTokenMiddleware(next, verifier).ServeHTTP(recorder, request)

		return recorder
	}

	t.Run("nobody gets through as nobody", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, call(t, "").Code)
		assert.True(t, reached, "a snippet belongs to nobody, so nobody may reach it")
		assert.Empty(t, seen)
	})

	t.Run("somebody gets through as themselves", func(t *testing.T) {
		valid := token(t, signer, "owner-uuid", auth.AccessToken, time.Now().Add(time.Minute))

		assert.Equal(t, http.StatusOK, call(t, "bearer "+valid).Code)
		assert.Equal(t, "owner-uuid", seen)
	})

	t.Run("a bad token is refused rather than taken for nobody", func(t *testing.T) {
		expired := token(t, signer, "owner-uuid", auth.AccessToken, time.Now().Add(-time.Minute))

		for _, header := range []string{"bearer nonsense", "bearer " + expired} {
			assert.Equal(t, http.StatusUnauthorized, call(t, header).Code)
			assert.False(t, reached,
				"presenting a bad token is not the same as presenting none")
		}
	})
}

func TestSubject(t *testing.T) {
	assert.Empty(t, Subject(context.Background()), "nothing verified a token, so nobody is asking")
	assert.Equal(t, "owner-uuid", Subject(WithSubject(context.Background(), "owner-uuid")))
}
