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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/cascade"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/kind/kindtest"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	nodesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/nodes"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
	snapshotsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/snapshots"
	tasksMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/tasks"
	vmsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/vms"
	logsMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/logs"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/permissions"
	infraDocker "github.com/khanzadimahdi/testproject/infrastructure/workload/docker"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

// controlPlane is the control plane as it serves VMs, snapshots and
// containers today, with every kind it runs registered as it registers them,
// over stores kept in memory.
type controlPlane struct {
	registry  *kind.Registry[kind.ControlPlaneBinding]
	resources *resourcesMemory.Repository
	vms       *ControlPlaneVMs
}

func served(t *testing.T) *controlPlane {
	t.Helper()

	registry, err := NewControlPlaneRegistry()
	require.NoError(t, err)

	resources := resourcesMemory.NewRepository()

	vms, err := NewControlPlaneVMs(configs.NewWorkloadControlPlane(), ControlPlaneVMStores{
		VMs:       vmsMemory.NewRepository(),
		Snapshots: snapshotsMemory.NewRepository(),
		Children:  cascade.New(registry, resources),
		Nodes:     nodesMemory.NewRepository(),
		Tasks:     tasksMemory.NewRepository(),
		TaskLogs:  logsMock.NewInMemoryRepository(),
	}, nil, &messagingMock.Recorder{}, slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	require.NoError(t, RegisterControlPlaneKinds(registry, vms, resources))

	return &controlPlane{registry: registry, resources: resources, vms: vms}
}

func kindsOf(registry *kind.Registry[kind.ControlPlaneBinding], resources *resourcesMemory.Repository) *ControlPlaneKinds {
	return NewControlPlaneKinds(
		registry,
		ControlPlaneKindStores{Resources: resources, Nodes: nodesMemory.NewRepository()},
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

	t.Run("stacks are served under their plural beside every route served today, and nothing of those changes", func(t *testing.T) {
		t.Parallel()

		plane := served(t)
		assert.Equal(t, []string{stackKind.Name}, kindNames(plane.registry.Descriptors()))

		kinds := kindsOf(plane.registry, plane.resources)

		mux := http.NewServeMux()
		plane.vms.Route(mux)
		require.NoError(t, kinds.Route(mux), "no route of today's is taken")

		status, body := get(t, mux, "/api/kinds")
		require.Equal(t, http.StatusOK, status)

		var described struct {
			Items []kind.Descriptor `json:"items"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &described))
		require.Len(t, described.Items, 1)
		assert.Equal(t, "stacks", described.Items[0].Plural)

		status, body = get(t, mux, "/api/stacks")
		require.Equal(t, http.StatusOK, status)
		assert.JSONEq(t, `{"items":[],"pagination":{"total_pages":0,"current_page":1}}`, body)

		status, _ = get(t, mux, "/api/vms")
		assert.Equal(t, http.StatusOK, status, "today's VMs are served as they were")

		assert.Equal(t, []string{kind.ResultName}, slices.Collect(maps.Keys(kinds.Subscribers)))
		for subject := range kinds.Subscribers {
			assert.NotContains(t, plane.vms.Subscribers, subject, "no subject of today's is heard twice")
		}

		assert.NotContains(t, plane.vms.Subscribers, "workloadStackCompleted", "a stack's results are every kind's results")
		assert.NotContains(t, plane.vms.Subscribers, "workloadStackFailed")

		assert.NoError(t, kinds.Reconcile.Execute(context.Background()), "a pass over no stack does nothing")
	})

	t.Run("a kind registered over routes served today is refused when the control plane is put together", func(t *testing.T) {
		t.Parallel()

		d := kindstest.Descriptor()
		d.Plural = "vms"

		registry := kind.NewRegistry[kind.ControlPlaneBinding]()
		require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](d, &kindstest.Fans{})))

		mux := http.NewServeMux()
		served(t).vms.Route(mux)

		assert.Error(t, kindsOf(registry, resourcesMemory.NewRepository()).Route(mux))
	})

	t.Run("a kind registered is served under its plural, and its results heard", func(t *testing.T) {
		t.Parallel()

		kinds := kindsOf(kindstest.Registry(&kindstest.Fans{Node: kindstest.NodeName}), resourcesMemory.NewRepository())

		mux := http.NewServeMux()
		require.NoError(t, kinds.Route(mux))

		status, body := get(t, mux, "/api/fans")
		require.Equal(t, http.StatusOK, status)
		assert.JSONEq(t, `{"items":[],"pagination":{"total_pages":0,"current_page":1}}`, body)

		result, err := json.Marshal(kind.Result{ID: "command-1", Kind: kindstest.Kind, UUID: "fan-uuid", Action: "start", OK: true})
		require.NoError(t, err)
		assert.NoError(t, kinds.Subscribers[kind.ResultName].Handle(context.Background(), result), "a result for a resource that is gone is never asked for again")
	})
}

// TestConformance holds every kind the services register, as they register
// them, to the rules every kind keeps: what a kind declares, a strategy
// wherever one of its actions runs, and permissions that exist, with a name.
func TestConformance(t *testing.T) {
	t.Parallel()

	engine := memory.New()

	nodes, err := nodeKinds(NodeKindDependencies{
		Engine:         engine,
		Daemons:        infraDocker.NewDaemons(engine, time.Second, slog.New(slog.DiscardHandler)),
		CommandTimeout: time.Minute,
	})
	require.NoError(t, err)

	ingress, err := ingressKinds()
	require.NoError(t, err)

	services := kind.Services{
		ControlPlane: served(t).registry,
		Node:         nodes,
		Ingress:      ingress,
	}

	kindtest.Conformance(t, services, permissions.NewRepository())

	assert.Equal(t, []string{stackKind.Name}, kindNames(services.Descriptors()), "every kind the services run")
}

func kindNames(descriptors []kind.Descriptor) []string {
	names := make([]string, len(descriptors))
	for i, d := range descriptors {
		names[i] = d.Name
	}

	return names
}
