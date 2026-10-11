package workload_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/createContainer"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/deleteContainer"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/createVM"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/deleteVM"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/volume/createVolume"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/volume/deleteVolume"
	"github.com/khanzadimahdi/testproject/domain"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
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
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/input"
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
		made := vmKind.ChoiceOf(v)
		made.Slug = ""
		assert.Equal(t, made, kept.Spec.VM, "what it keeps of its vm is what its vm was made with, and its slug once its vm goes")
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

		// its record says it runs once its node has said so, which may be a
		// beat after its vm's dockerd runs it.
		kept := w.containerKept(t, vmUUID, "web", "running again", func(c containerKind.Container) bool {
			return c.Status.Status.State == containerKind.Running
		})
		assert.Equal(t, root.UUID, kept.Metadata.UUID, "it is what it was")
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
	named := w.stored(t, vmUUID, "running", func(v vmKind.VM) bool { return v.Status.State == vmKind.Running })

	assert.Equal(t, stackKind.VMChoice{
		UUID:      vmUUID,
		Name:      "builds",
		Resources: &stackKind.Resources{CPUs: 2, Memory: 2 << 30, Disk: 20 << 30},
		Ports:     []port.Port{},
		Network:   &stackKind.Network{Ingress: "allow", Egress: "allow"},
		Slug:      named.Metadata.Slug,
	}, app.Spec.VM, "and it keeps the vm it named as its record has it, its slug among it")

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

// TestAVMDeletedWithWhatNeedsIt deletes, from the dashboard, the Docker VM a
// container's graph made for it, with what needs it (cascade): the answer
// names the container, which would have made it again; the container goes,
// with the network it owned, and the volume goes with the VM, as what lives in
// a VM always does; and nothing is made again.
func TestAVMDeletedWithWhatNeedsIt(t *testing.T) {
	t.Parallel()

	w := start(t)

	vmUUID, root := w.containerWithItsOwn(t, "web")

	deleted, err := deleteVM.NewUseCase(w.client, w.translator).Execute(t.Context(), &deleteVM.Request{UUID: vmUUID, OwnerUUID: ownerUUID, Cascade: true})
	require.NoError(t, err)
	require.Empty(t, deleted.ValidationErrors)

	assert.Equal(t, []presenter.Dependent{{Kind: containerKind.Name, UUID: root.UUID, Name: "web", Recreates: true}}, deleted.Dependents)
	assert.False(t, deleted.Recreated, "it goes with them, and none makes it again")

	w.gone(t, vmUUID)

	w.nothingIn(t, vmUUID)
}

// TestAVMWhoseLifetimeEndsUnderAContainer lets the lifetime of the Docker VM
// a container's graph made for it end: the container is deleted with it, as
// a delete with what needs it deletes it, and nothing is made again.
func TestAVMWhoseLifetimeEndsUnderAContainer(t *testing.T) {
	t.Parallel()

	w := start(t)
	ctx := t.Context()

	vmUUID, _ := w.containerWithItsOwn(t, "web")

	lifetime := 2 * time.Second

	_, err := w.client.UpdateVM(ctx, ownerUUID, vmUUID, workloadControlPlane.VMUpdate{Lifetime: &lifetime})
	require.NoError(t, err)

	w.gone(t, vmUUID)

	w.nothingIn(t, vmUUID)
}

