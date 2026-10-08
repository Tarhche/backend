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
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
)

// shopCompose is a stack of two services, a web server and a cache.
const shopCompose = "services:\n  web:\n    image: nginx:alpine\n  cache:\n    image: redis:alpine\n"

// TestAStack walks a stack through its life from the dashboard, as a kind
// the control plane runs and a node carries out: deployed into a Docker VM
// made for it, brought back when one of its containers falls over, stopped,
// started and restarted, waiting while its VM is stopped, and deleted with
// its volumes.
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

	assert.True(t, deployed.VM.Created, "its owner had no Docker VM, so one was made for it")
	assert.Equal(t, "waiting", deployed.Stack.State, "for the vm made for it to come up")
	assert.Equal(t, "running", deployed.Stack.ExpectedState)

	t.Run("its vm comes up, and it is deployed into it and runs", func(t *testing.T) {
		running := w.stackIn(t, stackUUID, "running", "running")

		assert.Equal(t, vmUUID, running.VMUUID)
		assert.Contains(t, running.Output, "Started")
		assert.Empty(t, running.Note)
		require.Len(t, running.Containers, 2)

		for _, c := range running.Containers {
			assert.Equal(t, slug, c.Stack)
			assert.Equal(t, stackUUID, c.StackUUID)
			assert.Equal(t, stackUUID, c.Labels[stackKind.LabelStack], "compose labelled it as the stack's")
		}

		assert.Equal(t, []string{"up -d --remove-orphans"}, w.dockerd.Ran(slug))
	})

	t.Run("the images its containers were made from are kept by it", func(t *testing.T) {
		w.images(t, vmUUID, "the stack's images kept", func(images map[string]presenter.Image) bool {
			web, pulled := images["nginx:alpine"]
			cache, alsoPulled := images["redis:alpine"]

			return pulled && alsoPulled && !web.Unmanaged && !cache.Unmanaged
		})
	})

	t.Run("a container that falls over leaves it degraded, and it is brought back", func(t *testing.T) {
		w.dockerd.Kill(slug, "cache")

		degraded := w.stack(t, stackUUID, "degraded", func(s presenter.StackDetail) bool { return s.State == "degraded" })
		assert.Equal(t, "running", degraded.ExpectedState)

		w.stackIn(t, stackUUID, "running", "running")

		assert.Equal(t, []string{"up -d --remove-orphans", "up -d --remove-orphans"}, w.dockerd.Ran(slug), "applied again")
	})

	t.Run("stopped", func(t *testing.T) {
		stopped, err := stopStack.NewUseCase(w.client, w.translator).Execute(ctx, &stopStack.Request{UUID: stackUUID, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, stopped.ValidationErrors)

		s := w.stackIn(t, stackUUID, "stopped", "exited")
		assert.Equal(t, "stopped", s.ExpectedState)
		assert.Contains(t, s.Output, "Stopped")
	})

	t.Run("started", func(t *testing.T) {
		started, err := startStack.NewUseCase(w.client, w.translator).Execute(ctx, &startStack.Request{UUID: stackUUID, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, started.ValidationErrors)

		s := w.stackIn(t, stackUUID, "running", "running")
		assert.Equal(t, "running", s.ExpectedState)
	})

	t.Run("restarted, which says so", func(t *testing.T) {
		restarted, err := restartStack.NewUseCase(w.client, w.translator).Execute(ctx, &restartStack.Request{UUID: stackUUID, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, restarted.ValidationErrors)

		s := w.stack(t, stackUUID, "restarted", func(s presenter.StackDetail) bool {
			return s.State == "running" && slices.Contains(w.dockerd.Ran(slug), "restart")
		})

		eventually(t, "what the restart said", func(ctx context.Context) (presenter.StackDetail, error) {
			return w.readStack(ctx, stackUUID)
		}, func(s presenter.StackDetail) bool { return strings.Contains(s.Output, "Restarted") })

		assert.Equal(t, "running", s.ExpectedState)
	})

	t.Run("its vm stopped, it waits on the vm, and runs again once the vm does", func(t *testing.T) {
		stopped, err := stopVM.NewUseCase(w.client, w.translator).Execute(ctx, &stopVM.Request{UUID: vmUUID, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, stopped.ValidationErrors)

		waiting := w.stack(t, stackUUID, "waiting on its vm", func(s presenter.StackDetail) bool {
			return s.State == "waiting" && s.Reason == "its vm is stopped"
		})
		assert.Equal(t, "running", waiting.ExpectedState, "it is still to be running, once it can be")
		assert.Equal(t, presenter.NoteVMNotRunning, waiting.Note)

		refused, err := stopStack.NewUseCase(w.client, w.translator).Execute(ctx, &stopStack.Request{UUID: stackUUID, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		assert.Contains(t, refused.ValidationErrors, "vm", "only a running vm's dockerd can stop it")

		started, err := startVM.NewUseCase(w.client, w.translator).Execute(ctx, &startVM.Request{UUID: vmUUID, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, started.ValidationErrors)

		w.stackIn(t, stackUUID, "running", "running")
	})

	t.Run("deleted with its volumes, it is gone", func(t *testing.T) {
		deleted, err := deleteStack.NewUseCase(w.client, w.translator).Execute(ctx, &deleteStack.Request{UUID: stackUUID, RemoveVolumes: true, OwnerUUID: ownerUUID})
		require.NoError(t, err)
		require.Empty(t, deleted.ValidationErrors)

		require.Eventually(t, func() bool {
			_, err := w.readStack(ctx, stackUUID)

			return errors.Is(err, domain.ErrNotExists)
		}, settle, beat, "the stack was never removed")

		assert.Equal(t, "down --remove-orphans --volumes", w.dockerd.Ran(slug)[len(w.dockerd.Ran(slug))-1], "its volumes went with it")
		assert.Empty(t, w.dockerd.States(slug), "and its containers")

		_, err = w.resources.GetOne(ctx, stackKind.Name, stackUUID)
		assert.ErrorIs(t, err, domain.ErrNotExists, "record and all")

		w.images(t, vmUUID, "the images it left behind nobody's", func(images map[string]presenter.Image) bool {
			web, left := images["nginx:alpine"]
			cache, alsoLeft := images["redis:alpine"]

			return left && alsoLeft && web.Unmanaged && cache.Unmanaged
		})
	})
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
			t.Fatalf("the stack failed: %s\n%s", s.Reason, s.Output)
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
