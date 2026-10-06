package workload_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/getVMContainers"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/input"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/createSnapshot"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/getSnapshot"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/createStack"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/getStack"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/getStacks"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/createVM"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/deleteVM"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/getVM"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/restoreVM"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/startVM"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/stopVM"
	"github.com/khanzadimahdi/testproject/domain"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	infraDocker "github.com/khanzadimahdi/testproject/infrastructure/workload/docker"
)

// TestAMachineVM walks a VM through its life from the dashboard: asked for,
// made on its node and reported running, stopped, snapshotted into the
// bucket, started, restored from the snapshot, and deleted.
func TestAMachineVM(t *testing.T) {
	t.Parallel()

	w := start(t)
	ctx := t.Context()

	created, err := createVM.NewUseCase(w.client, w.validator, w.translator, w.owners, ingressDomain).Execute(ctx, &createVM.Request{
		Name:           "box",
		Kind:           string(vm.KindMachine),
		Resources:      input.Resources{CPUs: 2, Memory: 1 << 30, Disk: 10 << 30},
		Ports:          []uint{8080},
		Network:        input.Network{Ingress: "allow", Egress: "deny"},
		PersistentDisk: true,
		OwnerUUID:      ownerUUID,
	})
	require.NoError(t, err)
	require.Empty(t, created.ValidationErrors)
	require.NotNil(t, created.VM)

	uuid := created.VM.UUID
	assert.Equal(t, "scheduled", created.VM.State, "it is asked of its node, which has not made it yet")

	t.Run("the control plane schedules it, its node makes it, and its heartbeat says it runs", func(t *testing.T) {
		running := w.vmIn(t, uuid, "running")

		assert.Equal(t, nodeName, running.NodeName)
		assert.Equal(t, []presenter.URL{{Port: 8080, URL: "https://" + running.Slug + "-8080." + ingressDomain}}, running.URLs)

		instance, err := w.engine.Inspect(ctx, uuid)
		require.NoError(t, err)
		assert.Equal(t, vm.InstanceRunning, instance.State)

		// what a node tells its VMs by, and opens a terminal for, is what
		// the control plane labelled it with.
		assert.Equal(t, map[string]string{
			vm.LabelOwner:   ownerUUID,
			vm.LabelVM:      uuid,
			vm.LabelSlug:    running.Slug,
			vm.LabelPurpose: vm.PurposeVM,
		}, instance.Labels)

		spec, err := w.engine.Spec(uuid)
		require.NoError(t, err)
		assert.Equal(t, vm.KindMachine, spec.Kind)
		assert.Equal(t, vm.Resources{CPUs: 2, Memory: 1 << 30, Disk: 10 << 30}, spec.Resources)
		assert.Equal(t, []port.Port{8080}, spec.Ports)
		assert.Equal(t, vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny}, spec.Network)
		assert.True(t, spec.PersistentDisk)
	})

	t.Run("what its engine samples is what the dashboard shows, cpu_percent of all its vCPUs untouched", func(t *testing.T) {
		// one and a half of its two vCPUs busy.
		require.NoError(t, w.engine.SetStats(uuid, vm.Stats{CPUPercent: 75, MemoryUsed: 256 << 20}))

		sampled := w.vm(t, uuid, "sampled", func(v presenter.VM) bool { return v.Stats != nil && v.Stats.CPUPercent > 0 })

		assert.Equal(t, 75.0, sampled.Stats.CPUPercent)
		assert.Equal(t, uint64(256<<20), sampled.Stats.MemoryUsed)
		assert.Equal(t, uint64(1<<30), sampled.Stats.MemoryLimit)
	})

	t.Run("stopped", func(t *testing.T) {
		stopped, err := stopVM.NewUseCase(w.client, w.translator).Execute(ctx, &stopVM.Request{UUID: uuid, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, stopped.ValidationErrors)

		w.vmIn(t, uuid, "stopped")

		instance, err := w.engine.Inspect(ctx, uuid)
		require.NoError(t, err)
		assert.Equal(t, vm.InstanceStopped, instance.State)
	})

	require.NoError(t, w.engine.SetDisk(uuid, []byte("what was on it")))

	var snapshotUUID string

	t.Run("a snapshot of its disk is ready once its archive is in the bucket", func(t *testing.T) {
		taken, err := createSnapshot.NewUseCase(w.client, w.validator, w.translator, w.owners).Execute(ctx, &createSnapshot.Request{
			VMUUID:    uuid,
			Name:      "before the upgrade",
			OwnerUUID: ownerUUID,
		})
		require.NoError(t, err)
		require.Empty(t, taken.ValidationErrors)
		require.NotNil(t, taken.Snapshot)

		snapshotUUID = taken.Snapshot.UUID

		ready := eventually(t, "the snapshot being ready", func(ctx context.Context) (presenter.Snapshot, error) {
			read, err := getSnapshot.NewUseCase(w.client, w.owners).Execute(ctx, &getSnapshot.Request{UUID: snapshotUUID, OwnerUUID: ownerUUID})
			if err != nil {
				return presenter.Snapshot{}, err
			}

			return read.Snapshot, nil
		}, func(s presenter.Snapshot) bool { return s.State == "ready" })

		archive, stored := w.archives.Object(snapshot.ObjectKey(snapshotUUID))
		require.True(t, stored, "the node stored the archive where the snapshot says")
		assert.Equal(t, int64(len(archive)), ready.Size)
		assert.Equal(t, "memory/1", ready.Engine)
		assert.Equal(t, uuid, ready.VMUUID)
	})

	require.NotEmpty(t, snapshotUUID)

	t.Run("started again", func(t *testing.T) {
		started, err := startVM.NewUseCase(w.client, w.translator).Execute(ctx, &startVM.Request{UUID: uuid, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, started.ValidationErrors)

		w.vmIn(t, uuid, "running")
	})

	require.NoError(t, w.engine.SetDisk(uuid, []byte("what was written since")))

	t.Run("restored from the snapshot, it has the snapshot's disk and runs again", func(t *testing.T) {
		restored, err := restoreVM.NewUseCase(w.client, w.validator, w.translator).Execute(ctx, &restoreVM.Request{
			UUID:         uuid,
			OwnerUUID:    ownerUUID,
			SnapshotUUID: snapshotUUID,
		})
		require.NoError(t, err)
		require.Empty(t, restored.ValidationErrors)

		eventually(t, "the vm running from the snapshot", func(ctx context.Context) (vm.VM, error) {
			return w.vms.GetOne(ctx, uuid)
		}, func(v vm.VM) bool { return v.CurrentState == vm.Running && len(v.RestoreFrom) == 0 })

		disk, err := w.engine.Disk(uuid)
		require.NoError(t, err)
		assert.Equal(t, "what was on it", string(disk))

		assert.Equal(t, "running", w.vmIn(t, uuid, "running").State)
	})

	t.Run("deleted, it is gone from its node and then from the control plane, and its snapshot outlives it", func(t *testing.T) {
		deleted, err := deleteVM.NewUseCase(w.client, w.translator).Execute(ctx, &deleteVM.Request{UUID: uuid, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, deleted.ValidationErrors)

		require.Eventually(t, func() bool {
			_, err := w.vms.GetOne(ctx, uuid)

			return errors.Is(err, domain.ErrNotExists)
		}, settle, beat, "the record was never removed")

		_, err = w.engine.Inspect(ctx, uuid)
		assert.ErrorIs(t, err, domain.ErrNotExists)

		_, err = getVM.NewUseCase(w.client, w.owners, ingressDomain).Execute(ctx, &getVM.Request{UUID: uuid, OwnerUUID: ownerUUID})
		assert.ErrorIs(t, err, domain.ErrNotExists, "the dashboard finds it gone")

		_, err = w.snapshots.GetOne(ctx, snapshotUUID)
		assert.NoError(t, err)
	})
}

// TestADockerVM walks a Docker VM from the dashboard: asked for and running,
// its dockerd asked through the control plane and its node, a stack deployed
// into it by compose, and the VM deleted with its stack.
func TestADockerVM(t *testing.T) {
	t.Parallel()

	w := start(t)
	ctx := t.Context()

	created, err := createVM.NewUseCase(w.client, w.validator, w.translator, w.owners, ingressDomain).Execute(ctx, &createVM.Request{
		Name:           "builds",
		Kind:           string(vm.KindDocker),
		Resources:      input.Resources{CPUs: 2, Memory: 2 << 30, Disk: 20 << 30},
		Ports:          []uint{80},
		Network:        input.Network{Ingress: "allow", Egress: "allow"},
		PersistentDisk: true,
		OwnerUUID:      ownerUUID,
	})
	require.NoError(t, err)
	require.Empty(t, created.ValidationErrors)
	require.NotNil(t, created.VM)

	uuid := created.VM.UUID

	w.vmIn(t, uuid, "running")

	spec, err := w.engine.Spec(uuid)
	require.NoError(t, err)
	assert.Equal(t, vm.KindDocker, spec.Kind)
	assert.Equal(t, "docker:29-dind", spec.Image, "a Docker VM boots from the workload's own image")

	t.Run("its dockerd answers, through the control plane and its node", func(t *testing.T) {
		require.NoError(t, w.client.Docker(ownerUUID, uuid).Ping(ctx))

		listed, err := getVMContainers.NewUseCase(w.client, w.translator).Execute(ctx, &getVMContainers.Request{VMUUID: uuid, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, listed.ValidationErrors)

		require.Len(t, listed.Items, 1)
		assert.Equal(t, "c0ffee", listed.Items[0].ID)
		assert.Equal(t, "db", listed.Items[0].Name)
		assert.Equal(t, "running", listed.Items[0].State)
	})

	const compose = "services:\n  web:\n    image: nginx:1.27\n"

	var stackUUID, slug string

	t.Run("a stack is deployed into it by compose, and runs", func(t *testing.T) {
		deployed, err := createStack.NewUseCase(w.client, w.validator, w.translator, w.owners).Execute(ctx, &createStack.Request{
			Name:      "shop",
			Compose:   compose,
			VMUUID:    uuid,
			OwnerUUID: ownerUUID,
		})
		require.NoError(t, err)
		require.Empty(t, deployed.ValidationErrors)
		require.NotNil(t, deployed.Stack)

		stackUUID, slug = deployed.Stack.UUID, deployed.Stack.Slug
		assert.Equal(t, uuid, deployed.VM.UUID)
		assert.Equal(t, "builds", deployed.Stack.VMName)

		detail := eventually(t, "the stack running", func(ctx context.Context) (presenter.StackDetail, error) {
			read, err := getStack.NewUseCase(w.client, w.owners).Execute(ctx, &getStack.Request{UUID: stackUUID, OwnerUUID: ownerUUID})
			if err != nil {
				return presenter.StackDetail{}, err
			}

			return read.StackDetail, nil
		}, func(s presenter.StackDetail) bool { return s.State == "running" })

		labelled, err := infraDocker.Labelled(compose, stackUUID)
		require.NoError(t, err)

		composed, ok := w.dockerd.Composed(slug)
		require.True(t, ok, "compose was run on the stack's own project")
		assert.Equal(t, labelled, composed, "with its file, every service of it labelled as the stack's")

		assert.Equal(t, "builds", detail.VMName)
		assert.Contains(t, detail.Output, "Started")
		assert.Empty(t, detail.Note)

		require.Len(t, detail.Containers, 1, "the containers compose made for it, and no other")
		assert.Equal(t, slug, detail.Containers[0].Stack)
		assert.Equal(t, "web", detail.Containers[0].Service)
		assert.Equal(t, stackUUID, detail.Containers[0].StackUUID)

		listing, err := getStacks.NewUseCase(w.client, w.owners).Execute(ctx, &getStacks.Request{OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Len(t, listing.Items, 1)
		assert.Equal(t, "builds", listing.Items[0].VMName)
	})

	require.NotEmpty(t, stackUUID)

	t.Run("deleted, it takes its stack with it", func(t *testing.T) {
		deleted, err := deleteVM.NewUseCase(w.client, w.translator).Execute(ctx, &deleteVM.Request{UUID: uuid, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, deleted.ValidationErrors)

		require.Eventually(t, func() bool {
			_, err := w.vms.GetOne(ctx, uuid)

			return errors.Is(err, domain.ErrNotExists)
		}, settle, beat, "the record was never removed")

		_, err = w.resources.GetOne(ctx, stackKind.Name, stackUUID)
		assert.ErrorIs(t, err, domain.ErrNotExists, "a vm's stacks go with it")

		_, err = w.engine.Inspect(ctx, uuid)
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}

// vmIn is the VM as the dashboard shows it, once it is in state.
func (w *workload) vmIn(t *testing.T, uuid string, state string) presenter.VM {
	t.Helper()

	return w.vm(t, uuid, state, func(v presenter.VM) bool { return v.State == state })
}

// vm is the VM as the dashboard shows it, once condition holds of it.
func (w *workload) vm(t *testing.T, uuid string, what string, condition func(presenter.VM) bool) presenter.VM {
	t.Helper()

	shown := getVM.NewUseCase(w.client, w.owners, ingressDomain)

	return eventually(t, "the vm "+what, func(ctx context.Context) (presenter.VM, error) {
		read, err := shown.Execute(ctx, &getVM.Request{UUID: uuid, OwnerUUID: ownerUUID})
		if err != nil {
			return presenter.VM{}, err
		}

		return read.VM, nil
	}, condition)
}

// eventually is what read answers once condition holds of it. A test that
// waits in vain fails with what read last answered, which is what says where
// things stopped.
func eventually[T any](t *testing.T, what string, read func(ctx context.Context) (T, error), condition func(T) bool) T {
	t.Helper()

	var (
		lock    sync.Mutex
		last    T
		failure error
	)

	held := assert.Eventually(t, func() bool {
		answered, err := read(t.Context())

		lock.Lock()
		defer lock.Unlock()

		last, failure = answered, err

		return err == nil && condition(answered)
	}, settle, beat)

	lock.Lock()
	defer lock.Unlock()

	if !held {
		t.Fatalf("%s never happened: last read %+v, %v", what, last, failure)
	}

	return last
}
