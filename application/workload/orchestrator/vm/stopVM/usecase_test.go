package stopVM

import (
	"errors"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/internal/vmcommand"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/lock"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
	messaging "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/engine"
	memory "github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

const nodeName = "workload-orchestrator-01"

type validates struct{}

func (validates) Validate(value any) domain.ValidationErrors {
	return value.(domain.Validatable).Validate()
}

// forgotten keeps which VMs were let go of.
type forgotten struct {
	lock sync.Mutex
	vms  []string
}

func (f *forgotten) Forget(vmUUID string) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.vms = append(f.vms, vmUUID)
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name    string
		exited  bool
		request Request

		wantState    vm.InstanceState
		wantSubjects []string
	}{
		{
			name:      "a running VM is stopped, keeping its disk",
			request:   Request{VMUUID: "vm-1"},
			wantState: vm.InstanceStopped,
		},
		{
			name:      "a VM whose main process exited is not running, which is what was asked for",
			exited:    true,
			request:   Request{VMUUID: "vm-1"},
			wantState: vm.InstanceExited,
		},
		{
			name:      "a VM that is not here is not running either",
			request:   Request{VMUUID: "vm-2"},
			wantState: vm.InstanceRunning,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := memory.New()
			_, err := e.Create(t.Context(), vmcommand.Spec("vm-1", vm.Spec{Kind: vm.KindMachine, Image: "ubuntu:24.04"}))
			require.NoError(t, err)

			if tt.exited {
				require.NoError(t, e.Exit("vm-1", 0))
			}

			producer := &messaging.Recorder{}
			connections := &forgotten{}

			_, err = NewUseCase(e, connections, lock.New(), producer, validates{}, nodeName).Execute(t.Context(), &tt.request)
			require.NoError(t, err)

			assert.Equal(t, tt.wantSubjects, producer.Subjects())
			assert.Equal(t, []string{tt.request.VMUUID}, connections.vms, "nothing opened into a stopped VM leads anywhere")

			instance, err := e.Inspect(t.Context(), "vm-1")
			require.NoError(t, err)
			assert.Equal(t, tt.wantState, instance.State)
		})
	}

	t.Run("an engine that would not stop it says why", func(t *testing.T) {
		t.Parallel()

		var e engine.MockEngine
		e.On("Inspect", mock.Anything, "vm-1").Return(vm.Instance{ID: "vm-1", State: vm.InstanceRunning}, nil)
		e.On("Stop", mock.Anything, "vm-1").Return(errors.New("the vmhost is away"))
		defer e.AssertExpectations(t)

		producer := &messaging.Recorder{}

		_, err := NewUseCase(&e, &forgotten{}, lock.New(), producer, validates{}, nodeName).Execute(t.Context(), &Request{VMUUID: "vm-1"})
		require.NoError(t, err)

		failed, err := messaging.Produced[events.VMFailed](producer, events.VMFailedName)
		require.NoError(t, err)
		require.Len(t, failed, 1)
		assert.Equal(t, "the vmhost is away", failed[0].Reason)
	})
}

func TestVMStopRequestedHandler_Handle(t *testing.T) {
	t.Parallel()

	e := memory.New()
	_, err := e.Create(t.Context(), vmcommand.Spec("vm-1", vm.Spec{Kind: vm.KindMachine, Image: "ubuntu:24.04"}))
	require.NoError(t, err)

	producer := &messaging.Recorder{}
	handler := NewVMStopRequestedHandler(NewUseCase(e, &forgotten{}, lock.New(), producer, validates{}, nodeName), producer, nodeName, slog.New(slog.DiscardHandler))

	require.NoError(t, handler.Handle(t.Context(), []byte(`{"vm_uuid":"vm-1","node_name":"workload-orchestrator-02"}`)))

	instance, err := e.Inspect(t.Context(), "vm-1")
	require.NoError(t, err)
	assert.Equal(t, vm.InstanceRunning, instance.State, "another node's command is not this node's")

	require.NoError(t, handler.Handle(t.Context(), []byte(`{"vm_uuid":"vm-1","node_name":"`+nodeName+`"}`)))

	instance, err = e.Inspect(t.Context(), "vm-1")
	require.NoError(t, err)
	assert.Equal(t, vm.InstanceStopped, instance.State)
}
