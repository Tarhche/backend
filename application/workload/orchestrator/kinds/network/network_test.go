package network_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/blocks"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/blocks/blockstest"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/network"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
)

func aNetwork(uuid string, name string) networkKind.Network {
	return networkKind.Network{
		Kind:     networkKind.Name,
		Metadata: kind.Metadata{UUID: uuid, Owners: []kind.Reference{{Kind: "vm", UUID: "vm-1"}}, Node: "node-1"},
		Spec:     networkKind.Spec{Name: name, Internal: true},
		Status:   networkKind.Status{Status: kind.Status{State: networkKind.Creating, Expected: networkKind.Present}},
	}
}

func newNode(t *testing.T) (*network.Node, *blockstest.Dockerd) {
	t.Helper()

	node := blockstest.NewNode()
	dockerd := node.DockerVM(t, "vm-1")

	return network.New(blocks.NewReader(node.Engine, node), time.Minute), dockerd
}

func TestNode_Execute(t *testing.T) {
	t.Parallel()

	t.Run("made, it is labelled as the resource it is, once however often it is asked", func(t *testing.T) {
		t.Parallel()

		strategy, dockerd := newNode(t)

		for range 2 {
			outcome, err := strategy.Execute(t.Context(), aNetwork("n-uuid", "backend"), networkKind.ActionCreate, nil)
			require.NoError(t, err)

			assert.Equal(t, networkKind.Present, outcome.Status.State)
			assert.Equal(t, "backend", outcome.Status.Docker.Name)
			assert.True(t, outcome.Status.Docker.Internal)
			assert.Equal(t, "n-uuid", outcome.Status.Docker.Labels["workload.network"])
		}

		assert.Equal(t, 1, dockerd.Calls("CreateNetwork"))
	})

	t.Run("one of a name taken is refused, in docker's words", func(t *testing.T) {
		t.Parallel()

		strategy, dockerd := newNode(t)
		dockerd.HoldNetwork(docker.Network{ID: "n1", Name: "backend"})

		_, err := strategy.Execute(t.Context(), aNetwork("n-uuid", "backend"), networkKind.ActionCreate, nil)
		assert.ErrorIs(t, err, docker.ErrInvalid)
	})

	t.Run("removed, it is gone, and one gone already is gone", func(t *testing.T) {
		t.Parallel()

		strategy, dockerd := newNode(t)

		_, err := strategy.Execute(t.Context(), aNetwork("n-uuid", "backend"), networkKind.ActionCreate, nil)
		require.NoError(t, err)

		for range 2 {
			_, err := strategy.Execute(t.Context(), aNetwork("n-uuid", "backend"), networkKind.ActionDelete, nil)
			require.NoError(t, err)
		}

		assert.Equal(t, 1, dockerd.Calls("RemoveNetwork"))
	})

	t.Run("one with a container on it is refused as asked, and left as it was", func(t *testing.T) {
		t.Parallel()

		strategy, dockerd := newNode(t)

		_, err := strategy.Execute(t.Context(), aNetwork("n-uuid", "backend"), networkKind.ActionCreate, nil)
		require.NoError(t, err)

		dockerd.Hold(docker.Container{ID: "c1", Name: "api", State: "running", Networks: []string{"backend"}})

		outcome, err := strategy.Execute(t.Context(), aNetwork("n-uuid", "backend"), networkKind.ActionDelete, nil)
		assert.ErrorIs(t, err, kind.ErrRefused)
		assert.ErrorIs(t, err, docker.ErrInvalid)

		assert.Equal(t, networkKind.Present, outcome.Status.State)
		assert.Equal(t, "backend", outcome.Status.Docker.Name)
		require.NotNil(t, outcome.Status.Failure)
		assert.Contains(t, outcome.Status.Failure.Message, "active endpoints", "in docker's words")
	})

	t.Run("one nobody keeps a record of is found by its docker id", func(t *testing.T) {
		t.Parallel()

		strategy, dockerd := newNode(t)
		dockerd.HoldNetwork(docker.Network{ID: "n1", Name: "made-by-hand"})

		unrecorded := aNetwork("derived-uuid", "")
		unrecorded.Status.Docker = &networkKind.Docker{ID: "n1"}

		_, err := strategy.Execute(t.Context(), unrecorded, networkKind.ActionDelete, nil)
		require.NoError(t, err)

		assert.Equal(t, 1, dockerd.Calls("RemoveNetwork"))
	})
}

func TestNode_State(t *testing.T) {
	t.Parallel()

	strategy, dockerd := newNode(t)

	_, err := strategy.Execute(t.Context(), aNetwork("n-uuid", "backend"), networkKind.ActionCreate, nil)
	require.NoError(t, err)

	dockerd.HoldNetwork(docker.Network{ID: "s1", Name: "shop_default", Labels: map[string]string{"workload.managed": "true", "workload.network": "shop-network-uuid"}})
	dockerd.Hold(docker.Container{ID: "c1", Name: "shop_web", State: "running", Networks: []string{"shop_default"}, Labels: map[string]string{"workload.stack": "stack-uuid"}})
	dockerd.HoldNetwork(docker.Network{ID: "h1", Name: "made-by-hand"})

	report, err := strategy.State(t.Context())
	require.NoError(t, err)

	byName := map[string]kind.Observed[networkKind.Status]{}
	for _, observed := range report.Instances {
		byName[observed.Status.Docker.Name] = observed
	}

	assert.Equal(t, "n-uuid", byName["backend"].UUID)
	assert.Equal(t, "shop-network-uuid", byName["shop_default"].UUID, "a stack's is the resource it is, as anything the platform made")
	assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-1"}}, byName["shop_default"].Owners, "living in its vm, whatever owns its record")
	assert.Equal(t, []string{"shop_web"}, byName["shop_default"].Status.Docker.Containers)
	assert.Empty(t, byName["made-by-hand"].UUID, "one made from its vm's terminal is nobody's")
	assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-1"}}, byName["made-by-hand"].Owners)
	assert.Empty(t, byName["bridge"].UUID, "and so are the networks every dockerd has")
}
