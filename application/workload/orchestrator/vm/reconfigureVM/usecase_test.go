package reconfigureVM

import (
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/internal/vmcommand"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/lock"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
	messaging "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	memory "github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

const nodeName = "workload-orchestrator-01"

type validates struct{}

func (validates) Validate(value any) domain.ValidationErrors {
	return value.(domain.Validatable).Validate()
}

type forgotten struct {
	lock sync.Mutex
	vms  []string
}

func (f *forgotten) Forget(vmUUID string) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.vms = append(f.vms, vmUUID)
}

func spec(ports ...port.Port) vm.Spec {
	return vm.Spec{
		Kind:      vm.KindMachine,
		Image:     "ubuntu:24.04",
		Resources: vm.Resources{CPUs: 1, Memory: 512 << 20, Disk: 2 << 30},
		Ports:     ports,
		Network:   vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
		Labels:    map[string]string{vm.LabelOwner: "owner-uuid", vm.LabelSlug: "box-abcde"},
	}
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name    string
		request Request

		wantPorts      []port.Port
		wantSubjects   []string
		wantValidation domain.ValidationErrors
	}{
		{
			name:      "a VM is given the ports, network and resources it now has",
			request:   Request{VMUUID: "vm-1", Spec: spec(80, 443)},
			wantPorts: []port.Port{80, 443},
		},
		{
			name: "a VM given more than this node has is not changed, and that is said",
			request: func() Request {
				grown := spec(80, 443)
				grown.Resources.Memory = 8 << 30

				return Request{VMUUID: "vm-1", Spec: grown}
			}(),
			wantPorts:    []port.Port{80},
			wantSubjects: []string{events.VMFailedName},
		},
		{
			name:         "a VM that is not here cannot be changed here",
			request:      Request{VMUUID: "vm-2", Spec: spec(80)},
			wantPorts:    []port.Port{80},
			wantSubjects: []string{events.VMFailedName},
		},
		{
			name:           "a network nobody knows is refused",
			request:        Request{VMUUID: "vm-1", Spec: vm.Spec{Network: vm.Network{Ingress: "open"}}},
			wantPorts:      []port.Port{80},
			wantValidation: domain.ValidationErrors{"spec.network.ingress": "invalid_value", "spec.network.egress": "invalid_value"},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := memory.New(memory.WithCapacity(4, 4<<30, 100<<30))
			_, err := e.Create(t.Context(), vmcommand.Spec("vm-1", spec(80)))
			require.NoError(t, err)

			producer := &messaging.Recorder{}

			response, err := NewUseCase(e, &forgotten{}, lock.New(), producer, validates{}, nodeName).Execute(t.Context(), &tt.request)
			require.NoError(t, err)

			if tt.wantValidation != nil {
				assert.Equal(t, tt.wantValidation, response.ValidationErrors)
			}

			assert.Equal(t, tt.wantSubjects, producer.Subjects())

			given, err := e.Spec("vm-1")
			require.NoError(t, err)
			assert.Equal(t, tt.wantPorts, given.Ports)
			assert.Equal(t, vm.PurposeVM, given.Labels[vm.LabelPurpose])
		})
	}
}

func TestVMReconfigureRequestedHandler_Handle(t *testing.T) {
	t.Parallel()

	e := memory.New()
	_, err := e.Create(t.Context(), vmcommand.Spec("vm-1", spec(80)))
	require.NoError(t, err)

	producer := &messaging.Recorder{}
	connections := &forgotten{}
	handler := NewVMReconfigureRequestedHandler(NewUseCase(e, connections, lock.New(), producer, validates{}, nodeName), producer, nodeName, slog.New(slog.DiscardHandler))

	payload := `{"vm_uuid":"vm-1","node_name":"` + nodeName + `","spec":{"ports":[8080],"network":{"ingress":"allow","egress":"allow"}}}`
	require.NoError(t, handler.Handle(t.Context(), []byte(payload)))

	given, err := e.Spec("vm-1")
	require.NoError(t, err)
	assert.Equal(t, []port.Port{8080}, given.Ports)
	assert.Equal(t, []string{"vm-1"}, connections.vms, "a VM that may have restarted has nothing open into it any more")
}
