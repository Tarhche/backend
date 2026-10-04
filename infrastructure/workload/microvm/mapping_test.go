package microvm

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const orchestrator = "workload-orchestrator-01"

// service is a run as the orchestrator's runTask asks for one: a public
// service of a stack, with everything a run can carry.
func service() task.Execution {
	return task.Execution{
		Name:             "shop-api-xkfqz",
		TaskUUID:         "0199a0e0-0000-7000-8000-000000000001",
		TaskName:         "api",
		Slug:             "shop-api-xkfqz",
		Kind:             task.KindService,
		NodeName:         orchestrator,
		OwnerUUID:        "0199a0e0-0000-7000-8000-0000000000aa",
		StackUUID:        "0199a0e0-0000-7000-8000-0000000000bb",
		Attempt:          2,
		Interactive:      true,
		TTL:              90 * time.Second,
		Image:            "ghcr.io/tarhche/shop-api:1",
		ResourceLimits:   task.ResourceLimits{Cpu: 0.5, Memory: 256 << 20, Disk: 1 << 30},
		RestartPolicy:    "on-failure:3",
		WorkingDirectory: "/srv",
		ExposedPorts:     port.PortSet{8080: {}, 80: {}},
		PortBindings:     port.PortMap{80: {{HostIP: "0.0.0.0"}}, 8080: {{HostIP: "0.0.0.0"}}},
		Networks:         network.Attachments(network.PolicyPublic, "shop-xkfqz", "api"),
		AutoRemove:       true,
		Environment:      []string{"PORT=8080"},
		Entrypoint:       []string{"/entrypoint.sh"},
		Command:          []string{"serve", "--port", "8080"},
		ReadOnly:         true,
	}
}

func TestSpecOf(t *testing.T) {
	t.Parallel()

	t.Run("a run is asked of vmhost in the words a task is asked for in", func(t *testing.T) {
		t.Parallel()

		run := service()

		spec, err := specOf(&run, orchestrator)
		require.NoError(t, err)

		assert.Equal(t, vm.Spec{
			Name:     "shop-api-xkfqz",
			Hostname: "shop-api-xkfqz",
			Image:    "ghcr.io/tarhche/shop-api:1",
			Labels: map[string]string{
				"task.uuid":        "0199a0e0-0000-7000-8000-000000000001",
				"task.name":        "api",
				"task.slug":        "shop-api-xkfqz",
				"task.kind":        "service",
				"node.name":        orchestrator,
				"task.owner":       "0199a0e0-0000-7000-8000-0000000000aa",
				"task.stack":       "0199a0e0-0000-7000-8000-0000000000bb",
				"task.attempt":     "2",
				"task.interactive": "true",
				"task.ttl":         "90",
			},
			Entrypoint:    []string{"/entrypoint.sh"},
			Command:       []string{"serve", "--port", "8080"},
			Env:           []string{"PORT=8080"},
			WorkingDir:    "/srv",
			Resources:     vm.Resources{CPU: 0.5, Memory: 256 << 20, Disk: 1 << 30},
			ReadOnly:      true,
			RestartPolicy: "on-failure:3",
			AutoRemove:    true,
			Networks: []vm.Attachment{
				{Network: "workload-stack-shop-xkfqz", Aliases: []string{"api"}},
				{Network: vm.PublicNetwork, Gateway: true},
			},
			ExposedPorts: []uint16{80, 8080},
		}, spec)
	})

	t.Run("a task with no network joins none", func(t *testing.T) {
		t.Parallel()

		run := service()
		run.Networks = network.Attachments(network.PolicyNone, "", "")
		run.ExposedPorts = nil

		spec, err := specOf(&run, orchestrator)
		require.NoError(t, err)

		assert.Empty(t, spec.Networks)
		assert.Empty(t, spec.ExposedPorts)
	})

	t.Run("a standalone isolated task joins the shared network alone", func(t *testing.T) {
		t.Parallel()

		run := service()
		run.Networks = network.Attachments(network.PolicyIsolated, "", "")

		spec, err := specOf(&run, orchestrator)
		require.NoError(t, err)

		assert.Equal(t, []vm.Attachment{{Network: network.IsolatedNetworkName}}, spec.Networks)
	})

	t.Run("a run that names no node is this node's", func(t *testing.T) {
		t.Parallel()

		run := service()
		run.NodeName = ""

		spec, err := specOf(&run, orchestrator)
		require.NoError(t, err)

		assert.Equal(t, orchestrator, spec.Labels["node.name"])
	})

	t.Run("a run for another node is not made here, where neither node would find it", func(t *testing.T) {
		t.Parallel()

		run := service()
		run.NodeName = "workload-orchestrator-02"

		_, err := specOf(&run, orchestrator)

		assert.ErrorIs(t, err, vm.ErrInvalid)
	})

	t.Run("a port that cannot be one is refused rather than dropped", func(t *testing.T) {
		t.Parallel()

		run := service()
		run.ExposedPorts = port.PortSet{70000: {}}

		_, err := specOf(&run, orchestrator)

		assert.ErrorIs(t, err, vm.ErrInvalid)
	})
}

