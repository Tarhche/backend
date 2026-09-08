package ingress

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRouter(t *testing.T) {
	answer := func(who string) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.Header().Set("X-Answered-By", who)
		})
	}

	router := NewRouter(answer("containers"), answer("ingress"), "runner.localhost")

	tests := []struct {
		name string
		host string
		want string
	}{
		{name: "a container's hostname", host: "nginx-xkfqz.runner.localhost", want: "containers"},
		{name: "a container's hostname with a port", host: "nginx-xkfqz.runner.localhost:8021", want: "containers"},
		{name: "a container's hostname in capitals", host: "Nginx-XKFQZ.Runner.Localhost", want: "containers"},
		{name: "the ingress's own name", host: "runner-ingress", want: "ingress"},
		{name: "the domain itself names no container", host: "runner.localhost", want: "ingress"},
		{name: "a hostname that only ends in something similar", host: "notrunner.localhost", want: "ingress"},
		{name: "an address rather than a name", host: "127.0.0.1:8030", want: "ingress"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
			request.Host = tt.host

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			assert.Equal(t, tt.want, recorder.Header().Get("X-Answered-By"))
		})
	}

	t.Run("no domain at all routes everything to the ingress", func(t *testing.T) {
		router := NewRouter(answer("containers"), answer("ingress"), "")

		request := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
		request.Host = "nginx-xkfqz.runner.localhost"

		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		assert.Equal(t, "ingress", recorder.Header().Get("X-Answered-By"))
	})
}
