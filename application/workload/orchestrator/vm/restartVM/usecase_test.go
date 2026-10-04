package restartVM

import (
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/internal/vmcommand"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/lock"
	"github.com/khanzadimahdi/testproject/domain"
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

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name    string
		stopped bool
		request Request

		wantRebooted bool
		wantSubjects []string
		wantForgot   []string
	}{
		{
			name:         "a running VM is booted again in place",
			request:      Request{VMUUID: "vm-1"},
			wantRebooted: true,
			wantForgot:   []string{"vm-1"},
		},
		{
			name:         "a stopped VM is booted, which is what a restart ends with",
			stopped:      true,
			request:      Request{VMUUID: "vm-1"},
			wantRebooted: true,
			wantForgot:   []string{"vm-1"},
		},
		{
			name:         "a VM that is not here cannot be restarted here, and that is said",
			request:      Request{VMUUID: "vm-2"},
			wantSubjects: []string{events.VMFailedName},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
			e := memory.New(memory.WithClock(func() time.Time { return now }))

			_, err := e.Create(t.Context(), vmcommand.Spec("vm-1", vm.Spec{Kind: vm.KindMachine, Image: "ubuntu:24.04"}))
			require.NoError(t, err)

			if tt.stopped {
				require.NoError(t, e.Stop(t.Context(), "vm-1"))
			}

			now = now.Add(time.Minute)

			producer := &messaging.Recorder{}
			connections := &forgotten{}

			_, err = NewUseCase(e, connections, lock.New(), producer, validates{}, nodeName).Execute(t.Context(), &tt.request)
			require.NoError(t, err)

			assert.Equal(t, tt.wantSubjects, producer.Subjects())
			assert.Equal(t, tt.wantForgot, connections.vms)

			instance, err := e.Inspect(t.Context(), "vm-1")
			require.NoError(t, err)
			assert.Equal(t, vm.InstanceRunning, instance.State)
			assert.Equal(t, tt.wantRebooted, instance.StartedAt.Equal(now))
		})
	}
}

func TestVMRestartRequestedHandler_Handle(t *testing.T) {
	t.Parallel()

	e := memory.New()
	_, err := e.Create(t.Context(), vmcommand.Spec("vm-1", vm.Spec{Kind: vm.KindMachine, Image: "ubuntu:24.04"}))
	require.NoError(t, err)
	require.NoError(t, e.Stop(t.Context(), "vm-1"))

	producer := &messaging.Recorder{}
	handler := NewVMRestartRequestedHandler(NewUseCase(e, &forgotten{}, lock.New(), producer, validates{}, nodeName), producer, nodeName, slog.New(slog.DiscardHandler))

	require.NoError(t, handler.Handle(t.Context(), []byte(`{"vm_uuid":"vm-1","node_name":"`+nodeName+`"}`)))

	instance, err := e.Inspect(t.Context(), "vm-1")
	require.NoError(t, err)
	assert.Equal(t, vm.InstanceRunning, instance.State)
}
