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

	getresourceendpoint "github.com/khanzadimahdi/testproject/application/workload/orchestrator/getResourceEndpoint"
	orchestratorTasks "github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/task"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/task/vmruntime"
	memory "github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

func TestProxyHandler(t *testing.T) {
	t.Parallel()

	// what the task serves, where its engine published it.
	upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(rw, r.Host+" "+r.URL.Path+"?"+r.URL.RawQuery)
	}))
	defer upstream.Close()

	host, published, err := net.SplitHostPort(upstream.Listener.Addr().String())
	require.NoError(t, err)

	// the memory engine hands out host ports from 20000; the proxy is pointed
	// at the upstream through an engine whose endpoints say where it is.
	e := memory.New(memory.WithHost(host))

	for id, held := range map[string]struct {
		purpose string
		slug    string
	}{
		"execution-1": {purpose: vm.PurposeTask, slug: "snippet-abcde"},
		"vm-1":        {purpose: vm.PurposeVM, slug: "box-abcde"},
	} {
		_, err = e.Create(t.Context(), vm.Spec{
			ID:      id,
			Kind:    vm.KindMachine,
			Image:   "ubuntu:24.04",
			Ports:   []port.Port{80},
			Network: vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
			Labels:  map[string]string{vm.LabelPurpose: held.purpose, vm.LabelSlug: held.slug, vm.LabelTask: id},
		})
		require.NoError(t, err)
	}

	// the code runner's tasks, whose ports are served as any kind's are.
	runtime := vmruntime.New(redirected{Engine: e, to: net.JoinHostPort(host, published)}, slog.New(slog.DiscardHandler))

	kinds := kind.NewRegistry[kind.NodeBinding]()
	require.NoError(t, kinds.Register(kind.BindNode[taskKind.Spec, taskKind.Status](taskKind.Descriptor(), orchestratorTasks.New(runtime, "workload-orchestrator-01"))))

	handler := NewResourceProxyHandler(getresourceendpoint.NewUseCase(kinds), taskKind.Name, slog.New(slog.DiscardHandler))

	mux := http.NewServeMux()
	mux.Handle("/tasks/{slug}/{port}/{path...}", handler)

	testcases := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{name: "a task's port is reached by its slug", path: "/tasks/snippet-abcde/0/index.html?a=1", wantStatus: http.StatusOK, wantBody: "snippet-abcde-80.workload.localhost /index.html?a=1"},
		{name: "and the port named", path: "/tasks/snippet-abcde/80/", wantStatus: http.StatusOK, wantBody: "snippet-abcde-80.workload.localhost /?"},
		{name: "a port it does not expose is not found", path: "/tasks/snippet-abcde/9999/", wantStatus: http.StatusNotFound},
		{name: "a slug this node does not hold is not found", path: "/tasks/other-fghij/0/", wantStatus: http.StatusNotFound},
		{name: "nor is a vm's, whose ports are its kind's to serve", path: "/tasks/box-abcde/80/", wantStatus: http.StatusNotFound},
		{name: "a port that is no port is refused", path: "/tasks/snippet-abcde/http/", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			request.Host = "snippet-abcde-80.workload.localhost"

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
