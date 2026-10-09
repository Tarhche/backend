package vmhost

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/vmhost/wire"
)

// running is a server whose engine holds a running VM named vm-1.
func running(t *testing.T, engine vm.Engine) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(NewServer(engine, slog.New(slog.DiscardHandler)))
	t.Cleanup(server.Close)

	_, err := engine.Create(t.Context(), vm.Spec{ID: "vm-1", Image: "ubuntu:24.04"})
	require.NoError(t, err)

	return server
}

// ask makes a request and reads what it was answered.
func ask(t *testing.T, request *http.Request) (*http.Response, []byte) {
	t.Helper()

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	return response, body
}

func request(t *testing.T, method string, url string, body string) *http.Request {
	t.Helper()

	r, err := http.NewRequestWithContext(t.Context(), method, url, strings.NewReader(body))
	require.NoError(t, err)

	return r
}

// TestServer_Refuses holds every request the API does not have, or cannot
// read, to an answer in the shape errors are read in.
func TestServer_Refuses(t *testing.T) {
	t.Parallel()

	server := running(t, memory.New())

	spec, err := wire.EncodeHeader(wire.NewSpec(vm.Spec{}))
	require.NoError(t, err)

	testcases := []struct {
		name    string
		request func(t *testing.T) *http.Request

		wantStatus int
		wantCode   wire.Code
	}{
		{
			name:       "a route there is not",
			request:    func(t *testing.T) *http.Request { return request(t, http.MethodGet, server.URL+"/v2/info", "") },
			wantStatus: http.StatusBadRequest,
			wantCode:   wire.CodeInvalid,
		},
		{
			name:       "a route asked with another method",
			request:    func(t *testing.T) *http.Request { return request(t, http.MethodDelete, server.URL+wire.PathInfo, "") },
			wantStatus: http.StatusBadRequest,
			wantCode:   wire.CodeInvalid,
		},
		{
			name:       "a vm with no name",
			request:    func(t *testing.T) *http.Request { return request(t, http.MethodGet, server.URL+wire.PathVMs+"/", "") },
			wantStatus: http.StatusBadRequest,
			wantCode:   wire.CodeInvalid,
		},
		{
			name:       "a spec that is not JSON",
			request:    func(t *testing.T) *http.Request { return request(t, http.MethodPost, server.URL+wire.PathVMs, "{") },
			wantStatus: http.StatusBadRequest,
			wantCode:   wire.CodeInvalid,
		},
		{
			name: "a vm created under no id",
			request: func(t *testing.T) *http.Request {
				return request(t, http.MethodPost, server.URL+wire.PathVMs, `{"image":"x"}`)
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   wire.CodeInvalid,
		},
		{
			name: "a spec for another vm than the path's",
			request: func(t *testing.T) *http.Request {
				return request(t, http.MethodPut, server.URL+wire.PathVM("vm-1"), `{"id":"vm-2"}`)
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   wire.CodeInvalid,
		},
		{
			name: "a restore that does not say what it restores",
			request: func(t *testing.T) *http.Request {
				return request(t, http.MethodPost, server.URL+wire.PathRestore, "archive")
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   wire.CodeInvalid,
		},
		{
			name: "a restore whose spec is not base64",
			request: func(t *testing.T) *http.Request {
				r := request(t, http.MethodPost, server.URL+wire.PathRestore, "archive")
				r.Header.Set(wire.SpecHeader, "{not base64}")

				return r
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   wire.CodeInvalid,
		},
		{
			name: "a restore under no id",
			request: func(t *testing.T) *http.Request {
				r := request(t, http.MethodPost, server.URL+wire.PathRestore, "archive")
				r.Header.Set(wire.SpecHeader, spec)

				return r
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   wire.CodeInvalid,
		},
		{
			name: "an exec that does not upgrade its connection",
			request: func(t *testing.T) *http.Request {
				return request(t, http.MethodPost, server.URL+wire.PathVM("vm-1", wire.ActionExec), `{"command":["sh"]}`)
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   wire.CodeInvalid,
		},
		{
			name: "an exec of no command",
			request: func(t *testing.T) *http.Request {
				r := request(t, http.MethodPost, server.URL+wire.PathVM("vm-1", wire.ActionExec), `{"command":[]}`)
				r.Header.Set("Connection", "Upgrade")
				r.Header.Set("Upgrade", wire.ExecProtocol)

				return r
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   wire.CodeInvalid,
		},
		{
			name: "a log since a moment that is not a time",
			request: func(t *testing.T) *http.Request {
				return request(t, http.MethodGet, server.URL+wire.PathVM("vm-1", wire.ActionLogs)+"?since=yesterday", "")
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   wire.CodeInvalid,
		},
		{
			name: "a log's tail that is not a number",
			request: func(t *testing.T) *http.Request {
				return request(t, http.MethodGet, server.URL+wire.PathVM("vm-1", wire.ActionLogs)+"?tail=-1", "")
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   wire.CodeInvalid,
		},
		{
			name: "a vm that is not there",
			request: func(t *testing.T) *http.Request {
				return request(t, http.MethodGet, server.URL+wire.PathVM("missing"), "")
			},
			wantStatus: http.StatusNotFound,
			wantCode:   wire.CodeNotFound,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			response, body := ask(t, tc.request(t))

			assert.Equal(t, tc.wantStatus, response.StatusCode)
			assert.Equal(t, "application/json", response.Header.Get("Content-Type"))

			var answered wire.Error
			require.NoError(t, json.Unmarshal(body, &answered), "%s", body)

			assert.Equal(t, tc.wantCode, answered.Code)
			assert.NotEmpty(t, answered.Message, "it says what was wrong")
		})
	}
}

func TestServer_Statuses(t *testing.T) {
	t.Parallel()

	server := running(t, memory.New())

	spec := `{"id":"vm-2","kind":"machine","image":"ubuntu:24.04"}`

	for _, step := range []struct {
		method string
		path   string
		body   string
		want   int
	}{
		{method: http.MethodPost, path: wire.PathVMs, body: spec, want: http.StatusCreated},
		{method: http.MethodPost, path: wire.PathVM("vm-2", wire.ActionStop), want: http.StatusNoContent},
		{method: http.MethodPost, path: wire.PathVM("vm-2", wire.ActionStart), want: http.StatusNoContent},
		{method: http.MethodPost, path: wire.PathVM("vm-2", wire.ActionRestart), want: http.StatusNoContent},
		{method: http.MethodPut, path: wire.PathVM("vm-2"), body: `{"image":"ubuntu:24.04"}`, want: http.StatusOK},
		{method: http.MethodGet, path: wire.PathVM("vm-2", wire.ActionStats), want: http.StatusOK},
		{method: http.MethodGet, path: wire.PathVM("vm-2", wire.ActionLogs) + "?tail=10&since=2026-10-05T12:00:00Z", want: http.StatusOK},
		{method: http.MethodDelete, path: wire.PathVM("vm-2"), want: http.StatusNoContent},
		{method: http.MethodDelete, path: wire.PathVM("vm-2"), want: http.StatusNoContent},
		{method: http.MethodGet, path: wire.PathInfo, want: http.StatusOK},
	} {
		response, body := ask(t, request(t, step.method, server.URL+step.path, step.body))
		assert.Equal(t, step.want, response.StatusCode, "%s %s: %s", step.method, step.path, body)
	}
}

// TestServer_SnapshotTrailers holds a snapshot to saying what it wrote after
// the archive, where a client reads it.
func TestServer_SnapshotTrailers(t *testing.T) {
	t.Parallel()

	engine := memory.New()
	server := running(t, engine)

	require.NoError(t, engine.SetDisk("vm-1", []byte("a disk")))

	response, body := ask(t, request(t, http.MethodPost, server.URL+wire.PathVM("vm-1", wire.ActionSnapshot), ""))

	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "application/octet-stream", response.Header.Get("Content-Type"))
	assert.Empty(t, response.Trailer.Get(wire.ErrorTrailer))

	var written wire.Archive
	require.NoError(t, wire.DecodeHeader(response.Trailer.Get(wire.ArchiveTrailer), &written))

	assert.Equal(t, int64(len(body)), written.Size)
	assert.Equal(t, memory.Name+"/"+memory.Version, written.Engine)
}

// panicking is an engine that panics when it is asked what it offers.
type panicking struct {
	vm.Engine
}

func (panicking) Info(context.Context) (vm.Info, error) {
	panic("the engine fell over")
}

func TestServer_Panics(t *testing.T) {
	t.Parallel()

	server := running(t, panicking{Engine: memory.New()})

	response, body := ask(t, request(t, http.MethodGet, server.URL+wire.PathInfo, ""))

	assert.Equal(t, http.StatusInternalServerError, response.StatusCode)

	var answered wire.Error
	require.NoError(t, json.Unmarshal(body, &answered))
	assert.Equal(t, wire.CodeInternal, answered.Code)
	assert.Contains(t, answered.Message, "the engine fell over")

	response, _ = ask(t, request(t, http.MethodGet, server.URL+wire.PathVMs, ""))
	assert.Equal(t, http.StatusOK, response.StatusCode, "and it goes on serving")
}
