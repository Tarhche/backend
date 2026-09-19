package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouter(t *testing.T) {
	t.Parallel()

	answered := false
	handler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		answered = true
		rw.WriteHeader(http.StatusTeapot)
	})

	router := New()
	router.Handle("GET /api/things", handler)
	router.Handle("POST /api/things/{uuid}", handler)

	t.Run("it remembers what it registered, in order", func(t *testing.T) {
		assert.Equal(t, []string{"GET /api/things", "POST /api/things/{uuid}"}, router.Patterns())
	})

	t.Run("what it remembers is a copy", func(t *testing.T) {
		patterns := router.Patterns()
		patterns[0] = "changed"

		assert.Equal(t, "GET /api/things", router.Patterns()[0])
	})

	t.Run("it says which pattern answers a request", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/api/things/an-uuid", nil)

		found, pattern := router.Handler(request)
		require.NotNil(t, found)
		assert.Equal(t, "POST /api/things/{uuid}", pattern)
	})

	t.Run("nothing answers a route that is not there", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/api/nothing", nil)

		_, pattern := router.Handler(request)
		assert.Empty(t, pattern)
	})

	t.Run("it serves what it registered", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/things", nil))

		assert.True(t, answered)
		assert.Equal(t, http.StatusTeapot, recorder.Code)
	})
}
