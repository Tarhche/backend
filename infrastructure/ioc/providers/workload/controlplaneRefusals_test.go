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

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
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
// it. A repository told of the kinds it keeps is told of every kind the
// control plane runs first, as the serve command tells it.
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

	// the Docker VM what is asked for goes into.
	raw, err := kind.Encode(vmtest.Docker("vm-1", "owner"))
	require.NoError(t, err)

	_, err = resources.Create(ctx, resource.Record{Raw: raw})
	require.NoError(t, err)

	mux := http.NewServeMux()
	require.NoError(t, workload.Route(mux))

	for name, tt := range unkept {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/"+tt.plural+"?owner=owner"+tt.query, strings.NewReader(tt.body)))

			require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())

			var answered struct {
				Errors domain.ValidationErrors `json:"errors"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answered))
			assert.Equal(t, tt.want, answered.Errors)

			for _, d := range workload.Registry.Descriptors() {
				kept, total, err := resources.GetAll(ctx, d.Name, resource.Filter{OwnerUUID: "owner"}, 0, 0)
				require.NoError(t, err)

				if d.Name == "vm" {
					assert.Equal(t, uint(1), total, "but the vm it goes into, nothing is kept: %v", kept)

					continue
				}

				assert.Zero(t, total, "nothing is kept of a %s: %v", d.Name, kept)
			}
		})
	}
}
