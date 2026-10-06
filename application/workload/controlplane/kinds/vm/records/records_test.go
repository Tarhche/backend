package records_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/records"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

func recordsOf(t *testing.T, vms ...vmKind.VM) *records.Records {
	t.Helper()

	kept := make([]resource.Record, len(vms))
	for i := range vms {
		kept[i] = vmtest.Record(vms[i])
	}

	return records.New(resourcesMemory.NewRepository(kept...))
}

func uuidsOf(vms []vmKind.VM) []string {
	uuids := make([]string, len(vms))
	for i := range vms {
		uuids[i] = vms[i].Metadata.UUID
	}

	return uuids
}

func TestRecords(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	elsewhere := vmtest.In(vmtest.Docker("03", "other"), func(v *vmKind.VM) { v.Metadata.Node = "workload-orchestrator-02" })

	r := recordsOf(t, vmtest.Running("01", "owner"), vmtest.Docker("02", "owner"), elsewhere)

	t.Run("a vm is read as its kind's manifest, by its uuid and by its slug", func(t *testing.T) {
		t.Parallel()

		v, err := r.GetOne(ctx, "01")
		require.NoError(t, err)
		assert.Equal(t, vmtest.Running("01", "owner").Spec, v.Spec)

		v, err = r.GetOneBySlug(ctx, "box-02")
		require.NoError(t, err)
		assert.Equal(t, "02", v.Metadata.UUID)

		_, err = r.GetOne(ctx, "09")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("one's own is theirs alone", func(t *testing.T) {
		t.Parallel()

		v, err := r.GetOneByOwner(ctx, "owner", "01")
		require.NoError(t, err)
		assert.Equal(t, "01", v.Metadata.UUID)

		_, err = r.GetOneByOwner(ctx, "other", "01")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("a person's, a node's, and those of a flavor", func(t *testing.T) {
		t.Parallel()

		owned, err := r.Owned(ctx, "owner")
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"01", "02"}, uuidsOf(owned))

		held, err := r.Held(ctx, vmtest.Node)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"01", "02"}, uuidsOf(held))

		docker, err := r.All(ctx, resource.Filter{Labels: map[string]string{vmKind.LabelFlavor: "docker"}})
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"02", "03"}, uuidsOf(docker))

		nobodys, err := r.Owned(ctx, "")
		require.NoError(t, err)
		assert.Empty(t, nobodys, "nobody owns nothing")

		nowhere, err := r.Held(ctx, "")
		require.NoError(t, err)
		assert.Empty(t, nowhere, "and nothing is held nowhere")
	})
}

func TestRecords_Down(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	r := recordsOf(t,
		vmtest.Docker("running", "owner"),
		vmtest.In(vmtest.Docker("stopped", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Stopped }),
		vmtest.In(vmtest.Docker("starting", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Starting }),
	)

	for name, tt := range map[string]struct {
		parent kind.Reference
		down   kind.State
	}{
		"a vm that runs says nothing":                    {parent: kind.Reference{Kind: vmKind.Name, UUID: "running"}},
		"one that does not run says what it is doing":    {parent: kind.Reference{Kind: vmKind.Name, UUID: "stopped"}, down: vmKind.Stopped},
		"one on its way up is not running yet":           {parent: kind.Reference{Kind: vmKind.Name, UUID: "starting"}, down: vmKind.Starting},
		"one that is gone says nothing: it is gone":      {parent: kind.Reference{Kind: vmKind.Name, UUID: "gone"}},
		"a parent that is not a vm is not for it to say": {parent: kind.Reference{Kind: stackKind.Name, UUID: "running"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			down, err := r.Down(ctx, tt.parent)
			require.NoError(t, err)
			assert.Equal(t, tt.down, down)
		})
	}

	t.Run("records that cannot be read are said", func(t *testing.T) {
		t.Parallel()

		_, err := records.New(unreadable{}).Down(ctx, kind.Reference{Kind: vmKind.Name, UUID: "running"})
		assert.Error(t, err)
	})
}

// unreadable is a store of resources that cannot be read.
type unreadable struct {
	resource.Repository
}

func (unreadable) GetOne(context.Context, string, string) (resource.Record, error) {
	return resource.Record{}, errors.New("the database is gone")
}

func TestGoing(t *testing.T) {
	t.Parallel()

	assert.False(t, records.Going(vmtest.Running("01", "owner")))
	assert.True(t, records.Going(vmtest.In(vmtest.Running("01", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Deleting })))
	assert.True(t, records.Going(vmtest.In(vmtest.Running("01", "owner"), func(v *vmKind.VM) { v.Status.Expected = vmKind.Deleted })))
}

func TestDecode(t *testing.T) {
	t.Parallel()

	_, err := records.Decode(resource.Record{Raw: kind.Raw{Kind: stackKind.Name}})
	assert.ErrorIs(t, err, kind.ErrUnknownKind, "a stack is no vm")
}

func TestEntities(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	e := records.NewEntities(recordsOf(t, vmtest.Running("01", "owner"), vmtest.Docker("02", "owner"), vmtest.Docker("03", "other")))

	t.Run("a vm is read as the vm package's entity, as the parts not on the framework yet read one", func(t *testing.T) {
		t.Parallel()

		v, err := e.GetOne(ctx, "02")
		require.NoError(t, err)
		assert.Equal(t, "02", v.UUID)
		assert.Equal(t, "owner", v.OwnerUUID)
		assert.Equal(t, vm.KindDocker, v.Kind)
		assert.Equal(t, vm.Running, v.CurrentState)
		assert.Equal(t, vm.Resources{CPUs: 2, Memory: 2 * vmtest.GiB, Disk: 20 * vmtest.GiB}, v.Resources)
		assert.Equal(t, vmtest.Node, v.NodeName)
		assert.False(t, v.LastHeartbeatAt.IsZero(), "when its node last observed it")

		_, err = e.GetOneByOwner(ctx, "other", "02")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("a person's of one flavor", func(t *testing.T) {
		t.Parallel()

		docker, err := e.GetAllByOwnerAndKind(ctx, "owner", vm.KindDocker)
		require.NoError(t, err)
		require.Len(t, docker, 1)
		assert.Equal(t, "02", docker[0].UUID)
	})

	t.Run("anybody's, a page at a time", func(t *testing.T) {
		t.Parallel()

		first, err := e.GetAll(ctx, 0, 2)
		require.NoError(t, err)
		assert.Len(t, first, 2)

		rest, err := e.GetAll(ctx, 2, 2)
		require.NoError(t, err)
		assert.Len(t, rest, 1)
	})
}
