package workload

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	nodesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/nodes"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
	snapshotsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/snapshots"
	stacksMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/stacks"
	tasksMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/tasks"
	vmsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/vms"
	logsMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/logs"
)

// today is the control plane as it serves VMs, snapshots, containers and
// stacks today, before any of them is a kind.
func today(t *testing.T) *ControlPlaneVMs {
	t.Helper()

	vms, err := NewControlPlaneVMs(configs.NewWorkloadControlPlane(), ControlPlaneVMStores{
		VMs:       vmsMemory.NewRepository(),
		Snapshots: snapshotsMemory.NewRepository(),
		Stacks:    stacksMemory.NewRepository(),
		Nodes:     nodesMemory.NewRepository(),
		Tasks:     tasksMemory.NewRepository(),
		TaskLogs:  logsMock.NewInMemoryRepository(),
	}, nil, &messagingMock.Recorder{}, slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	return vms
}

func kindsOf(registry *kind.Registry[kind.ControlPlaneBinding]) *ControlPlaneKinds {
	return NewControlPlaneKinds(
		registry,
		ControlPlaneKindStores{Resources: resourcesMemory.NewRepository(), Nodes: nodesMemory.NewRepository()},
		&messagingMock.Requester{},
		&messagingMock.Recorder{},
		slog.New(slog.DiscardHandler),
	)
}

func get(t *testing.T, handler http.Handler, target string) (int, string) {
	t.Helper()

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))

	return recorder.Code, recorder.Body.String()
}

func TestNewControlPlaneKinds(t *testing.T) {
	t.Parallel()

	t.Run("with no kind registered yet, the kinds are served beside every route served today, and nothing of those changes", func(t *testing.T) {
		t.Parallel()

		registry, err := NewControlPlaneRegistry()
		require.NoError(t, err)
		assert.Empty(t, registry.Descriptors())

		vms := today(t)
		kinds := kindsOf(registry)

		mux := http.NewServeMux()
		vms.Route(mux)
		require.NoError(t, kinds.Route(mux), "no route of today's is taken")

		status, body := get(t, mux, "/api/kinds")
		require.Equal(t, http.StatusOK, status)
		assert.JSONEq(t, `{"items":[]}`, body)

		status, _ = get(t, mux, "/api/vms")
		assert.Equal(t, http.StatusOK, status, "today's VMs are served as they were")

		assert.Equal(t, []string{kind.ResultName}, slices.Collect(maps.Keys(kinds.Subscribers)))
		for subject := range kinds.Subscribers {
			assert.NotContains(t, vms.Subscribers, subject, "no subject of today's is heard twice")
		}

		assert.NoError(t, kinds.Reconcile.Execute(context.Background()), "a pass over no kind does nothing")
	})

	t.Run("a kind registered over routes served today is refused when the control plane is put together", func(t *testing.T) {
		t.Parallel()

		d := kindstest.Descriptor()
		d.Plural = "vms"

		registry := kind.NewRegistry[kind.ControlPlaneBinding]()
		require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](d, &kindstest.Fans{})))

		mux := http.NewServeMux()
		today(t).Route(mux)

		assert.Error(t, kindsOf(registry).Route(mux))
	})

	t.Run("a kind registered is served under its plural, and its results heard", func(t *testing.T) {
		t.Parallel()

		kinds := kindsOf(kindstest.Registry(&kindstest.Fans{Node: kindstest.NodeName}))

		mux := http.NewServeMux()
		require.NoError(t, kinds.Route(mux))

		status, body := get(t, mux, "/api/fans")
		require.Equal(t, http.StatusOK, status)
		assert.JSONEq(t, `{"items":[],"pagination":{"total_pages":0,"current_page":1}}`, body)

		status, body = get(t, mux, "/api/kinds")
		require.Equal(t, http.StatusOK, status)

		var described struct {
			Items []kind.Descriptor `json:"items"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &described))
		require.Len(t, described.Items, 1)
		assert.Equal(t, kindstest.Plural, described.Items[0].Plural)

		result, err := json.Marshal(kind.Result{ID: "command-1", Kind: kindstest.Kind, UUID: "fan-uuid", Action: "start", OK: true})
		require.NoError(t, err)
		assert.NoError(t, kinds.Subscribers[kind.ResultName].Handle(context.Background(), result), "a result for a resource that is gone is never asked for again")
	})
}
