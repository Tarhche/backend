package workload_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/createSnapshot"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/deleteSnapshot"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/getSnapshot"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/getSnapshots"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/renameSnapshot"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/createVM"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/restoreVM"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	snapshotKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/snapshot"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/input"
)

// TestASnapshot walks a snapshot through its life from the dashboard, as a
// kind the control plane keeps and a node takes: taken of a VM's disk into
// the bucket, restored onto the VM and as a new VM, renamed, and deleted with
// its archive; and refused as what a VM of another flavor, or with too small
// a disk, is restored from.
func TestASnapshot(t *testing.T) {
	t.Parallel()

	w := start(t)
	ctx := t.Context()

	vmUUID := w.machine(t, "box", 10<<30)
	w.vmIn(t, vmUUID, "running")

	require.NoError(t, w.engine.SetDisk(vmUUID, []byte("what was on it")))

	taken, err := createSnapshot.NewUseCase(w.client, w.validator, w.translator, w.owners).Execute(ctx, &createSnapshot.Request{
		VMUUID:    vmUUID,
		Name:      "before the upgrade",
		OwnerUUID: ownerUUID,
	})
	require.NoError(t, err)
	require.Empty(t, taken.ValidationErrors)
	require.NotNil(t, taken.Snapshot)

	snapshotUUID := taken.Snapshot.UUID

	assert.Equal(t, "creating", taken.Snapshot.State, "its vm's node is asked to take it")
	assert.Equal(t, vmUUID, taken.Snapshot.VMUUID)
	assert.Equal(t, "box", taken.Snapshot.VMName)
	assert.Equal(t, "machine", taken.Snapshot.Kind)

	t.Run("taken of a running vm, it is ready once its archive is in the bucket", func(t *testing.T) {
		ready := w.snapshotIn(t, snapshotUUID, "ready")

		archive, stored := w.archives.Object(snapshotKind.ObjectKey(snapshotUUID))
		require.True(t, stored, "the node stored the archive where the snapshot says")

		assert.Equal(t, int64(len(archive)), ready.Size)
		assert.Equal(t, "memory/1", ready.Engine)
		assert.Equal(t, uint64(10<<30), ready.Disk)
		assert.NotNil(t, ready.CompletedAt)

		r, err := w.resources.GetOne(ctx, snapshotKind.Name, snapshotUUID)
		require.NoError(t, err)
		assert.Nil(t, r.Pending, "its create was answered, and nothing waits on it")
		assert.Equal(t, []kind.Reference{{Kind: vmKind.Name, UUID: vmUUID}}, r.Metadata.Owners, "it is taken of its vm")

		listed, err := getSnapshots.NewUseCase(w.client, w.owners).Execute(ctx, &getSnapshots.Request{VMUUID: vmUUID, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Len(t, listed.Items, 1, "it is among its vm's snapshots")
		assert.Equal(t, snapshotUUID, listed.Items[0].UUID)
	})

	require.NoError(t, w.engine.SetDisk(vmUUID, []byte("what was written since")))

	t.Run("restored onto its vm, the vm has its disk again", func(t *testing.T) {
		restored, err := restoreVM.NewUseCase(w.client, w.validator, w.translator).Execute(ctx, &restoreVM.Request{
			UUID:         vmUUID,
			OwnerUUID:    ownerUUID,
			SnapshotUUID: snapshotUUID,
		})
		require.NoError(t, err)
		require.Empty(t, restored.ValidationErrors)

		w.stored(t, vmUUID, "running from the snapshot", func(v vmKind.VM) bool {
			return v.Status.State == vmKind.Running && v.Status.Expected == vmKind.Running
		})

		disk, err := w.engine.Disk(vmUUID)
		require.NoError(t, err)
		assert.Equal(t, "what was on it", string(disk))
	})

	t.Run("restored as a new vm, the vm is made with its disk", func(t *testing.T) {
		made, err := createVM.NewUseCase(w.client, w.validator, w.translator, w.owners, ingressDomain).Execute(ctx, &createVM.Request{
			Name:         "box again",
			Kind:         string(vm.KindMachine),
			Resources:    input.Resources{CPUs: 1, Memory: 1 << 30},
			Network:      input.Network{Ingress: "allow", Egress: "allow"},
			SnapshotUUID: snapshotUUID,
			OwnerUUID:    ownerUUID,
		})
		require.NoError(t, err)
		require.Empty(t, made.ValidationErrors)
		require.NotNil(t, made.VM)

		again := w.vmIn(t, made.VM.UUID, "running")
		assert.Equal(t, uint64(10<<30), again.Resources.Disk, "its disk is the snapshot's, which it asked no larger than")

		disk, err := w.engine.Disk(made.VM.UUID)
		require.NoError(t, err)
		assert.Equal(t, "what was on it", string(disk))
	})

	t.Run("it is not restored onto a vm of another flavor", func(t *testing.T) {
		created, err := createVM.NewUseCase(w.client, w.validator, w.translator, w.owners, ingressDomain).Execute(ctx, &createVM.Request{
			Name:      "builds",
			Kind:      string(vm.KindDocker),
			Resources: input.Resources{CPUs: 1, Memory: 1 << 30, Disk: 20 << 30},
			Network:   input.Network{Ingress: "allow", Egress: "allow"},
			OwnerUUID: ownerUUID,
		})
		require.NoError(t, err)
		require.Empty(t, created.ValidationErrors)

		w.vmIn(t, created.VM.UUID, "running")

		refused, err := restoreVM.NewUseCase(w.client, w.validator, w.translator).Execute(ctx, &restoreVM.Request{
			UUID:         created.VM.UUID,
			OwnerUUID:    ownerUUID,
			SnapshotUUID: snapshotUUID,
		})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"snapshot_uuid": "the snapshot is of a VM of another kind"}, refused.ValidationErrors)
	})

	t.Run("nor onto one whose disk it does not fit in", func(t *testing.T) {
		small := w.machine(t, "small", 5<<30)
		w.vmIn(t, small, "running")

		refused, err := restoreVM.NewUseCase(w.client, w.validator, w.translator).Execute(ctx, &restoreVM.Request{
			UUID:         small,
			OwnerUUID:    ownerUUID,
			SnapshotUUID: snapshotUUID,
		})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"snapshot_uuid": "the disk is smaller than the snapshot's"}, refused.ValidationErrors)

		disk, err := w.engine.Disk(small)
		require.NoError(t, err)
		assert.Empty(t, disk, "and its own disk is as it was")
	})

	t.Run("renamed, it is called what it was given", func(t *testing.T) {
		renamed, err := renameSnapshot.NewUseCase(w.client, w.validator, w.translator, w.owners).Execute(ctx, &renameSnapshot.Request{
			UUID:      snapshotUUID,
			OwnerUUID: ownerUUID,
			Name:      "after the upgrade",
		})
		require.NoError(t, err)
		require.Empty(t, renamed.ValidationErrors)
		assert.Equal(t, "after the upgrade", renamed.Snapshot.Name)

		read := w.snapshotIn(t, snapshotUUID, "ready")
		assert.Equal(t, "after the upgrade", read.Name)
	})

	t.Run("deleted, it is gone, and so is its archive", func(t *testing.T) {
		deleted, err := deleteSnapshot.NewUseCase(w.client, w.translator).Execute(ctx, &deleteSnapshot.Request{UUID: snapshotUUID, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, deleted.ValidationErrors)

		_, err = getSnapshot.NewUseCase(w.client, w.owners).Execute(ctx, &getSnapshot.Request{UUID: snapshotUUID, OwnerUUID: ownerUUID})
		assert.ErrorIs(t, err, domain.ErrNotExists)

		_, stored := w.archives.Object(snapshotKind.ObjectKey(snapshotUUID))
		assert.False(t, stored)

		w.vmIn(t, vmUUID, "running")
	})
}

