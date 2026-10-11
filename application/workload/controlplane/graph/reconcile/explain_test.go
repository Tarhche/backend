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
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
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

// moveTo has the record of the root the node id names say it is in state
// from now on, as what a person asks of a stack has it say.
func (f *fixture) moveTo(ctx context.Context, id graph.NodeID, state kind.State) error {
	r, err := f.records.GetOne(ctx, id.Kind(), id.Name())
	if err != nil {
		return err
	}

	common, err := r.Common()
	if err != nil {
		return err
	}

	common.State = state
	common.Since = f.clock.Now()

	if err := r.SetCommon(common); err != nil {
		return err
	}

	_, err = f.records.Update(ctx, r)

	return err
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

	return f.saidOfRoot(t, web)
}

// saidOfRoot is what the record of the root the node id names says of it
// now.
func (f *fixture) saidOfRoot(t *testing.T, id graph.NodeID) said {
	t.Helper()

	r, err := f.records.GetOne(context.Background(), id.Kind(), id.Name())
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

func TestReconciler_Reconcile_askedToGoWhileItsPlanRuns(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// web is asked to go once its vm is found there, and its vm is deleted
	// with it, as a cascade does: its plan, which read it before, makes
	// nothing after.
	kinds := container()
	kinds.Hold(vm)

	var f *fixture

	kinds.On(graphtest.Exists, vm, func(ctx context.Context) error {
		r, err := f.records.GetOne(ctx, containerKind.Name, web.Name())
		if err != nil {
			return err
		}

		deleting(&r.Metadata)

		_, err = f.records.Update(ctx, r)

		return err
	})

	f = newFixture(t, kinds, rooted(stated(containerKind.Running)))
	r := f.reconciler(t, "one", patient(), containerKind.Name)

	require.NoError(t, r.Reconcile(ctx, refOf(web)), "it waits: it is no failure")

	assert.Empty(t, kinds.Asked(graphtest.Create), "nothing is made for it")

	lease := f.lease(t, web)
	assert.Zero(t, lease.Failures, "and it is not backed off, to be removed when it is next reconciled")

	var waiting []string
	for _, e := range f.kept(t, web) {
		if e.State == graph.StateWaiting {
			waiting = append(waiting, e.Reason)
		}
	}

	assert.Contains(t, waiting, "its root was asked to go")
}

// store is the root of a stack, made after its vm and its network, which is
// made after the vm.
var store = graph.ID(stackKind.Name, "store")

// stacked are the kinds of the graph of the stack store.
func stacked() *graphtest.Kind {
	kinds := graphtest.NewKind()
	kinds.Require(store, requirement(vm), requirement(network))
	kinds.Require(network, requirement(vm))

	return kinds
}

// aStack is the record of the stack store, in the vm, owned by nothing, doing
// state.
func aStack(state kind.State) resource.Record {
	r := record(refOf(store), []graph.Ref{refOf(vm)}, nil)

	status, err := resource.WithCommon(nil, kind.Status{State: state, Expected: stackKind.Running})
	if err != nil {
		panic(err)
	}

	r.Status = status

	return r
}

func TestReconciler_Reconcile_explainsAStack(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	refused := &graph.Refused{Code: "no_capacity", Message: "the vm was refused: vm: no_capacity"}

	t.Run("a stack, whose state nothing but its plan says, fails once its plan fails, whatever it was doing, saying why and what it was refused for", func(t *testing.T) {
		t.Parallel()

		kinds := stacked()
		kinds.On(graphtest.Create, vm, func(context.Context) error { return refused })

		f := newFixture(t, kinds, aStack(stackKind.Running))
		r := f.reconciler(t, "one", patient(), stackKind.Name)

		require.ErrorIs(t, r.Reconcile(ctx, refOf(store)), refused)

		now := f.saidOfRoot(t, store)
		assert.Equal(t, stackKind.Failed, now.State)
		assert.Equal(t, "vm/v-uuid: the vm was refused: vm: no_capacity", now.Reason)
		require.NotNil(t, now.Failure)
		assert.Equal(t, "no_capacity", now.Failure.Code)
	})

	t.Run("and waits once its plan waits on what it needs", func(t *testing.T) {
		t.Parallel()

		kinds := stacked()
		kinds.On(graphtest.Create, network, func(context.Context) error { return &graph.Waiting{Reason: "its vm is stopped"} })
		kinds.Hold(vm)

		f := newFixture(t, kinds, aStack(stackKind.Running))
		r := f.reconciler(t, "one", patient(), stackKind.Name)

		require.NoError(t, r.Reconcile(ctx, refOf(store)))

		now := f.saidOfRoot(t, store)
		assert.Equal(t, stackKind.Waiting, now.State)
		assert.Equal(t, "network/s_default: its vm is stopped", now.Reason)
	})

	t.Run("but one being started goes on starting, saying what it waits on, to start once it can", func(t *testing.T) {
		t.Parallel()

		kinds := stacked()
		kinds.On(graphtest.Create, network, func(context.Context) error { return &graph.Waiting{Reason: "its vm is stopped"} })
		kinds.Hold(vm)

		f := newFixture(t, kinds, aStack(stackKind.Starting))
		r := f.reconciler(t, "one", patient(), stackKind.Name)

		require.NoError(t, r.Reconcile(ctx, refOf(store)))

		now := f.saidOfRoot(t, store)
		assert.Equal(t, stackKind.Starting, now.State)
		assert.Equal(t, "network/s_default: its vm is stopped", now.Reason)
	})

	t.Run("but a state that began after its run read it, a person's start say, is no failure of it: it is left as it is, and the stack runs again, which fails it then", func(t *testing.T) {
		t.Parallel()

		kinds := stacked()

		var f *fixture

		// started while its plan runs, which then fails.
		kinds.On(graphtest.Create, vm, func(ctx context.Context) error {
			if err := f.moveTo(ctx, store, stackKind.Starting); err != nil {
				return err
			}

			return refused
		})

		f = newFixture(t, kinds, aStack(stackKind.Degraded))
		r := f.reconciler(t, "one", patient(), stackKind.Name)

		require.ErrorIs(t, r.Reconcile(ctx, refOf(store)), refused)

		now := f.saidOfRoot(t, store)
		assert.Equal(t, stackKind.Starting, now.State, "what it was asked is not failed by a run that read it before")
		assert.Empty(t, now.Reason)
		assert.Nil(t, now.Failure)
		assert.Equal(t, []graph.Ref{refOf(store)}, r.Queued(), "and it runs again, to carry it out")

		// the next run, which reads it starting, fails it.
		kinds.On(graphtest.Create, vm, func(context.Context) error { return refused })

		require.ErrorIs(t, r.Reconcile(ctx, refOf(store)), refused)

		now = f.saidOfRoot(t, store)
		assert.Equal(t, stackKind.Failed, now.State)
		require.NotNil(t, now.Failure)
		assert.Equal(t, "no_capacity", now.Failure.Code)
	})

	t.Run("and a run that could not keep how its plan's nodes went, the database gone say, fails nothing of it, and says nothing of it", func(t *testing.T) {
		t.Parallel()

		gone := errors.New("the database is gone")

		kinds := stacked()
		kinds.Hold(vm, network, store)

		f := newFixture(t, kinds, aStack(stackKind.Running))
		f.executions.Fail(gone)

		r := f.reconciler(t, "one", patient(), stackKind.Name)

		require.ErrorIs(t, r.Reconcile(ctx, refOf(store)), gone)

		now := f.saidOfRoot(t, store)
		assert.Equal(t, stackKind.Running, now.State, "it is what it was")
		assert.Empty(t, now.Reason)
		assert.NotContains(t, f.resources.Writes(), "update stack/store", "and nothing is written on it")

		lease := f.lease(t, store)
		assert.Equal(t, 1, lease.Failures, "its run failed all the same, and is let go of as one that did")
		assert.Contains(t, lease.Reason, "could not be read or written")
	})

	t.Run("but one whose plan is refused fails as much when how its nodes went could not be kept besides: what it was refused for is what its plan failed with", func(t *testing.T) {
		t.Parallel()

		gone := errors.New("the database is gone")

		kinds := stacked()

		var f *fixture

		// the database goes as the vm is refused, so that how the vm went is
		// not kept.
		kinds.On(graphtest.Create, vm, func(context.Context) error {
			f.executions.Fail(gone)

			return refused
		})

		f = newFixture(t, kinds, aStack(stackKind.Running))
		r := f.reconciler(t, "one", patient(), stackKind.Name)

		err := r.Reconcile(ctx, refOf(store))
		require.ErrorIs(t, err, refused)
		assert.NotErrorIs(t, err, gone, "which is logged beside it")

		now := f.saidOfRoot(t, store)
		assert.Equal(t, stackKind.Failed, now.State)
		require.NotNil(t, now.Failure)
		assert.Equal(t, "no_capacity", now.Failure.Code)
	})

	t.Run("and a run cut short, its control plane stopping, says nothing of it", func(t *testing.T) {
		t.Parallel()

		stopping, stop := context.WithCancel(ctx)

		kinds := stacked()
		kinds.On(graphtest.Create, vm, func(ctx context.Context) error {
			stop()
			<-ctx.Done()

			return ctx.Err()
		})

		f := newFixture(t, kinds, aStack(stackKind.Running))
		r := f.reconciler(t, "one", patient(), stackKind.Name)

		require.Error(t, r.Reconcile(stopping, refOf(store)))

		now := f.saidOfRoot(t, store)
		assert.Equal(t, stackKind.Running, now.State, "it is what it was")
		assert.Empty(t, now.Reason)
		assert.NotContains(t, f.resources.Writes(), "update stack/store", "and nothing is written on it")
	})
}

// TestReconciler_Reconcile_changedWhileItsPlanRuns holds a run of a root's
// plan to making nothing of a plan the root's record says otherwise of since
// the run read it: a VM's delete keeps a root that lives in it, to make the
// VM again as it was, a generation later, before the VM's record goes, and a
// run that planned it before would make the VM again as its plan says, named
// by its uuid alone.
func TestReconciler_Reconcile_changedWhileItsPlanRuns(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// web's vm is deleted once the run looks for it, as a cascade deletes
	// it: web is kept a generation later, and then the vm's record goes.
	kinds := container()
	kinds.Hold(network, volume)

	var f *fixture

	kinds.On(graphtest.Exists, vm, func(ctx context.Context) error {
		r, err := f.records.GetOne(ctx, containerKind.Name, web.Name())
		if err != nil {
			return err
		}

		r.Metadata.Generation++

		_, err = f.records.Update(ctx, r)

		return err
	})

	f = newFixture(t, kinds, rooted(stated(containerKind.Running)))
	r := f.reconciler(t, "one", patient(), containerKind.Name)

	require.NoError(t, r.Reconcile(ctx, refOf(web)), "it waits: it is no failure")

	assert.Empty(t, kinds.Asked(graphtest.Create), "nothing of its plan is made")
	assert.Equal(t, []graph.Ref{refOf(web)}, r.Queued(), "and it is queued, to be planned again")

	var waiting []string
	for _, e := range f.kept(t, web) {
		if e.State == graph.StateWaiting {
			waiting = append(waiting, e.Reason)
		}
	}

	assert.Contains(t, waiting, "its root was changed since its plan was made, which is made again")

	lease := f.lease(t, web)
	assert.Zero(t, lease.Failures)
}
