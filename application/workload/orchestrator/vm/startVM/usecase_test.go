package startVM

import (
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/internal/vmcommand"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/lock"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
	messaging "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	engine "github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

const nodeName = "workload-orchestrator-01"

type validates struct{}

func (validates) Validate(value any) domain.ValidationErrors {
	return value.(domain.Validatable).Validate()
}

// holding is an engine holding vm-1, in the state given.
func holding(t *testing.T, state vm.InstanceState) *engine.Engine {
	t.Helper()

	e := engine.New()

	_, err := e.Create(t.Context(), vmcommand.Spec("vm-1", vm.Spec{Kind: vm.KindMachine, Image: "ubuntu:24.04"}))
	require.NoError(t, err)

	if state == vm.InstanceStopped {
		require.NoError(t, e.Stop(t.Context(), "vm-1"))
	}

	return e
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name    string
		held    vm.InstanceState
		request Request

		wantState      vm.InstanceState
		wantSubjects   []string
		wantReason     string
		wantValidation domain.ValidationErrors
	}{
		{
			name:      "a stopped VM boots",
			held:      vm.InstanceStopped,
			request:   Request{VMUUID: "vm-1"},
			wantState: vm.InstanceRunning,
		},
		{
			name:      "a running VM is what was asked for",
			held:      vm.InstanceRunning,
			request:   Request{VMUUID: "vm-1"},
			wantState: vm.InstanceRunning,
		},
		{
			name:         "a VM that is not here cannot be booted here, and that is said",
			held:         vm.InstanceRunning,
			request:      Request{VMUUID: "vm-2"},
			wantState:    vm.InstanceRunning,
			wantSubjects: []string{events.VMFailedName},
			wantReason:   "this node does not hold the vm",
		},
		{
			name:           "a command naming no VM is refused",
			held:           vm.InstanceRunning,
			request:        Request{},
			wantState:      vm.InstanceRunning,
			wantValidation: domain.ValidationErrors{"vm_uuid": "required_field"},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := holding(t, tt.held)
			producer := &messaging.Recorder{}

			response, err := NewUseCase(e, lock.New(), producer, validates{}, nodeName).Execute(t.Context(), &tt.request)
			require.NoError(t, err)

			if tt.wantValidation != nil {
				assert.Equal(t, tt.wantValidation, response.ValidationErrors)
			}

			assert.Equal(t, tt.wantSubjects, producer.Subjects())

			if len(tt.wantReason) > 0 {
				failed, err := messaging.Produced[events.VMFailed](producer, events.VMFailedName)
				require.NoError(t, err)
				assert.Contains(t, failed[0].Reason, tt.wantReason)
				assert.Equal(t, tt.request.VMUUID, failed[0].VMUUID)
			}

			instance, err := e.Inspect(t.Context(), "vm-1")
			require.NoError(t, err)
			assert.Equal(t, tt.wantState, instance.State)
		})
	}
}

func TestVMStartRequestedHandler_Handle(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name      string
		payload   string
		wantState vm.InstanceState
	}{
		{name: "this node's VM is booted", payload: `{"vm_uuid":"vm-1","node_name":"` + nodeName + `"}`, wantState: vm.InstanceRunning},
		{name: "another node's is not this node's to boot", payload: `{"vm_uuid":"vm-1","node_name":"workload-orchestrator-02"}`, wantState: vm.InstanceStopped},
		{name: "a message that cannot be read is dropped", payload: `{`, wantState: vm.InstanceStopped},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := holding(t, vm.InstanceStopped)
			producer := &messaging.Recorder{}
			useCase := NewUseCase(e, lock.New(), producer, validates{}, nodeName)

			require.NoError(t, NewVMStartRequestedHandler(useCase, producer, nodeName, slog.New(slog.DiscardHandler)).Handle(t.Context(), []byte(tt.payload)))

			instance, err := e.Inspect(t.Context(), "vm-1")
			require.NoError(t, err)
			assert.Equal(t, tt.wantState, instance.State)
		})
	}

	t.Run("a command naming no VM has nobody to report to", func(t *testing.T) {
		t.Parallel()

		producer := &messaging.Recorder{}
		useCase := NewUseCase(engine.New(), lock.New(), producer, validates{}, nodeName)

		payload, err := json.Marshal(events.VMStartRequested{NodeName: nodeName})
		require.NoError(t, err)

		require.NoError(t, NewVMStartRequestedHandler(useCase, producer, nodeName, slog.New(slog.DiscardHandler)).Handle(t.Context(), payload))
		assert.Empty(t, producer.Messages())
	})
}
