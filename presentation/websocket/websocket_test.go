package websocket

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWebsocketHandler(t *testing.T) {
	t.Parallel()

	t.Run("passes the request to the wrapped handler", func(t *testing.T) {
		t.Parallel()

		var received *http.Request
		wrapped := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			received = r
			rw.WriteHeader(http.StatusSwitchingProtocols)
		})

		request := httptest.NewRequest(http.MethodGet, "/api/ws", nil)
		recorder := httptest.NewRecorder()

		NewWebsocketHandler(wrapped).ServeHTTP(recorder, request)

		assert.Same(t, request, received)
		assert.Equal(t, http.StatusSwitchingProtocols, recorder.Code)
	})
}
