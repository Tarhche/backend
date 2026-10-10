package getPlan_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getPlan"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	executionsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/executions"
	leasesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/leases"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

type fixture struct {
	executions *executionsMemory.Repository
	leases     *leasesMemory.Repository
	useCase    *getPlan.UseCase
}

// newFixture is how the plans of fans kept in memory went, fans being roots
// of their own graphs.
func newFixture(t *testing.T, records ...resource.Record) *fixture {
	t.Helper()

	resources := resourcesMemory.NewRepository()

	for _, r := range records {
		_, err := resources.Create(context.Background(), r)
		require.NoError(t, err)
	}

	f := &fixture{executions: executionsMemory.NewRepository(), leases: leasesMemory.NewRepository(nil)}
	f.useCase = getPlan.NewUseCase(kindstest.Registry(&kindstest.Fans{}), resources, f.executions, f.leases, graph.Roots{kindstest.Kind})

	return f
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := graph.Ref{Kind: kindstest.Kind, UUID: "web"}

	t.Run("a root's plan is how each of its nodes went, and what its lease keeps of its failures", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("web", kindstest.Running, kindstest.Running, func(fan *kindstest.Fan) { fan.Metadata.Generation = 2 }))

		started := kindstest.Moment
		executions := []graph.Execution{
			{Root: root, Generation: 2, Node: "house/house-1", Kind: "house", Action: graph.ActionCreate, Level: 0, State: graph.StateDone, Ref: graph.Ref{Kind: "house", UUID: "house-1"}, StartedAt: started, FinishedAt: started.Add(time.Second)},
			{Root: root, Generation: 2, Node: "fan/web", Kind: kindstest.Kind, Action: graph.ActionCreate, Level: 1, State: graph.StateFailed, Reason: "it has no blades", StartedAt: started.Add(time.Second), FinishedAt: started.Add(2 * time.Second)},
		}

		for _, e := range executions {
			require.NoError(t, f.executions.Save(ctx, e))
		}

		_, err := f.leases.Take(ctx, "web", "one", time.Minute)
		require.NoError(t, err)

		released, err := f.leases.Release(ctx, "web", "one", graph.Ended{State: graph.StateFailed, Reason: "fan/web: it has no blades", Backoff: graph.Backoff{First: 15 * time.Second, Most: time.Minute}})
		require.NoError(t, err)

		response, err := f.useCase.Execute(ctx, &getPlan.Request{Kind: kindstest.Kind, OwnerUUID: kindstest.OwnerUUID, UUID: "web"})
		require.NoError(t, err)

		assert.Equal(t, root, response.Root)
		assert.Equal(t, int64(2), response.Generation)

		kept, err := f.executions.GetByRoot(ctx, "web")
		require.NoError(t, err)
		assert.Equal(t, kept, response.Executions)

		require.NotNil(t, response.Lease)
		assert.Equal(t, 1, response.Lease.Failures)
		assert.Equal(t, "fan/web: it has no blades", response.Lease.Reason)
		assert.Equal(t, released.NextAt, response.Lease.NextAt)
	})

	t.Run("one never carried out has no nodes, and no lease", func(t *testing.T) {
		t.Parallel()

		response, err := newFixture(t, kindstest.AFan("web", kindstest.Pending, kindstest.Running)).useCase.Execute(ctx, &getPlan.Request{Kind: kindstest.Kind, UUID: "web"})
		require.NoError(t, err)

		assert.Empty(t, response.Executions)
		assert.Nil(t, response.Lease)
		assert.Equal(t, int64(1), response.Generation, "a record kept before generations is at its first")
	})

	for name, tt := range map[string]struct {
		record  resource.Record
		request getPlan.Request
	}{
		"what is no root is not there": {
			record: kindstest.AFan("web", kindstest.Running, kindstest.Running, func(fan *kindstest.Fan) {
				fan.Metadata.Owners = []kind.Reference{{Kind: kindstest.Kind, UUID: "owner"}}
			}),
			request: getPlan.Request{Kind: kindstest.Kind, UUID: "web"},
		},
		"nor is somebody else's": {
			record:  kindstest.AFan("web", kindstest.Running, kindstest.Running),
			request: getPlan.Request{Kind: kindstest.Kind, OwnerUUID: "somebody-else", UUID: "web"},
		},
		"nor what is not there at all": {
			record:  kindstest.AFan("other", kindstest.Running, kindstest.Running),
			request: getPlan.Request{Kind: kindstest.Kind, UUID: "web"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			request := tt.request

			_, err := newFixture(t, tt.record).useCase.Execute(ctx, &request)
			assert.ErrorIs(t, err, domain.ErrNotExists)
		})
	}

	t.Run("a kind that is not run here is not one", func(t *testing.T) {
		t.Parallel()

		_, err := newFixture(t).useCase.Execute(ctx, &getPlan.Request{Kind: "lamp", UUID: "web"})
		assert.ErrorIs(t, err, kind.ErrUnknownKind)
	})

	t.Run("what reading fails with is what it fails with", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("web", kindstest.Running, kindstest.Running))
		f.leases.Fail = errors.New("the database is gone")

		_, err := f.useCase.Execute(ctx, &getPlan.Request{Kind: kindstest.Kind, UUID: "web"})
		assert.ErrorIs(t, err, f.leases.Fail)
	})
}