// TestAVolumeAContainerMounts deletes a volume a container's graph made for
// it, in a Docker VM it named, as the control plane's API is asked and as the
// dashboard asks: alone, its node refuses, since the container mounts it, and
// the answer names the container beside the refusal; with what needs it, the
// container goes first, and then the volume, and the VM stays.
func TestAVolumeAContainerMounts(t *testing.T) {
	t.Parallel()

	w := start(t)
	ctx := t.Context()

	vmUUID := w.dockerVM(t, "builds")

	created, err := createContainer.NewUseCase(w.client, w.validator, w.translator).Execute(ctx, &createContainer.Request{
		VMUUID:        vmUUID,
		Name:          "app",
		Image:         "nginx:1.27",
		RestartPolicy: containerKind.RestartAlways,
		Mounts:        []createContainer.Mount{{Type: containerKind.MountVolume, Source: "data", Target: "/data"}},
		OwnerUUID:     ownerUUID,
	})
	require.NoError(t, err)
	require.Empty(t, created.ValidationErrors)

	w.containerIn(t, vmUUID, "app", "running")

	app := w.containerRecord(t, vmUUID, "app")
	require.Empty(t, app.Spec.External, "the volume is its own, made by its graph")

	volume := w.blockRecord(t, volumeKind.Name, vmUUID, "data")
	needs := []dependentShown{{Kind: containerKind.Name, UUID: app.Metadata.UUID, Name: "app", Recreates: true}}

	t.Run("alone, it is refused, since the container mounts it, and the answer names the container", func(t *testing.T) {
		status, body := w.controlPlane(t, http.MethodDelete, "/api/volumes/"+volume.Metadata.UUID, url.Values{"wait": {"10s"}})
		require.Equal(t, http.StatusAccepted, status, string(body))

		answer := deletedOf(t, body)
		require.NotNil(t, answer.Result, "its node answered")
		assert.False(t, answer.Result.OK)
		assert.Contains(t, answer.Result.Reason, "volume is in use")
		assert.Equal(t, needs, answer.Dependents)
		assert.True(t, answer.Recreated)

		kept := w.blockRecord(t, volumeKind.Name, vmUUID, "data")
		assert.Equal(t, volume.Metadata.UUID, kept.Metadata.UUID, "it stays as it was")

		common, err := kept.Common()
		require.NoError(t, err)
		assert.Equal(t, blockKinds.Present, common.State)
		assert.Equal(t, blockKinds.Present, common.Expected)

		assert.True(t, w.dockerd.Holds(t, vmUUID).Volume("data"))
		w.containerIn(t, vmUUID, "app", "running")
	})

	t.Run("and so it is from the dashboard, which says the refusal and the container beside it", func(t *testing.T) {
		refused, err := deleteVolume.NewUseCase(w.client, w.translator).Execute(ctx, &deleteVolume.Request{VMUUID: vmUUID, Name: "data", OwnerUUID: ownerUUID})
		require.NoError(t, err)

		assert.Contains(t, refused.ValidationErrors["docker"], "volume is in use")
		assert.Equal(t, []presenter.Dependent{{Kind: containerKind.Name, UUID: app.Metadata.UUID, Name: "app", Recreates: true}}, refused.Dependents)
		assert.True(t, refused.Recreated)

		_, kept := w.findBlock(t, volumeKind.Name, vmUUID, "data")
		assert.True(t, kept)
	})

	t.Run("with what needs it, the container goes first, and then the volume, and the vm stays", func(t *testing.T) {
		deleted, err := deleteVolume.NewUseCase(w.client, w.translator).Execute(ctx, &deleteVolume.Request{VMUUID: vmUUID, Name: "data", OwnerUUID: ownerUUID, Cascade: true})
		require.NoError(t, err)
		require.Empty(t, deleted.ValidationErrors)

		assert.Equal(t, []presenter.Dependent{{Kind: containerKind.Name, UUID: app.Metadata.UUID, Name: "app", Recreates: true}}, deleted.Dependents)
		assert.False(t, deleted.Recreated)

		assert.Empty(t, w.containerRecords(t, vmUUID))

		_, kept := w.findBlock(t, volumeKind.Name, vmUUID, "data")
		assert.False(t, kept)

		held := w.dockerd.Holds(t, vmUUID)

		_, there := held.Container("app")
		assert.False(t, there)
		assert.False(t, held.Volume("data"))

		w.stored(t, vmUUID, "kept", func(v vmKind.VM) bool { return v.Status.State == vmKind.Running })
	})
}

