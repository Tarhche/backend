package workload_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/connectNetwork"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/createContainer"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/deleteContainer"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/disconnectNetwork"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/getContainerLogs"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/getVMContainers"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/startContainer"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/stopContainer"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/image/deleteImage"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/image/getImages"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/image/pullImage"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/network/createNetwork"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/network/deleteNetwork"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/network/getNetworks"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/createSnapshot"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/getSnapshot"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/createVM"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/restoreVM"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/startVM"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/stopVM"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/volume/createVolume"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/volume/deleteVolume"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/volume/getVolumes"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	blockKinds "github.com/khanzadimahdi/testproject/domain/workload/kinds/blocks"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/input"
)

// TestAContainer walks a container through its life from the dashboard, as
// a kind the control plane keeps and a node carries out: asked for naming no
// Docker VM, made in one made for it, made again when it is removed behind
// the platform's back, stopped and left stopped, read, started and deleted.
func TestAContainer(t *testing.T) {
	t.Parallel()

	w := start(t)
	ctx := t.Context()

	created, err := createContainer.NewUseCase(w.client, w.validator, w.translator).Execute(ctx, &createContainer.Request{
		Name:          "web",
		Image:         "nginx:1.27",
		RestartPolicy: containerKind.RestartAlways,
		Ports:         []createContainer.PortBinding{{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"}},
		OwnerUUID:     ownerUUID,
	})
	require.NoError(t, err)
	require.Empty(t, created.ValidationErrors)
	require.NotNil(t, created.Container)

	vmUUID := created.VM.UUID

	t.Run("it named no docker vm, so one was made for it, and it was made in it once it came up", func(t *testing.T) {
		assert.True(t, created.VM.Created)
		assert.Equal(t, "web", created.Container.Name)
		assert.False(t, created.Container.Unmanaged)

		running := w.containerIn(t, vmUUID, "web", "running")
		assert.Equal(t, "nginx:1.27", running.Image)
		assert.Equal(t, []presenter.PortBinding{{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"}}, running.Ports)

		c, there := w.dockerd.Holds(t, vmUUID).Container("web")
		require.True(t, there, "its vm's dockerd has it")
		assert.Equal(t, container.StateRunning, c.State)
		assert.Equal(t, "true", c.Labels[blockKinds.LabelManaged], "labelled as the platform's")
		assert.Equal(t, w.containerRecord(t, vmUUID, "web").Metadata.UUID, c.Labels[blockKinds.Label(containerKind.Name)], "as the resource it is")
	})

	t.Run("removed behind the platform's back, it is made again", func(t *testing.T) {
		before, _ := w.dockerd.Holds(t, vmUUID).Container("web")

		w.dockerd.Remove(t, vmUUID, "web")

		again := w.container(t, vmUUID, "web", "made again", func(c presenter.Container) bool {
			return c.State == "running" && c.ID != before.ID
		})

		c, there := w.dockerd.Holds(t, vmUUID).Container("web")
		require.True(t, there)
		assert.Equal(t, again.ID, c.ID)
	})

	t.Run("stopped, it stays stopped", func(t *testing.T) {
		stopped, err := stopContainer.NewUseCase(w.client, w.translator).Execute(ctx, &stopContainer.Request{VMUUID: vmUUID, ID: "web", OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, stopped.ValidationErrors)

		w.containerIn(t, vmUUID, "web", "exited")

		// a while of heartbeats and reconcile passes, none of which starts it.
		time.Sleep(20 * beat)

		c, there := w.dockerd.Holds(t, vmUUID).Container("web")
		require.True(t, there)
		assert.Equal(t, container.StateExited, c.State)

		record := w.containerRecord(t, vmUUID, "web")
		assert.Equal(t, containerKind.Stopped, record.Status.Expected)
	})

	t.Run("its log is read from its dockerd", func(t *testing.T) {
		logs, err := getContainerLogs.NewUseCase(w.client, w.translator).Execute(ctx, &getContainerLogs.Request{VMUUID: vmUUID, ID: "web", OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, logs.ValidationErrors)

		require.Len(t, logs.Items, 1)
		assert.Equal(t, "web is ready", logs.Items[0].Line)
	})

	t.Run("started again", func(t *testing.T) {
		started, err := startContainer.NewUseCase(w.client, w.translator).Execute(ctx, &startContainer.Request{VMUUID: vmUUID, ID: "web", OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, started.ValidationErrors)

		w.containerIn(t, vmUUID, "web", "running")
	})

	t.Run("one that runs is not deleted but by force, and deleted, it is gone", func(t *testing.T) {
		refused, err := deleteContainer.NewUseCase(w.client, w.translator).Execute(ctx, &deleteContainer.Request{VMUUID: vmUUID, ID: "web", OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.NotEmpty(t, refused.ValidationErrors, "as docker refuses it")

		deleted, err := deleteContainer.NewUseCase(w.client, w.translator).Execute(ctx, &deleteContainer.Request{VMUUID: vmUUID, ID: "web", Force: true, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, deleted.ValidationErrors)

		require.Eventually(t, func() bool {
			_, there := w.dockerd.Holds(t, vmUUID).Container("web")

			return !there && len(w.containerRecords(t, vmUUID)) == 0
		}, settle, beat, "it was never removed")
	})
}

// TestTheBuildingBlocksOfADockerVM walks a Docker VM's networks, volumes and
// images from the dashboard, beside a container put on a network and taken
// off it, and what its dockerd holds that nobody keeps a record of, which is
// listed and never brought back.
func TestTheBuildingBlocksOfADockerVM(t *testing.T) {
	t.Parallel()

	w := start(t)
	ctx := t.Context()

	vmUUID := w.dockerVM(t, "builds")

	t.Run("what its dockerd holds that nobody keeps is listed as unmanaged, and never brought back", func(t *testing.T) {
		db := w.containerIn(t, vmUUID, "db", "running")
		assert.True(t, db.Unmanaged)

		w.dockerd.Exit(t, vmUUID, "db", 1)

		w.containerIn(t, vmUUID, "db", "exited")

		time.Sleep(20 * beat)

		c, _ := w.dockerd.Holds(t, vmUUID).Container("db")
		assert.Equal(t, container.StateExited, c.State, "nothing started it again")
		assert.Empty(t, w.containerRecords(t, vmUUID), "and nothing keeps a record of it")

		// it is still its owner's to start, through its node.
		started, err := startContainer.NewUseCase(w.client, w.translator).Execute(ctx, &startContainer.Request{VMUUID: vmUUID, ID: "db", OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, started.ValidationErrors)

		c, _ = w.dockerd.Holds(t, vmUUID).Container("db")
		assert.Equal(t, container.StateRunning, c.State)
	})

	var networkID string

	t.Run("a network is made in it", func(t *testing.T) {
		made, err := createNetwork.NewUseCase(w.client, w.validator, w.translator).Execute(ctx, &createNetwork.Request{VMUUID: vmUUID, Name: "backend", Internal: true, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, made.ValidationErrors)
		require.NotNil(t, made.DockerNetwork)

		networkID = made.ID

		n, there := w.dockerd.Holds(t, vmUUID).Network("backend")
		require.True(t, there)
		assert.Equal(t, n.ID, networkID)
		assert.True(t, n.Internal)
		assert.Equal(t, "true", n.Labels[blockKinds.LabelManaged])

		listed := eventually(t, "the network listed", func(ctx context.Context) ([]presenter.DockerNetwork, error) {
			listed, err := getNetworks.NewUseCase(w.client, w.translator).Execute(ctx, &getNetworks.Request{VMUUID: vmUUID, OwnerUUID: ownerUUID})
			if err != nil {
				return nil, err
			}

			return listed.Items, nil
		}, func(items []presenter.DockerNetwork) bool {
			return slices.ContainsFunc(items, func(n presenter.DockerNetwork) bool { return n.Name == "backend" })
		})

		for _, n := range listed {
			assert.Equal(t, n.Name != "backend", n.Unmanaged, "%s: what the platform made is the platform's, and what every dockerd has nobody's", n.Name)
		}
	})

	t.Run("a container is put on it, and the network is not removed while it is", func(t *testing.T) {
		made, err := createContainer.NewUseCase(w.client, w.validator, w.translator).Execute(ctx, &createContainer.Request{
			VMUUID:        vmUUID,
			Name:          "api",
			Image:         "nginx:1.27",
			RestartPolicy: containerKind.RestartUnlessStopped,
			OwnerUUID:     ownerUUID,
		})
		require.NoError(t, err)
		require.Empty(t, made.ValidationErrors)

		w.containerIn(t, vmUUID, "api", "running")

		connected, err := connectNetwork.NewUseCase(w.client, w.validator, w.translator).Execute(ctx, &connectNetwork.Request{VMUUID: vmUUID, ID: "api", Network: networkID, Aliases: []string{"backend-api"}, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, connected.ValidationErrors)

		c, _ := w.dockerd.Holds(t, vmUUID).Container("api")
		require.Contains(t, c.NetworkSettings.Networks, "backend")
		assert.Equal(t, []string{"backend-api"}, c.NetworkSettings.Networks["backend"].Aliases)

		on := w.container(t, vmUUID, "api", "on the network", func(c presenter.Container) bool { return slices.Contains(c.Networks, "backend") })
		assert.ElementsMatch(t, []string{"backend", "bridge"}, on.Networks)

		spec := w.containerRecord(t, vmUUID, "api").Spec
		assert.Equal(t, []string{"backend"}, spec.Networks, "its spec names it, by its name")

		refused, err := deleteNetwork.NewUseCase(w.client, w.translator).Execute(ctx, &deleteNetwork.Request{VMUUID: vmUUID, ID: "backend", OwnerUUID: ownerUUID})
		require.NoError(t, err)
		assert.NotEmpty(t, refused.ValidationErrors, "a network with a container on it is not removed")
	})

	t.Run("taken off it behind the platform's back, it is put back on it", func(t *testing.T) {
		w.dockerd.Disconnect(t, vmUUID, "api", "backend")

		require.Eventually(t, func() bool {
			c, _ := w.dockerd.Holds(t, vmUUID).Container("api")

			return c.NetworkSettings.Networks["backend"] != nil
		}, settle, beat, "it was never put back on the network")

		c, _ := w.dockerd.Holds(t, vmUUID).Container("api")
		assert.Equal(t, []string{"backend-api"}, c.NetworkSettings.Networks["backend"].Aliases, "under the names it was put on it with")
	})

	t.Run("taken off it, it stays off it, and the network is removed", func(t *testing.T) {
		disconnected, err := disconnectNetwork.NewUseCase(w.client, w.translator).Execute(ctx, &disconnectNetwork.Request{VMUUID: vmUUID, ID: "api", Network: "backend", OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, disconnected.ValidationErrors)

		w.container(t, vmUUID, "api", "off the network", func(c presenter.Container) bool { return !slices.Contains(c.Networks, "backend") })
		assert.Empty(t, w.containerRecord(t, vmUUID, "api").Spec.Networks)

		time.Sleep(20 * beat)

		c, _ := w.dockerd.Holds(t, vmUUID).Container("api")
		assert.NotContains(t, c.NetworkSettings.Networks, "backend", "nothing put it back on it")

		deleted, err := deleteNetwork.NewUseCase(w.client, w.translator).Execute(ctx, &deleteNetwork.Request{VMUUID: vmUUID, ID: "backend", OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, deleted.ValidationErrors)

		require.Eventually(t, func() bool {
			_, there := w.dockerd.Holds(t, vmUUID).Network("backend")

			return !there
		}, settle, beat, "the network was never removed")
	})

	t.Run("taken off a network a moment before the network is removed, before its node says so, the network is removed all the same", func(t *testing.T) {
		made, err := createNetwork.NewUseCase(w.client, w.validator, w.translator).Execute(ctx, &createNetwork.Request{VMUUID: vmUUID, Name: "frontend", OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, made.ValidationErrors)

		connected, err := connectNetwork.NewUseCase(w.client, w.validator, w.translator).Execute(ctx, &connectNetwork.Request{VMUUID: vmUUID, ID: "api", Network: "frontend", OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, connected.ValidationErrors)

		// what the node says of the network is that the container is on it.
		eventually(t, "the network seen with the container on it", func(ctx context.Context) ([]presenter.DockerNetwork, error) {
			listed, err := getNetworks.NewUseCase(w.client, w.translator).Execute(ctx, &getNetworks.Request{VMUUID: vmUUID, OwnerUUID: ownerUUID})
			if err != nil {
				return nil, err
			}

			return listed.Items, nil
		}, func(networks []presenter.DockerNetwork) bool {
			return slices.ContainsFunc(networks, func(n presenter.DockerNetwork) bool {
				return n.Name == "frontend" && slices.Contains(n.Containers, "api")
			})
		})

		// and goes on saying it: the node says nothing more until the network
		// is removed.
		w.beating.Store(false)
		defer w.beating.Store(true)

		disconnected, err := disconnectNetwork.NewUseCase(w.client, w.translator).Execute(ctx, &disconnectNetwork.Request{VMUUID: vmUUID, ID: "api", Network: "frontend", OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, disconnected.ValidationErrors)

		deleted, err := deleteNetwork.NewUseCase(w.client, w.translator).Execute(ctx, &deleteNetwork.Request{VMUUID: vmUUID, ID: "frontend", OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, deleted.ValidationErrors, "whether a container is on it is its dockerd's to say, and nothing is")

		_, there := w.dockerd.Holds(t, vmUUID).Network("frontend")
		assert.False(t, there, "removed")
	})

	t.Run("a volume is made in it, and removed", func(t *testing.T) {
		made, err := createVolume.NewUseCase(w.client, w.validator, w.translator).Execute(ctx, &createVolume.Request{VMUUID: vmUUID, Name: "data", OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, made.ValidationErrors)
		require.NotNil(t, made.Volume)
		assert.Equal(t, "data", made.Name)

		assert.True(t, w.dockerd.Holds(t, vmUUID).Volume("data"))

		listed := eventually(t, "the volume listed", func(ctx context.Context) ([]presenter.Volume, error) {
			listed, err := getVolumes.NewUseCase(w.client, w.translator).Execute(ctx, &getVolumes.Request{VMUUID: vmUUID, OwnerUUID: ownerUUID})
			if err != nil {
				return nil, err
			}

			return listed.Items, nil
		}, func(items []presenter.Volume) bool { return len(items) > 0 })

		require.Len(t, listed, 1)
		assert.False(t, listed[0].Unmanaged)

		deleted, err := deleteVolume.NewUseCase(w.client, w.translator).Execute(ctx, &deleteVolume.Request{VMUUID: vmUUID, Name: "data", OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, deleted.ValidationErrors)

		require.Eventually(t, func() bool { return !w.dockerd.Holds(t, vmUUID).Volume("data") }, settle, beat, "the volume was never removed")
	})

	t.Run("an image is pulled into it, and removed; one a container it keeps uses is not", func(t *testing.T) {
		pulled, err := pullImage.NewUseCase(w.client, w.validator, w.translator).Execute(ctx, &pullImage.Request{VMUUID: vmUUID, Reference: "redis:7", OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, pulled.ValidationErrors)
		require.NotNil(t, pulled.Image)
		assert.Equal(t, []string{"redis:7"}, pulled.Tags)

		assert.True(t, w.dockerd.Holds(t, vmUUID).Image("redis:7"))

		images := eventually(t, "the image listed", func(ctx context.Context) ([]presenter.Image, error) {
			listed, err := getImages.NewUseCase(w.client, w.translator).Execute(ctx, &getImages.Request{VMUUID: vmUUID, OwnerUUID: ownerUUID})
			if err != nil {
				return nil, err
			}

			return listed.Items, nil
		}, func(items []presenter.Image) bool {
			return slices.ContainsFunc(items, func(i presenter.Image) bool { return slices.Contains(i.Tags, "redis:7") })
		})

		for _, i := range images {
			switch {
			case slices.Contains(i.Tags, "redis:7"):
				assert.False(t, i.Unmanaged, "pulled through the platform")
			case slices.Contains(i.Tags, "nginx:1.27"):
				assert.False(t, i.Unmanaged, "pulled for a container it keeps, which keeps it")
			case slices.Contains(i.Tags, "postgres:17"):
				assert.True(t, i.Unmanaged, "pulled from its terminal")
			}
		}

		refused, err := deleteImage.NewUseCase(w.client, w.translator).Execute(ctx, &deleteImage.Request{VMUUID: vmUUID, ID: "nginx:1.27", OwnerUUID: ownerUUID})
		require.NoError(t, err)
		assert.NotEmpty(t, refused.ValidationErrors, "the image of a container it keeps")
		assert.True(t, w.dockerd.Holds(t, vmUUID).Image("nginx:1.27"))

		deleted, err := deleteImage.NewUseCase(w.client, w.translator).Execute(ctx, &deleteImage.Request{VMUUID: vmUUID, ID: "redis:7", OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, deleted.ValidationErrors)

		require.Eventually(t, func() bool { return !w.dockerd.Holds(t, vmUUID).Image("redis:7") }, settle, beat, "the image was never removed")
	})
}

// TestARestoredDockerVM holds what the platform keeps of a Docker VM's
// building blocks to what its disk holds once it is restored from a
// snapshot: what was made since is forgotten, and what was deleted since is
// kept again.
func TestARestoredDockerVM(t *testing.T) {
	t.Parallel()

	w := start(t)
	ctx := t.Context()

	vmUUID := w.dockerVM(t, "builds")

	w.runContainer(t, vmUUID, "kept")
	keptUUID := w.containerRecord(t, vmUUID, "kept").Metadata.UUID

	stopped, err := stopVM.NewUseCase(w.client, w.translator).Execute(ctx, &stopVM.Request{UUID: vmUUID, OwnerUUID: ownerUUID})
	require.NoError(t, err)
	require.Empty(t, stopped.ValidationErrors)

	w.vmIn(t, vmUUID, "stopped")

	taken, err := createSnapshot.NewUseCase(w.client, w.validator, w.translator, w.owners).Execute(ctx, &createSnapshot.Request{VMUUID: vmUUID, Name: "before", OwnerUUID: ownerUUID})
	require.NoError(t, err)
	require.Empty(t, taken.ValidationErrors)

	snapshotUUID := taken.Snapshot.UUID

	eventually(t, "the snapshot being ready", func(ctx context.Context) (presenter.Snapshot, error) {
		read, err := getSnapshot.NewUseCase(w.client, w.owners).Execute(ctx, &getSnapshot.Request{UUID: snapshotUUID, OwnerUUID: ownerUUID})
		if err != nil {
			return presenter.Snapshot{}, err
		}

		return read.Snapshot, nil
	}, func(s presenter.Snapshot) bool { return s.State == "ready" })

	started, err := startVM.NewUseCase(w.client, w.translator).Execute(ctx, &startVM.Request{UUID: vmUUID, OwnerUUID: ownerUUID})
	require.NoError(t, err)
	require.Empty(t, started.ValidationErrors)

	w.vmIn(t, vmUUID, "running")
	w.containerIn(t, vmUUID, "kept", "running")

	w.runContainer(t, vmUUID, "later")

	deleted, err := deleteContainer.NewUseCase(w.client, w.translator).Execute(ctx, &deleteContainer.Request{VMUUID: vmUUID, ID: "kept", Force: true, OwnerUUID: ownerUUID})
	require.NoError(t, err)
	require.Empty(t, deleted.ValidationErrors)

	require.Eventually(t, func() bool {
		_, there := w.dockerd.Holds(t, vmUUID).Container("kept")

		return !there && len(w.containerRecords(t, vmUUID)) == 1
	}, settle, beat, "kept was never deleted")

	restored, err := restoreVM.NewUseCase(w.client, w.validator, w.translator).Execute(ctx, &restoreVM.Request{UUID: vmUUID, OwnerUUID: ownerUUID, SnapshotUUID: snapshotUUID})
	require.NoError(t, err)
	require.Empty(t, restored.ValidationErrors)

	w.stored(t, vmUUID, "running from the snapshot", func(v vmKind.VM) bool {
		return v.Status.State == vmKind.Running && !v.Status.RestoredAt.IsZero()
	})

	t.Run("its records are what its restored disk holds", func(t *testing.T) {
		held := w.dockerd.Holds(t, vmUUID)

		_, there := held.Container("kept")
		assert.True(t, there, "the disk it had when the snapshot was taken")

		_, there = held.Container("later")
		assert.False(t, there)

		require.Eventually(t, func() bool {
			records := w.containerRecords(t, vmUUID)

			return len(records) == 1 && records[0].Metadata.UUID == keptUUID
		}, settle, beat, "what was made since was never forgotten, or what was deleted since never kept again")

		kept := w.containerIn(t, vmUUID, "kept", "running")
		assert.False(t, kept.Unmanaged, "kept again as what it was")

		listed, err := getVMContainers.NewUseCase(w.client, w.translator).Execute(ctx, &getVMContainers.Request{VMUUID: vmUUID, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		assert.False(t, slices.ContainsFunc(listed.Items, func(c presenter.Container) bool { return c.Name == "later" }))
	})
}

// dockerVM is a Docker VM of the owner's, running.
func (w *workload) dockerVM(t *testing.T, name string) string {
	t.Helper()

	created, err := createVM.NewUseCase(w.client, w.validator, w.translator, w.owners, ingressDomain).Execute(t.Context(), &createVM.Request{
		Name:           name,
		Kind:           string(vm.KindDocker),
		Resources:      input.Resources{CPUs: 2, Memory: 2 << 30, Disk: 20 << 30},
		Network:        input.Network{Ingress: "allow", Egress: "allow"},
		PersistentDisk: true,
		OwnerUUID:      ownerUUID,
	})
	require.NoError(t, err)
	require.Empty(t, created.ValidationErrors)

	w.vmIn(t, created.VM.UUID, "running")

	return created.VM.UUID
}

// runContainer is a container of the owner's in a Docker VM, once it runs.
func (w *workload) runContainer(t *testing.T, vmUUID string, name string) {
	t.Helper()

	made, err := createContainer.NewUseCase(w.client, w.validator, w.translator).Execute(t.Context(), &createContainer.Request{
		VMUUID:        vmUUID,
		Name:          name,
		Image:         "nginx:1.27",
		RestartPolicy: containerKind.RestartAlways,
		OwnerUUID:     ownerUUID,
	})
	require.NoError(t, err)
	require.Empty(t, made.ValidationErrors)

	w.containerIn(t, vmUUID, name, "running")
}

// containerIn is a container of a Docker VM as the dashboard lists it, once
// docker says it is in state.
func (w *workload) containerIn(t *testing.T, vmUUID string, name string, state string) presenter.Container {
	t.Helper()

	return w.container(t, vmUUID, name, state, func(c presenter.Container) bool { return c.State == state })
}

// container is a container of a Docker VM as the dashboard lists it, once
// condition holds of it.
func (w *workload) container(t *testing.T, vmUUID string, name string, what string, condition func(presenter.Container) bool) presenter.Container {
	t.Helper()

	listing := getVMContainers.NewUseCase(w.client, w.translator)

	return eventually(t, "the container "+name+" "+what, func(ctx context.Context) (presenter.Container, error) {
		listed, err := listing.Execute(ctx, &getVMContainers.Request{VMUUID: vmUUID, OwnerUUID: ownerUUID})
		if err != nil {
			return presenter.Container{}, err
		}

		for _, c := range listed.Items {
			if c.Name == name {
				return c, nil
			}
		}

		return presenter.Container{}, nil
	}, func(c presenter.Container) bool { return c.Name == name && condition(c) })
}

// containerRecords are the containers the control plane keeps in a Docker
// VM.
func (w *workload) containerRecords(t *testing.T, vmUUID string) []containerKind.Container {
	t.Helper()

	records, _, err := w.resources.GetAll(t.Context(), containerKind.Name, resource.Filter{Parent: kind.Reference{Kind: containerKind.Parent, UUID: vmUUID}}, 0, 0)
	require.NoError(t, err)

	containers := make([]containerKind.Container, len(records))
	for i := range records {
		containers[i], err = kind.Decode[containerKind.Spec, containerKind.Status](records[i].Raw)
		require.NoError(t, err)
	}

	return containers
}

// containerRecord is the container the control plane keeps in a Docker VM
// under name.
func (w *workload) containerRecord(t *testing.T, vmUUID string, name string) containerKind.Container {
	t.Helper()

	for _, c := range w.containerRecords(t, vmUUID) {
		if c.Spec.Name == name {
			return c
		}
	}

	t.Fatalf("no container named %s is kept in %s", name, vmUUID)

	return containerKind.Container{}
}
