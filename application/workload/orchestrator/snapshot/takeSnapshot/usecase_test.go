package takeSnapshot

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/lock"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot/events"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	messaging "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/engine"
	storage "github.com/khanzadimahdi/testproject/infrastructure/storage/memory"
	memory "github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

const nodeName = "workload-orchestrator-01"

type validates struct{}

func (validates) Validate(value any) domain.ValidationErrors {
	return value.(domain.Validatable).Validate()
}

// recorded keeps what was recorded about snapshots.
type recorded struct {
	lock  sync.Mutex
	sizes []int64
	took  []time.Duration
}

func (r *recorded) Snapshot(_ context.Context, took time.Duration, size int64) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.took = append(r.took, took)
	r.sizes = append(r.sizes, size)
}

// holding is an engine holding vm-1, whose disk holds disk.
func holding(t *testing.T, disk string) *memory.Engine {
	t.Helper()

	e := memory.New()

	_, err := e.Create(t.Context(), vm.Spec{
		ID:        "vm-1",
		Kind:      vm.KindDocker,
		Image:     "docker:29-dind",
		Resources: vm.Resources{CPUs: 2, Memory: 2 << 30, Disk: 20 << 30},
		Labels:    map[string]string{vm.LabelVM: "vm-1", vm.LabelPurpose: vm.PurposeVM},
	})
	require.NoError(t, err)
	require.NoError(t, e.SetDisk("vm-1", []byte(disk)))

	return e
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("a VM's disk is streamed into the store, and that it is stored is said", func(t *testing.T) {
		t.Parallel()

		e := holding(t, "what was written")
		store := storage.New()
		producer := &messaging.Recorder{}
		recorder := &recorded{}

		_, err := NewUseCase(e, store, lock.New(), producer, validates{}, recorder, nodeName).
			Execute(t.Context(), &Request{SnapshotUUID: "snapshot-1", VMUUID: "vm-1"})
		require.NoError(t, err)

		archive, stored := store.Object(snapshot.ObjectKey("snapshot-1"))
		require.True(t, stored)

		completed, err := messaging.Produced[events.SnapshotCompleted](producer, events.SnapshotCompletedName)
		require.NoError(t, err)
		require.Len(t, completed, 1)

		assert.Equal(t, events.SnapshotCompleted{
			SnapshotUUID: "snapshot-1",
			NodeName:     nodeName,
			Size:         int64(len(archive)),
			Engine:       "memory/1",
			Disk:         20 << 30,
			At:           completed[0].At,
		}, completed[0])

		assert.Equal(t, []int64{int64(len(archive))}, recorder.sizes)
		require.Len(t, recorder.took, 1)

		// what was stored is all a restore needs.
		restored := memory.New()
		_, err = restored.Restore(t.Context(), vm.Spec{ID: "vm-2", Kind: vm.KindDocker}, bytes.NewReader(archive))
		require.NoError(t, err)

		disk, err := restored.Disk("vm-2")
		require.NoError(t, err)
		assert.Equal(t, "what was written", string(disk))
	})

	testcases := []struct {
		name    string
		engine  func(t *testing.T) vm.Engine
		store   func() *storage.Storage
		request Request

		wantReason string
	}{
		{
			name:       "a VM that is not here leaves nothing stored, and says why",
			engine:     func(t *testing.T) vm.Engine { return holding(t, "") },
			store:      storage.New,
			request:    Request{SnapshotUUID: "snapshot-1", VMUUID: "vm-2"},
			wantReason: "not exists",
		},
		{
			name:   "a store that is away leaves nothing stored, and lets the engine go",
			engine: func(t *testing.T) vm.Engine { return holding(t, "what was written") },
			store: func() *storage.Storage {
				s := storage.New()
				s.Err = errors.New("S3 is away")

				return s
			},
			request:    Request{SnapshotUUID: "snapshot-1", VMUUID: "vm-1"},
			wantReason: "S3 is away",
		},
		{
			name: "an engine that fails half way through leaves nothing stored",
			engine: func(t *testing.T) vm.Engine {
				var e engine.MockEngine
				e.On("Snapshot", mock.Anything, "vm-1", mock.Anything).
					Run(func(args mock.Arguments) {
						_, _ = io.WriteString(args.Get(2).(io.Writer), "half an archive")
					}).
					Return(vm.Archive{}, errors.New("the disk went away"))

				return &e
			},
			store:      storage.New,
			request:    Request{SnapshotUUID: "snapshot-1", VMUUID: "vm-1"},
			wantReason: "the disk went away",
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := tt.store()
			producer := &messaging.Recorder{}
			recorder := &recorded{}

			_, err := NewUseCase(tt.engine(t), store, lock.New(), producer, validates{}, recorder, nodeName).
				Execute(t.Context(), &tt.request)
			require.NoError(t, err)

			store.Err = nil
			assert.Empty(t, store.Names(), "nothing of a failed snapshot is left behind")
			assert.Empty(t, recorder.sizes)

			failed, err := messaging.Produced[events.SnapshotFailed](producer, events.SnapshotFailedName)
			require.NoError(t, err)
			require.Len(t, failed, 1)

			assert.Equal(t, "snapshot-1", failed[0].SnapshotUUID)
			assert.Equal(t, nodeName, failed[0].NodeName)
			assert.Contains(t, failed[0].Reason, tt.wantReason)
		})
	}

	t.Run("a command that does not name what to take is refused", func(t *testing.T) {
		t.Parallel()

		response, err := NewUseCase(memory.New(), storage.New(), lock.New(), &messaging.Recorder{}, validates{}, &recorded{}, nodeName).
			Execute(t.Context(), &Request{VMUUID: "vm-1"})
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{"snapshot_uuid": "required_field"}, response.ValidationErrors)
	})

	t.Run("saying what became of it failing is worth another delivery", func(t *testing.T) {
		t.Parallel()

		producer := &messaging.Recorder{Err: errors.New("nats is away")}

		_, err := NewUseCase(holding(t, "disk"), storage.New(), lock.New(), producer, validates{}, &recorded{}, nodeName).
			Execute(t.Context(), &Request{SnapshotUUID: "snapshot-1", VMUUID: "vm-1"})
		assert.Error(t, err)
	})
}