// containerWithItsOwn is a container named name, which names no Docker VM,
// joins a network and mounts a volume it names, once its graph made the VM,
// the network and the volume for it, and then it; and its VM, and it.
func (w *workload) containerWithItsOwn(t *testing.T, name string) (string, graph.Ref) {
	t.Helper()

	created, err := createContainer.NewUseCase(w.client, w.validator, w.translator).Execute(t.Context(), &createContainer.Request{
		Name:          name,
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
	made := w.containerRecord(t, vmUUID, name)
	root := graph.Ref{Kind: containerKind.Name, UUID: made.Metadata.UUID}

	w.plan(t, root, "made", func(executions []graph.Execution) bool {
		return len(executions) == 4 && !slices.ContainsFunc(executions, func(e graph.Execution) bool { return e.State != graph.StateDone })
	})

	return vmUUID, root
}

// nothingIn holds a Docker VM that is gone to staying gone, and what lived in
// it with it: no container, network or volume is kept in it, now or after a
// while of heartbeats, reconcile passes and resyncs, and its node holds none
// of it.
func (w *workload) nothingIn(t *testing.T, vmUUID string) {
	t.Helper()

	empty := func() bool {
		if len(w.containerRecords(t, vmUUID)) > 0 {
			return false
		}

		for _, kindName := range []string{networkKind.Name, volumeKind.Name} {
			records, _, err := w.resources.GetAll(t.Context(), kindName, resource.Filter{Parent: kind.Reference{Kind: vmKind.Name, UUID: vmUUID}}, 0, 0)
			if err != nil || len(records) > 0 {
				return false
			}
		}

		return true
	}

	require.Eventually(t, empty, settle, beat, "what lived in it was never taken away")

	time.Sleep(20 * beat)

	assert.True(t, empty(), "and nothing of it was made again")

	_, err := w.resources.GetOne(t.Context(), vmKind.Name, vmUUID)
	assert.ErrorIs(t, err, domain.ErrNotExists, "nor was it")

	_, err = w.engine.Inspect(t.Context(), vmUUID)
	assert.ErrorIs(t, err, domain.ErrNotExists, "and its node holds none of it")
}

// dependentShown is a root that needs what was asked to go, as the control
// plane's API says it.
type dependentShown struct {
	Kind      string `json:"kind"`
	UUID      string `json:"uuid"`
	Name      string `json:"name"`
	Recreates bool   `json:"recreates"`
}

// deletedAnswer is what the control plane's API answers the delete of what
// roots need with.
type deletedAnswer struct {
	Resource *kind.Raw `json:"resource"`

	Result *struct {
		OK     bool   `json:"ok"`
		Reason string `json:"reason"`
	} `json:"result"`

	Dependents []dependentShown `json:"dependents"`
	Recreated  bool             `json:"recreated"`
}

func deletedOf(t *testing.T, body []byte) deletedAnswer {
	t.Helper()

	var answer deletedAnswer
	require.NoError(t, json.Unmarshal(body, &answer), string(body))

	return answer
}

// TestDryRunsAndPlans asks the control plane what admitting a container, and
// deleting one or its VM, would do, which keeps, sends and changes nothing,
// and how a container's plan went.
func TestDryRunsAndPlans(t *testing.T) {
	t.Parallel()

	w := start(t)
	ctx := t.Context()

	vmUUID := w.dockerVM(t, "builds")

	t.Run("a container's admission, in a docker vm it names, is answered with its graph's levels, and nothing is kept", func(t *testing.T) {
		status, body := w.controlPlaneWith(t, http.MethodPost, "/api/containers", url.Values{"dry_run": {"true"}}, `{
			"metadata": {"name": "web"},
			"spec": {"name": "web", "image": "nginx:1.27", "vm": {"uuid": "`+vmUUID+`"}, "networks": ["backend"], "mounts": [{"type": "volume", "source": "data", "target": "/data"}]}
		}`)
		require.Equal(t, http.StatusOK, status, string(body))

		answer := admittedOf(t, body)
		assert.Equal(t, vmUUID, answer.Resource.Spec.VM.UUID)
		assert.Equal(t, containerKind.Pending, answer.Resource.Status.Status.State, "its graph's to make")

		assert.Equal(t, [][]previewedNode{
			{{Node: graph.ID(vmKind.Name, vmUUID), Kind: vmKind.Name, Verdict: "exists"}},
			{
				{Node: graph.ID(networkKind.Name, "backend"), Kind: networkKind.Name, Verdict: "create", Reason: "it is not there"},
				{Node: graph.ID(volumeKind.Name, "data"), Kind: volumeKind.Name, Verdict: "create", Reason: "it is not there"},
			},
			{{Node: graph.ID(containerKind.Name, "web"), Kind: containerKind.Name, Verdict: "create", Reason: "it is not there"}},
		}, answer.levels())

		assert.Empty(t, w.containerRecords(t, vmUUID), "nothing is kept")

		_, kept := w.findBlock(t, networkKind.Name, vmUUID, "backend")
		assert.False(t, kept)

		_, there := w.dockerd.Holds(t, vmUUID).Container("web")
		assert.False(t, there, "nor made")
	})

	t.Run("one naming no vm is answered with the vm its graph would make, kept nowhere", func(t *testing.T) {
		before, _, err := w.resources.GetAll(ctx, vmKind.Name, resource.Filter{OwnerUUID: ownerUUID}, 0, 0)
		require.NoError(t, err)

		status, body := w.controlPlaneWith(t, http.MethodPost, "/api/containers", url.Values{"dry_run": {"true"}}, `{
			"metadata": {"name": "api"},
			"spec": {"name": "api", "image": "nginx:1.27"}
		}`)
		require.Equal(t, http.StatusOK, status, string(body))

		answer := admittedOf(t, body)
		planned := answer.Resource.Spec.VM.UUID
		require.NotEmpty(t, planned)

		assert.Equal(t, [][]previewedNode{
			{{Node: graph.ID(vmKind.Name, planned), Kind: vmKind.Name, Verdict: "create", Reason: "it is not there"}},
			{{Node: graph.ID(containerKind.Name, "api"), Kind: containerKind.Name, Verdict: "create", Reason: "it is not there"}},
		}, answer.levels())

		after, _, err := w.resources.GetAll(ctx, vmKind.Name, resource.Filter{OwnerUUID: ownerUUID}, 0, 0)
		require.NoError(t, err)
		assert.Len(t, after, len(before), "no vm is kept")
	})

	// a container, a root, in the vm, joining a network of its own.
	created, err := createContainer.NewUseCase(w.client, w.validator, w.translator).Execute(ctx, &createContainer.Request{
		VMUUID:        vmUUID,
		Name:          "app",
		Image:         "nginx:1.27",
		RestartPolicy: containerKind.RestartAlways,
		Networks:      []string{"backend"},
		OwnerUUID:     ownerUUID,
	})
	require.NoError(t, err)
	require.Empty(t, created.ValidationErrors)

	app := w.containerRecord(t, vmUUID, "app")
	root := graph.Ref{Kind: containerKind.Name, UUID: app.Metadata.UUID}

	w.plan(t, root, "made", func(executions []graph.Execution) bool {
		return len(executions) == 3 && !slices.ContainsFunc(executions, func(e graph.Execution) bool { return e.State != graph.StateDone })
	})

	t.Run("a root's delete is answered with what removing it would do, and a vm's with the roots that need it, and nothing changes", func(t *testing.T) {
		status, body := w.controlPlane(t, http.MethodDelete, "/api/containers/"+root.UUID, url.Values{"dry_run": {"true"}})
		require.Equal(t, http.StatusOK, status, string(body))

		var removing struct {
			Plan *struct {
				Levels [][]previewedNode `json:"levels"`
			} `json:"plan"`
		}
		require.NoError(t, json.Unmarshal(body, &removing))
		require.NotNil(t, removing.Plan)

		network := w.blockRecord(t, networkKind.Name, vmUUID, "backend")

		assert.Equal(t, [][]previewedNode{
			{{Node: graph.ID(containerKind.Name, "app"), Kind: containerKind.Name, Verdict: "delete", Ref: &kind.Reference{Kind: containerKind.Name, UUID: root.UUID}}},
			{{Node: graph.ID(networkKind.Name, "backend"), Kind: networkKind.Name, Verdict: "delete", Ref: &kind.Reference{Kind: networkKind.Name, UUID: network.Metadata.UUID}}},
			{{Node: graph.ID(vmKind.Name, vmUUID), Kind: vmKind.Name, Verdict: "keep", Reason: "the root does not own it"}},
		}, removing.Plan.Levels, "it, then the network it owns, and its vm never touched")

		status, body = w.controlPlane(t, http.MethodDelete, "/api/vms/"+vmUUID, url.Values{"dry_run": {"true"}})
		require.Equal(t, http.StatusOK, status, string(body))
		assert.JSONEq(t, `{"dependents": [{"kind": "container", "uuid": "`+root.UUID+`", "name": "app", "recreates": true}], "recreated": true}`, string(body))

		status, body = w.controlPlane(t, http.MethodDelete, "/api/vms/"+vmUUID, url.Values{"dry_run": {"true"}, "cascade": {"true"}})
		require.Equal(t, http.StatusOK, status, string(body))

		var cascading struct {
			Dependents []struct {
				dependentShown

				Plan *struct {
					Levels [][]previewedNode `json:"levels"`
				} `json:"plan"`
			} `json:"dependents"`
			Recreated bool `json:"recreated"`
		}
		require.NoError(t, json.Unmarshal(body, &cascading))
		require.Len(t, cascading.Dependents, 1)
		require.NotNil(t, cascading.Dependents[0].Plan, "what removing the container would do")
		assert.Len(t, cascading.Dependents[0].Plan.Levels, 3)
		assert.False(t, cascading.Recreated)

		kept := w.containerRecord(t, vmUUID, "app")
		assert.Equal(t, containerKind.Running, kept.Status.Expected, "nothing changed")
		assert.True(t, kept.Metadata.DeletionRequestedAt.IsZero())

		w.stored(t, vmUUID, "running", func(v vmKind.VM) bool { return v.Status.State == vmKind.Running && v.Status.Expected == vmKind.Running })
		w.containerIn(t, vmUUID, "app", "running")
	})

	t.Run("a root's plan says how each of its nodes went, as the run that made it left them, which no run since that found them as they were changes; a vm has none", func(t *testing.T) {
		// what its plan says of its nodes, as a wait's goroutine reads it.
		said := func() (string, bool) {
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, w.api+"/api/containers/"+root.UUID+"/plan?"+url.Values{"owner": {ownerUUID}}.Encode(), nil)
			if err != nil {
				return "", false
			}

			response, err := http.DefaultClient.Do(request)
			if err != nil {
				return "", false
			}
			defer response.Body.Close()

			var plan struct {
				Nodes json.RawMessage `json:"nodes"`
			}

			if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&plan) != nil {
				return "", false
			}

			return string(plan.Nodes), true
		}

		made, read := said()
		require.True(t, read)

		// every resync runs its plan again, and finds each node there.
		assert.Never(t, func() bool {
			now, read := said()

			return read && now != made
		}, 10*4*beat, beat, "a run that found every node as it was changed what its plan says of them")

		status, body := w.controlPlane(t, http.MethodGet, "/api/containers/"+root.UUID+"/plan", nil)
		require.Equal(t, http.StatusOK, status, string(body))

		var plan struct {
			Root       kind.Reference `json:"root"`
			Generation int64          `json:"generation"`
			Nodes      []struct {
				Node       graph.NodeID    `json:"node"`
				Level      int             `json:"level"`
				Kind       string          `json:"kind"`
				Action     string          `json:"action"`
				State      string          `json:"state"`
				Ref        *kind.Reference `json:"ref"`
				StartedAt  time.Time       `json:"started_at"`
				FinishedAt time.Time       `json:"finished_at"`
			} `json:"nodes"`
			Lease *struct {
				Failures int `json:"failures"`
			} `json:"lease"`
		}
		require.NoError(t, json.Unmarshal(body, &plan))

		assert.Equal(t, root, plan.Root)
		assert.Equal(t, int64(1), plan.Generation)

		levels := map[graph.NodeID]int{}
		for _, node := range plan.Nodes {
			levels[node.Node] = node.Level

			assert.Equal(t, "create", node.Action, "%s", node.Node)
			assert.Equal(t, "done", node.State, "%s", node.Node)
			assert.NotNil(t, node.Ref, "%s", node.Node)
			assert.False(t, node.FinishedAt.Before(node.StartedAt), "%s", node.Node)
		}

		assert.Equal(t, map[graph.NodeID]int{
			graph.ID(vmKind.Name, vmUUID):         0,
			graph.ID(networkKind.Name, "backend"): 1,
			graph.ID(containerKind.Name, "app"):   2,
		}, levels)

		require.NotNil(t, plan.Lease, "its lease says how it went lately")
		assert.Zero(t, plan.Lease.Failures)

		status, _ = w.controlPlane(t, http.MethodGet, "/api/vms/"+vmUUID+"/plan", nil)
		assert.Equal(t, http.StatusNotFound, status, "a vm is no root")
	})
}

