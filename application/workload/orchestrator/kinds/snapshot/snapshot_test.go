package snapshot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/lock"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	snapshotKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/engine"
	storage "github.com/khanzadimahdi/testproject/infrastructure/storage/memory"
	memory "github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

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

func (r *recorded) recordedSizes() []int64 {
	r.lock.Lock()
	defer r.lock.Unlock()

	return append([]int64(nil), r.sizes...)
}

// holding is an engine holding vm-1, a Docker VM whose disk holds disk.
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

// aSnapshot is a snapshot of vm-1 as the control plane records it when it
// asks for it to be taken: creating, with what it took of the VM.
func aSnapshot(uuid string) snapshotKind.Snapshot {
	return snapshotKind.Snapshot{
		Kind: snapshotKind.Name,
		Metadata: kind.Metadata{
			UUID:      uuid,
			Name:      "before the upgrade",
			OwnerUUID: "owner-uuid",
			Owners:    []kind.Reference{{Kind: "vm", UUID: "vm-1"}},
			Node:      "workload-orchestrator-01",
		},
		Spec: snapshotKind.Spec{VM: snapshotKind.VMRef{UUID: "vm-1", Name: "builds"}},
		Status: snapshotKind.Status{
			Status: kind.Status{State: snapshotKind.Creating, Expected: snapshotKind.Ready},
			Flavor: vm.KindDocker,
			Image:  "docker:29-dind",
			Disk:   10 << 30,
		},
	}
}

func TestNode_Execute(t *testing.T) {
	t.Parallel()

	t.Run("a vm's disk is streamed into the bucket, and the snapshot is ready, saying how large it came out and what wrote it", func(t *testing.T) {
		t.Parallel()

		e := holding(t, "what was written")
		archives := storage.New()
		recorder := &recorded{}

		node := New(e, archives, lock.New(), recorder)
		node.now = func() time.Time { return time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC) }

		outcome, err := node.Execute(t.Context(), aSnapshot("snapshot-1"), snapshotKind.ActionCreate, nil)
		require.NoError(t, err)

		archive, stored := archives.Object(snapshotKind.ObjectKey("snapshot-1"))
		require.True(t, stored, "under the snapshot's own key")

		status := outcome.Status
		assert.Equal(t, snapshotKind.Ready, status.State)
		assert.Equal(t, int64(len(archive)), status.Size)
		assert.Equal(t, "memory/1", status.Engine)
		assert.Equal(t, uint64(20<<30), status.Disk, "the disk the engine says a restore needs")
		assert.Equal(t, time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC), status.CompletedAt)
		assert.Equal(t, vm.KindDocker, status.Flavor, "what was taken of the vm is as it was")
		assert.Equal(t, "docker:29-dind", status.Image)

		assert.Equal(t, []int64{int64(len(archive))}, recorder.recordedSizes())

		// what was stored is all a restore needs.
		restored := memory.New()
		_, err = restored.Restore(t.Context(), vm.Spec{ID: "vm-2", Kind: vm.KindDocker}, bytes.NewReader(archive))
		require.NoError(t, err)

		disk, err := restored.Disk("vm-2")
		require.NoError(t, err)
		assert.Equal(t, "what was written", string(disk))
	})

	for _, tt := range []struct {
		name       string
		engine     func(t *testing.T) vm.Engine
		archives   func() *storage.Storage
		wantReason string
	}{
		{
			name:       "a vm that is not here leaves nothing stored, and says why",
			engine:     func(t *testing.T) vm.Engine { return memory.New() },
			archives:   storage.New,
			wantReason: "not exists",
		},
		{
			name:   "a bucket that is away leaves nothing stored, and lets the engine go",
			engine: func(t *testing.T) vm.Engine { return holding(t, "what was written") },
			archives: func() *storage.Storage {
				s := storage.New()
				s.Err = errors.New("S3 is away")

				return s
			},
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
			archives:   storage.New,
			wantReason: "the disk went away",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			archives := tt.archives()
			recorder := &recorded{}

			_, err := New(tt.engine(t), archives, lock.New(), recorder).Execute(t.Context(), aSnapshot("snapshot-1"), snapshotKind.ActionCreate, nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantReason)

			archives.Err = nil
			assert.Empty(t, archives.Names(), "nothing of a failed snapshot is left behind")
			assert.Empty(t, recorder.recordedSizes())
		})
	}

	t.Run("it is taken under its vm's lock, once whatever else is done to the vm is done", func(t *testing.T) {
		t.Parallel()

		locks := lock.New()
		archives := storage.New()

		release, err := locks.Lock(t.Context(), "vm-1")
		require.NoError(t, err)

		done := make(chan error, 1)

		go func() {
			_, err := New(holding(t, "disk"), archives, locks, &recorded{}).Execute(t.Context(), aSnapshot("snapshot-1"), snapshotKind.ActionCreate, nil)
			done <- err
		}()

		select {
		case <-done:
			t.Fatal("it was taken while something else held its vm")
		case <-time.After(50 * time.Millisecond):
		}

		_, stored := archives.Object(snapshotKind.ObjectKey("snapshot-1"))
		assert.False(t, stored)

		release()

		require.NoError(t, <-done)

		_, stored = archives.Object(snapshotKind.ObjectKey("snapshot-1"))
		assert.True(t, stored)
	})

	t.Run("and one whose vm stays held is let go of when it is given up on", func(t *testing.T) {
		t.Parallel()

		locks := lock.New()

		_, err := locks.Lock(t.Context(), "vm-1")
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		defer cancel()

		node := New(holding(t, "disk"), storage.New(), locks, &recorded{})

		_, err = node.Execute(ctx, aSnapshot("snapshot-1"), snapshotKind.ActionCreate, nil)
		assert.ErrorIs(t, err, context.DeadlineExceeded)

		_, took := node.remembered("snapshot-1")
		assert.False(t, took, "it was not taken, and is taken by whoever is asked next")
	})

	t.Run("one asked for again is what it came to the first time, rather than taken again over it", func(t *testing.T) {
		t.Parallel()

		e := holding(t, "the first disk")
		archives := storage.New()
		recorder := &recorded{}
		node := New(e, archives, lock.New(), recorder)

		first, err := node.Execute(t.Context(), aSnapshot("snapshot-1"), snapshotKind.ActionCreate, nil)
		require.NoError(t, err)

		require.NoError(t, e.SetDisk("vm-1", []byte("written since")))

		again, err := node.Execute(t.Context(), aSnapshot("snapshot-1"), snapshotKind.ActionCreate, nil)
		require.NoError(t, err)
		assert.Equal(t, first.Status, again.Status)

		archive, _ := archives.Object(snapshotKind.ObjectKey("snapshot-1"))

		restored := memory.New()
		_, err = restored.Restore(t.Context(), vm.Spec{ID: "vm-2", Kind: vm.KindDocker}, bytes.NewReader(archive))
		require.NoError(t, err)

		disk, err := restored.Disk("vm-2")
		require.NoError(t, err)
		assert.Equal(t, "the first disk", string(disk), "the archive is the first one")
		assert.Len(t, recorder.recordedSizes(), 1)
	})

	t.Run("and so is one that failed", func(t *testing.T) {
		t.Parallel()

		var e engine.MockEngine
		e.On("Snapshot", mock.Anything, "vm-1", mock.Anything).Once().Return(vm.Archive{}, errors.New("the disk went away"))

		node := New(&e, storage.New(), lock.New(), &recorded{})

		_, err := node.Execute(t.Context(), aSnapshot("snapshot-1"), snapshotKind.ActionCreate, nil)
		require.Error(t, err)

		_, again := node.Execute(t.Context(), aSnapshot("snapshot-1"), snapshotKind.ActionCreate, nil)
		assert.Equal(t, err, again)

		e.AssertNumberOfCalls(t, "Snapshot", 1)
	})

	t.Run("only so many are remembered, the oldest let go of first", func(t *testing.T) {
		t.Parallel()

		node := New(memory.New(), storage.New(), lock.New(), &recorded{})

		for i := range remembered + 1 {
			node.remember(fmt.Sprintf("snapshot-%d", i), taken{})
		}

		_, kept := node.remembered("snapshot-0")
		assert.False(t, kept)

		_, kept = node.remembered(fmt.Sprintf("snapshot-%d", remembered))
		assert.True(t, kept)
		assert.Len(t, node.taken, remembered)
	})

	t.Run("a snapshot that names no vm is refused", func(t *testing.T) {
		t.Parallel()

		s := aSnapshot("snapshot-1")
		s.Metadata.Owners = nil
		s.Spec.VM = snapshotKind.VMRef{}

		_, err := New(memory.New(), storage.New(), lock.New(), &recorded{}).Execute(t.Context(), s, snapshotKind.ActionCreate, nil)
		assert.ErrorIs(t, err, kind.ErrInvalidPayload)
	})

	t.Run("nothing but taking it is done on a node", func(t *testing.T) {
		t.Parallel()

		_, err := New(memory.New(), storage.New(), lock.New(), &recorded{}).Execute(t.Context(), aSnapshot("snapshot-1"), snapshotKind.ActionDelete, nil)
		assert.ErrorIs(t, err, kind.ErrUnknownAction)
	})
}