func TestSnapshotRequestedHandler_Handle(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name         string
		payload      string
		wantSubjects []string
		wantStored   bool
	}{
		{
			name:         "a snapshot asked of this node is taken here",
			payload:      `{"snapshot_uuid":"snapshot-1","vm_uuid":"vm-1","node_name":"` + nodeName + `"}`,
			wantSubjects: []string{events.SnapshotCompletedName},
			wantStored:   true,
		},
		{
			name:    "one asked of another node is not this node's",
			payload: `{"snapshot_uuid":"snapshot-1","vm_uuid":"vm-1","node_name":"workload-orchestrator-02"}`,
		},
		{
			name:         "one this node refuses is said to have failed",
			payload:      `{"snapshot_uuid":"snapshot-1","node_name":"` + nodeName + `"}`,
			wantSubjects: []string{events.SnapshotFailedName},
		},
		{
			name:    "a message that cannot be read is dropped",
			payload: `{`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := storage.New()
			producer := &messaging.Recorder{}
			useCase := NewUseCase(holding(t, "disk"), store, lock.New(), producer, validates{}, &recorded{}, nodeName)

			require.NoError(t, NewSnapshotRequestedHandler(useCase, nodeName, slog.New(slog.DiscardHandler)).Handle(t.Context(), []byte(tt.payload)))

			assert.Equal(t, tt.wantSubjects, producer.Subjects())

			_, stored := store.Object(snapshot.ObjectKey("snapshot-1"))
			assert.Equal(t, tt.wantStored, stored)
		})
	}
}