// previewedNode is a node of a plan, as a dry run says it.
type previewedNode struct {
	Node     graph.NodeID    `json:"node"`
	Kind     string          `json:"kind"`
	Verdict  string          `json:"verdict"`
	Reason   string          `json:"reason,omitempty"`
	External bool            `json:"external,omitempty"`
	Ref      *kind.Reference `json:"ref,omitempty"`
}

// admittedAnswer is what a dry run of a container's admission is answered
// with.
type admittedAnswer struct {
	Resource containerKind.Container `json:"resource"`

	Plan *struct {
		Levels [][]previewedNode `json:"levels"`
	} `json:"plan"`
}

// levels are what the plan says of each node, but its record.
func (a admittedAnswer) levels() [][]previewedNode {
	if a.Plan == nil {
		return nil
	}

	levels := make([][]previewedNode, len(a.Plan.Levels))
	for i, level := range a.Plan.Levels {
		for _, node := range level {
			node.Ref = nil
			levels[i] = append(levels[i], node)
		}
	}

	return levels
}

func admittedOf(t *testing.T, body []byte) admittedAnswer {
	t.Helper()

	var answer admittedAnswer
	require.NoError(t, json.Unmarshal(body, &answer), string(body))

	return answer
}

// TestAVMDeletedUnderAContainer deletes, alone, the Docker VM a container's
// graph made for it, once an update renamed and resized it: the answer names
// the container, which makes it again; the VM comes back under its uuid and
// the slug it had, as the update left it, and then the container in it.
func TestAVMDeletedUnderAContainer(t *testing.T) {
	t.Parallel()

	w := start(t)
	ctx := t.Context()

	vmUUID, root := w.containerWithItsOwn(t, "web")

	name := "renamed"
	resized := vm.Resources{CPUs: 3, Memory: 3 << 30, Disk: 20 << 30}

	_, err := w.client.UpdateVM(ctx, ownerUUID, vmUUID, workloadControlPlane.VMUpdate{Name: &name, Resources: &resized})
	require.NoError(t, err)

	updated := w.stored(t, vmUUID, "reconfigured as it was updated", func(v vmKind.VM) bool {
		return v.Status.State == vmKind.Running && v.Status.Applied != nil && v.Status.Applied.Equal(v.Spec.Config()) && v.Metadata.Name == name
	})

	gone := time.Now()

	status, body := w.controlPlane(t, http.MethodDelete, "/api/vms/"+vmUUID, nil)
	require.Equal(t, http.StatusAccepted, status, string(body))

	answer := deletedOf(t, body)
	assert.Equal(t, []dependentShown{{Kind: containerKind.Name, UUID: root.UUID, Name: "web", Recreates: true}}, answer.Dependents)
	assert.True(t, answer.Recreated, "the container makes it again")

	again := w.stored(t, vmUUID, "made again", func(v vmKind.VM) bool {
		return v.Status.StartedAt.After(gone) && v.Status.State == vmKind.Running
	})

	assert.Equal(t, updated.Metadata.Slug, again.Metadata.Slug, "under the slug it had, so that it is reached where it was")
	assert.Equal(t, name, again.Metadata.Name, "as the update left it")
	assert.Equal(t, vmKind.ResourcesOf(resized), again.Spec.Resources)

	require.Eventually(t, func() bool {
		c, there := w.dockerd.Holds(t, vmUUID).Container("web")

		return there && c.State == container.StateRunning
	}, settle, beat, "the container was never made again in it")

	kept := w.containerRecord(t, vmUUID, "web")
	assert.Equal(t, root.UUID, kept.Metadata.UUID, "it is what it was")
	assert.Equal(t, vmKind.ChoiceOf(updated), kept.Spec.VM, "and keeps its vm as it went")
}

