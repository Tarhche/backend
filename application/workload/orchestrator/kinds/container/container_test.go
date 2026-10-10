package container_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/blocks"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/blocks/blockstest"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/container"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// aContainer is a container in vm-1 as the control plane recorded it, as
// changes say otherwise.
func aContainer(uuid string, changes ...func(c *containerKind.Container)) containerKind.Container {
	c := containerKind.Container{
		Kind: containerKind.Name,
		Metadata: kind.Metadata{
			UUID:      uuid,
			OwnerUUID: "owner-uuid",
			Owners:    []kind.Reference{{Kind: "vm", UUID: "vm-1"}},
			Node:      "node-1",
		},
		Spec: containerKind.Spec{
			Name:          "web",
			Image:         "nginx:1.27",
			Ports:         []containerKind.PortBinding{{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"}},
			RestartPolicy: containerKind.RestartAlways,
		},
		Status: containerKind.Status{Status: kind.Status{State: containerKind.Creating, Expected: containerKind.Running}},
	}

	for _, change := range changes {
		change(&c)
	}

	return c
}

func newNode(t *testing.T) (*container.Node, *blockstest.Dockerd, *blockstest.Node) {
	t.Helper()

	node := blockstest.NewNode()
	dockerd := node.DockerVM(t, "vm-1")

	return container.New(blocks.NewReader(node.Engine, node), time.Minute), dockerd, node
}

func TestNode_Execute(t *testing.T) {
	t.Parallel()

	t.Run("made, it is labelled as the resource it is, with its spec, and runs", func(t *testing.T) {
		t.Parallel()

		strategy, dockerd, _ := newNode(t)

		outcome, err := strategy.Execute(t.Context(), aContainer("c-uuid"), containerKind.ActionCreate, nil)
		require.NoError(t, err)

		assert.Equal(t, containerKind.Running, outcome.Status.Status.State)
		require.NotNil(t, outcome.Status.Docker)
		assert.Equal(t, "web", outcome.Status.Docker.Name)
		assert.Equal(t, containerKind.RestartAlways, outcome.Status.Docker.RestartPolicy)
		assert.Equal(t, &noderequest.Error{}, outcome.Status.Failure, "and nothing failed")

		held := dockerd.Held()
		require.Len(t, held, 1)
		assert.Equal(t, "true", held[0].Labels["workload.managed"])
		assert.Equal(t, "c-uuid", held[0].Labels["workload.container"])

		spec, labelled := containerKind.SpecFromLabels(held[0].Labels)
		require.True(t, labelled, "its spec is beside it")
		assert.Equal(t, "nginx:1.27", spec.Image)
		assert.Empty(t, spec.VM, "but for the vm it is in")
	})

	t.Run("made again, by a create asked again, it is the one there already", func(t *testing.T) {
		t.Parallel()

		strategy, dockerd, _ := newNode(t)

		_, err := strategy.Execute(t.Context(), aContainer("c-uuid"), containerKind.ActionCreate, nil)
		require.NoError(t, err)

		outcome, err := strategy.Execute(t.Context(), aContainer("c-uuid"), containerKind.ActionCreate, nil)
		require.NoError(t, err)

		assert.Len(t, dockerd.Held(), 1)
		assert.Equal(t, containerKind.Running, outcome.Status.Status.State)
	})

	t.Run("one that is to be stopped is stopped once it is made", func(t *testing.T) {
		t.Parallel()

		strategy, dockerd, _ := newNode(t)

		outcome, err := strategy.Execute(t.Context(), aContainer("c-uuid", func(c *containerKind.Container) {
			c.Status.Expected = containerKind.Stopped
		}), containerKind.ActionCreate, nil)
		require.NoError(t, err)

		assert.Equal(t, containerKind.Stopped, outcome.Status.Status.State)
		assert.Equal(t, "exited", dockerd.Held()[0].State)
	})

	t.Run("what docker refuses fails it, in docker's words and the code every side knows", func(t *testing.T) {
		t.Parallel()

		strategy, dockerd, _ := newNode(t)
		dockerd.Hold(docker.Container{ID: "other", Name: "other", State: "running", Ports: []docker.PortBinding{{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"}}})

		outcome, err := strategy.Execute(t.Context(), aContainer("c-uuid"), containerKind.ActionCreate, nil)
		require.ErrorIs(t, err, docker.ErrInvalid)

		assert.Equal(t, containerKind.Failed, outcome.Status.Status.State)
		require.NotNil(t, outcome.Status.Failure)
		assert.Equal(t, noderequest.CodeInvalid, outcome.Status.Failure.Code)
		assert.Contains(t, outcome.Status.Failure.Message, "port is already allocated")
	})

	t.Run("in a vm that is not running, it fails as not running", func(t *testing.T) {
		t.Parallel()

		strategy, _, node := newNode(t)
		node.Stop(t, "vm-1")

		outcome, err := strategy.Execute(t.Context(), aContainer("c-uuid"), containerKind.ActionCreate, nil)
		require.Error(t, err)

		assert.Equal(t, noderequest.CodeNotRunning, outcome.Status.Failure.Code)
	})

	t.Run("and in one whose dockerd does not come up, as unavailable", func(t *testing.T) {
		t.Parallel()

		strategy, dockerd, _ := newNode(t)
		dockerd.Down = true

		outcome, err := strategy.Execute(t.Context(), aContainer("c-uuid"), containerKind.ActionCreate, nil)
		require.ErrorIs(t, err, docker.ErrUnavailable)

		assert.Equal(t, noderequest.CodeDockerUnavailable, outcome.Status.Failure.Code)
	})

	t.Run("stopped, started and restarted by its docker id", func(t *testing.T) {
		t.Parallel()

		strategy, dockerd, _ := newNode(t)

		made, err := strategy.Execute(t.Context(), aContainer("c-uuid"), containerKind.ActionCreate, nil)
		require.NoError(t, err)

		recorded := aContainer("c-uuid", func(c *containerKind.Container) { c.Status.Docker = made.Status.Docker })

		stopped, err := strategy.Execute(t.Context(), recorded, containerKind.ActionStop, nil)
		require.NoError(t, err)
		assert.Equal(t, containerKind.Stopped, stopped.Status.Status.State, "one docker restarts by itself is stopped, not completed")

		started, err := strategy.Execute(t.Context(), recorded, containerKind.ActionStart, nil)
		require.NoError(t, err)
		assert.Equal(t, containerKind.Running, started.Status.Status.State)

		restarted, err := strategy.Execute(t.Context(), recorded, containerKind.ActionRestart, nil)
		require.NoError(t, err)
		assert.Equal(t, containerKind.Running, restarted.Status.Status.State)

		assert.Equal(t, 1, dockerd.Calls("StopContainer"))
		assert.Equal(t, 1, dockerd.Calls("StartContainer"))
		assert.Equal(t, 1, dockerd.Calls("RestartContainer"))
		assert.Len(t, dockerd.Held(), 1)
	})

	t.Run("a one-off stopped is completed, as its restart policy says", func(t *testing.T) {
		t.Parallel()

		strategy, _, _ := newNode(t)

		oneOff := aContainer("c-uuid", func(c *containerKind.Container) { c.Spec.RestartPolicy = containerKind.RestartNo })

		_, err := strategy.Execute(t.Context(), oneOff, containerKind.ActionCreate, nil)
		require.NoError(t, err)

		stopped, err := strategy.Execute(t.Context(), oneOff, containerKind.ActionStop, nil)
		require.NoError(t, err)
		assert.Equal(t, containerKind.Completed, stopped.Status.Status.State)
	})

	t.Run("started when its vm has none of it, it is made", func(t *testing.T) {
		t.Parallel()

		strategy, dockerd, _ := newNode(t)

		outcome, err := strategy.Execute(t.Context(), aContainer("c-uuid", func(c *containerKind.Container) {
			c.Status.Docker = &containerKind.Docker{ID: "gone"}
		}), containerKind.ActionStart, nil)
		require.NoError(t, err)

		assert.Equal(t, containerKind.Running, outcome.Status.Status.State)
		assert.Len(t, dockerd.Held(), 1)
	})

	t.Run("stopped when its vm has none of it, it is not running, which is what was asked", func(t *testing.T) {
		t.Parallel()

		strategy, dockerd, _ := newNode(t)

		outcome, err := strategy.Execute(t.Context(), aContainer("c-uuid"), containerKind.ActionStop, nil)
		require.NoError(t, err)

		assert.Equal(t, containerKind.Stopped, outcome.Status.Status.State)
		assert.Empty(t, dockerd.Held(), "and nothing is made for it")
	})

	t.Run("put on a network and taken off it", func(t *testing.T) {
		t.Parallel()

		strategy, dockerd, _ := newNode(t)
		dockerd.HoldNetwork(docker.Network{ID: "n1", Name: "backend"})

		_, err := strategy.Execute(t.Context(), aContainer("c-uuid"), containerKind.ActionCreate, nil)
		require.NoError(t, err)

		connected, err := strategy.Execute(t.Context(), aContainer("c-uuid"), containerKind.ActionConnect, containerKind.ConnectPayload{Network: "backend", Aliases: []string{"api"}})
		require.NoError(t, err)
		assert.Contains(t, connected.Status.Docker.Networks, "backend")

		disconnected, err := strategy.Execute(t.Context(), aContainer("c-uuid"), containerKind.ActionDisconnect, containerKind.DisconnectPayload{Network: "backend"})
		require.NoError(t, err)
		assert.NotContains(t, disconnected.Status.Docker.Networks, "backend")

		_, err = strategy.Execute(t.Context(), aContainer("c-uuid"), containerKind.ActionConnect, containerKind.ConnectPayload{Network: "nowhere"})
		assert.ErrorIs(t, err, domain.ErrNotExists, "a network that is not there is not there")
	})

	t.Run("removed, running or not, and one already gone is gone", func(t *testing.T) {
		t.Parallel()

		strategy, dockerd, _ := newNode(t)

		_, err := strategy.Execute(t.Context(), aContainer("c-uuid"), containerKind.ActionCreate, nil)
		require.NoError(t, err)

		outcome, err := strategy.Execute(t.Context(), aContainer("c-uuid"), containerKind.ActionDelete, containerKind.DeletePayload{})
		require.NoError(t, err)
		assert.Empty(t, outcome.Status.Status.State)
		assert.Empty(t, dockerd.Held(), "whether one that runs may be removed was asked before it was sent here")

		_, err = strategy.Execute(t.Context(), aContainer("c-uuid"), containerKind.ActionDelete, containerKind.DeletePayload{})
		assert.NoError(t, err)
	})

	t.Run("and one whose vm is not on this node is gone with it", func(t *testing.T) {
		t.Parallel()

		strategy, _, _ := newNode(t)

		_, err := strategy.Execute(t.Context(), aContainer("c-uuid", func(c *containerKind.Container) {
			c.Metadata.Owners = []kind.Reference{{Kind: "vm", UUID: "vm-elsewhere"}}
		}), containerKind.ActionDelete, containerKind.DeletePayload{})
		assert.NoError(t, err)
	})

	t.Run("one nobody keeps a record of is found by its docker id", func(t *testing.T) {
		t.Parallel()

		strategy, dockerd, _ := newNode(t)
		dockerd.Hold(docker.Container{ID: "c0ffee", Name: "db", State: "running"})

		outcome, err := strategy.Execute(t.Context(), aContainer("derived-uuid", func(c *containerKind.Container) {
			c.Status.Docker = &containerKind.Docker{ID: "c0ffee"}
		}), containerKind.ActionStop, nil)
		require.NoError(t, err)

		assert.Equal(t, "exited", outcome.Status.Docker.State)
	})
}

func TestNode_Query(t *testing.T) {
	t.Parallel()

	strategy, _, _ := newNode(t)

	_, err := strategy.Execute(t.Context(), aContainer("c-uuid"), containerKind.ActionCreate, nil)
	require.NoError(t, err)

	t.Run("its log, the last lines asked for", func(t *testing.T) {
		t.Parallel()

		answer, err := strategy.Query(t.Context(), aContainer("c-uuid"), containerKind.ActionLogs, containerKind.LogsPayload{Tail: 2})
		require.NoError(t, err)

		logs := answer.(containerKind.Logs)
		require.Len(t, logs.Lines, 2)
		assert.Equal(t, "line 3 of c0001", logs.Lines[1].Line)
		assert.False(t, logs.Truncated)
	})

	t.Run("what it uses", func(t *testing.T) {
		t.Parallel()

		answer, err := strategy.Query(t.Context(), aContainer("c-uuid"), containerKind.ActionStats, nil)
		require.NoError(t, err)

		assert.InDelta(t, 12.5, answer.(containerKind.Stats).CPUPercent, 0.001)
	})

	t.Run("one its vm has none of is not there", func(t *testing.T) {
		t.Parallel()

		_, err := strategy.Query(t.Context(), aContainer("another"), containerKind.ActionLogs, containerKind.LogsPayload{})
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}

func TestNode_State(t *testing.T) {
	t.Parallel()

	t.Run("the platform's by the resource each is, a stack's and nobody's as they are", func(t *testing.T) {
		t.Parallel()

		strategy, dockerd, node := newNode(t)

		oneOff := aContainer("one-off", func(c *containerKind.Container) {
			c.Spec.Name = "migrate"
			c.Spec.Ports = nil
			c.Spec.RestartPolicy = containerKind.RestartNo
		})

		for _, c := range []containerKind.Container{aContainer("c-uuid"), oneOff} {
			_, err := strategy.Execute(t.Context(), c, containerKind.ActionCreate, nil)
			require.NoError(t, err)
		}

		dockerd.Exit("migrate", 0)

		dockerd.Hold(docker.Container{ID: "s1", Name: "shop-web-1", State: "running", Labels: map[string]string{"workload.stack": "stack-uuid", docker.LabelComposeProject: "shop", docker.LabelComposeService: "web"}})
		dockerd.Hold(docker.Container{ID: "u1", Name: "db", State: "exited"})

		node.DockerVM(t, "vm-2").Down = true

		report, err := strategy.State(t.Context())
		require.NoError(t, err)

		assert.Equal(t, []string{"vm-1"}, report.Read)
		assert.Equal(t, []string{"vm-2"}, report.Unseen)

		byName := map[string]kind.Observed[containerKind.Status]{}
		for _, observed := range report.Instances {
			byName[observed.Status.Docker.Name] = observed
		}

		require.Len(t, byName, 4)

		assert.Equal(t, "c-uuid", byName["web"].UUID)
		assert.Equal(t, containerKind.Running, byName["web"].Status.Status.State)
		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-1"}}, byName["web"].Owners)

		assert.Equal(t, "one-off", byName["migrate"].UUID)
		assert.Equal(t, containerKind.Completed, byName["migrate"].Status.Status.State, "a one-off that ran to its end")
		assert.Equal(t, containerKind.RestartNo, byName["migrate"].Status.Docker.RestartPolicy, "as its spec says")

		assert.Empty(t, byName["shop-web-1"].UUID, "a stack's is not one of the kind's records")
		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-1"}, {Kind: "stack", UUID: "stack-uuid"}}, byName["shop-web-1"].Owners)

		assert.Empty(t, byName["db"].UUID, "and nor is nobody's")
		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-1"}}, byName["db"].Owners)
	})

	t.Run("an engine that cannot say which vms there are is no report at all", func(t *testing.T) {
		t.Parallel()

		_, err := container.New(blocks.NewReader(blockstest.Failing{}, blockstest.NewNode()), time.Minute).State(t.Context())
		assert.Error(t, err)
	})
}

func TestObserved(t *testing.T) {
	t.Parallel()

	spec, err := containerKind.SpecLabel(containerKind.Spec{Image: "nginx", RestartPolicy: containerKind.RestartUnlessStopped})
	require.NoError(t, err)

	observed := container.Observed("vm-1", docker.Container{ID: "c1", State: "exited", Labels: map[string]string{
		"workload.managed":      "true",
		"workload.container":    "c-uuid",
		containerKind.LabelSpec: spec,
	}})

	assert.Equal(t, "c-uuid", observed.UUID)
	assert.Equal(t, containerKind.Stopped, observed.Status.Status.State, "docker starts it again by itself")
	assert.Equal(t, containerKind.RestartUnlessStopped, observed.Status.Docker.RestartPolicy)
}
