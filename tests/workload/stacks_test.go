package workload_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/deleteContainer"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/getVMContainers"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/image/getImages"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/network/getNetworkDependents"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/createStack"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/deleteStack"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/getStack"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/restartStack"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/startStack"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/stopStack"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/getVMDependents"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/startVM"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/stopVM"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/volume/deleteVolume"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/volume/getVolumeDependents"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	blockKinds "github.com/khanzadimahdi/testproject/domain/workload/kinds/blocks"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// shopCompose is a stack of two services, a web server and a cache.
const shopCompose = "services:\n  web:\n    image: nginx:alpine\n  cache:\n    image: redis:alpine\n"

// TestAStack walks a stack through its life from the dashboard, as a kind
// the control plane keeps and its graph makes, all of it the platform's: made
// in a Docker VM made for it, its containers listed as its own, degraded when
// one of its containers falls over and stays down, as a container on its own
// does, stopped, started and restarted, waiting while its VM is stopped, and
// deleted, its containers and its network with it, and its images left to
// nobody.
func TestAStack(t *testing.T) {
	t.Parallel()

	w := start(t)
	ctx := t.Context()

	deployed, err := createStack.NewUseCase(w.client, w.validator, w.translator, w.owners).Execute(ctx, &createStack.Request{
		Name:      "shop",
		Compose:   shopCompose,
		OwnerUUID: ownerUUID,
	})
	require.NoError(t, err)
	require.Empty(t, deployed.ValidationErrors)
	require.NotNil(t, deployed.Stack)

	stackUUID, slug, vmUUID := deployed.Stack.UUID, deployed.Stack.Slug, deployed.VM.UUID

	assert.True(t, deployed.VM.Created, "it named no Docker VM, so its graph makes one for it")
	assert.Equal(t, "docker", deployed.VM.Name, "called what it keeps of it")
	assert.Equal(t, "deploying", deployed.Stack.State, "its graph makes it")
	assert.Equal(t, "running", deployed.Stack.ExpectedState)

	t.Run("its graph makes its vm, its network and its containers, and it runs", func(t *testing.T) {
		running := w.stackIn(t, stackUUID, "running", "running")

		assert.Equal(t, vmUUID, running.VMUUID)
		assert.Equal(t, "docker", running.VMName)
		assert.Empty(t, running.Note)
		require.Len(t, running.Containers, 2)

		for _, c := range running.Containers {
			assert.Equal(t, slug, c.Stack)
			assert.Equal(t, stackUUID, c.StackUUID)
			assert.Equal(t, stackUUID, c.Labels[stackKind.LabelStack], "labelled as the stack's")
			assert.Equal(t, slug+"_"+c.Service, c.Name, "named under its slug, as its service")
		}

		held := w.dockerd.Holds(t, vmUUID)

		_, networked := held.Network(slug + "_default")
		assert.True(t, networked, "each joins the default network, as neither names one")

		for _, service := range []string{"web", "cache"} {
			c, there := held.Container(slug + "_" + service)
			require.True(t, there)
			require.Contains(t, c.NetworkSettings.Networks, slug+"_default")
			assert.Equal(t, []string{service}, c.NetworkSettings.Networks[slug+"_default"].Aliases, "%s is reached by its service's name there", service)
		}
	})

	t.Run("its vm's containers are listed as its own, by its slug, its uuid and their services, to whoever can see it", func(t *testing.T) {
		for _, owner := range []string{ownerUUID, ""} {
			listed, err := getVMContainers.NewUseCase(w.client, w.translator).Execute(ctx, &getVMContainers.Request{VMUUID: vmUUID, OwnerUUID: owner})
			require.NoError(t, err)
			require.Empty(t, listed.ValidationErrors)

			services := map[string]presenter.Container{}
			for _, c := range listed.Items {
				if len(c.Service) > 0 {
					services[c.Service] = c
				}
			}

			require.Len(t, services, 2, "its web server and its cache")

			for service, c := range services {
				assert.Equal(t, slug, c.Stack, "%s is the stack's", service)
				assert.Equal(t, stackUUID, c.StackUUID)
				assert.Equal(t, slug+"_"+service, c.Name)
			}
		}
	})

	t.Run("the images its containers were made from are kept by them", func(t *testing.T) {
		w.images(t, vmUUID, "the stack's images kept", func(images map[string]presenter.Image) bool {
			web, pulled := images["nginx:alpine"]
			cache, alsoPulled := images["redis:alpine"]

			return pulled && alsoPulled && !web.Unmanaged && !cache.Unmanaged
		})
	})

	t.Run("a container that falls over leaves it degraded, and stays down as a container on its own would, until it is started", func(t *testing.T) {
		w.dockerd.Exit(t, vmUUID, slug+"_cache", 137)

		degraded := w.stack(t, stackUUID, "degraded", func(s presenter.StackDetail) bool { return s.State == "degraded" })
		assert.Equal(t, "running", degraded.ExpectedState)

		started, err := startStack.NewUseCase(w.client, w.translator).Execute(ctx, &startStack.Request{UUID: stackUUID, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, started.ValidationErrors)

		w.stackIn(t, stackUUID, "running", "running")
	})

	t.Run("stopped", func(t *testing.T) {
		stopped, err := stopStack.NewUseCase(w.client, w.translator).Execute(ctx, &stopStack.Request{UUID: stackUUID, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, stopped.ValidationErrors)

		s := w.stackIn(t, stackUUID, "stopped", "exited")
		assert.Equal(t, "stopped", s.ExpectedState)
	})

	t.Run("started", func(t *testing.T) {
		started, err := startStack.NewUseCase(w.client, w.translator).Execute(ctx, &startStack.Request{UUID: stackUUID, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, started.ValidationErrors)

		s := w.stackIn(t, stackUUID, "running", "running")
		assert.Equal(t, "running", s.ExpectedState)
	})

	t.Run("restarted, its containers stopped and started again", func(t *testing.T) {
		before := w.containerRecord(t, vmUUID, slug+"_web").Status.Since

		restarted, err := restartStack.NewUseCase(w.client, w.translator).Execute(ctx, &restartStack.Request{UUID: stackUUID, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, restarted.ValidationErrors)

		s := w.stack(t, stackUUID, "restarted", func(s presenter.StackDetail) bool {
			web := w.containerRecord(t, vmUUID, slug+"_web")

			return s.State == "running" && web.Status.Status.State == containerKind.Running && web.Status.Since.After(before)
		})

		assert.Equal(t, "running", s.ExpectedState)
	})

	t.Run("its vm stopped, it waits on the vm, and runs again once the vm does", func(t *testing.T) {
		stopped, err := stopVM.NewUseCase(w.client, w.translator).Execute(ctx, &stopVM.Request{UUID: vmUUID, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, stopped.ValidationErrors)

		waiting := w.stack(t, stackUUID, "waiting on its vm", func(s presenter.StackDetail) bool {
			return s.State == "waiting" && strings.HasSuffix(s.Reason, "its vm is stopped")
		})
		assert.Equal(t, "running", waiting.ExpectedState, "it is still to be running, once it can be")
		assert.Equal(t, presenter.NoteVMNotRunning, waiting.Note)

		refused, err := stopStack.NewUseCase(w.client, w.translator).Execute(ctx, &stopStack.Request{UUID: stackUUID, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		assert.Contains(t, refused.ValidationErrors, "vm", "nothing in a vm that is not running can be stopped")

		started, err := startVM.NewUseCase(w.client, w.translator).Execute(ctx, &startVM.Request{UUID: vmUUID, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, started.ValidationErrors)

		w.stackIn(t, stackUUID, "running", "running")
	})

	t.Run("deleted, it is gone, and its containers and its network with it", func(t *testing.T) {
		deleted, err := deleteStack.NewUseCase(w.client, w.translator).Execute(ctx, &deleteStack.Request{UUID: stackUUID, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, deleted.ValidationErrors)

		require.Eventually(t, func() bool {
			_, err := w.readStack(ctx, stackUUID)

			return errors.Is(err, domain.ErrNotExists)
		}, settle, beat, "the stack was never removed")

		_, err = w.resources.GetOne(ctx, stackKind.Name, stackUUID)
		assert.ErrorIs(t, err, domain.ErrNotExists, "record and all")

		held := w.dockerd.Holds(t, vmUUID)

		for _, c := range held.Containers {
			assert.False(t, strings.HasPrefix(c.Names[0], "/"+slug+"_"), "%s went with it", c.Names[0])
		}

		_, networked := held.Network(slug + "_default")
		assert.False(t, networked, "and so did its network")

		w.images(t, vmUUID, "the images it left behind nobody's", func(images map[string]presenter.Image) bool {
			web, left := images["nginx:alpine"]
			cache, alsoLeft := images["redis:alpine"]

			return left && alsoLeft && web.Unmanaged && cache.Unmanaged
		})

		w.vmIn(t, vmUUID, "running")
	})
}

// TestARefusedStack asks the dashboard for stacks whose files the platform
// does not carry out, which are refused as they are asked for, each saying
// what of its file it is refused for, and nothing is kept.
func TestARefusedStack(t *testing.T) {
	t.Parallel()

	w := start(t)
	ctx := t.Context()

	vmUUID := w.dockerVM(t, "builds")

	for name, tt := range map[string]struct {
		compose string
		refused map[string]string
	}{
		"one that builds its image": {
			compose: "services:\n  web:\n    build: .\n    image: shop\n",
			refused: map[string]string{"compose.services.web.build": "a stack's file does not take this key"},
		},
		"one with a key of its own": {
			compose: "services:\n  web:\n    image: nginx:alpine\nnonsense: true\n",
			refused: map[string]string{"compose.nonsense": "a stack's file does not take this key"},
		},
		"one whose services depend on one another in a ring": {
			compose: "services:\n  web:\n    image: nginx:alpine\n    depends_on: [api]\n  api:\n    image: shop/api\n    depends_on: [web]\n",
			refused: map[string]string{
				"compose.services.web.depends_on": "the services depend on one another in a ring, which no order starts",
				"compose.services.api.depends_on": "the services depend on one another in a ring, which no order starts",
			},
		},
	} {
		t.Run(name+" is refused, saying what of it", func(t *testing.T) {
			refused, err := createStack.NewUseCase(w.client, w.validator, w.translator, w.owners).Execute(ctx, &createStack.Request{
				Name:      "broken",
				Compose:   tt.compose,
				VMUUID:    vmUUID,
				OwnerUUID: ownerUUID,
			})
			require.NoError(t, err)

			assert.Equal(t, domain.ValidationErrors(tt.refused), refused.ValidationErrors)
			assert.Nil(t, refused.Stack)
		})
	}

	kept, _, err := w.resources.GetAll(ctx, stackKind.Name, resource.Filter{}, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, kept, "and nothing is kept")
}

// images are the images the dashboard lists in the Docker VM vmUUID names,
// by each of their tags, once condition holds of them.
func (w *workload) images(t *testing.T, vmUUID string, what string, condition func(map[string]presenter.Image) bool) map[string]presenter.Image {
	t.Helper()

	return eventually(t, what, func(ctx context.Context) (map[string]presenter.Image, error) {
		listed, err := getImages.NewUseCase(w.client, w.translator).Execute(ctx, &getImages.Request{VMUUID: vmUUID, OwnerUUID: ownerUUID})
		if err != nil {
			return nil, err
		}

		images := make(map[string]presenter.Image)
		for _, i := range listed.Items {
			for _, tag := range i.Tags {
				images[tag] = i
			}
		}

		return images, nil
	}, condition)
}

// stackIn is the stack as the dashboard shows it, once it is in state with
// both of its containers in containers.
func (w *workload) stackIn(t *testing.T, uuid string, state string, containers string) presenter.StackDetail {
	t.Helper()

	return w.stack(t, uuid, state+" with its containers "+containers, func(s presenter.StackDetail) bool {
		return s.State == state && len(s.Containers) == 2 && !slices.ContainsFunc(s.Containers, func(c presenter.Container) bool {
			return c.State != containers
		})
	})
}

// stack is the stack as the dashboard shows it, once condition holds of it.
func (w *workload) stack(t *testing.T, uuid string, what string, condition func(presenter.StackDetail) bool) presenter.StackDetail {
	t.Helper()

	return eventually(t, "the stack "+what, func(ctx context.Context) (presenter.StackDetail, error) {
		return w.readStack(ctx, uuid)
	}, func(s presenter.StackDetail) bool {
		// a stack that failed on the way says why, which is worth seeing
		// rather than waiting out.
		if s.State == "failed" {
			t.Fatalf("the stack failed: %s", s.Reason)
		}

		return condition(s)
	})
}

func (w *workload) readStack(ctx context.Context, uuid string) (presenter.StackDetail, error) {
	read, err := getStack.NewUseCase(w.client, w.owners).Execute(ctx, &getStack.Request{UUID: uuid, OwnerUUID: ownerUUID})
	if err != nil {
		return presenter.StackDetail{}, err
	}

	return read.StackDetail, nil
}

// storeCompose is a stack whose web server depends on its database, both on
// a network it declares, the database with a volume it declares.
const storeCompose = `
services:
  db:
    image: postgres:17
    restart: unless-stopped
    volumes: ["data:/var/lib/postgresql/data"]
    networks: [backend]
  web:
    image: nginx:alpine
    depends_on: [db]
    ports: ["8080:80"]
    networks: [backend]
volumes:
  data:
networks:
  backend:
`

// TestAStacksGraph walks a stack that names no Docker VM through its graph,
// from the dashboard and the control plane's API: its VM is made, then its
// network and its volume, then its database and then its web server, and
// then it, which runs; one of its containers, deleted, is made again, and so
// is its volume, once nothing mounts it, and its VM, under its uuid and its
// slug, and then everything in it; stopped and started, its containers stop
// and start in the order they depend on one another; stopped, one of its
// containers, deleted, is made again stopped, and it stays stopped; and
// deleted, it takes its containers and its network with it, and leaves its
// volume and its VM.
func TestAStacksGraph(t *testing.T) {
	t.Parallel()

	w := start(t)
	ctx := t.Context()

	deployed, err := createStack.NewUseCase(w.client, w.validator, w.translator, w.owners).Execute(ctx, &createStack.Request{
		Name:      "store",
		Compose:   storeCompose,
		OwnerUUID: ownerUUID,
	})
	require.NoError(t, err)
	require.Empty(t, deployed.ValidationErrors)
	require.NotNil(t, deployed.Stack)

	stackUUID, slug, vmUUID := deployed.Stack.UUID, deployed.Stack.Slug, deployed.VM.UUID
	root := graph.Ref{Kind: stackKind.Name, UUID: stackUUID}

	named := func(name string) string { return slug + "_" + name }

	t.Run("its graph made its vm, then its network and its volume, then db, then web, and then it, which runs", func(t *testing.T) {
		plan := w.plan(t, root, "made", func(executions []graph.Execution) bool {
			return len(executions) == 6 && !slices.ContainsFunc(executions, func(e graph.Execution) bool { return e.State != graph.StateDone })
		})

		levels := map[graph.NodeID]int{}
		for _, e := range plan {
			levels[e.Node] = e.Level
		}

		assert.Equal(t, map[graph.NodeID]int{
			graph.ID(vmKind.Name, vmUUID):                0,
			graph.ID(networkKind.Name, named("backend")): 1,
			graph.ID(volumeKind.Name, named("data")):     1,
			graph.ID(containerKind.Name, named("db")):    2,
			graph.ID(containerKind.Name, named("web")):   3,
			graph.ID(stackKind.Name, stackUUID):          4,
		}, levels, "its vm first, then what lives in it, its services as they depend on one another, then it; and no default network, which none joins")

		for _, e := range plan {
			for _, before := range plan {
				if before.Level < e.Level {
					assert.False(t, e.StartedAt.Before(before.FinishedAt), "%s was started once %s was done", e.Node, before.Node)
				}
			}
		}

		assert.Less(t, slices.Index(w.dockerd.Did(vmUUID), "start "+named("db")), slices.Index(w.dockerd.Did(vmUUID), "create "+named("web")), "web was made once db ran")

		w.stack(t, stackUUID, "running", func(s presenter.StackDetail) bool { return s.State == "running" })

		s := w.stackRecord(t, stackUUID)
		db := w.containerRecord(t, vmUUID, named("db"))
		web := w.containerRecord(t, vmUUID, named("web"))

		assert.Equal(t, []stackKind.ServiceStatus{
			{Name: "db", State: stackKind.ServiceRunning, Containers: []stackKind.ContainerStatus{{ID: db.Metadata.UUID, Name: named("db"), State: "running"}}},
			{Name: "web", State: stackKind.ServiceRunning, Containers: []stackKind.ContainerStatus{{ID: web.Metadata.UUID, Name: named("web"), State: "running"}}},
		}, s.Status.Services, "its record names its containers, its foreign keys")

		assert.Equal(t, []kind.Reference{{Kind: stackKind.Name, UUID: stackUUID}}, db.Metadata.Owners, "its containers are its own")
		assert.Equal(t, []string{named("data")}, []string{db.Spec.Mounts[0].Source})

		backend := w.blockRecord(t, networkKind.Name, vmUUID, named("backend"))
		assert.ElementsMatch(t, []kind.Reference{
			{Kind: stackKind.Name, UUID: stackUUID},
			{Kind: containerKind.Name, UUID: db.Metadata.UUID},
			{Kind: containerKind.Name, UUID: web.Metadata.UUID},
		}, backend.Metadata.Owners, "and its network is its own and theirs")

		data := w.blockRecord(t, volumeKind.Name, vmUUID, named("data"))
		assert.Empty(t, data.Metadata.Owners, "and its volume is used, not owned")

		c, there := w.dockerd.Holds(t, vmUUID).Container(named("db"))
		require.True(t, there)
		assert.Equal(t, []string{"db"}, c.NetworkSettings.Networks[named("backend")].Aliases, "reached by its service's name")
	})

	t.Run("one of its containers, deleted, is made again by it, as the delete says", func(t *testing.T) {
		before := w.containerRecord(t, vmUUID, named("web"))

		status, body := w.controlPlane(t, http.MethodDelete, "/api/containers/"+before.Metadata.UUID, url.Values{"dry_run": {"true"}})
		require.Equal(t, http.StatusOK, status, string(body))
		assert.JSONEq(t, `{"dependents": [{"kind": "stack", "uuid": "`+stackUUID+`", "name": "store", "recreates": true}], "recreated": true}`, string(body))

		deleted, err := deleteContainer.NewUseCase(w.client, w.translator).Execute(ctx, &deleteContainer.Request{VMUUID: vmUUID, ID: named("web"), Force: true, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, deleted.ValidationErrors)

		again := w.containerKept(t, vmUUID, named("web"), "made again", func(c containerKind.Container) bool {
			return c.Metadata.UUID != before.Metadata.UUID && c.Status.Status.State == containerKind.Running
		})

		assert.Equal(t, []kind.Reference{{Kind: stackKind.Name, UUID: stackUUID}}, again.Metadata.Owners, "the stack's, as it was")

		w.stack(t, stackUUID, "running with it", func(s presenter.StackDetail) bool {
			return s.State == "running" && w.stackRecord(t, stackUUID).Status.Services[1].Containers[0].ID == again.Metadata.UUID
		})
	})

	t.Run("its volume, deleted once nothing in its dockerd mounts it, is made again, and then what mounts it", func(t *testing.T) {
		before := w.blockRecord(t, volumeKind.Name, vmUUID, named("data"))

		// docker removes no volume a container mounts, stopped or not: the
		// container goes behind the platform's back first.
		w.dockerd.Remove(t, vmUUID, named("db"))

		deleted, err := deleteVolume.NewUseCase(w.client, w.translator).Execute(ctx, &deleteVolume.Request{VMUUID: vmUUID, Name: named("data"), OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, deleted.ValidationErrors)

		require.Eventually(t, func() bool {
			again, there := w.findBlock(t, volumeKind.Name, vmUUID, named("data"))
			if !there || again.Metadata.UUID == before.Metadata.UUID {
				return false
			}

			held := w.dockerd.Holds(t, vmUUID)
			c, running := held.Container(named("db"))

			return held.Volume(named("data")) && running && c.State == "running"
		}, settle, beat, "the volume, and then the container mounting it, were never made again")

		w.stack(t, stackUUID, "running", func(s presenter.StackDetail) bool { return s.State == "running" })
	})

	t.Run("what needs its vm, its network and its volume is read from the dashboard, as each one's delete would say, and nothing is deleted or asked for it", func(t *testing.T) {
		did := len(w.dockerd.Did(vmUUID))
		store := []presenter.Dependent{{Kind: stackKind.Name, UUID: stackUUID, Name: "store", Recreates: true}}

		vmNeeds, err := getVMDependents.NewUseCase(w.client, w.translator).Execute(ctx, &getVMDependents.Request{UUID: vmUUID, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, vmNeeds.ValidationErrors)
		assert.Equal(t, store, vmNeeds.Dependents, "it makes its vm again")
		assert.True(t, vmNeeds.Recreated)

		networkNeeds, err := getNetworkDependents.NewUseCase(w.client, w.translator).Execute(ctx, &getNetworkDependents.Request{VMUUID: vmUUID, ID: named("backend"), OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, networkNeeds.ValidationErrors)
		assert.Equal(t, store, networkNeeds.Dependents, "and its network, named as it is in its vm")

		volumeNeeds, err := getVolumeDependents.NewUseCase(w.client, w.translator).Execute(ctx, &getVolumeDependents.Request{VMUUID: vmUUID, Name: named("data"), OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, volumeNeeds.ValidationErrors)
		assert.Equal(t, store, volumeNeeds.Dependents, "and its volume, which it uses")

		_, err = getVMDependents.NewUseCase(w.client, w.translator).Execute(ctx, &getVMDependents.Request{UUID: vmUUID, OwnerUUID: "somebody-else"})
		assert.ErrorIs(t, err, domain.ErrNotExists, "somebody else's vm is not there to ask about")

		v := w.stored(t, vmUUID, "as it was", func(v vmKind.VM) bool { return v.Status.State == vmKind.Running })
		assert.Equal(t, vmKind.Running, v.Status.Expected, "asking deleted nothing")
		assert.True(t, v.Metadata.DeletionRequestedAt.IsZero())
		assert.Equal(t, stackKind.Running, w.stackRecord(t, stackUUID).Status.Expected)
		assert.Len(t, w.dockerd.Did(vmUUID), did, "and nothing was done to its containers")
	})

	t.Run("its vm, deleted, is made again under its uuid and its slug, and then everything in it", func(t *testing.T) {
		v := w.stored(t, vmUUID, "running", func(v vmKind.VM) bool { return v.Status.State == vmKind.Running })
		gone := time.Now()

		status, body := w.controlPlane(t, http.MethodDelete, "/api/vms/"+vmUUID, nil)
		require.Equal(t, http.StatusAccepted, status, string(body))

		answer := deletedOf(t, body)
		assert.Equal(t, []dependentShown{{Kind: stackKind.Name, UUID: stackUUID, Name: "store", Recreates: true}}, answer.Dependents)
		assert.True(t, answer.Recreated, "the stack makes it again")

		again := w.stored(t, vmUUID, "made again", func(v vmKind.VM) bool {
			return v.Status.StartedAt.After(gone) && v.Status.State == vmKind.Running
		})

		assert.Equal(t, v.Metadata.Slug, again.Metadata.Slug, "under the slug it had, reached where it was")
		assert.Equal(t, v.Metadata.Name, again.Metadata.Name)

		require.Eventually(t, func() bool {
			held := w.dockerd.Holds(t, vmUUID)
			db, dbThere := held.Container(named("db"))
			web, webThere := held.Container(named("web"))
			_, networked := held.Network(named("backend"))

			return dbThere && webThere && db.State == "running" && web.State == "running" && networked && held.Volume(named("data"))
		}, settle, beat, "what was in it was never made again")

		s := w.stack(t, stackUUID, "running again", func(s presenter.StackDetail) bool { return s.State == "running" })
		assert.Equal(t, stackUUID, s.UUID, "it is what it was")
		assert.Equal(t, vmKind.ChoiceOf(again), w.stackRecord(t, stackUUID).Spec.VM, "and keeps its vm as it is")
	})

	t.Run("stopped, its containers stop in the reverse of the order they depend on one another, and started, in that order", func(t *testing.T) {
		before := len(w.dockerd.Did(vmUUID))

		stopped, err := stopStack.NewUseCase(w.client, w.translator).Execute(ctx, &stopStack.Request{UUID: stackUUID, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, stopped.ValidationErrors)

		w.stack(t, stackUUID, "stopped", func(s presenter.StackDetail) bool { return s.State == "stopped" })

		did := w.dockerd.Did(vmUUID)[before:]
		assert.Equal(t, []string{"stop " + named("web"), "stop " + named("db")}, did, "web, which depends on db, first")

		before = len(w.dockerd.Did(vmUUID))

		started, err := startStack.NewUseCase(w.client, w.translator).Execute(ctx, &startStack.Request{UUID: stackUUID, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, started.ValidationErrors)

		w.stack(t, stackUUID, "running", func(s presenter.StackDetail) bool { return s.State == "running" })

		did = w.dockerd.Did(vmUUID)[before:]
		assert.Equal(t, []string{"start " + named("db"), "start " + named("web")}, did, "db, which web depends on, first")
	})

	t.Run("stopped, one of its containers, deleted, is made again stopped, and it stays stopped", func(t *testing.T) {
		stopped, err := stopStack.NewUseCase(w.client, w.translator).Execute(ctx, &stopStack.Request{UUID: stackUUID, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, stopped.ValidationErrors)

		w.stack(t, stackUUID, "stopped", func(s presenter.StackDetail) bool { return s.State == "stopped" })

		before := w.containerRecord(t, vmUUID, named("web"))
		did := len(w.dockerd.Did(vmUUID))

		deleted, err := deleteContainer.NewUseCase(w.client, w.translator).Execute(ctx, &deleteContainer.Request{VMUUID: vmUUID, ID: named("web"), Force: true, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, deleted.ValidationErrors)

		again := w.containerKept(t, vmUUID, named("web"), "made again", func(c containerKind.Container) bool {
			return c.Metadata.UUID != before.Metadata.UUID && c.Status.Docker != nil
		})

		assert.Equal(t, containerKind.Stopped, again.Status.Expected, "admitted expected stopped, as its stack is")

		s := w.stack(t, stackUUID, "stopped with it", func(s presenter.StackDetail) bool {
			return w.stackRecord(t, stackUUID).Status.Services[1].Containers[0].ID == again.Metadata.UUID
		})
		assert.Equal(t, "stopped", s.State, "and not degraded")

		c, there := w.dockerd.Holds(t, vmUUID).Container(named("web"))
		require.True(t, there)
		assert.NotEqual(t, "running", c.State, "it is left stopped")

		made := w.dockerd.Did(vmUUID)[did:]
		require.Contains(t, made, "create "+named("web"))
		assert.Equal(t, "stop "+named("web"), made[len(made)-1], "what docker starts as it makes it is stopped once it is made, and nothing starts it after")
	})

	t.Run("deleted, it takes its containers and its network with it, and leaves its volume and its vm", func(t *testing.T) {
		before := len(w.dockerd.Did(vmUUID))

		deleted, err := deleteStack.NewUseCase(w.client, w.translator).Execute(ctx, &deleteStack.Request{UUID: stackUUID, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, deleted.ValidationErrors)

		require.Eventually(t, func() bool {
			_, err := w.resources.GetOne(ctx, stackKind.Name, stackUUID)

			return errors.Is(err, domain.ErrNotExists)
		}, settle, beat, "the stack was never removed")

		assert.Empty(t, w.containerRecords(t, vmUUID))

		_, kept := w.findBlock(t, networkKind.Name, vmUUID, named("backend"))
		assert.False(t, kept)

		held := w.dockerd.Holds(t, vmUUID)

		for _, service := range []string{"db", "web"} {
			_, there := held.Container(named(service))
			assert.False(t, there, "%s went with it", service)
		}

		_, networked := held.Network(named("backend"))
		assert.False(t, networked, "and so did its network")

		did := w.dockerd.Did(vmUUID)[before:]
		require.Contains(t, did, "remove "+named("web"))
		require.Contains(t, did, "remove "+named("db"))
		assert.Less(t, slices.Index(did, "remove "+named("web")), slices.Index(did, "remove "+named("db")), "web, which depends on db, first")

		assert.True(t, held.Volume(named("data")), "its volume stays")

		_, kept = w.findBlock(t, volumeKind.Name, vmUUID, named("data"))
		assert.True(t, kept)

		w.stored(t, vmUUID, "kept", func(v vmKind.VM) bool { return v.Status.State == vmKind.Running })
	})

	t.Run("a file it does not take is refused by the control plane's api, saying what of it", func(t *testing.T) {
		status, body := w.controlPlaneWith(t, http.MethodPost, "/api/stacks", nil, `{
			"metadata": {"name": "broken"},
			"spec": {"vm": {"uuid": "`+vmUUID+`"}, "compose": "services:\n  web:\n    image: nginx\n    healthcheck:\n      test: [CMD, true]\n"}
		}`)
		require.Equal(t, http.StatusBadRequest, status, string(body))
		assert.Contains(t, string(body), `"compose.services.web.healthcheck"`)
	})
}

// TestAStoppedStacksContainerWhoseMakingFails stops a stack, and deletes its
// web server while no registry has its image and its VM holds none: the
// stack's graph makes it again, expected stopped as its stack is, and its
// create fails, docker never having it, which fails the stack's plan; the
// stack's detail lists it all the same, as its record says it is the
// stack's. Once the image can be pulled, the run after makes it by its
// create, still expected stopped, what docker starts as it makes it stopped,
// and nothing asks it to run: the stack is stopped again, and not degraded.
func TestAStoppedStacksContainerWhoseMakingFails(t *testing.T) {
	t.Parallel()

	w := start(t)
	ctx := t.Context()

	deployed, err := createStack.NewUseCase(w.client, w.validator, w.translator, w.owners).Execute(ctx, &createStack.Request{
		Name:      "store",
		Compose:   storeCompose,
		OwnerUUID: ownerUUID,
	})
	require.NoError(t, err)
	require.Empty(t, deployed.ValidationErrors)
	require.NotNil(t, deployed.Stack)

	stackUUID, slug, vmUUID := deployed.Stack.UUID, deployed.Stack.Slug, deployed.VM.UUID
	web := slug + "_web"

	w.stack(t, stackUUID, "running", func(s presenter.StackDetail) bool { return s.State == "running" })

	stopped, err := stopStack.NewUseCase(w.client, w.translator).Execute(ctx, &stopStack.Request{UUID: stackUUID, OwnerUUID: ownerUUID})
	require.NoError(t, err)
	require.Empty(t, stopped.ValidationErrors)

	w.stack(t, stackUUID, "stopped", func(s presenter.StackDetail) bool { return s.State == "stopped" })

	before := w.containerRecord(t, vmUUID, web)
	did := len(w.dockerd.Did(vmUUID))

	// its registry is down, and its VM holds no copy of its image.
	w.dockerd.Refuse("nginx:alpine")
	w.dockerd.Forget(t, vmUUID, "nginx:alpine")

	deleted, err := deleteContainer.NewUseCase(w.client, w.translator).Execute(ctx, &deleteContainer.Request{VMUUID: vmUUID, ID: web, Force: true, OwnerUUID: ownerUUID})
	require.NoError(t, err)
	require.Empty(t, deleted.ValidationErrors)

	failed := w.containerKept(t, vmUUID, web, "made again, and failed", func(c containerKind.Container) bool {
		return c.Metadata.UUID != before.Metadata.UUID && c.Status.Status.State == containerKind.Failed
	})

	assert.Nil(t, failed.Status.Docker, "docker never had it")
	assert.Equal(t, containerKind.Stopped, failed.Status.Expected, "admitted expected stopped, as its stack is")

	// being made again, which its vm's dockerd has said nothing of, it is
	// listed by the stack's detail all the same, as its record says it is
	// the stack's. What a heartbeat taken before the one deleted went says
	// of it may be shown beside it for a while, as nobody's.
	detail, err := w.readStack(ctx, stackUUID)
	require.NoError(t, err)

	remade := slices.IndexFunc(detail.Containers, func(c presenter.Container) bool { return c.Name == web && !c.Unmanaged })
	require.GreaterOrEqual(t, remade, 0, "the stack's detail lists %s, being made again: %+v", web, detail.Containers)
	assert.Empty(t, detail.Containers[remade].ID, "which docker never had")
	assert.Equal(t, "web", detail.Containers[remade].Service)
	assert.Equal(t, stackUUID, detail.Containers[remade].StackUUID)
	assert.Contains(t, []string{"created", "dead"}, detail.Containers[remade].State, "being made, or its making failed, in docker's words")
	assert.True(t, slices.ContainsFunc(detail.Containers, func(c presenter.Container) bool { return c.Service == "db" && !c.Unmanaged }), "beside db")

	w.dockerd.Allow("nginx:alpine")

	again := w.containerKept(t, vmUUID, web, "made once its image can be pulled", func(c containerKind.Container) bool {
		return c.Metadata.UUID == failed.Metadata.UUID && c.Status.Docker != nil
	})

	assert.Equal(t, containerKind.Stopped, again.Status.Expected, "expected stopped still, by the run after the one that failed")

	// failed while web could not be made, it is stopped once web is: it is
	// waited on as it is, failed on the way.
	s := eventually(t, "the stack stopped with it", func(ctx context.Context) (presenter.StackDetail, error) {
		return w.readStack(ctx, stackUUID)
	}, func(s presenter.StackDetail) bool {
		return s.State == "stopped" && w.stackRecord(t, stackUUID).Status.Services[1].Containers[0].ID == again.Metadata.UUID
	})
	assert.Equal(t, "stopped", s.State, "and not degraded")

	c, there := w.dockerd.Holds(t, vmUUID).Container(web)
	require.True(t, there)
	assert.NotEqual(t, "running", c.State, "it is left stopped")

	made := w.dockerd.Did(vmUUID)[did:]
	require.Contains(t, made, "create "+web)
	assert.Equal(t, "stop "+web, made[len(made)-1], "made by its create, what docker starts as it makes it stopped once it is made, and nothing starts it after")
}

// TestAStackWhoseFileCannotBeRead deletes a stack whose file the platform
// does not read as it is kept, as one kept before the platform read files
// as it does now would be: its graph cannot be planned, and it is removed
// all the same, through what its records say it owns, its containers and its
// network, and its record goes, and its volume and its VM, which it only
// uses, stay.
func TestAStackWhoseFileCannotBeRead(t *testing.T) {
	t.Parallel()

	w := start(t)
	ctx := t.Context()

	deployed, err := createStack.NewUseCase(w.client, w.validator, w.translator, w.owners).Execute(ctx, &createStack.Request{
		Name:      "store",
		Compose:   storeCompose,
		OwnerUUID: ownerUUID,
	})
	require.NoError(t, err)
	require.Empty(t, deployed.ValidationErrors)
	require.NotNil(t, deployed.Stack)

	stackUUID, slug, vmUUID := deployed.Stack.UUID, deployed.Stack.Slug, deployed.VM.UUID
	named := func(name string) string { return slug + "_" + name }

	w.stack(t, stackUUID, "running", func(s presenter.StackDetail) bool { return s.State == "running" })

	// its file, written straight to what keeps it, says what the platform
	// does not take.
	require.Eventually(t, func() bool {
		r, err := w.resources.GetOne(ctx, stackKind.Name, stackUUID)
		require.NoError(t, err)

		s, err := kind.Decode[stackKind.Spec, stackKind.Status](r.Raw)
		require.NoError(t, err)

		s.Spec.Compose = "services:\n  web:\n    image: nginx:alpine\n    build: .\n"

		raw, err := kind.Encode(s)
		require.NoError(t, err)

		r.Raw = raw

		_, err = w.resources.Update(ctx, r)

		return err == nil
	}, settle, beat, "its file was never written")

	deleted, err := deleteStack.NewUseCase(w.client, w.translator).Execute(ctx, &deleteStack.Request{UUID: stackUUID, OwnerUUID: ownerUUID})
	require.NoError(t, err)
	require.Empty(t, deleted.ValidationErrors)

	require.Eventually(t, func() bool {
		_, err := w.resources.GetOne(ctx, stackKind.Name, stackUUID)

		return errors.Is(err, domain.ErrNotExists)
	}, settle, beat, "the stack was never removed")

	assert.Empty(t, w.containerRecords(t, vmUUID), "its containers went with it")

	_, kept := w.findBlock(t, networkKind.Name, vmUUID, named("backend"))
	assert.False(t, kept, "and so did its network")

	held := w.dockerd.Holds(t, vmUUID)

	for _, service := range []string{"db", "web"} {
		_, there := held.Container(named(service))
		assert.False(t, there, "%s went with it", service)
	}

	_, networked := held.Network(named("backend"))
	assert.False(t, networked)

	assert.True(t, held.Volume(named("data")), "its volume stays")

	_, kept = w.findBlock(t, volumeKind.Name, vmUUID, named("data"))
	assert.True(t, kept)

	w.stored(t, vmUUID, "kept", func(v vmKind.VM) bool { return v.Status.State == vmKind.Running })
}

// stackRecord is the record of the stack uuid names, as the control plane
// keeps it.
func (w *workload) stackRecord(t *testing.T, uuid string) stackKind.Stack {
	t.Helper()

	r, err := w.resources.GetOne(t.Context(), stackKind.Name, uuid)
	require.NoError(t, err)

	s, err := kind.Decode[stackKind.Spec, stackKind.Status](r.Raw)
	require.NoError(t, err)

	return s
}

// TestAStackItsVMsSnapshotHolds restores a stack's Docker VM to a snapshot
// taken while the stack ran, after one of its containers was deleted, and
// made again by the stack's graph under a new record: the restore forgets
// that record, which the restored disk does not hold, and the container the
// disk holds, which no record speaks for, is taken in again as the stack's,
// as its graph would have made it. The stack runs with it, which its record
// names as its service's, and its delete removes it, as it removes any
// container it made, and keeps its volume and its VM.
func TestAStackItsVMsSnapshotHolds(t *testing.T) {
	t.Parallel()

	w := start(t)
	ctx := t.Context()

	vmUUID := w.dockerVM(t, "builds")

	deployed, err := createStack.NewUseCase(w.client, w.validator, w.translator, w.owners).Execute(ctx, &createStack.Request{
		Name:      "store",
		Compose:   storeCompose,
		VMUUID:    vmUUID,
		OwnerUUID: ownerUUID,
	})
	require.NoError(t, err)
	require.Empty(t, deployed.ValidationErrors)
	require.NotNil(t, deployed.Stack)

	stackUUID, slug := deployed.Stack.UUID, deployed.Stack.Slug
	named := func(name string) string { return slug + "_" + name }

	w.stack(t, stackUUID, "running", func(s presenter.StackDetail) bool { return s.State == "running" })

	snapshotted := w.containerRecord(t, vmUUID, named("web"))
	snapshotUUID := w.snapshot(t, vmUUID, "with the stack")

	deleted, err := deleteContainer.NewUseCase(w.client, w.translator).Execute(ctx, &deleteContainer.Request{VMUUID: vmUUID, ID: named("web"), Force: true, OwnerUUID: ownerUUID})
	require.NoError(t, err)
	require.Empty(t, deleted.ValidationErrors)

	since := w.containerKept(t, vmUUID, named("web"), "made again", func(c containerKind.Container) bool {
		return c.Metadata.UUID != snapshotted.Metadata.UUID && c.Status.Status.State == containerKind.Running
	})

	w.stack(t, stackUUID, "running with what was made again", func(s presenter.StackDetail) bool {
		return s.State == "running" && w.stackRecord(t, stackUUID).Status.Services[1].Containers[0].ID == since.Metadata.UUID
	})

	w.restore(t, vmUUID, snapshotUUID)

	t.Run("the container the restored disk holds is taken in again as the stack's, as its graph would have made it", func(t *testing.T) {
		adopted := eventually(t, "the container the restored disk holds taken in", func(ctx context.Context) (containerKind.Container, error) {
			r, err := w.resources.GetOne(ctx, containerKind.Name, snapshotted.Metadata.UUID)
			if err != nil {
				return containerKind.Container{}, err
			}

			return kind.Decode[containerKind.Spec, containerKind.Status](r.Raw)
		}, func(containerKind.Container) bool { return true })

		assert.Equal(t, []kind.Reference{{Kind: stackKind.Name, UUID: stackUUID}}, adopted.Metadata.Owners, "its stack's, from the moment it is taken in")
		assert.Equal(t, map[string]string{stackKind.LabelStack: stackUUID, stackKind.LabelService: "web"}, adopted.Metadata.Labels, "labelled as the stack's, and as its service")
		assert.Equal(t, named("web"), adopted.Metadata.Name)
		assert.Equal(t, named("web"), adopted.Spec.Name)
	})

	t.Run("and what was made since the snapshot is forgotten", func(t *testing.T) {
		require.Eventually(t, func() bool {
			_, err := w.resources.GetOne(ctx, containerKind.Name, since.Metadata.UUID)

			return errors.Is(err, domain.ErrNotExists)
		}, settle, beat, "what the restored disk does not hold was never forgotten")

		c, there := w.dockerd.Holds(t, vmUUID).Container(named("web"))
		require.True(t, there)
		assert.Equal(t, snapshotted.Metadata.UUID, c.Labels[blockKinds.Label(containerKind.Name)], "one web, the one the snapshot holds")
	})

	t.Run("the stack runs with it, which its record names as its service's", func(t *testing.T) {
		w.stack(t, stackUUID, "running with the container taken in", func(s presenter.StackDetail) bool {
			return s.State == "running" && w.stackRecord(t, stackUUID).Status.Services[1].Containers[0].ID == snapshotted.Metadata.UUID
		})

		web := w.stackRecord(t, stackUUID).Status.Services[1]
		assert.Equal(t, stackKind.ServiceStatus{Name: "web", State: stackKind.ServiceRunning, Containers: []stackKind.ContainerStatus{{ID: snapshotted.Metadata.UUID, Name: named("web"), State: "running"}}}, web)
	})

	t.Run("deleted, the stack removes it as any container it made, and keeps its volume and its vm", func(t *testing.T) {
		removed, err := deleteStack.NewUseCase(w.client, w.translator).Execute(ctx, &deleteStack.Request{UUID: stackUUID, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, removed.ValidationErrors)

		require.Eventually(t, func() bool {
			_, err := w.resources.GetOne(ctx, stackKind.Name, stackUUID)

			return errors.Is(err, domain.ErrNotExists)
		}, settle, beat, "the stack was never removed")

		assert.Empty(t, w.containerRecords(t, vmUUID))

		held := w.dockerd.Holds(t, vmUUID)

		for _, service := range []string{"db", "web"} {
			_, there := held.Container(named(service))
			assert.False(t, there, "%s went with it", service)
		}

		_, networked := held.Network(named("backend"))
		assert.False(t, networked, "and so did its network")

		assert.True(t, held.Volume(named("data")), "its volume stays")

		_, kept := w.findBlock(t, volumeKind.Name, vmUUID, named("data"))
		assert.True(t, kept)

		w.stored(t, vmUUID, "kept", func(v vmKind.VM) bool { return v.Status.State == vmKind.Running })
	})
}
