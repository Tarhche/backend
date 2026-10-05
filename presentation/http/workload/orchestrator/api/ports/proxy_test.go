package ports

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	getendpoint "github.com/khanzadimahdi/testproject/application/workload/orchestrator/getEndpoint"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	memory "github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

func TestProxyHandler(t *testing.T) {
	t.Parallel()

	// what the VM serves, where its engine published it.
	upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(rw, r.Host+" "+r.URL.Path+"?"+r.URL.RawQuery)
	}))
	defer upstream.Close()

	host, published, err := net.SplitHostPort(upstream.Listener.Addr().String())
	require.NoError(t, err)

	// the memory engine hands out host ports from 20000; the proxy is pointed
	// at the upstream through an engine whose endpoints say where it is.
	e := memory.New(memory.WithHost(host))

	_, err = e.Create(t.Context(), vm.Spec{
		ID:      "vm-1",
		Kind:    vm.KindMachine,
		Image:   "ubuntu:24.04",
		Ports:   []port.Port{80},
		Network: vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
		Labels:  map[string]string{vm.LabelPurpose: vm.PurposeVM, vm.LabelSlug: "box-abcde"},
	})
	require.NoError(t, err)

	instance, err := e.Inspect(t.Context(), "vm-1")
	require.NoError(t, err)
	require.Len(t, instance.Endpoints, 1)

	handler := NewProxyHandler(getendpoint.NewUseCase(redirected{Engine: e, to: net.JoinHostPort(host, published)}), slog.New(slog.DiscardHandler))

	mux := http.NewServeMux()
	mux.Handle("/tasks/{slug}/{port}/{path...}", handler)
	mux.Handle("/vms/{slug}/{port}/{path...}", handler)

	testcases := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{name: "a VM's port is reached by its slug", path: "/vms/box-abcde/0/index.html?a=1", wantStatus: http.StatusOK, wantBody: "box-abcde-80.workload.localhost /index.html?a=1"},
		{name: "the tasks' route reaches it too, since slugs are one namespace", path: "/tasks/box-abcde/80/", wantStatus: http.StatusOK, wantBody: "box-abcde-80.workload.localhost /?"},
		{name: "a port it does not expose is not found", path: "/vms/box-abcde/9999/", wantStatus: http.StatusNotFound},
		{name: "a slug this node does not hold is not found", path: "/vms/other-fghij/0/", wantStatus: http.StatusNotFound},
		{name: "a port that is no port is refused", path: "/vms/box-abcde/http/", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			request.Host = "box-abcde-80.workload.localhost"

			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, request)

			assert.Equal(t, tt.wantStatus, recorder.Code)

			if len(tt.wantBody) > 0 {
				assert.Equal(t, tt.wantBody, recorder.Body.String())
			}
		})
	}
}

// redirected is an engine whose endpoints all lead to one address, which is
// where the test's upstream listens.
type redirected struct {
	*memory.Engine
	to string
}

func (r redirected) List(ctx context.Context) ([]vm.Instance, error) {
	instances, err := r.Engine.List(ctx)
	for n := range instances {
		for m := range instances[n].Endpoints {
			instances[n].Endpoints[m].Address = r.to
		}
	}

	return instances, err
}
