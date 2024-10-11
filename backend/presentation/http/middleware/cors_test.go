package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCorsMiddleware(t *testing.T) {
	t.Run("adds headers", func(t *testing.T) {
		var (
			expectedReponse = "test tesponse"
		)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := bytes.NewBufferString(expectedReponse).WriteTo(w)
			assert.NoError(t, err)
		})

		middleware := NewCORSMiddleware(next)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		response := httptest.NewRecorder()

		middleware.ServeHTTP(response, request)

		assert.Equal(t, "*", response.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "true", response.Header().Get("Access-Control-Allow-Credentials"))
		assert.Equal(t, "POST, GET, OPTIONS, PUT, DELETE", response.Header().Get("Access-Control-Allow-Methods"))
		assert.Equal(t, "Origin, Content-Type, Accept, Authorization", response.Header().Get("Access-Control-Allow-Headers"))

		assert.Equal(t, expectedReponse, response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("prevent next handler on http options", func(t *testing.T) {
		var (
			expectedReponse = "test tesponse"
		)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := bytes.NewBufferString(expectedReponse).WriteTo(w)
			assert.NoError(t, err)
		})

		middleware := NewCORSMiddleware(next)

		request := httptest.NewRequest(http.MethodOptions, "/", nil)
		response := httptest.NewRecorder()

		middleware.ServeHTTP(response, request)

		assert.Equal(t, "*", response.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "true", response.Header().Get("Access-Control-Allow-Credentials"))
		assert.Equal(t, "POST, GET, OPTIONS, PUT, DELETE", response.Header().Get("Access-Control-Allow-Methods"))
		assert.Equal(t, "Origin, Content-Type, Accept, Authorization", response.Header().Get("Access-Control-Allow-Headers"))

		assert.NotEqual(t, expectedReponse, response.Body.String())
	})
}
