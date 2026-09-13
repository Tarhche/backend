package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
)

func TestTokenFromSubprotocol(t *testing.T) {
	signer, verifier := keys(t)
	valid := token(t, signer, "owner-uuid", auth.AccessToken, time.Now().Add(time.Minute))

	var seen string
	next := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		seen = Subject(r.Context())
		rw.WriteHeader(http.StatusOK)
	})

	call := func(t *testing.T, protocols string) *httptest.ResponseRecorder {
		t.Helper()

		seen = ""

		request := httptest.NewRequest(http.MethodGet, "/api/tasks/x/attach", nil)
		if len(protocols) > 0 {
			request.Header.Set("sec-websocket-protocol", protocols)
		}

		recorder := httptest.NewRecorder()
		NewTokenMiddleware(next, verifier).ServeHTTP(recorder, request)

		return recorder
	}

	t.Run("a browser carries its token as a subprotocol", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, call(t, "bearer, "+valid).Code)
		assert.Equal(t, "owner-uuid", seen)
	})

	t.Run("and it is the same token, so a bad one is still refused", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized, call(t, "bearer, nonsense").Code)
		assert.Empty(t, seen)
	})

	t.Run("a subprotocol that is not a token is not one", func(t *testing.T) {
		for _, offered := range []string{"chat", "bearer", valid, "bearer, " + valid + ", extra"} {
			assert.Equal(t, http.StatusUnauthorized, call(t, offered).Code, offered)
		}
	})

	t.Run("an Authorization header still wins", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/api/tasks/x/attach", nil)
		request.Header.Set("authorization", "bearer "+valid)
		request.Header.Set("sec-websocket-protocol", "bearer, nonsense")

		recorder := httptest.NewRecorder()
		NewTokenMiddleware(next, verifier).ServeHTTP(recorder, request)

		assert.Equal(t, http.StatusOK, recorder.Code)
	})
}
