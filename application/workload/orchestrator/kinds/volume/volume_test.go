package volume_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/blocks"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/blocks/blockstest"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
)

func aVolume(uuid string, name string) volumeKind.Volume {
	return volumeKind.Volume{
		Kind:     volumeKind.Name,
		Metadata: kind.Metadata{UUID: uuid, Owners: []kind.Reference{{Kind: "vm", UUID: "vm-1"}}, Node: "node-1"},
		Spec:     volumeKind.Spec{Name: name},
		Status:   volumeKind.Status{Status: kind.Status{State: volumeKind.Creating, Expected: volumeKind.Present}},
	}
}

func newNode(t *testing.T) (*volume.Node, *blockstest.Dockerd) {
	t.Helper()

	node := blockstest.NewNode()
	dockerd := node.DockerVM(t, "vm-1")

	return volume.New(blocks.NewReader(node.Engine, node, 0), time.Minute), dockerd
}

func TestNode_Execute(t *testing.T) {
	t.Parallel()

	t.Run("made, it is labelled as the resource it is, and has nothing to say", func(t *testing.T) {
		t.Parallel()

		strategy, dockerd := newNode(t)

		outcome, err := strategy.Execute(t.Context(), aVolume("v-uuid", "data"), volumeKind.ActionCreate, nil)
		require.NoError(t, err)

		assert.Equal(t, volumeKind.Present, outcome.Status.State)
		assert.Empty(t, outcome.Status.Reason)
		assert.Equal(t, "v-uuid", outcome.Status.Docker.Labels["workload.volume"])

		_, err = strategy.Execute(t.Context(), aVolume("v-uuid", "data"), volumeKind.ActionCreate, nil)
		require.NoError(t, err)
		assert.Equal(t, 1, dockerd.Calls("CreateVolume"), "made once however often it is asked")
	})

	t.Run("made again after it went missing, it says what was in it is gone", func(t *testing.T) {
		t.Parallel()

		strategy, _ := newNode(t)

		again := aVolume("v-uuid", "data")
		again.Status.Docker = &volumeKind.Docker{Name: "data"}

		outcome, err := strategy.Execute(t.Context(), again, volumeKind.ActionCreate, nil)
		require.NoError(t, err)

		assert.Contains(t, outcome.Status.Reason, "what was in it is gone")
		assert.NotEmpty(t, outcome.Status.Docker.Labels[volumeKind.LabelRecreated])

		report, err := strategy.State(t.Context())
		require.NoError(t, err)
		require.Len(t, report.Instances, 1)
		assert.Contains(t, report.Instances[0].Status.Reason, "what was in it is gone", "and says so for as long as it is there")
	})

	t.Run("one whose name another volume has is refused, rather than taken for it", func(t *testing.T) {
		t.Parallel()

		strategy, dockerd := newNode(t)
		dockerd.HoldVolume(docker.Volume{Name: "data"})

		_, err := strategy.Execute(t.Context(), aVolume("v-uuid", "data"), volumeKind.ActionCreate, nil)
		assert.ErrorIs(t, err, docker.ErrInvalid)
	})

	t.Run("removed, it is gone, and one gone already is gone", func(t *testing.T) {
		t.Parallel()

		strategy, dockerd := newNode(t)

		_, err := strategy.Execute(t.Context(), aVolume("v-uuid", "data"), volumeKind.ActionCreate, nil)
		require.NoError(t, err)

		for range 2 {
			_, err := strategy.Execute(t.Context(), aVolume("v-uuid", "data"), volumeKind.ActionDelete, volumeKind.DeletePayload{})
			require.NoError(t, err)
		}

		assert.Equal(t, 1, dockerd.Calls("RemoveVolume"))
	})
}

func TestNode_State(t *testing.T) {
	t.Parallel()

	strategy, dockerd := newNode(t)

	_, err := strategy.Execute(t.Context(), aVolume("v-uuid", "data"), volumeKind.ActionCreate, nil)
	require.NoError(t, err)

	dockerd.HoldVolume(docker.Volume{Name: "scratch"})
	dockerd.Hold(docker.Container{ID: "c1", Name: "web", State: "running", Mounts: []docker.Mount{{Type: "volume", Source: "data", Target: "/data"}}})

	report, err := strategy.State(t.Context())
	require.NoError(t, err)

	byName := map[string]kind.Observed[volumeKind.Status]{}
	for _, observed := range report.Instances {
		byName[observed.Status.Docker.Name] = observed
	}

	assert.Equal(t, "v-uuid", byName["data"].UUID)
	assert.True(t, byName["data"].Status.Docker.InUse, "a container mounts it")
	assert.Empty(t, byName["scratch"].UUID, "one made from the vm's terminal is nobody's")
	assert.False(t, byName["scratch"].Status.Docker.InUse)
}
