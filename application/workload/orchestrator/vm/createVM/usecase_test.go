package createVM

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/internal/vmcommand"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/lock"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
	messaging "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/storage/memory"
	engine "github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

const nodeName = "workload-orchestrator-01"

// validates is the requests' own rules, untranslated.
type validates struct{}

func (validates) Validate(value any) domain.ValidationErrors {
	return value.(domain.Validatable).Validate()
}

// sent is a spec as the control plane sends it.
func sent() vm.Spec {
	return vm.Spec{
		Kind:      vm.KindMachine,
		Image:     "ubuntu:24.04",
		Resources: vm.Resources{CPUs: 1, Memory: 512 << 20, Disk: 2 << 30},
		Ports:     []port.Port{80},
		Network:   vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessAllow},
		Labels:    map[string]string{vm.LabelOwner: "owner-uuid", vm.LabelSlug: "box-abcde"},
	}
}

// archived is a snapshot of a VM whose disk holds disk, as the store keeps it.
func archived(t *testing.T, disk string) []byte {
	t.Helper()

	source := engine.New()

	spec := sent()
	spec.ID = "source"

	_, err := source.Create(t.Context(), spec)
	require.NoError(t, err)
	require.NoError(t, source.SetDisk("source", []byte(disk)))

	var archive bytes.Buffer
	_, err = source.Snapshot(t.Context(), "source", &archive)
	require.NoError(t, err)

	return archive.Bytes()
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name string

		// given is what the node holds and stores before it is asked.
		given   func(t *testing.T, e *engine.Engine, store *memory.Storage)
		request Request

		wantErr          bool
		wantValidation   domain.ValidationErrors
		wantState        vm.InstanceState
		wantDisk         string
		wantSubjects     []string
		wantFailedReason string
	}{
		{
			name:    "a VM is made from its image and booted",
			request: Request{VMUUID: "vm-1", Spec: sent()},

			wantState:    vm.InstanceRunning,
			wantSubjects: nil,
		},
		{
			name: "a VM made from a snapshot has the snapshot's disk, and says so",
			given: func(t *testing.T, _ *engine.Engine, store *memory.Storage) {
				store.Put(snapshot.ObjectKey("snapshot-1"), archived(t, "what was kept"))
			},
			request: Request{VMUUID: "vm-1", Spec: sent(), SnapshotUUID: "snapshot-1"},

			wantState:    vm.InstanceRunning,
			wantDisk:     "what was kept",
			wantSubjects: []string{events.VMRestoredName},
		},
		{
			name: "a VM this node holds already is what was asked for",
			given: func(t *testing.T, e *engine.Engine, _ *memory.Storage) {
				_, err := e.Create(t.Context(), vmcommand.Spec("vm-1", sent()))
				require.NoError(t, err)
			},
			request: Request{VMUUID: "vm-1", Spec: sent()},

			wantState: vm.InstanceRunning,
		},
		{
			name: "a VM this node holds stopped is booted",
			given: func(t *testing.T, e *engine.Engine, _ *memory.Storage) {
				_, err := e.Create(t.Context(), vmcommand.Spec("vm-1", sent()))
				require.NoError(t, err)
				require.NoError(t, e.Stop(t.Context(), "vm-1"))
			},
			request: Request{VMUUID: "vm-1", Spec: sent()},

			wantState: vm.InstanceRunning,
		},
		{
			name: "a VM made from a snapshot and asked for again says again that it was",
			given: func(t *testing.T, e *engine.Engine, _ *memory.Storage) {
				_, err := e.Create(t.Context(), vmcommand.Spec("vm-1", sent()))
				require.NoError(t, err)
			},
			request: Request{VMUUID: "vm-1", Spec: sent(), SnapshotUUID: "snapshot-1"},

			wantState:    vm.InstanceRunning,
			wantSubjects: []string{events.VMRestoredName},
		},
		{
			name: "a node with no room says the VM failed, and why",
			given: func(t *testing.T, e *engine.Engine, _ *memory.Storage) {
				spec := sent()
				spec.ID = "big"
				spec.Resources.Memory = 1 << 30

				_, err := e.Create(t.Context(), spec)
				require.NoError(t, err)
			},
			request: Request{VMUUID: "vm-1", Spec: sent()},

			wantSubjects:     []string{events.VMFailedName},
			wantFailedReason: vm.ErrNoCapacity.Error(),
		},
		{
			name:    "a snapshot that is not stored fails the VM",
			request: Request{VMUUID: "vm-1", Spec: sent(), SnapshotUUID: "snapshot-1"},

			wantSubjects:     []string{events.VMFailedName},
			wantFailedReason: "the snapshot cannot be read",
		},
		{
			name:    "a command that does not say what to make is refused",
			request: Request{VMUUID: "vm-1", Spec: vm.Spec{Kind: "container"}},

			wantValidation: domain.ValidationErrors{"spec.kind": "invalid_value", "spec.image": "required_field"},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := engine.New(engine.WithCapacity(4, 1<<30, 100<<30))
			store := memory.New()
			producer := &messaging.Recorder{}

			if tt.given != nil {
				tt.given(t, e, store)
			}

			response, err := NewUseCase(e, store, lock.New(), producer, validates{}, nodeName).Execute(t.Context(), &tt.request)
			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantValidation, nilIfEmpty(response.ValidationErrors))
			assert.Equal(t, tt.wantSubjects, producer.Subjects())

			if len(tt.wantFailedReason) > 0 {
				failed, err := messaging.Produced[events.VMFailed](producer, events.VMFailedName)
				require.NoError(t, err)
				require.Len(t, failed, 1)

				assert.Equal(t, "vm-1", failed[0].VMUUID)
				assert.Equal(t, nodeName, failed[0].NodeName)
				assert.Contains(t, failed[0].Reason, tt.wantFailedReason)
			}

			if len(tt.wantState) == 0 {
				return
			}

			instance, err := e.Inspect(t.Context(), "vm-1")
			require.NoError(t, err)
			assert.Equal(t, tt.wantState, instance.State)

			spec, err := e.Spec("vm-1")
			require.NoError(t, err)
			assert.Equal(t, map[string]string{
				vm.LabelOwner:   "owner-uuid",
				vm.LabelSlug:    "box-abcde",
				vm.LabelVM:      "vm-1",
				vm.LabelPurpose: vm.PurposeVM,
			}, spec.Labels, "a VM is labelled as one, under its own uuid")

			if len(tt.wantDisk) > 0 {
				disk, err := e.Disk("vm-1")
				require.NoError(t, err)
				assert.Equal(t, tt.wantDisk, string(disk))

				restored, err := messaging.Produced[events.VMRestored](producer, events.VMRestoredName)
				require.NoError(t, err)
				require.Len(t, restored, 1)
				assert.Equal(t, events.VMRestored{VMUUID: "vm-1", NodeName: nodeName, SnapshotUUID: "snapshot-1", At: restored[0].At}, restored[0])
			}
		})
	}

	t.Run("a failure nobody could be told of is worth another delivery", func(t *testing.T) {
		t.Parallel()

		producer := &messaging.Recorder{Err: errors.New("nats is away")}

		_, err := NewUseCase(engine.New(), memory.New(), lock.New(), producer, validates{}, nodeName).
			Execute(t.Context(), &Request{VMUUID: "vm-1", Spec: sent(), SnapshotUUID: "missing"})

		assert.Error(t, err)
	})
}

