// Package workloadhttptest is what the dashboard's workload handlers are
// tested with: both sets of routes on one mux, and requests made the way a
// signed-in caller makes them.
package workloadhttptest

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

// Set is one of the two sets a workload route is served in, and whose things
// it asks for.
type Set struct {
	Name string

	// Prefix is where the set's routes are, as the blog registers them.
	Prefix string

	Owner workload.Owner

	// OwnerUUID is what the workload is asked with when the caller is
	// workloadtest.OwnerUUID.
	OwnerUUID string
}

// Sets are the workload's routes, over anybody's things, and the my routes,
// over the caller's own.
var Sets = []Set{
	{Name: "the workload's routes", Prefix: "/api/dashboard/workload", Owner: workload.Anybody, OwnerUUID: ""},
	{Name: "the my routes", Prefix: "/api/dashboard/my/workload", Owner: workload.Caller, OwnerUUID: workloadtest.OwnerUUID},
}

// Request is a request from workloadtest.OwnerUUID, signed in as the
// authentication middleware would have left them.
func Request(method string, target string, body string) *http.Request {
	var payload io.Reader
	if len(body) > 0 {
		payload = strings.NewReader(body)
	}

	request := httptest.NewRequest(method, target, payload)

	return request.WithContext(auth.ToContext(request.Context(), &user.User{UUID: workloadtest.OwnerUUID}))
}

// Serve answers a request with mux, as the blog would once it was let in.
func Serve(mux http.Handler, method string, target string, body string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, Request(method, target, body))

	return response
}
