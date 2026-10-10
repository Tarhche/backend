package image_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks/blockstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	blockKinds "github.com/khanzadimahdi/testproject/domain/workload/kinds/blocks"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	imageKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/image"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

func asked(reference string, vmUUID string) imageKind.Image {
	return imageKind.Image{
		Kind:     imageKind.Name,
		Metadata: kind.Metadata{OwnerUUID: "owner", Owners: []kind.Reference{{Kind: "vm", UUID: vmUUID}}},
		Spec:     imageKind.Spec{Reference: reference},
	}
}

func TestImages_Admit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("under the uuid its vm and its reference give it, which its node reports it by", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		admitted, invalid, err := w.Images.Admit(ctx, asked("nginx", "vm-1"))
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.Equal(t, imageKind.UUIDOf("vm-1", "nginx:latest"), admitted.Metadata.UUID)
		assert.Equal(t, "nginx:latest", admitted.Metadata.Name)
		assert.Equal(t, vmtest.Node, admitted.Metadata.Node)
		assert.Equal(t, imageKind.Pending, admitted.Status.State)
		assert.Equal(t, imageKind.Present, admitted.Status.Expected)
	})

	t.Run("one its vm keeps already is not kept twice", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(blockstest.A[imageKind.Spec, imageKind.Status](imageKind.Name, imageKind.UUIDOf("vm-1", "nginx"), "vm-1", imageKind.Spec{Reference: "nginx"}, imageKind.Status{Status: kind.Status{State: imageKind.Present}}))

		_, invalid, err := w.Images.Admit(ctx, asked("docker.io/library/nginx:latest", "vm-1"))
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"reference": "request_already_exists"}, invalid)
	})

	for name, tt := range map[string]struct {
		reference string
		vm        string
		want      domain.ValidationErrors
	}{
		"one with no reference":            {vm: "vm-1", want: domain.ValidationErrors{"reference": "invalid_image"}},
		"one with spaces in its reference": {reference: "nginx latest", vm: "vm-1", want: domain.ValidationErrors{"reference": "invalid_image"}},
		"one in no vm":                     {reference: "nginx", want: domain.ValidationErrors{"vm": "required_field"}},
		"one in a vm that is stopped":      {reference: "nginx", vm: "vm-stopped", want: domain.ValidationErrors{"vm": "not_running"}},
	} {
		t.Run("refused: "+name, func(t *testing.T) {
			t.Parallel()

			stopped := vmtest.In(vmtest.Docker("vm-stopped", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Stopped })
			w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner"), stopped))

			_, invalid, err := w.Images.Admit(ctx, asked(tt.reference, tt.vm))
			require.NoError(t, err)
			assert.Equal(t, tt.want, invalid)
		})
	}
}

func TestImages_Reconcile(t *testing.T) {
	t.Parallel()

	w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

	missing, err := kind.Decode[imageKind.Spec, imageKind.Status](blockstest.A[imageKind.Spec, imageKind.Status](imageKind.Name, "i", "vm-1", imageKind.Spec{Reference: "nginx"}, imageKind.Status{Status: kind.Status{State: imageKind.Missing, Expected: imageKind.Present}}))
	require.NoError(t, err)

	intents, err := w.Images.Reconcile(context.Background(), missing)
	require.NoError(t, err)
	assert.Equal(t, []kind.Intent{{Action: imageKind.ActionCreate, Reason: "it is not in its vm, which runs"}}, intents, "a missing image is pulled again")
}

func TestImages_Refuse(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
	w.Keep(blockstest.AContainer("c", "vm-1", containerKind.Running, containerKind.Running, func(c *containerKind.Container) {
		c.Spec.Image = "redis"
	}))

	image := func(reference string, inUse bool) kind.Raw {
		return blockstest.A[imageKind.Spec, imageKind.Status](imageKind.Name, "i", "vm-1", imageKind.Spec{Reference: reference}, imageKind.Status{
			Status: kind.Status{State: imageKind.Present},
			Docker: &imageKind.Docker{ID: "sha256:abc", Reference: imageKind.Normalized(reference), InUse: inUse},
		})
	}

	t.Run("one a container kept uses is not removed, by force or not", func(t *testing.T) {
		t.Parallel()

		err := w.Images.Refuse(ctx, image("docker.io/library/redis:latest", true), imageKind.ActionDelete, imageKind.DeletePayload{Force: true})

		var refused *noderequest.Error
		require.ErrorAs(t, err, &refused)
		assert.Equal(t, noderequest.CodeInvalid, refused.Code)
		assert.Contains(t, refused.Message, "used by the container web, which is kept")
	})

	t.Run("whether a container nobody keeps uses one is its dockerd's to say", func(t *testing.T) {
		t.Parallel()

		// what its node reported may be from before that container was
		// removed: docker refuses the delete unless it is forced, if it is
		// still used.
		assert.NoError(t, w.Images.Refuse(ctx, image("nginx:1.27", true), imageKind.ActionDelete, imageKind.DeletePayload{}))
		assert.NoError(t, w.Images.Refuse(ctx, image("nginx:1.27", true), imageKind.ActionDelete, imageKind.DeletePayload{Force: true}))
	})

	t.Run("and one nobody uses is removed", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, w.Images.Refuse(ctx, image("nginx:1.27", false), imageKind.ActionDelete, imageKind.DeletePayload{}))
	})

	t.Run("prepared as a record's delete, the same", func(t *testing.T) {
		t.Parallel()

		kept, err := kind.Decode[imageKind.Spec, imageKind.Status](image("redis", false))
		require.NoError(t, err)

		_, _, err = w.Images.Prepare(ctx, kept, imageKind.ActionDelete, imageKind.DeletePayload{})
		assert.ErrorAs(t, err, new(*noderequest.Error))

		_, _, err = w.Images.Prepare(ctx, kept, imageKind.ActionPull, nil)
		assert.NoError(t, err, "and pulled again whatever uses it")
	})
}