func TestVMScheduledHandler_Handle(t *testing.T) {
	t.Parallel()

	scheduled := func(t *testing.T, event events.VMScheduled) []byte {
		t.Helper()

		payload, err := json.Marshal(event)
		require.NoError(t, err)

		return payload
	}

	testcases := []struct {
		name         string
		payload      func(t *testing.T) []byte
		wantVM       bool
		wantSubjects []string
	}{
		{
			name: "a VM scheduled on this node is made here",
			payload: func(t *testing.T) []byte {
				return scheduled(t, events.VMScheduled{VMUUID: "vm-1", NodeName: nodeName, Spec: events.NewSpec(sent())})
			},
			wantVM: true,
		},
		{
			name: "one scheduled on another node is not this node's",
			payload: func(t *testing.T) []byte {
				return scheduled(t, events.VMScheduled{VMUUID: "vm-1", NodeName: "workload-orchestrator-02", Spec: events.NewSpec(sent())})
			},
		},
		{
			name: "one this node refuses is reported as failed rather than retried",
			payload: func(t *testing.T) []byte {
				return scheduled(t, events.VMScheduled{VMUUID: "vm-1", NodeName: nodeName})
			},
			wantSubjects: []string{events.VMFailedName},
		},
		{
			name:    "a message that cannot be read is dropped",
			payload: func(*testing.T) []byte { return []byte("{") },
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := engine.New()
			producer := &messaging.Recorder{}
			useCase := NewUseCase(e, memory.New(), lock.New(), producer, validates{}, nodeName)

			err := NewVMScheduledHandler(useCase, producer, nodeName, slog.New(slog.DiscardHandler)).Handle(t.Context(), tt.payload(t))
			require.NoError(t, err)

			_, inspectErr := e.Inspect(t.Context(), "vm-1")
			assert.Equal(t, tt.wantVM, inspectErr == nil)
			assert.Equal(t, tt.wantSubjects, producer.Subjects())
		})
	}
}

func nilIfEmpty(validationErrors domain.ValidationErrors) domain.ValidationErrors {
	if len(validationErrors) == 0 {
		return nil
	}

	return validationErrors
}
