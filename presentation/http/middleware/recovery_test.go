package middleware

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRecoveryMiddleware(t *testing.T) {
	t.Run("calls next handler", func(t *testing.T) {
		var (
			expectedReponse = "test tesponse"
		)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := bytes.NewBufferString(expectedReponse).WriteTo(w)
			assert.NoError(t, err)
		})

		middleware := NewRecoveryMiddleware(next, slog.New(slog.NewTextHandler(io.Discard, nil)))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		response := httptest.NewRecorder()

		assert.NotPanics(t, func() {
			middleware.ServeHTTP(response, request)
		})

		assert.Equal(t, expectedReponse, response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("recovers from panic with error value", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic(http.ErrHandlerTimeout)
		})

		middleware := NewRecoveryMiddleware(next, slog.New(slog.NewTextHandler(io.Discard, nil)))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		response := httptest.NewRecorder()

		assert.NotPanics(t, func() {
			middleware.ServeHTTP(response, request)
		})

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Empty(t, response.Body.String())
	})

	t.Run("recovers from panic with non-error value", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("boom")
		})

		middleware := NewRecoveryMiddleware(next, slog.New(slog.NewTextHandler(io.Discard, nil)))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		response := httptest.NewRecorder()

		assert.NotPanics(t, func() {
			middleware.ServeHTTP(response, request)
		})

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Empty(t, response.Body.String())
	})

	t.Run("re-panics http.ErrAbortHandler", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic(http.ErrAbortHandler)
		})

		middleware := NewRecoveryMiddleware(next, slog.New(slog.NewTextHandler(io.Discard, nil)))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		response := httptest.NewRecorder()

		// ErrAbortHandler must propagate so net/http can abort the
		// connection instead of being swallowed as a 500.
		assert.PanicsWithValue(t, http.ErrAbortHandler, func() {
			middleware.ServeHTTP(response, request)
		})

		assert.NotEqual(t, http.StatusInternalServerError, response.Code)
		assert.Empty(t, response.Body.String())
	})
}
