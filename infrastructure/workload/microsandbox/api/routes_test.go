package api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

const (
	runID  = "01926f3a8c4b7d2e9f1a3b5c7d9e0f12"
	execID = "01926f3c2e3f7b5c9d6e7f8a9b0c1d2e"
)

// routes pins every route, with a request it answers and the wildcards that
// request carries.
var routes = map[string]struct {
	route   string
	pattern string
	method  string
	target  string
	run     string
	exec    string
}{
	"info": {
		route: api.RouteInfo, pattern: "GET /v1/info",
		method: http.MethodGet, target: "/v1/info",
	},
	"list runs": {
		route: api.RouteListRuns, pattern: "GET /v1/runs",
		method: http.MethodGet, target: "/v1/runs?node=orchestrator-01&task=4f1c2a9e-8d3b-4c7a-9e2f-1a2b3c4d5e6f&slug=web-xkfqz",
	},
	"create a run": {
		route: api.RouteCreateRun, pattern: "POST /v1/runs",
		method: http.MethodPost, target: "/v1/runs",
	},
	"get a run": {
		route: api.RouteGetRun, pattern: "GET /v1/runs/{id}",
		method: http.MethodGet, target: "/v1/runs/" + runID, run: runID,
	},
	"delete a run": {
		route: api.RouteDeleteRun, pattern: "DELETE /v1/runs/{id}",
		method: http.MethodDelete, target: "/v1/runs/" + runID, run: runID,
	},
	"start a run": {
		route: api.RouteStartRun, pattern: "POST /v1/runs/{id}/start",
		method: http.MethodPost, target: "/v1/runs/" + runID + "/start", run: runID,
	},
	"stop a run": {
		route: api.RouteStopRun, pattern: "POST /v1/runs/{id}/stop",
		method: http.MethodPost, target: "/v1/runs/" + runID + "/stop", run: runID,
	},
	"kill a run": {
		route: api.RouteKillRun, pattern: "POST /v1/runs/{id}/kill",
		method: http.MethodPost, target: "/v1/runs/" + runID + "/kill", run: runID,
	},
	"restart a run": {
		route: api.RouteRestartRun, pattern: "POST /v1/runs/{id}/restart",
		method: http.MethodPost, target: "/v1/runs/" + runID + "/restart", run: runID,
	},
	"a run's logs": {
		route: api.RouteRunLogs, pattern: "GET /v1/runs/{id}/logs",
		method: http.MethodGet, target: "/v1/runs/" + runID + "/logs?since=2026-10-04T09%3A30%3A02.000000001Z&follow=true", run: runID,
	},
	"a run's stats": {
		route: api.RouteRunStats, pattern: "GET /v1/runs/{id}/stats",
		method: http.MethodGet, target: "/v1/runs/" + runID + "/stats", run: runID,
	},
	"exec in a run": {
		route: api.RouteExec, pattern: "GET /v1/runs/{id}/exec",
		method: http.MethodGet, target: "/v1/runs/" + runID + "/exec", run: runID,
	},
	"end an exec": {
		route: api.RouteEndExec, pattern: "POST /v1/runs/{id}/execs/{exec}/end",
		method: http.MethodPost, target: "/v1/runs/" + runID + "/execs/" + execID + "/end", run: runID, exec: execID,
	},
	"a node's stats": {
		route: api.RouteNodeStats, pattern: "GET /v1/stats",
		method: http.MethodGet, target: "/v1/stats?node=orchestrator-01",
	},
	"pull an image": {
		route: api.RoutePullImage, pattern: "POST /v1/images/pull",
		method: http.MethodPost, target: "/v1/images/pull",
	},
}

func TestRoutes(t *testing.T) {
	t.Parallel()

	// one mux holding every route, as the service's will: it refuses a
	// pattern it cannot parse and two that would answer the same request.
	mux := http.NewServeMux()
	for _, tt := range routes {
		mux.HandleFunc(tt.route, func(rw http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(rw, "%s|%s|%s", r.Pattern, r.PathValue(api.WildcardRun), r.PathValue(api.WildcardExec))
		})
	}

	for name, tt := range routes {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.pattern, tt.route)

			_, path, _ := strings.Cut(tt.route, " ")
			assert.True(t, strings.HasPrefix(path, "/v"+api.Version+"/"), "%s does not carry the version", tt.route)

			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(tt.method, tt.target, nil))

			assert.Equal(t, http.StatusOK, response.Code)
			assert.Equal(t, tt.route+"|"+tt.run+"|"+tt.exec, response.Body.String())
		})
	}
}

func TestQueryAndWildcards(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "node", api.QueryNode)
	assert.Equal(t, "task", api.QueryTask)
	assert.Equal(t, "slug", api.QuerySlug)
	assert.Equal(t, "since", api.QuerySince)
	assert.Equal(t, "follow", api.QueryFollow)
	assert.Equal(t, "id", api.WildcardRun)
	assert.Equal(t, "exec", api.WildcardExec)
}

func TestWire(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "1", api.Version)
	assert.Equal(t, "application/x-ndjson", api.ContentTypeNDJSON)
	assert.Equal(t, "workload-microsandbox.v1", api.ExecSubprotocol)
	assert.Equal(t, byte(1), api.OutputStdout)
	assert.Equal(t, byte(2), api.OutputStderr)
	assert.Equal(t, 65536, api.MaxLogContent)
}
