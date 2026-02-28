package openapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpenAPIHandler_ServeHTTP(t *testing.T) {
	t.Run("delegates to underlying handler", func(t *testing.T) {
		called := false
		h := &OpenAPIHandler{
			openAPI: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusTeapot)
			}),
		}

		req := httptest.NewRequest(http.MethodGet, "/openapi/whatever", nil)
		resp := httptest.NewRecorder()

		h.ServeHTTP(resp, req)

		assert.True(t, called, "underlying handler should be called")
		assert.Equal(t, http.StatusTeapot, resp.Code)
	})
}

func TestNewOpenAPIHandler(t *testing.T) {
	h := NewOpenAPIHandler()
	assert.NotNil(t, h)

	// verify it satisfies the http.Handler interface at compile time
	var _ http.Handler = h
}
