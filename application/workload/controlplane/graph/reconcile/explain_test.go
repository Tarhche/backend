package reconcile_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/reconcile"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/graph/graphtest"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// said is what a root's record says of it: its state, why, and what it
// failed with.
type said struct {
	State   kind.State
	Reason  string
	Failure *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
}

// stated has a record's status say it is in state, expected running.
func stated(state kind.State) func(r *resource.Record) {
	return func(r *resource.Record) {
		status, err := resource.WithCommon(r.Status, kind.Status{State: state, Expected: containerKind.Running})
		if err != nil {
			panic(err)
		}

		r.Status = status
	}
}

// rooted is the record of the root web, as changes say.
func rooted(changes ...func(r *resource.Record)) resource.Record {
	r := root()

	for _, change := range changes {
		change(&r)
	}

	return r
}

// saidOf is what the record of the root web says of it now.
func (f *fixture) saidOf(t *testing.T) said {
	t.Helper()

	r, err := f.records.GetOne(context.Background(), containerKind.Name, web.Name())
	require.NoError(t, err)

	common, err := r.Common()
	require.NoError(t, err)

	var status struct {
		Failure *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"failure"`
	}
	require.NoError(t, json.Unmarshal(r.Status, &status))

	return said{State: common.State, Reason: common.Reason, Failure: status.Failure}
}

// patient is a reconciler's config that backs nothing off, so that a root is
// carried out again at once.
func patient() reconcile.Config {
	config := reconcile.DefaultConfig()
	config.Backoff = graph.Backoff{}

	return config
}

func TestReconciler_Reconcile_explains(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	refused := &graph.Refused{Code: "no_capacity", Message: "the vm was refused: vm: no_capacity"}
	reason := "vm/v-uuid: the vm was refused: vm: no_capacity"

	t.Run("a root not made whose plan is refused says why, and what it was refused for, by its code", func(t *testing.T) {
		t.Parallel()

		kinds := container()
		kinds.On(graphtest.Create, vm, func(context.Context) error { return refused })

		f := newFixture(t, kinds, rooted(stated(containerKind.Pending)))
		r := f.reconciler(t, "one", patient(), containerKind.Name)

		require.ErrorIs(t, r.Reconcile(ctx, refOf(web)), refused)

		now := f.saidOf(t)
		assert.Equal(t, containerKind.Pending, now.State, "it is still to be made")
		assert.Equal(t, reason, now.Reason)
		require.NotNil(t, now.Failure)
		assert.Equal(t, "no_capacity", now.Failure.Code)
		assert.Equal(t, reason, now.Failure.Message)

		writes := len(f.resources.Writes())

		require.ErrorIs(t, r.Reconcile(ctx, refOf(web)), refused)
		assert.Len(t, f.resources.Writes(), writes, "refused again the same way, nothing is written again")
	})

	t.Run("and once a later plan waits, it says why, and lets go of what it was refused for", func(t *testing.T) {
		t.Parallel()

		kinds := container()
		kinds.On(graphtest.Create, vm, func(context.Context) error { return refused })

		f := newFixture(t, kinds, rooted(stated(kind.Missing)))
		r := f.reconciler(t, "one", patient(), containerKind.Name)

		require.Error(t, r.Reconcile(ctx, refOf(web)))
		require.NotNil(t, f.saidOf(t).Failure, "a root gone missing says it too")

		kinds.On(graphtest.Create, vm, func(context.Context) error { return &graph.Waiting{Reason: "it is being deleted"} })

		require.NoError(t, r.Reconcile(ctx, refOf(web)))

		now := f.saidOf(t)
		assert.Equal(t, "vm/v-uuid: it is being deleted", now.Reason)
		require.NotNil(t, now.Failure)
		assert.Empty(t, now.Failure.Code, "what it was refused for is let go of")
	})

	t.Run("and once a later plan is done", func(t *testing.T) {
		t.Parallel()

		kinds := container()
		kinds.On(graphtest.Create, vm, func(context.Context) error { return refused })

		f := newFixture(t, kinds, rooted(stated(containerKind.Pending)))
		r := f.reconciler(t, "one", patient(), containerKind.Name)

		require.Error(t, r.Reconcile(ctx, refOf(web)))

		kinds.On(graphtest.Create, vm, nil)

		require.NoError(t, r.Reconcile(ctx, refOf(web)))

		now := f.saidOf(t)
		require.NotNil(t, now.Failure)
		assert.Empty(t, now.Failure.Code)
	})

	t.Run("what fails for any other reason is a reason only", func(t *testing.T) {
		t.Parallel()

		broken := errors.New("the database is gone")

		kinds := container()
		kinds.On(graphtest.Create, vm, func(context.Context) error { return broken })

		f := newFixture(t, kinds, rooted(stated(containerKind.Pending)))
		r := f.reconciler(t, "one", patient(), containerKind.Name)

		require.ErrorIs(t, r.Reconcile(ctx, refOf(web)), broken)

		now := f.saidOf(t)
		assert.Equal(t, "vm/v-uuid: the database is gone", now.Reason)
		assert.Nil(t, now.Failure)
	})

	for name, record := range map[string]resource.Record{
		"a root made says nothing: what it is doing is its node's to say": rooted(stated(containerKind.Running)),
		"nor does one asked to go": rooted(stated(containerKind.Pending), func(r *resource.Record) { deleting(&r.Metadata) }),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			kinds := container()
			kinds.On(graphtest.Create, vm, func(context.Context) error { return refused })
			kinds.On(graphtest.Delete, web, func(context.Context) error { return refused })

			f := newFixture(t, kinds, record)
			r := f.reconciler(t, "one", patient(), containerKind.Name)

			_ = r.Reconcile(ctx, refOf(web))

			assert.NotContains(t, f.resources.Writes(), "update container/web", "nothing is said on it")
		})
	}
}

func TestReconciler_Reconcile_goneWhileItsPlanRuns(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// web's record goes while its vm is made, which is what its kind finds
	// when it comes to make web, and fails with.
	kinds := container()

	var f *fixture

	kinds.On(graphtest.Create, vm, func(ctx context.Context) error {
		return f.records.Delete(ctx, containerKind.Name, web.Name())
	})
	kinds.On(graphtest.Exists, web, func(context.Context) error { return nil })
	kinds.On(graphtest.Create, web, func(context.Context) error { return errors.New("the container web is gone") })

	f = newFixture(t, kinds, rooted(stated(containerKind.Pending)))
	r := f.reconciler(t, "one", patient(), containerKind.Name)

	require.NoError(t, r.Reconcile(ctx, refOf(web)), "a root gone is no failure")

	_, kept := f.leases.Lease(web.Name())
	assert.False(t, kept, "its lease is forgotten")
	assert.Empty(t, f.kept(t, web), "and how its plan went")
	assert.Empty(t, r.Queued(), "and nothing is queued to make it again")
}
