package restoreVM

import (
	"bytes"
	"log/slog"
	"sync"
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
	storage "github.com/khanzadimahdi/testproject/infrastructure/storage/memory"
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

func spec() vm.Spec {
	return vm.Spec{
		Kind:      vm.KindMachine,
		Image:     "ubuntu:24.04",
		Resources: vm.Resources{CPUs: 1, Memory: 512 << 20, Disk: 2 << 30},
		Ports:     []port.Port{80},
		Network:   vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
		Labels:    map[string]string{vm.LabelOwner: "owner-uuid", vm.LabelSlug: "box-abcde"},
	}
}

// snapshotted is an engine holding vm-1, whose disk was "before" when the
// snapshot in store was taken and is "after" now.
func snapshotted(t *testing.T, e *memory.Engine, store *storage.Storage) {
	t.Helper()

	_, err := e.Create(t.Context(), vmcommand.Spec("vm-1", spec()))
	require.NoError(t, err)
	require.NoError(t, e.SetDisk("vm-1", []byte("before")))

	var archive bytes.Buffer
	_, err = e.Snapshot(t.Context(), "vm-1", &archive)
	require.NoError(t, err)

	store.Put(snapshot.ObjectKey("snapshot-1"), archive.Bytes())
	require.NoError(t, e.SetDisk("vm-1", []byte("after")))
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name    string
		request Request

		wantDisk     string
		wantSubjects []string
		wantReason   string
	}{
		{
			name:         "a VM's disk is replaced with the snapshot's, and that is said",
			request:      Request{VMUUID: "vm-1", SnapshotUUID: "snapshot-1", Spec: spec()},
			wantDisk:     "before",
			wantSubjects: []string{events.VMRestoredName},
		},
		{
			name:         "a snapshot that is not stored leaves the VM as it was, and says so",
			request:      Request{VMUUID: "vm-1", SnapshotUUID: "snapshot-2", Spec: spec()},
			wantDisk:     "after",
			wantSubjects: []string{events.VMFailedName},
			wantReason:   "the snapshot cannot be read",
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := memory.New()
			store := storage.New()
			snapshotted(t, e, store)

			before, err := e.Inspect(t.Context(), "vm-1")
			require.NoError(t, err)

			producer := &messaging.Recorder{}
			connections := &forgotten{}

			_, err = NewUseCase(e, store, connections, lock.New(), producer, validates{}, nodeName).Execute(t.Context(), &tt.request)
			require.NoError(t, err)

			assert.Equal(t, tt.wantSubjects, producer.Subjects())
			assert.Equal(t, []string{"vm-1"}, connections.vms)

			disk, err := e.Disk("vm-1")
			require.NoError(t, err)
			assert.Equal(t, tt.wantDisk, string(disk))

			if len(tt.wantReason) > 0 {
				failed, err := messaging.Produced[events.VMFailed](producer, events.VMFailedName)
				require.NoError(t, err)
				assert.Contains(t, failed[0].Reason, tt.wantReason)

				return
			}

			restored, err := messaging.Produced[events.VMRestored](producer, events.VMRestoredName)
			require.NoError(t, err)
			assert.Equal(t, "snapshot-1", restored[0].SnapshotUUID)

			after, err := e.Inspect(t.Context(), "vm-1")
			require.NoError(t, err)
			assert.Equal(t, vm.InstanceRunning, after.State)
			assert.Equal(t, before.Endpoints, after.Endpoints, "a restored VM keeps its ports")
		})
	}

	t.Run("a command that names no snapshot is refused", func(t *testing.T) {
		t.Parallel()

		response, err := NewUseCase(memory.New(), storage.New(), &forgotten{}, lock.New(), &messaging.Recorder{}, validates{}, nodeName).
			Execute(t.Context(), &Request{VMUUID: "vm-1", Spec: spec()})
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{"snapshot_uuid": "required_field"}, response.ValidationErrors)
	})
}

func TestVMRestoreRequestedHandler_Handle(t *testing.T) {
	t.Parallel()

	e := memory.New()
	store := storage.New()
	snapshotted(t, e, store)

	producer := &messaging.Recorder{}
	handler := NewVMRestoreRequestedHandler(NewUseCase(e, store, &forgotten{}, lock.New(), producer, validates{}, nodeName), producer, nodeName, slog.New(slog.DiscardHandler))

	payload := `{"vm_uuid":"vm-1","node_name":"` + nodeName + `","snapshot_uuid":"snapshot-1","spec":{"kind":"machine","image":"ubuntu:24.04","network":{"ingress":"allow","egress":"deny"},"ports":[80]}}`
	require.NoError(t, handler.Handle(t.Context(), []byte(payload)))

	disk, err := e.Disk("vm-1")
	require.NoError(t, err)
	assert.Equal(t, "before", string(disk))
}
