package workload

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks/blockstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	executionsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/executions"
	leasesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/leases"
	nodesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/nodes"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
	logsMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/logs"
)

// asked is what somebody asks the control plane's API for, and what it is
// refused with.
type asked struct {
	// plural and query are where it is asked, beside its owner.
	plural string
	query  string

	body string
	want domain.ValidationErrors
}

// unkept are what somebody may ask for through the API that no record could
// keep as it is: refused, each, as it is asked.
var unkept = map[string]asked{
	"a stack whose file holds a service to more memory than a record keeps as a whole number": {
		plural: "stacks",
		body:   `{"metadata":{"name":"shop"},"spec":{"vm":{"uuid":"vm-1"},"compose":"services:\n  web:\n    image: nginx:1.27\n    resources:\n      limits:\n        memory: 10000000000g\n"}}`,
		want:   domain.ValidationErrors{"compose.services.web.resources.limits.memory": "too_large"},
	},
	"a container held to as much": {
		plural: "containers",
		query:  "&parent=vm-1",
		body:   `{"metadata":{"name":"web"},"spec":{"image":"nginx:1.27","memory":10737418240000000000}}`,
		want:   domain.ValidationErrors{"memory": "too_large"},
	},
	"a task given as much": {
		plural: "tasks",
		body:   `{"metadata":{"name":"job"},"spec":{"image":"python:3.13","limits":{"cpu":1,"memory":9223372036854775808,"disk":1073741824}}}`,
		want:   domain.ValidationErrors{"limits.memory": "too_large"},
	},
	"a network labelled with a NUL in a label's key, which no key of a record can hold": {
		plural: "networks",
		query:  "&parent=vm-1",
		body:   `{"metadata":{"name":"front"},"spec":{"name":"front","labels":{"tier":"web","a\u0000b":"c"}}}`,
		want:   domain.ValidationErrors{"labels": "invalid_value"},
	},
	"or in a label's value": {
		plural: "networks",
		query:  "&parent=vm-1",
		body:   `{"metadata":{"name":"front"},"spec":{"name":"front","labels":{"tier":"w\u0000eb"}}}`,
		want:   domain.ValidationErrors{"labels": "invalid_value"},
	},
	"a volume labelled so": {
		plural: "volumes",
		query:  "&parent=vm-1",
		body:   `{"metadata":{"name":"data"},"spec":{"name":"data","labels":{"a\u0000b":"c"}}}`,
		want:   domain.ValidationErrors{"labels": "invalid_value"},
	},
	"a stack whose own labels hold one": {
		plural: "stacks",
		body:   `{"metadata":{"name":"shop","labels":{"team":"sh\u0000op"}},"spec":{"vm":{"uuid":"vm-1"},"compose":"services:\n  web:\n    image: nginx:1.27\n"}}`,
		want:   domain.ValidationErrors{"labels": "invalid_value"},
	},
	"a container aliased on a network whose name holds one": {
		plural: "containers",
		query:  "&parent=vm-1",
		body:   `{"metadata":{"name":"api"},"spec":{"image":"nginx:1.27","networks":["front"],"aliases":{"fr\u0000nt":["api"]}}}`,
		want:   domain.ValidationErrors{"aliases": "invalid_value"},
	},
	"and one asked to be connected to such a network, its names on it kept under its name": {
		plural: "containers/c-1/actions/connect",
		query:  "&parent=vm-1",
		body:   `{"network":"fr\u0000nt","aliases":["web"]}`,
		want:   domain.ValidationErrors{"network": "invalid_value"},
	},
}

// TestNewControlPlaneWorkload_refused holds the control plane, as it serves
// over resources kept in memory, to refusing through its API what no record
// could keep as it is.
func TestNewControlPlaneWorkload_refused(t *testing.T) {
	t.Parallel()

	refused(t, resourcesMemory.NewRepository())
}

// refused holds the control plane, as it serves over resources, to refusing
// through its API what no record could keep as it is (unkept): each asked is
// answered 400, under the fields that cannot be kept, and nothing is kept of
// it, nor written. A repository told of the kinds it keeps is told of every
// kind the control plane runs first, as the serve command tells it.
func refused(t *testing.T, resources resource.Repository) {
	t.Helper()

	ctx := context.Background()

	workload, err := NewControlPlaneWorkload(configs.NewWorkloadControlPlane(), ControlPlaneStores{
		Resources:  resources,
		Nodes:      nodesMemory.NewRepository(vmtest.Alive(vmtest.Node)),
		TaskLogs:   logsMock.NewInMemoryRepository(),
		Executions: executionsMemory.NewRepository(),
		Leases:     leasesMemory.NewRepository(nil),
	}, nil, &messagingMock.Recorder{}, slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	if told, tells := resources.(interface {
		EnsureKind(ctx context.Context, d kind.Descriptor) error
	}); tells {
		for _, d := range workload.Registry.Descriptors() {
			require.NoError(t, told.EnsureKind(ctx, d))
		}
	}

	// the Docker VM what is asked for goes into, and a container in it.
	raw, err := kind.Encode(vmtest.Docker("vm-1", "owner"))
	require.NoError(t, err)

	_, err = resources.Create(ctx, resource.Record{Raw: raw})
	require.NoError(t, err)

	_, err = resources.Create(ctx, resource.Record{Raw: blockstest.AContainer("c-1", "vm-1", containerKind.Running, containerKind.Running)})
	require.NoError(t, err)

	mux := http.NewServeMux()
	require.NoError(t, workload.Route(mux))

	// kept is every record kept, by its kind and uuid, at its version.
	kept := func(t *testing.T) map[string]int64 {
		t.Helper()

		versions := make(map[string]int64)

		for _, d := range workload.Registry.Descriptors() {
			records, _, err := resources.GetAll(ctx, d.Name, resource.Filter{}, 0, 0)
			require.NoError(t, err)

			for _, r := range records {
				versions[r.Kind+"/"+r.Metadata.UUID] = r.Version
			}
		}

		return versions
	}

	for name, tt := range unkept {
		t.Run(name, func(t *testing.T) {
			before := kept(t)

			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/"+tt.plural+"?owner=owner"+tt.query, strings.NewReader(tt.body)))

			require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())

			var answered struct {
				Errors domain.ValidationErrors `json:"errors"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answered))
			assert.Equal(t, tt.want, answered.Errors)

			assert.Equal(t, before, kept(t), "nothing is kept, nor written")
		})
	}
}
