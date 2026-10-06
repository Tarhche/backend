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

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/kind/kindtest"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	snapshotEvents "github.com/khanzadimahdi/testproject/domain/workload/snapshot/events"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	nodesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/nodes"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
	snapshotsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/snapshots"
	logsMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/logs"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/permissions"
	infraDocker "github.com/khanzadimahdi/testproject/infrastructure/workload/docker"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/task/vmruntime"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

// controlPlane is the control plane as the serve command wires it, with
// every kind it runs registered as it registers them, over stores kept in
// memory.
type controlPlane struct {
	workload  *ControlPlaneWorkload
	resources *resourcesMemory.Repository
}

func served(t *testing.T) *controlPlane {
	t.Helper()

	resources := resourcesMemory.NewRepository()

	workload, err := NewControlPlaneWorkload(configs.NewWorkloadControlPlane(), ControlPlaneStores{
		Resources: resources,
		Snapshots: snapshotsMemory.NewRepository(),
		Nodes:     nodesMemory.NewRepository(),
		TaskLogs:  logsMock.NewInMemoryRepository(),
	}, nil, &messagingMock.Recorder{}, slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	return &controlPlane{workload: workload, resources: resources}
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

	t.Run("vms, stacks and tasks are served under their plurals beside what is not a kind yet", func(t *testing.T) {
		t.Parallel()

		plane := served(t)
		assert.Equal(t, []string{vmKind.Name, stackKind.Name, taskKind.Name}, kindNames(plane.workload.Registry.Descriptors()))

		mux := http.NewServeMux()
		require.NoError(t, plane.workload.Route(mux), "no kind takes a route of what is not a kind yet")

		status, body := get(t, mux, "/api/kinds")
		require.Equal(t, http.StatusOK, status)

		var described struct {
			Items []kind.Descriptor `json:"items"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &described))
		require.Len(t, described.Items, 3)
		assert.Equal(t, "vms", described.Items[0].Plural)
		assert.Equal(t, "stacks", described.Items[1].Plural)
		assert.Equal(t, "tasks", described.Items[2].Plural)

		for _, plural := range []string{"vms", "stacks", "tasks"} {
			status, body = get(t, mux, "/api/"+plural)
			require.Equal(t, http.StatusOK, status)
			assert.JSONEq(t, `{"items":[],"pagination":{"total_pages":0,"current_page":1}}`, body)
		}

		status, _ = get(t, mux, "/api/snapshots")
		assert.Equal(t, http.StatusOK, status, "a vm's snapshots are served as they were")

		status, _ = get(t, mux, "/api/containers")
		assert.Equal(t, http.StatusOK, status, "and so are the containers in docker vms")

		subjects := slices.Collect(maps.Keys(plane.workload.Subscribers))
		assert.ElementsMatch(t, []string{kind.ResultName, snapshotEvents.SnapshotCompletedName, snapshotEvents.SnapshotFailedName}, subjects, "a vm's results are every kind's results")

		assert.NoError(t, plane.workload.Reconcile.Execute(context.Background()), "a pass over nothing does nothing")
	})

	t.Run("a kind registered over routes served is refused when the control plane is put together", func(t *testing.T) {
		t.Parallel()

		d := kindstest.Descriptor()
		d.Plural = "snapshots"

		registry := kind.NewRegistry[kind.ControlPlaneBinding]()
		require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](d, &kindstest.Fans{})))

		mux := http.NewServeMux()
		require.NoError(t, served(t).workload.Route(mux))

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
		Tasks:          vmruntime.New(engine, slog.New(slog.DiscardHandler)),
		Daemons:        infraDocker.NewDaemons(engine, time.Second, slog.New(slog.DiscardHandler)),
		NodeName:       "workload-orchestrator-01",
		CommandTimeout: time.Minute,
	})
	require.NoError(t, err)

	plane := served(t)

	ingress, err := ingressKinds(plane.resources)
	require.NoError(t, err)

	services := kind.Services{
		ControlPlane: plane.workload.Registry,
		Node:         nodes,
		Ingress:      ingress,
	}

	kindtest.Conformance(t, services, permissions.NewRepository())

	assert.Equal(t, []string{vmKind.Name, stackKind.Name, taskKind.Name}, kindNames(services.Descriptors()), "every kind the services run")
}

func kindNames(descriptors []kind.Descriptor) []string {
	names := make([]string, len(descriptors))
	for i, d := range descriptors {
		names[i] = d.Name
	}

	return names
}
