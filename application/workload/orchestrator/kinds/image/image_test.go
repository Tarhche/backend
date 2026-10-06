package image_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/blocks"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/blocks/blockstest"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/image"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	blockKinds "github.com/khanzadimahdi/testproject/domain/workload/kinds/blocks"
	imageKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/image"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

func anImage(reference string) imageKind.Image {
	return imageKind.Image{
		Kind:     imageKind.Name,
		Metadata: kind.Metadata{UUID: imageKind.UUIDOf("vm-1", reference), Owners: []kind.Reference{{Kind: "vm", UUID: "vm-1"}}, Node: "node-1"},
		Spec:     imageKind.Spec{Reference: reference},
		Status:   imageKind.Status{Status: kind.Status{State: imageKind.Pulling, Expected: imageKind.Present}},
	}
}

func newNode(t *testing.T) (*image.Node, *blockstest.Dockerd) {
	t.Helper()

	node := blockstest.NewNode()
	dockerd := node.DockerVM(t, "vm-1")

	return image.New(blocks.NewReader(node.Engine, node, 0), time.Minute), dockerd
}

func TestNode_Execute(t *testing.T) {
	t.Parallel()

	t.Run("pulled, it is present, under the reference it was pulled as", func(t *testing.T) {
		t.Parallel()

		strategy, dockerd := newNode(t)

		outcome, err := strategy.Execute(t.Context(), anImage("nginx:1.27"), imageKind.ActionCreate, nil)
		require.NoError(t, err)

		assert.Equal(t, imageKind.Present, outcome.Status.State)
		assert.Equal(t, "nginx:1.27", outcome.Status.Docker.Reference)
		assert.Equal(t, &noderequest.Error{}, outcome.Status.Failure)
		assert.Equal(t, 1, dockerd.Calls("PullImage"))

		_, err = strategy.Execute(t.Context(), anImage("nginx:1.27"), imageKind.ActionPull, nil)
		require.NoError(t, err)
		assert.Equal(t, 2, dockerd.Calls("PullImage"), "and pulled again when asked")
	})

	t.Run("one no registry has fails, in docker's words", func(t *testing.T) {
		t.Parallel()

		strategy, dockerd := newNode(t)
		dockerd.Registry = map[string]bool{}

		outcome, err := strategy.Execute(t.Context(), anImage("nope:latest"), imageKind.ActionCreate, nil)
		require.ErrorIs(t, err, docker.ErrInvalid)

		assert.Equal(t, imageKind.Failed, outcome.Status.State)
		assert.Equal(t, noderequest.CodeInvalid, outcome.Status.Failure.Code)
	})

	t.Run("removed, it is gone, and one gone already is gone", func(t *testing.T) {
		t.Parallel()

		strategy, dockerd := newNode(t)
		dockerd.HoldImage(docker.Image{ID: "sha256:nginx", Tags: []string{"nginx:1.27"}})
		dockerd.Hold(docker.Container{ID: "c1", Name: "web", Image: "nginx:1.27", State: "running"})

		_, err := strategy.Execute(t.Context(), anImage("nginx:1.27"), imageKind.ActionDelete, imageKind.DeletePayload{})
		require.NoError(t, err, "whether one a container uses may be removed was asked before it was sent here")

		_, err = strategy.Execute(t.Context(), anImage("nginx:1.27"), imageKind.ActionDelete, imageKind.DeletePayload{})
		assert.NoError(t, err)
	})
}

func TestNode_State(t *testing.T) {
	t.Parallel()

	strategy, dockerd := newNode(t)
	dockerd.HoldImage(docker.Image{ID: "sha256:nginx", Tags: []string{"nginx:1.27", "nginx:latest"}})
	dockerd.HoldImage(docker.Image{ID: "sha256:dangling"})

	report, err := strategy.State(t.Context())
	require.NoError(t, err)

	assert.Equal(t, []string{"vm-1"}, report.Read)

	uuids := map[string]string{}
	for _, observed := range report.Instances {
		uuids[observed.Status.Docker.Reference+"|"+observed.Status.Docker.ID] = observed.UUID
		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-1"}}, observed.Owners)
		assert.Equal(t, imageKind.Present, observed.Status.State)
	}

	assert.Equal(t, map[string]string{
		"nginx:1.27|sha256:nginx":   imageKind.UUIDOf("vm-1", "nginx:1.27"),
		"nginx:latest|sha256:nginx": imageKind.UUIDOf("vm-1", "nginx"),
		"|sha256:dangling":          blockKinds.Derived(imageKind.Name, "vm-1", "sha256:dangling"),
	}, uuids, "under the uuid a record of each reference has, and one nothing names under its id")
}