func TestImages_Witnessed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	at := time.Now()

	w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

	// kept in vm-1, asked for redis:7 and made from what the tag named before
	// it was pulled again, which docker lists by its id.
	w.Keep(blockstest.AContainer("kept", "vm-1", containerKind.Running, containerKind.Running, func(c *containerKind.Container) {
		c.Spec.Image = "redis:7"
		c.Status.Docker.Image = "sha256:redis-before"
	}))

	w.Keep(blockstest.A[imageKind.Spec, imageKind.Status](imageKind.Name, imageKind.UUIDOf("vm-1", "alpine:3"), "vm-1", imageKind.Spec{Reference: "alpine:3"}, imageKind.Status{Status: kind.Status{State: imageKind.Present}}))

	for _, container := range []kind.Observation{
		observed(t, "", containerKind.Status{Docker: &containerKind.Docker{ID: "c-shop", Name: "shop-web-1", Image: "nginx:alpine", State: "running", Labels: map[string]string{docker.LabelComposeProject: "shop"}}}, kind.Reference{Kind: "stack", UUID: "stack-uuid"}),
		observed(t, "", containerKind.Status{Docker: &containerKind.Docker{ID: "c-db", Name: "db", Image: "postgres:17", State: "running"}}),
	} {
		require.NoError(t, w.Containers.Witnessed(ctx, vmtest.Node, container, false, at))
	}

	for _, image := range []kind.Observation{
		held(t, "sha256:redis", "redis:7"),
		held(t, "sha256:redis-before", ""),
		held(t, "sha256:nginx", "nginx:alpine"),
		held(t, "sha256:postgres", "postgres:17"),
		held(t, "sha256:busybox", "busybox:latest"),
	} {
		require.NoError(t, w.Images.Witnessed(ctx, vmtest.Node, image, false, at))
	}

	require.NoError(t, w.Images.Witnessed(ctx, vmtest.Node, held(t, "sha256:alpine", "alpine:3"), true, at))

	extras, err := w.Images.All(ctx)
	require.NoError(t, err)

	keepers := make(map[string]string, len(extras))
	for _, r := range extras {
		keepers[blocks.ObservedOf(r.Status).Docker.ID] = r.Metadata.Labels[blockKinds.LabelManagedBy]
	}

	assert.Equal(t, map[string]string{
		"sha256:redis":        blockKinds.ManagedByContainer,
		"sha256:redis-before": blockKinds.ManagedByContainer,
		"sha256:nginx":        blockKinds.ManagedByStack,
		"sha256:postgres":     blockKinds.ManagedByNobody,
		"sha256:busybox":      blockKinds.ManagedByNobody,
	}, keepers, "a kept container's images are the container's, both the one it was asked for and the one it was made from; a stack's container's, the stack's; and one only a container nobody keeps uses, or nothing does, nobody's. One kept itself is a record, not an extra")
}

// observed is something as vm-1's node reports it, under uuid.
func observed(t *testing.T, uuid string, status any, owners ...kind.Reference) kind.Observation {
	t.Helper()

	encoded, err := json.Marshal(status)
	require.NoError(t, err)

	return kind.Observation{UUID: uuid, Owners: append([]kind.Reference{{Kind: "vm", UUID: "vm-1"}}, owners...), Status: encoded}
}

// held is an image vm-1's dockerd holds, as its node reports it: under the
// reference, or under its Docker id when nothing names it.
func held(t *testing.T, id string, reference string) kind.Observation {
	t.Helper()

	uuid := blockKinds.Derived(imageKind.Name, "vm-1", id)
	if len(reference) > 0 {
		uuid = imageKind.UUIDOf("vm-1", reference)
	}

	return observed(t, uuid, imageKind.Status{Status: kind.Status{State: imageKind.Present}, Docker: &imageKind.Docker{ID: id, Reference: reference}})
}

func TestImages_Named(t *testing.T) {
	t.Parallel()

	w := blockstest.New()

	kept := blockstest.A[imageKind.Spec, imageKind.Status](imageKind.Name, "i", "vm-1", imageKind.Spec{Reference: "nginx"}, imageKind.Status{})

	assert.True(t, w.Images.Named(kept, "nginx:latest"), "however it is written")
	assert.True(t, w.Images.Named(kept, "docker.io/library/nginx"))
	assert.False(t, w.Images.Named(kept, "nginx:1.27"))

	_, adopted := w.Images.Adopt(kind.Observation{UUID: "i"})
	assert.False(t, adopted, "nothing labels an image, so nothing is known to have been the platform's")
}