func TestExecutionOf(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

	t.Run("what a run was asked to be travels there and back", func(t *testing.T) {
		t.Parallel()

		run := service()

		spec, err := specOf(&run, orchestrator)
		require.NoError(t, err)

		back := executionOf(vm.VM{ID: "0123456789abcdef", Spec: spec, State: vm.StateCreated, CreatedAt: created}, runtime.Firecracker)

		expected := run
		expected.ID = "0123456789abcdef"
		expected.Runtime = runtime.Firecracker
		expected.Status = task.StatusCreated
		expected.CreatedAt = created

		// nothing is published anywhere: a VM's ports are reached through
		// vmhost, so there are no bindings to say where.
		expected.PortBindings = nil

		assert.Equal(t, expected, back)
	})

	t.Run("a running vm on a network is reachable on every port it exposes", func(t *testing.T) {
		t.Parallel()

		started := created.Add(time.Minute)

		back := executionOf(vm.VM{
			ID:           "0123456789abcdef",
			Spec:         vm.Spec{ExposedPorts: []uint16{8080, 80}, Networks: []vm.Attachment{{Network: "workload-isolated"}}},
			State:        vm.StateRunning,
			RestartCount: 2,
			CreatedAt:    created,
			StartedAt:    started,
			Interfaces:   []vm.Interface{{Network: "workload-isolated", Address: "10.250.1.2/24"}},
		}, runtime.Firecracker)

		assert.Equal(t, task.StatusRunning, back.Status)
		assert.Equal(t, []port.Port{80, 8080}, back.Endpoints)
		assert.Equal(t, uint(2), back.RestartCount)
		assert.Equal(t, started, back.StartedAt)
		assert.Equal(t, task.Running, task.EvaluateState(back.Status, back.Kind, back.ExitCode))
	})

	t.Run("an ended vm says what its task returned, and is reachable on nothing", func(t *testing.T) {
		t.Parallel()

		for _, exited := range []struct {
			code  int
			kind  string
			state task.State
		}{
			{code: 0, kind: "job", state: task.Completed},
			{code: 1, kind: "job", state: task.Failed},
			{code: 137, kind: "job", state: task.Completed},
			{code: 1, kind: "service", state: task.Stopped},
		} {
			back := executionOf(vm.VM{
				ID:       "0123456789abcdef",
				Spec:     vm.Spec{ExposedPorts: []uint16{80}, Labels: map[string]string{"task.kind": exited.kind}},
				State:    vm.StateExited,
				ExitCode: exited.code,
			}, runtime.Firecracker)

			assert.Equal(t, exited.code, back.ExitCode)
			assert.Empty(t, back.Endpoints)
			assert.Equal(t, exited.state, task.EvaluateState(back.Status, back.Kind, back.ExitCode), "%s exiting %d", exited.kind, exited.code)
		}
	})

	t.Run("a vm with no labels is still one this node holds", func(t *testing.T) {
		t.Parallel()

		back := executionOf(vm.VM{ID: "0123456789abcdef", State: vm.StateDead}, runtime.Firecracker)

		assert.Equal(t, task.DefaultKind, back.Kind)
		assert.Equal(t, task.StatusDead, back.Status)
		assert.Equal(t, []network.Attachment{{Name: network.NoNetworkName}}, back.Networks)
	})
}

func TestStatusOf(t *testing.T) {
	t.Parallel()

	for state, status := range map[vm.State]task.Status{
		vm.StateCreated:    task.StatusCreated,
		vm.StateRunning:    task.StatusRunning,
		vm.StateRestarting: task.StatusRestarting,
		vm.StateExited:     task.StatusExited,
		vm.StateDead:       task.StatusDead,
		vm.StateRemoving:   task.StatusRemoving,
	} {
		assert.Equal(t, status, statusOf(state), state)
	}

	assert.Equal(t, task.StatusDead, statusOf("paused"), "a state from a newer vmhost is one nobody can account for")
}

func TestNetworkName(t *testing.T) {
	t.Parallel()

	t.Run("the workload's networks keep their names, and docker's bridge is vmhost's public network", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, network.IsolatedNetworkName, networkName(network.IsolatedNetworkName))
		assert.Equal(t, "workload-stack-shop-xkfqz", networkName(network.StackNetworkName("shop-xkfqz")))
		assert.Equal(t, vm.PublicNetwork, networkName(network.PublicNetworkName))
	})

	t.Run("a stack whose slug is as long as a slug gets has a network vmhost takes", func(t *testing.T) {
		t.Parallel()

		long := network.StackNetworkName(strings.Repeat("a", 57) + "-xkfqz")
		other := network.StackNetworkName(strings.Repeat("a", 57) + "-bcdef")

		require.Greater(t, len(long), maxNetworkName)

		named := networkName(long)

		assert.True(t, vm.IsNetworkName(named), named)
		assert.Equal(t, named, networkName(long), "the same name is cut the same way")
		assert.NotEqual(t, named, networkName(other), "two names cut short stay two")
		assert.True(t, strings.HasPrefix(named, "workload-stack-aaa"))
	})
}

func TestLabels(t *testing.T) {
	t.Parallel()

	run := service()

	var read task.Execution
	identify(&read, labelsOf(&run, orchestrator))

	assert.Equal(t, run.TaskUUID, read.TaskUUID)
	assert.Equal(t, run.TaskName, read.TaskName)
	assert.Equal(t, run.Slug, read.Slug)
	assert.Equal(t, run.Kind, read.Kind)
	assert.Equal(t, orchestrator, read.NodeName)
	assert.Equal(t, run.OwnerUUID, read.OwnerUUID)
	assert.Equal(t, run.StackUUID, read.StackUUID)
	assert.Equal(t, run.Attempt, read.Attempt)
	assert.Equal(t, run.Interactive, read.Interactive)
	assert.Equal(t, run.TTL, read.TTL)
}
