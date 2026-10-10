package workload_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/image/getImages"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/createStack"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/deleteStack"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/getStack"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/restartStack"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/startStack"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/stopStack"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/startVM"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/stopVM"
	"github.com/khanzadimahdi/testproject/domain"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// shopCompose is a stack of two services, a web server and a cache.
const shopCompose = "services:\n  web:\n    image: nginx:alpine\n  cache:\n    image: redis:alpine\n"

// TestAStack walks a stack through its life from the dashboard, as a kind
// the control plane keeps and its graph makes, no compose anywhere: made in a
// Docker VM made for it, degraded when one of its containers falls over and
// stays down, as a container on its own does, stopped, started and
// restarted, waiting while its VM is stopped, and deleted, its containers and
// its network with it, and its images left to nobody.
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
		assert.Empty(t, running.Output, "nothing runs compose, which nothing hears from")
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
