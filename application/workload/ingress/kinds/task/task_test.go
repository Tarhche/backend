package task_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ingressTasks "github.com/khanzadimahdi/testproject/application/workload/ingress/kinds/task"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

// held is a task on a node, serving ports under a policy, doing state.
func held(uuid string, state kind.State, policy network.Policy, ports ...port.Port) taskKind.Task {
	return taskKind.Task{
		Kind:     taskKind.Name,
		Metadata: kind.Metadata{UUID: uuid, Slug: "request-" + uuid, OwnerUUID: "guest", Node: "workload-orchestrator-01"},
		Spec:     taskKind.Spec{Image: "busybox", Ports: ports, NetworkPolicy: policy},
		Status:   taskKind.Status{Status: kind.Status{State: state, Expected: taskKind.Running}},
	}
}

func ingressOver(t *testing.T, tasks ...taskKind.Task) *ingressTasks.Ingress {
	t.Helper()

	records := make([]resource.Record, len(tasks))
	for i := range tasks {
		raw, err := kind.Encode(tasks[i])
		require.NoError(t, err)

		records[i] = resource.Record{Raw: raw}
	}

	return ingressTasks.New(resourcesMemory.NewRepository(records...))
}

// broken is a store of records that cannot be read.
type broken struct{}

func (broken) GetOne(context.Context, string, string) (resource.Record, error) {
	return resource.Record{}, errors.New("the database is gone")
}

func (broken) GetOneBySlug(context.Context, string, string) (resource.Record, error) {
	return resource.Record{}, errors.New("the database is gone")
}

func TestIngress_ByUUID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ingress := ingressOver(t,
		held("01", taskKind.Running, network.PolicyIsolated, 3000),
		held("02", taskKind.Completed, network.PolicyNone),
		taskKind.Task{Kind: taskKind.Name, Metadata: kind.Metadata{UUID: "03", Slug: "request-03"}, Status: taskKind.Status{Status: kind.Status{State: taskKind.Created}}},
	)

	t.Run("a task's terminal is carried to the node holding it", func(t *testing.T) {
		t.Parallel()

		location, err := ingress.ByUUID(ctx, "01")
		require.NoError(t, err)
		assert.Equal(t, kind.Location{UUID: "01", Node: "workload-orchestrator-01", Ports: []port.Port{3000}}, location)
	})

	t.Run("whatever it is doing: its node says whether it can be opened", func(t *testing.T) {
		t.Parallel()

		location, err := ingress.ByUUID(ctx, "02")
		require.NoError(t, err)
		assert.Equal(t, kind.Location{UUID: "02", Node: "workload-orchestrator-01"}, location)
	})

	t.Run("one on no node is there, and on none", func(t *testing.T) {
		t.Parallel()

		location, err := ingress.ByUUID(ctx, "03")
		require.NoError(t, err)
		assert.Empty(t, location.Node)
	})

	t.Run("one nobody keeps is not there", func(t *testing.T) {
		t.Parallel()

		_, err := ingress.ByUUID(ctx, "09")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("records that cannot be read are said", func(t *testing.T) {
		t.Parallel()

		_, err := ingressTasks.New(broken{}).ByUUID(ctx, "01")
		require.Error(t, err)
		assert.NotErrorIs(t, err, domain.ErrNotExists)
	})
}

func TestIngress_BySlug(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ingress := ingressOver(t,
		held("01", taskKind.Running, network.PolicyPublic, 3000, 8080),
		held("02", taskKind.Scheduled, network.PolicyIsolated, 3000),
		held("03", taskKind.Running, network.PolicyNone),
		held("04", taskKind.Running, ""),
	)

	t.Run("a running task's ports are reached on the node holding it", func(t *testing.T) {
		t.Parallel()

		location, err := ingress.BySlug(ctx, "request-01")
		require.NoError(t, err)
		assert.Equal(t, kind.Location{UUID: "01", Node: "workload-orchestrator-01", Ports: []port.Port{3000, 8080}}, location)
	})

	t.Run("one that is not running cannot be reached now, and says which ports it would let in", func(t *testing.T) {
		t.Parallel()

		location, err := ingress.BySlug(ctx, "request-02")
		assert.ErrorIs(t, err, kind.ErrUnreachable)
		assert.ErrorContains(t, err, "the task is not running")
		assert.Equal(t, []port.Port{3000}, location.Ports)
	})

	t.Run("one with no network lets nothing in, and one with no policy is isolated", func(t *testing.T) {
		t.Parallel()

		location, err := ingress.BySlug(ctx, "request-03")
		require.NoError(t, err)
		assert.Empty(t, location.Ports)

		location, err = ingress.BySlug(ctx, "request-04")
		require.NoError(t, err)
		assert.Equal(t, "04", location.UUID)
	})

	t.Run("a slug no task has is not there, for the next kind to be asked", func(t *testing.T) {
		t.Parallel()

		_, err := ingress.BySlug(ctx, "request-09")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}