// TestAContainerInABootingVM asks for a container in a Docker VM that is still
// coming up, with no resync to fall back on: its graph waits on the VM, and
// once the VM comes up, which its node says, it is made, at once, rather than
// once its node's silence about it says it went missing, which would also
// have its graph carried out again, a while later.
func TestAContainerInABootingVM(t *testing.T) {
	t.Parallel()

	w := start(t, withoutResync())
	ctx := t.Context()

	release := w.holdBoots()
	defer release()

	booting, err := createVM.NewUseCase(w.client, w.validator, w.translator, w.owners, ingressDomain).Execute(ctx, &createVM.Request{
		Name:      "builds",
		Kind:      string(vm.KindDocker),
		Resources: input.Resources{CPUs: 2, Memory: 2 << 30, Disk: 20 << 30},
		Network:   input.Network{Ingress: "allow", Egress: "allow"},
		OwnerUUID: ownerUUID,
	})
	require.NoError(t, err)
	require.Empty(t, booting.ValidationErrors)

	vmUUID := booting.VM.UUID

	w.stored(t, vmUUID, "coming up", func(v vmKind.VM) bool { return v.Status.State == vmKind.Scheduled })

	// the vm comes up once the container's graph waits on it, then.
	released := make(chan time.Time, 1)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(beat):
			}

			records, _, err := w.resources.GetAll(context.Background(), containerKind.Name, resource.Filter{Parent: kind.Reference{Kind: vmKind.Name, UUID: vmUUID}}, 0, 0)
			if err != nil || len(records) == 0 {
				continue
			}

			executions, err := w.executions.GetByRoot(context.Background(), records[0].Metadata.UUID)
			if err == nil && slices.ContainsFunc(executions, func(e graph.Execution) bool { return e.State == graph.StateWaiting }) {
				released <- time.Now()
				release()

				return
			}
		}
	}()

	asking, cancel := context.WithTimeout(ctx, settle)
	defer cancel()

	created, err := createContainer.NewUseCase(w.client, w.validator, w.translator).Execute(asking, &createContainer.Request{
		VMUUID:        vmUUID,
		Name:          "web",
		Image:         "nginx:1.27",
		RestartPolicy: containerKind.RestartAlways,
		OwnerUUID:     ownerUUID,
	})
	require.NoError(t, err, "it was never made once its vm came up")
	require.Empty(t, created.ValidationErrors)
	assert.Equal(t, "running", created.Container.State)

	select {
	case at := <-released:
		assert.Less(t, time.Since(at), patience.ResourceSilentAfter, "made as soon as its vm came up, and not once its node's silence said it went")
	default:
		t.Fatal("its graph never waited on its vm")
	}

	web := w.containerRecord(t, vmUUID, "web")

	waited := false
	for _, e := range w.plan(t, graph.Ref{Kind: containerKind.Name, UUID: web.Metadata.UUID}, "made", func(executions []graph.Execution) bool {
		return !slices.ContainsFunc(executions, func(e graph.Execution) bool { return e.State != graph.StateDone })
	}) {
		waited = waited || e.Reason != ""
	}

	assert.False(t, waited, "its last plan made it, waiting on nothing")
}
