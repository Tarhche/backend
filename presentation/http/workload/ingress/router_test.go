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

	router := NewRouter(answer("tasks"), answer("ingress"), "workload.localhost")

	tests := []struct {
		name string
		host string
		want string
	}{
		{name: "a task's hostname", host: "nginx-xkfqz.workload.localhost", want: "tasks"},
		{name: "a task's hostname with a port", host: "nginx-xkfqz.workload.localhost:8021", want: "tasks"},
		{name: "a task's hostname in capitals", host: "Nginx-XKFQZ.Workload.Localhost", want: "tasks"},
		{name: "the ingress's own name", host: "workload-ingress", want: "ingress"},
		{name: "the domain itself names no task", host: "workload.localhost", want: "ingress"},
		{name: "a hostname that only ends in something similar", host: "notworkload.localhost", want: "ingress"},
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
		router := NewRouter(answer("tasks"), answer("ingress"), "")

		request := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
		request.Host = "nginx-xkfqz.workload.localhost"

		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		assert.Equal(t, "ingress", recorder.Header().Get("X-Answered-By"))
	})
}
