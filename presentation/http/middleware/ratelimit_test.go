package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRateLimitMiddleware(t *testing.T) {
	t.Run("calls next handler", func(t *testing.T) {
		var (
			expectedReponse               = "test tesponse"
			tokens          uint64        = 1
			duration        time.Duration = 1 * time.Minute
		)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := bytes.NewBufferString(expectedReponse).WriteTo(w)
			assert.NoError(t, err)
		})

		middleware := NewRateLimitMiddleware(next, tokens, duration)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		response := httptest.NewRecorder()

		middleware.ServeHTTP(response, request)

		assert.Equal(t, "1", response.Header().Get("x-ratelimit-limit"))
		assert.Equal(t, "0", response.Header().Get("x-ratelimit-remaining"))
		assert.Equal(t, expectedReponse, response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("limit", func(t *testing.T) {
		var (
			expectedReponse               = "test tesponse"
			tokens          uint64        = 1
			duration        time.Duration = 1 * time.Minute
		)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := bytes.NewBufferString(expectedReponse).WriteTo(w)
			assert.NoError(t, err)
		})

		middleware := NewRateLimitMiddleware(next, tokens, duration)

		// first call
		request1 := httptest.NewRequest(http.MethodGet, "/", nil)
		response1 := httptest.NewRecorder()

		middleware.ServeHTTP(response1, request1)

		assert.Equal(t, "1", response1.Header().Get("x-ratelimit-limit"))
		assert.Equal(t, "0", response1.Header().Get("x-ratelimit-remaining"))
		assert.Equal(t, expectedReponse, response1.Body.String())
		assert.Equal(t, http.StatusOK, response1.Code)

		// second call
		request2 := httptest.NewRequest(http.MethodGet, "/", nil)
		response2 := httptest.NewRecorder()

		middleware.ServeHTTP(response2, request2)

		assert.Equal(t, "1", response2.Header().Get("x-ratelimit-limit"))
		assert.Equal(t, "0", response2.Header().Get("x-ratelimit-remaining"))
		assert.NotEqual(t, expectedReponse, response2.Body.String())
		assert.Equal(t, http.StatusTooManyRequests, response2.Code)
	})
}
