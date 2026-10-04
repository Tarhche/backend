package deleteVM

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

	for _, tt := range []struct {
		name    string
		request Request
	}{
		{name: "a VM is removed, disk and all, and that is said", request: Request{VMUUID: "vm-1"}},
		{name: "one that is not here is gone already, and that is said too", request: Request{VMUUID: "vm-2"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := memory.New()
			_, err := e.Create(t.Context(), vmcommand.Spec("vm-1", vm.Spec{Kind: vm.KindMachine, Image: "ubuntu:24.04"}))
			require.NoError(t, err)

			producer := &messaging.Recorder{}
			connections := &forgotten{}

			_, err = NewUseCase(e, connections, lock.New(), producer, validates{}, nodeName).Execute(t.Context(), &tt.request)
			require.NoError(t, err)

			_, err = e.Inspect(t.Context(), tt.request.VMUUID)
			assert.ErrorIs(t, err, domain.ErrNotExists)
			assert.Equal(t, []string{tt.request.VMUUID}, connections.vms)

			deleted, err := messaging.Produced[events.VMDeleted](producer, events.VMDeletedName)
			require.NoError(t, err)
			require.Len(t, deleted, 1)
			assert.Equal(t, tt.request.VMUUID, deleted[0].VMUUID)
			assert.Equal(t, nodeName, deleted[0].NodeName)
		})
	}

	t.Run("an engine that would not remove it says why, and nothing is said gone", func(t *testing.T) {
		t.Parallel()

		var e engine.MockEngine
		e.On("Delete", mock.Anything, "vm-1").Return(errors.New("the vmhost is away"))
		defer e.AssertExpectations(t)

		producer := &messaging.Recorder{}

		_, err := NewUseCase(&e, &forgotten{}, lock.New(), producer, validates{}, nodeName).Execute(t.Context(), &Request{VMUUID: "vm-1"})
		require.NoError(t, err)

		assert.Equal(t, []string{events.VMFailedName}, producer.Subjects())
	})

	t.Run("saying it is gone failing is worth another delivery", func(t *testing.T) {
		t.Parallel()

		producer := &messaging.Recorder{Err: errors.New("nats is away")}

		_, err := NewUseCase(memory.New(), &forgotten{}, lock.New(), producer, validates{}, nodeName).Execute(t.Context(), &Request{VMUUID: "vm-1"})
		assert.Error(t, err)
	})
}

func TestVMDeleteRequestedHandler_Handle(t *testing.T) {
	t.Parallel()

	producer := &messaging.Recorder{}
	handler := NewVMDeleteRequestedHandler(NewUseCase(memory.New(), &forgotten{}, lock.New(), producer, validates{}, nodeName), producer, nodeName, slog.New(slog.DiscardHandler))

	require.NoError(t, handler.Handle(t.Context(), []byte(`{"vm_uuid":"vm-1","node_name":"workload-orchestrator-02"}`)))
	assert.Empty(t, producer.Messages(), "another node's command is not this node's")

	require.NoError(t, handler.Handle(t.Context(), []byte(`{"vm_uuid":"vm-1","node_name":"`+nodeName+`"}`)))
	assert.Equal(t, []string{events.VMDeletedName}, producer.Subjects())
}