func TestNode_Query(t *testing.T) {
	t.Parallel()

	_, err := New(memory.New(), storage.New(), lock.New(), &recorded{}).Query(t.Context(), aSnapshot("snapshot-1"), snapshotKind.ActionState, nil)
	assert.ErrorIs(t, err, kind.ErrUnknownAction, "a snapshot's state is its record's")
}

func TestNode_State(t *testing.T) {
	t.Parallel()

	report, err := New(holding(t, "disk"), storage.New(), lock.New(), &recorded{}).State(t.Context())
	require.NoError(t, err)
	assert.Empty(t, report.Instances, "a node holds nothing of a snapshot")
}

// TestNode_bound holds what is bound to the kind to carrying a command out as
// a node does: a command delivered names the snapshot it carries.
func TestNode_bound(t *testing.T) {
	t.Parallel()

	binding := kind.BindNode[snapshotKind.Spec, snapshotKind.Status](snapshotKind.Descriptor(), New(holding(t, "disk"), storage.New(), lock.New(), &recorded{}))

	raw, err := kind.Encode(aSnapshot("snapshot-1"))
	require.NoError(t, err)

	result := binding.Execute(t.Context(), kind.Command{ID: "command-1", Kind: snapshotKind.Name, UUID: "snapshot-1", Action: snapshotKind.ActionCreate, Resource: raw})
	require.True(t, result.OK, result.Reason)

	var status snapshotKind.Status
	require.NoError(t, json.Unmarshal(result.Status, &status))
	assert.Equal(t, snapshotKind.Ready, status.State)
	assert.Positive(t, status.Size)

	failed := binding.Execute(t.Context(), kind.Command{ID: "command-2", Kind: snapshotKind.Name, UUID: "snapshot-2", Action: snapshotKind.ActionCreate, Resource: raw})
	assert.False(t, failed.OK, "a command carrying another snapshot than it names")

	_, err = binding.Query(t.Context(), kind.Query{Kind: snapshotKind.Name, UUID: "snapshot-1", Action: snapshotKind.ActionState, Resource: raw})
	assert.True(t, errors.Is(err, kind.ErrUnknownAction) || errors.Is(err, domain.ErrNotExists), "a node is never asked a snapshot's state")
}
