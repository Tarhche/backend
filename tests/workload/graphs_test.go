package workload_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/createContainer"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/deleteContainer"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/deleteVM"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/volume/createVolume"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/volume/deleteVolume"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	blockKinds "github.com/khanzadimahdi/testproject/domain/workload/kinds/blocks"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
)

// TestAContainersGraph walks a container that names no Docker VM through its
// graph, from the dashboard: the VM it goes into is made for it by its graph,
// rather than when it is asked for, then the network it joins and the volume
// it mounts, and then it; its own volume, deleted, is made again, and so is
// its VM, under its uuid, and then what lives in it; and deleted itself, it
// takes the network it owned with it, and leaves its volume and its VM.
func TestAContainersGraph(t *testing.T) {
	t.Parallel()

	w := start(t)
	ctx := t.Context()

	created, err := createContainer.NewUseCase(w.client, w.validator, w.translator).Execute(ctx, &createContainer.Request{
		Name:          "web",
		Image:         "nginx:1.27",
		RestartPolicy: containerKind.RestartAlways,
		Networks:      []string{"backend"},
		Mounts:        []createContainer.Mount{{Type: containerKind.MountVolume, Source: "data", Target: "/data"}},
		OwnerUUID:     ownerUUID,
	})
	require.NoError(t, err)
	require.Empty(t, created.ValidationErrors)
	require.NotNil(t, created.Container)

	vmUUID := created.VM.UUID
	web := w.containerRecord(t, vmUUID, "web")
	root := graph.Ref{Kind: containerKind.Name, UUID: web.Metadata.UUID}

	t.Run("its graph made the vm it named none of, then the network and the volume it names, and then it, which runs", func(t *testing.T) {
		assert.True(t, created.VM.Created, "it named no docker vm, so one was made for it")
		assert.Equal(t, "docker", created.VM.Name)
		assert.Equal(t, "running", created.Container.State)

		plan := w.plan(t, root, "made", func(executions []graph.Execution) bool {
			return len(executions) == 4 && !slices.ContainsFunc(executions, func(e graph.Execution) bool { return e.State != graph.StateDone })
		})

		levels := map[graph.NodeID]int{}
		for _, e := range plan {
			levels[e.Node] = e.Level
		}

		assert.Equal(t, map[graph.NodeID]int{
			graph.ID(vmKind.Name, vmUUID):         0,
			graph.ID(networkKind.Name, "backend"): 1,
			graph.ID(volumeKind.Name, "data"):     1,
			graph.ID(containerKind.Name, "web"):   2,
		}, levels, "its vm first, then what lives in it, then it")

		for _, e := range plan {
			for _, before := range plan {
				if before.Level < e.Level {
					assert.False(t, e.StartedAt.Before(before.FinishedAt), "%s was started once %s was done", e.Node, before.Node)
				}
			}
		}

		v := w.stored(t, vmUUID, "running", func(v vmKind.VM) bool { return v.Status.State == vmKind.Running })
		assert.Equal(t, "docker", v.Metadata.Name)
		assert.True(t, vmKind.DockerVM(v, configs.NewWorkloadControlPlane().VMDockerImage), "a docker vm")

		kept := w.containerRecord(t, vmUUID, "web")
		assert.Equal(t, vmKind.ChoiceOf(v), kept.Spec.VM, "what it keeps of its vm is what its vm was made with")
		assert.Empty(t, kept.Spec.External, "nothing was there before it")
		assert.Equal(t, nodeName, kept.Metadata.Node, "and it is where its vm is")

		network := w.blockRecord(t, networkKind.Name, vmUUID, "backend")
		assert.Equal(t, []kind.Reference{{Kind: containerKind.Name, UUID: root.UUID}}, network.Metadata.Owners, "the network is its own")

		volume := w.blockRecord(t, volumeKind.Name, vmUUID, "data")
		assert.Empty(t, volume.Metadata.Owners, "and the volume is used, not owned")
		assert.Contains(t, kept.Metadata.Refs, kind.Reference{Kind: volumeKind.Name, UUID: volume.Metadata.UUID})

		held := w.dockerd.Holds(t, vmUUID)

		c, there := held.Container("web")
		require.True(t, there)
		assert.Equal(t, container.StateRunning, c.State)
		assert.Contains(t, c.NetworkSettings.Networks, "backend")

		n, there := held.Network("backend")
		require.True(t, there)
		assert.Equal(t, "true", n.Labels[blockKinds.LabelManaged], "made by the platform")
		assert.True(t, held.Volume("data"))
	})

	t.Run("its own volume, deleted once nothing mounts it, is made again, and then it", func(t *testing.T) {
		before := w.blockRecord(t, volumeKind.Name, vmUUID, "data")

		// docker removes no volume a container mounts, stopped or not: the
		// container goes behind the platform's back first.
		w.dockerd.Remove(t, vmUUID, "web")

		deleted, err := deleteVolume.NewUseCase(w.client, w.translator).Execute(ctx, &deleteVolume.Request{VMUUID: vmUUID, Name: "data", OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, deleted.ValidationErrors)

		require.Eventually(t, func() bool {
			again, there := w.findBlock(t, volumeKind.Name, vmUUID, "data")
			if !there || again.Metadata.UUID == before.Metadata.UUID {
				return false
			}

			c, running := w.dockerd.Holds(t, vmUUID).Container("web")

			return w.dockerd.Holds(t, vmUUID).Volume("data") && running && c.State == container.StateRunning
		}, settle, beat, "the volume, and then the container, were never made again")
	})

	t.Run("its vm, deleted, is made again under its uuid, and then what lives in it, and it", func(t *testing.T) {
		gone := time.Now()

		deleted, err := deleteVM.NewUseCase(w.client, w.translator).Execute(ctx, &deleteVM.Request{UUID: vmUUID, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, deleted.ValidationErrors)

		// started since it was deleted: a record keeps when it was made to
		// the millisecond, and one made again may be within the one it was
		// deleted in.
		again := w.stored(t, vmUUID, "made again", func(v vmKind.VM) bool {
			return v.Status.StartedAt.After(gone) && v.Status.State == vmKind.Running
		})

		assert.Equal(t, "docker", again.Metadata.Name, "as it was")
		assert.Equal(t, web.Spec.VM.Resources.CPUs, again.Spec.Resources.CPUs)

		require.Eventually(t, func() bool {
			held := w.dockerd.Holds(t, vmUUID)
			c, there := held.Container("web")
			_, networked := held.Network("backend")

			return there && c.State == container.StateRunning && networked && held.Volume("data")
		}, settle, beat, "what lives in it, and it, were never made again")

		kept := w.containerRecord(t, vmUUID, "web")
		assert.Equal(t, root.UUID, kept.Metadata.UUID, "it is what it was")
		assert.Equal(t, containerKind.Running, kept.Status.Status.State)
	})

	t.Run("deleted, it takes the network it owned with it, and leaves its volume and its vm", func(t *testing.T) {
		deleted, err := deleteContainer.NewUseCase(w.client, w.translator).Execute(ctx, &deleteContainer.Request{VMUUID: vmUUID, ID: "web", Force: true, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, deleted.ValidationErrors)

		require.Eventually(t, func() bool {
			_, kept := w.findBlock(t, networkKind.Name, vmUUID, "backend")

			return !kept && len(w.containerRecords(t, vmUUID)) == 0
		}, settle, beat, "it, and then its network, were never removed")

		held := w.dockerd.Holds(t, vmUUID)

		_, there := held.Container("web")
		assert.False(t, there)

		_, there = held.Network("backend")
		assert.False(t, there)

		assert.True(t, held.Volume("data"), "its volume stays")

		_, kept := w.findBlock(t, volumeKind.Name, vmUUID, "data")
		assert.True(t, kept)

		w.stored(t, vmUUID, "kept", func(v vmKind.VM) bool { return v.Status.State == vmKind.Running })
	})
}

// TestAContainerWaitingOnWhatIsNotItsOwn walks a container that mounts a
// volume that was there before it, made by its owner: the volume is not its
// own, and deleted, its graph does not make it again, and waits on it, and
// the container with it.
func TestAContainerWaitingOnWhatIsNotItsOwn(t *testing.T) {
	t.Parallel()

	w := start(t)
	ctx := t.Context()

	vmUUID := w.dockerVM(t, "builds")

	made, err := createVolume.NewUseCase(w.client, w.validator, w.translator).Execute(ctx, &createVolume.Request{VMUUID: vmUUID, Name: "shared", OwnerUUID: ownerUUID})
	require.NoError(t, err)
	require.Empty(t, made.ValidationErrors)

	created, err := createContainer.NewUseCase(w.client, w.validator, w.translator).Execute(ctx, &createContainer.Request{
		VMUUID:        vmUUID,
		Name:          "app",
		Image:         "nginx:1.27",
		RestartPolicy: containerKind.RestartAlways,
		Mounts:        []createContainer.Mount{{Type: containerKind.MountVolume, Source: "shared", Target: "/shared"}},
		OwnerUUID:     ownerUUID,
	})
	require.NoError(t, err)
	require.Empty(t, created.ValidationErrors)

	w.containerIn(t, vmUUID, "app", "running")

	app := w.containerRecord(t, vmUUID, "app")
	root := graph.Ref{Kind: containerKind.Name, UUID: app.Metadata.UUID}

	assert.Equal(t, []string{"volume/shared"}, app.Spec.External, "what was there before it is not its own")
	assert.Equal(t, stackKind.VMChoice{
		UUID:      vmUUID,
		Name:      "builds",
		Resources: &stackKind.Resources{CPUs: 2, Memory: 2 << 30, Disk: 20 << 30},
		Ports:     []port.Port{},
		Network:   &stackKind.Network{Ingress: "allow", Egress: "allow"},
	}, app.Spec.VM, "and it keeps the vm it named as its record has it")

	// docker removes no volume a container mounts, stopped or not: the
	// container goes behind the platform's back first.
	w.dockerd.Remove(t, vmUUID, "app")

	deleted, err := deleteVolume.NewUseCase(w.client, w.translator).Execute(ctx, &deleteVolume.Request{VMUUID: vmUUID, Name: "shared", OwnerUUID: ownerUUID})
	require.NoError(t, err)
	require.Empty(t, deleted.ValidationErrors)

	w.plan(t, root, "waiting on the volume", func(executions []graph.Execution) bool {
		return slices.ContainsFunc(executions, func(e graph.Execution) bool {
			return e.Node == graph.ID(volumeKind.Name, "shared") && e.State == graph.StateWaiting && e.Reason == "it is external, and missing"
		})
	})

	w.containerKept(t, vmUUID, "app", "missing, its graph waiting", func(c containerKind.Container) bool {
		return c.Status.Status.State == containerKind.Missing
	})

	// a while of heartbeats, reconcile passes and resyncs, none of which
	// makes the volume again, or the container without it.
	time.Sleep(20 * beat)

	held := w.dockerd.Holds(t, vmUUID)
	assert.False(t, held.Volume("shared"), "the volume was not made again")

	_, there := held.Container("app")
	assert.False(t, there, "and the container was not made without it")

	_, kept := w.findBlock(t, volumeKind.Name, vmUUID, "shared")
	assert.False(t, kept)

	waiting := w.containerRecord(t, vmUUID, "app")
	assert.Equal(t, containerKind.Missing, waiting.Status.Status.State)
}

// plan is how each node of root's plan went, the last time it was carried
// out, once condition holds of it.
func (w *workload) plan(t *testing.T, root graph.Ref, what string, condition func([]graph.Execution) bool) []graph.Execution {
	t.Helper()

	return eventually(t, "the plan of "+root.UUID+" "+what, func(ctx context.Context) ([]graph.Execution, error) {
		return w.executions.GetByRoot(ctx, root.UUID)
	}, condition)
}

// containerKept is the record of a container kept in a Docker VM under name,
// once condition holds of it.
func (w *workload) containerKept(t *testing.T, vmUUID string, name string, what string, condition func(containerKind.Container) bool) containerKind.Container {
	t.Helper()

	return eventually(t, "the container "+name+" "+what, func(context.Context) (containerKind.Container, error) {
		for _, c := range w.containerRecords(t, vmUUID) {
			if c.Spec.Name == name {
				return c, nil
			}
		}

		return containerKind.Container{}, nil
	}, func(c containerKind.Container) bool { return c.Spec.Name == name && condition(c) })
}

// findBlock is the record of a network or a volume kept in a Docker VM under
// name, and whether there is one.
func (w *workload) findBlock(t *testing.T, kindName string, vmUUID string, name string) (resource.Record, bool) {
	t.Helper()

	records, _, err := w.resources.GetAll(t.Context(), kindName, resource.Filter{Parent: kind.Reference{Kind: vmKind.Name, UUID: vmUUID}}, 0, 0)
	require.NoError(t, err)

	for _, r := range records {
		if r.Metadata.Name == name {
			return r, true
		}
	}

	return resource.Record{}, false
}

// blockRecord is the record of a network or a volume kept in a Docker VM
// under name.
func (w *workload) blockRecord(t *testing.T, kindName string, vmUUID string, name string) resource.Record {
	t.Helper()

	r, kept := w.findBlock(t, kindName, vmUUID, name)
	require.True(t, kept, "no %s named %s is kept in %s", kindName, name, vmUUID)

	return r
}
