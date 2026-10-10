package workload

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	ingressMemory "github.com/khanzadimahdi/testproject/infrastructure/workload/ingress/memory"
)

// TestNewIngressWorkload holds the ingress to finding the tasks and the VMs
// where what it hears says they are: their nodes' heartbeats, each kind's on
// its own subject, and the commands sent to their nodes and what came of
// them, and nothing of any other kind.
func TestNewIngressWorkload(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	workload, err := NewIngressWorkload(ingressMemory.NewLocations(time.Minute), slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	t.Run("a task's slug is looked for before a vm's, and a stack is reached through its vm", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, []string{taskKind.Name, vmKind.Name, stackKind.Name}, kindNames(workload.Kinds.Descriptors()))

		stacks, registered := workload.Kinds.Lookup(stackKind.Name)
		require.True(t, registered)

		_, err := stacks.BySlug(ctx, "web-xkfqz")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("it hears the heartbeats of the task kind and of the vm kind, and every command and its result", func(t *testing.T) {
		t.Parallel()

		heard := slices.Sorted(maps.Keys(workload.Subscribers))

		assert.Equal(t, []string{
			kind.ActOnResourceName,
			kind.ResourceActedOnName,
			kind.HeartbeatName(taskKind.Name),
			kind.HeartbeatName(vmKind.Name),
		}, heard)
	})

	t.Run("and finds a vm where its heartbeat says it is, until a command sent to its node takes it away", func(t *testing.T) {
		t.Parallel()

		status, err := json.Marshal(vmKind.Status{
			Status:    kind.Status{State: vmKind.Running},
			Slug:      "box-xkfqz",
			Endpoints: []vmKind.Endpoint{{Port: 80, Address: "vmhost:20000"}},
		})
		require.NoError(t, err)

		heartbeat, err := json.Marshal(kind.Heartbeat{
			Node:     "workload-orchestrator-02",
			At:       time.Now(),
			Observed: kind.Observation{Kind: vmKind.Name, UUID: "vm-uuid", Status: status},
		})
		require.NoError(t, err)

		require.NoError(t, workload.Subscribers[kind.HeartbeatName(vmKind.Name)].Handle(ctx, heartbeat))

		vms, registered := workload.Kinds.Lookup(vmKind.Name)
		require.True(t, registered)

		location, err := vms.BySlug(ctx, "box-xkfqz")
		require.NoError(t, err)
		assert.Equal(t, kind.Location{UUID: "vm-uuid", Node: "workload-orchestrator-02", Ports: []port.Port{80}}, location)

		tasks, registered := workload.Kinds.Lookup(taskKind.Name)
		require.True(t, registered)

		_, err = tasks.BySlug(ctx, "box-xkfqz")
		assert.ErrorIs(t, err, domain.ErrNotExists, "a vm's slug names no task")

		recorded, err := kind.Encode(vmKind.VM{
			Kind:     vmKind.Name,
			Metadata: kind.Metadata{UUID: "vm-uuid", Slug: "box-xkfqz", Node: "workload-orchestrator-02"},
			Spec:     vmKind.Spec{Ports: []port.Port{80}, Network: vmKind.Network{Ingress: vm.AccessAllow, Egress: vm.AccessAllow}},
		})
		require.NoError(t, err)

		command, err := json.Marshal(kind.ActOnResource{ID: "command-1", Kind: vmKind.Name, UUID: "vm-uuid", Action: vmKind.ActionStop, Node: "workload-orchestrator-02", Resource: recorded})
		require.NoError(t, err)

		require.NoError(t, workload.Subscribers[kind.ActOnResourceName].Handle(ctx, command))

		_, err = vms.BySlug(ctx, "box-xkfqz")
		assert.ErrorIs(t, err, kind.ErrUnreachable, "stopped from the moment the stop is sent")
	})
}