// machine is a machine VM asked for from the dashboard, with a disk of that
// many bytes.
func (w *workload) machine(t *testing.T, name string, disk uint64) string {
	t.Helper()

	created, err := createVM.NewUseCase(w.client, w.validator, w.translator, w.owners, ingressDomain).Execute(t.Context(), &createVM.Request{
		Name:           name,
		Kind:           string(vm.KindMachine),
		Resources:      input.Resources{CPUs: 1, Memory: 1 << 30, Disk: disk},
		Network:        input.Network{Ingress: "allow", Egress: "allow"},
		PersistentDisk: true,
		OwnerUUID:      ownerUUID,
	})
	require.NoError(t, err)
	require.Empty(t, created.ValidationErrors)
	require.NotNil(t, created.VM)

	return created.VM.UUID
}

// snapshotIn is the snapshot as the dashboard shows it, once it is in state.
func (w *workload) snapshotIn(t *testing.T, uuid string, state string) presenter.Snapshot {
	t.Helper()

	shown := getSnapshot.NewUseCase(w.client, w.owners)

	return eventually(t, "the snapshot "+state, func(ctx context.Context) (presenter.Snapshot, error) {
		read, err := shown.Execute(ctx, &getSnapshot.Request{UUID: uuid, OwnerUUID: ownerUUID})
		if err != nil {
			return presenter.Snapshot{}, err
		}

		return read.Snapshot, nil
	}, func(s presenter.Snapshot) bool { return s.State == state })
}
