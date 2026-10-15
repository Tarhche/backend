package recordResult_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/observe"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/recordResult"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/waiters"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/graph/graphtest"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

type fixture struct {
	memory  *resourcesMemory.Repository
	racing  *kindstest.Racing
	waiters *waiters.Waiters
	clock   *kindstest.Clock
	handler *recordResult.ResourceActedOnHandler
}

func newFixture(t *testing.T, records ...resource.Record) *fixture {
	t.Helper()

	f := &fixture{memory: resourcesMemory.NewRepository(), waiters: waiters.New(), clock: kindstest.NewClock()}
	f.racing = &kindstest.Racing{Repository: f.memory}

	for _, r := range records {
		_, err := f.memory.Create(context.Background(), r)
		require.NoError(t, err)
	}

	f.handler = recordResult.NewResourceActedOnHandler(kindstest.Registry(&kindstest.Fans{}), f.racing, f.waiters, slog.New(slog.DiscardHandler), f.clock.Now)

	return f
}

func (f *fixture) stored(t *testing.T, uuid string) (resource.Record, bool) {
	t.Helper()

	r, err := f.memory.GetOne(context.Background(), kindstest.Kind, uuid)
	if err != nil {
		return resource.Record{}, false
	}

	return r, true
}

func message(t *testing.T, result kind.ResourceActedOn) []byte {
	t.Helper()

	payload, err := json.Marshal(result)
	require.NoError(t, err)

	return payload
}

// waiting is a fan waiting on command-1, a start.
func waiting(state kind.State, expected kind.State, action string) resource.Record {
	r := kindstest.AFan("fan-uuid", state, expected)
	r.Pending = &resource.Pending{Action: action, IDs: []string{"command-0", "command-1"}, SentAt: kindstest.Moment}
	r.Attempts = 2

	return r
}

func TestResourceActedOnHandler_Handle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	at := kindstest.Moment.Add(time.Minute)

	started := kind.ResourceActedOn{ID: "command-1", Kind: kindstest.Kind, UUID: "fan-uuid", Action: "start", Node: kindstest.NodeName, Attempt: 1, OK: true, Status: json.RawMessage(`{"state":"running","speed":2}`), Output: "started", At: at}

	t.Run("what a command left its resource as is taken, and whoever waits for it is told", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, waiting(kindstest.Starting, kindstest.Running, "start"))

		wait := f.waiters.Expect("command-1")
		defer wait.Done()

		require.NoError(t, f.handler.Handle(ctx, message(t, started)))

		stored, _ := f.stored(t, "fan-uuid")
		fan := kindstest.Typed(stored)

		assert.Equal(t, kindstest.Running, fan.Status.State)
		assert.Equal(t, 2, fan.Status.Speed)
		assert.Equal(t, at, fan.Status.ObservedAt, "when the node said so")
		assert.Nil(t, stored.Pending, "it waits on nothing any more")
		assert.Equal(t, 2, stored.Attempts, "the tries it took count until it has stayed what it is expected to be")
		require.NotNil(t, stored.Answer)
		assert.Equal(t, "started", stored.Answer.Output, "the answer is kept, for whoever waits for it elsewhere")
		assert.Equal(t, f.clock.Now(), stored.Metadata.UpdatedAt)

		result, answered := wait.For(ctx, time.Second, 0, nil)
		require.True(t, answered)
		assert.Equal(t, "started", result.Output)
	})

	t.Run("one that failed for good fails its resource, and is not asked for again", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, waiting(kindstest.Starting, kindstest.Running, "start"))

		failed := kind.ResourceActedOn{ID: "command-1", Kind: kindstest.Kind, UUID: "fan-uuid", Action: "start", OK: false, Reason: "the blades are stuck"}

		assert.NoError(t, f.handler.Handle(ctx, message(t, failed)), "redelivered, it would fail the same way")

		stored, _ := f.stored(t, "fan-uuid")
		fan := kindstest.Typed(stored)

		assert.Equal(t, kind.Failed, fan.Status.State)
		assert.Equal(t, "the blades are stuck", fan.Status.Reason)
		assert.Equal(t, kindstest.Running, fan.Status.Expected, "it is brought back by the reconcile loop")
		assert.Equal(t, f.clock.Now(), fan.Status.ObservedAt, "a result that says nothing of when was heard now")
		assert.Nil(t, stored.Pending)
		assert.Equal(t, 2, stored.Attempts, "its tries count towards its backoff")
	})

	t.Run("one that deleted its resource takes its record away", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, waiting(kindstest.Deleting, kind.Deleted, "delete"))

		wait := f.waiters.Expect("command-1")
		defer wait.Done()

		require.NoError(t, f.handler.Handle(ctx, message(t, kind.ResourceActedOn{ID: "command-1", Kind: kindstest.Kind, UUID: "fan-uuid", Action: "delete", OK: true})))

		_, kept := f.stored(t, "fan-uuid")
		assert.False(t, kept)

		_, answered := wait.For(ctx, time.Second, 0, nil)
		assert.True(t, answered)
	})

	t.Run("a result crossing a heartbeat is taken on what the heartbeat wrote, never over it", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, waiting(kindstest.Starting, kindstest.Running, "start"))

		f.racing.Cross(kindstest.Rewrite(f.memory, func(r *resource.Record) {
			r.Metadata.Labels = map[string]string{"seen": "by the heartbeat"}
		}))

		require.NoError(t, f.handler.Handle(ctx, message(t, started)))

		stored, _ := f.stored(t, "fan-uuid")

		assert.Equal(t, kindstest.Running, kindstest.Typed(stored).Status.State)
		assert.Equal(t, "by the heartbeat", stored.Metadata.Labels["seen"])
		assert.Equal(t, int64(3), stored.Version)
	})

	t.Run("one crossed every time it is written is asked for again later", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, waiting(kindstest.Starting, kindstest.Running, "start"))

		wait := f.waiters.Expect("command-1")
		defer wait.Done()

		for range 10 {
			f.racing.Cross(kindstest.Rewrite(f.memory, func(*resource.Record) {}))
		}

		err := f.handler.Handle(ctx, message(t, started))

		assert.ErrorIs(t, err, resource.ErrConflict, "it may go another way next time, so it is redelivered")

		_, answered := wait.For(ctx, 10*time.Millisecond, 0, nil)
		assert.False(t, answered, "whoever waits is told once it is taken")
	})

	t.Run("and so is one the database could not be asked about", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, waiting(kindstest.Starting, kindstest.Running, "start"))
		f.memory.Fail = errors.New("the database is gone")

		assert.ErrorIs(t, f.handler.Handle(ctx, message(t, started)), f.memory.Fail)
	})

	for name, tt := range map[string]struct {
		message func(t *testing.T) []byte
	}{
		"a result for a command its resource no longer waits on is too late to take": {
			message: func(t *testing.T) []byte {
				late := started
				late.ID = "command-from-before"

				return message(t, late)
			},
		},
		"and so is one for a resource that is gone": {
			message: func(t *testing.T) []byte {
				gone := started
				gone.UUID = "another-uuid"

				return message(t, gone)
			},
		},
		"or of a kind not run here": {
			message: func(t *testing.T) []byte {
				kettle := started
				kettle.Kind = "kettle"

				return message(t, kettle)
			},
		},
		"or with a status that is not one": {
			message: func(t *testing.T) []byte {
				broken := started
				broken.Status = json.RawMessage(`["running"]`)

				return message(t, broken)
			},
		},
		"a result that names no resource is nobody's": {
			message: func(t *testing.T) []byte {
				nameless := started
				nameless.UUID = ""

				return message(t, nameless)
			},
		},
		"and one that cannot be read is nothing": {
			message: func(*testing.T) []byte { return []byte(`{"id":`) },
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, waiting(kindstest.Starting, kindstest.Running, "start"))
			before, _ := f.stored(t, "fan-uuid")

			require.NoError(t, f.handler.Handle(ctx, tt.message(t)), "never asked for again: it would be refused the same way")

			after, _ := f.stored(t, "fan-uuid")
			assert.Equal(t, before, after, "nothing is taken")
		})
	}

	t.Run("whoever waits for a command too late to take is still told what came of it", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, waiting(kindstest.Stopping, kindstest.Stopped, "stop"))

		wait := f.waiters.Expect("command-1")
		defer wait.Done()

		require.NoError(t, f.handler.Handle(ctx, message(t, started)))

		result, answered := wait.For(ctx, time.Second, 0, nil)
		require.True(t, answered)
		assert.Equal(t, "started", result.Output)

		stored, _ := f.stored(t, "fan-uuid")
		assert.Equal(t, kindstest.Stopping, kindstest.Typed(stored).Status.State)
	})
}

// restorer records the parents it was told were restored, and as of when,
// and fails as it is told to.
type restorer struct {
	restored []kind.Reference
	at       []time.Time
	failure  error
}

func (r *restorer) Restored(_ context.Context, parent kind.Reference, at time.Time) error {
	if r.failure != nil {
		return r.failure
	}

	r.restored = append(r.restored, parent)
	r.at = append(r.at, at)

	return nil
}

func TestResourceActedOnHandler_Handle_restores(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// the fan's start restores it, as a VM's restore does: what lives in it
	// is what it holds afterwards. Each record waiting on a start was sent
	// it, as the dispatcher keeps a restore's ids on the record it sends it
	// to.
	restoring := func(t *testing.T, told *restorer, records ...resource.Record) *fixture {
		t.Helper()

		for i := range records {
			if records[i].Pending != nil && records[i].Pending.Action == "start" {
				records[i].Restores = slices.Clone(records[i].Pending.IDs)
			}
		}

		f := newFixture(t, records...)

		d := kindstest.Descriptor()
		for i := range d.Actions {
			if d.Actions[i].Name == "start" {
				d.Actions[i].Restores = true
			}
		}

		registry := kind.NewRegistry[kind.ControlPlaneBinding]()
		require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](d, &kindstest.Fans{})))

		f.handler = recordResult.NewResourceActedOnHandler(registry, f.racing, f.waiters, slog.New(slog.DiscardHandler), f.clock.Now, recordResult.WithRestorer(told))

		return f
	}

	restored := kind.ResourceActedOn{ID: "command-1", Kind: kindstest.Kind, UUID: "fan-uuid", Action: "start", OK: true, Status: json.RawMessage(`{"state":"running"}`)}

	t.Run("a restore carried out resets what lives in its resource, and is taken", func(t *testing.T) {
		t.Parallel()

		told := &restorer{}
		f := restoring(t, told, waiting(kindstest.Starting, kindstest.Running, "start"))

		require.NoError(t, f.handler.Handle(ctx, message(t, restored)))

		assert.Equal(t, []kind.Reference{{Kind: kindstest.Kind, UUID: "fan-uuid"}}, told.restored)

		r, _ := f.stored(t, "fan-uuid")
		assert.Equal(t, kindstest.Running, kindstest.Typed(r).Status.State)
	})

	t.Run("one that failed resets nothing", func(t *testing.T) {
		t.Parallel()

		told := &restorer{}
		f := restoring(t, told, waiting(kindstest.Starting, kindstest.Running, "start"))

		failed := restored
		failed.OK, failed.Status, failed.Reason = false, nil, "the archive is from another engine"

		require.NoError(t, f.handler.Handle(ctx, message(t, failed)))

		assert.Empty(t, told.restored)
	})

	t.Run("nor does one the resource is not waiting on", func(t *testing.T) {
		t.Parallel()

		told := &restorer{}
		f := restoring(t, told, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		require.NoError(t, f.handler.Handle(ctx, message(t, restored)))

		assert.Empty(t, told.restored)
	})

	// the fan's start as a VM's restore is answered: saying when its node
	// carried it out, by its own clock.
	restoredAt := kindstest.Moment.Add(time.Minute)

	saying := func(t *testing.T, restoredAt time.Time) kind.ResourceActedOn {
		t.Helper()

		status, err := json.Marshal(struct {
			kindstest.Status
			kind.Restore
		}{Status: kindstest.Status{Status: kind.Status{State: kindstest.Running}}, Restore: kind.Restore{RestoredAt: restoredAt}})
		require.NoError(t, err)

		said := restored
		said.Status = status
		said.At = restoredAt.Add(time.Second)

		return said
	}

	// restoreOf is what a record says of its resource's last restore.
	restoreOf := func(t *testing.T, r resource.Record) time.Time {
		t.Helper()

		restore, err := resource.RestoreOf(r.Status)
		require.NoError(t, err)

		return restore.RestoredAt
	}

	t.Run("what lives in it is reset as of when the restore was carried out, as its result says, and the record says that restore once it is taken", func(t *testing.T) {
		t.Parallel()

		told := &restorer{}
		f := restoring(t, told, waiting(kindstest.Starting, kindstest.Running, "start"))

		require.NoError(t, f.handler.Handle(ctx, message(t, saying(t, restoredAt))))

		require.Len(t, told.at, 1)
		assert.True(t, told.at[0].Equal(restoredAt), "as of the restore, not of when its result was said: %s", told.at[0])

		r, _ := f.stored(t, "fan-uuid")
		assert.True(t, restoreOf(t, r).Equal(restoredAt))
	})

	t.Run("and as of when its result was said, when it says nothing of when it was carried out, which the record says too, for the look into it since to decide", func(t *testing.T) {
		t.Parallel()

		told := &restorer{}
		f := restoring(t, told, waiting(kindstest.Starting, kindstest.Running, "start"))

		said := restored
		said.At = restoredAt

		require.NoError(t, f.handler.Handle(ctx, message(t, said)))

		require.Len(t, told.at, 1)
		assert.True(t, told.at[0].Equal(restoredAt))

		r, _ := f.stored(t, "fan-uuid")
		assert.True(t, restoreOf(t, r).Equal(restoredAt), "the record says the restore it was reset for: %s", restoreOf(t, r))
		assert.Equal(t, kindstest.Running, kindstest.Typed(r).Status.State, "and the rest as its result says")
	})

	t.Run("and so is one its resource waits on no more", func(t *testing.T) {
		t.Parallel()

		told := &restorer{}
		f := restoring(t, told, waiting(kindstest.Starting, kindstest.Running, "start"))

		// failed as its node went quiet, waiting on nothing any more.
		f.racing.Cross(kindstest.Rewrite(f.memory, func(r *resource.Record) {
			require.NoError(t, observe.Fail(r, "node_lost", kindstest.Moment))
		}))

		said := restored
		said.At = restoredAt

		require.NoError(t, f.handler.Handle(ctx, message(t, said)))

		r, _ := f.stored(t, "fan-uuid")
		assert.True(t, restoreOf(t, r).Equal(restoredAt), "the record says the restore it was reset for: %s", restoreOf(t, r))
		require.NotEmpty(t, told.at)
		assert.True(t, told.at[len(told.at)-1].Equal(restoredAt))
	})

	t.Run("one its resource waits on no more, failed as its node went quiet once what lives in it was reset and before the restore was taken onto it, is taken all the same, and only what it says of the restore", func(t *testing.T) {
		t.Parallel()

		told := &restorer{}
		f := restoring(t, told, waiting(kindstest.Starting, kindstest.Running, "start"))

		// the reconcile loop fails it, its node gone quiet, between the
		// reset and the restore's write.
		f.racing.Cross(kindstest.Rewrite(f.memory, func(r *resource.Record) {
			require.NoError(t, observe.Fail(r, "node_lost", kindstest.Moment))
		}))

		require.NoError(t, f.handler.Handle(ctx, message(t, saying(t, restoredAt))))

		r, _ := f.stored(t, "fan-uuid")
		assert.True(t, restoreOf(t, r).Equal(restoredAt), "the restore that reset what lives in it is taken onto it")

		fan := kindstest.Typed(r)
		assert.Equal(t, kind.Failed, fan.Status.State, "and what it is doing is its node's to say next")
		assert.Equal(t, "node_lost", fan.Status.Reason)
		assert.Nil(t, r.Answer, "nor is it answered, which nothing waits on")

		assert.Len(t, told.restored, 2, "what lives in it reset again, as of the same restore")
	})

	t.Run("and so is one heard once its resource was asked something else", func(t *testing.T) {
		t.Parallel()

		// its delete asked as it was restored: the restore's id kept.
		deleting := kindstest.AFan("fan-uuid", kindstest.Deleting, kind.Deleted)
		deleting.Pending = &resource.Pending{Action: "delete", IDs: []string{"delete-1"}, SentAt: kindstest.Moment}
		deleting.Restores = []string{"command-1"}

		told := &restorer{}
		f := restoring(t, told, deleting)

		require.NoError(t, f.handler.Handle(ctx, message(t, saying(t, restoredAt))))

		r, _ := f.stored(t, "fan-uuid")
		assert.True(t, restoreOf(t, r).Equal(restoredAt))
		assert.Equal(t, kindstest.Deleting, kindstest.Typed(r).Status.State, "what it was asked since is as it was")
		require.NotNil(t, r.Pending)
		assert.Equal(t, "delete", r.Pending.Action)
		assert.Empty(t, r.Restores, "the restore is taken")
		assert.Empty(t, told.restored, "and what lives in it, which goes with it, is not reset")
	})

	t.Run("but one heard by a record that was not sent it, made again under its uuid since, takes nothing, and resets nothing", func(t *testing.T) {
		t.Parallel()

		// made again under its uuid once the one restored went: waiting on its
		// create, never sent the restore.
		remade := kindstest.AFan("fan-uuid", kindstest.Pending, kindstest.Running)
		remade.Pending = &resource.Pending{Action: "create", IDs: []string{"create-1"}, SentAt: kindstest.Moment}

		told := &restorer{}
		f := restoring(t, told, remade)

		require.NoError(t, f.handler.Handle(ctx, message(t, saying(t, restoredAt))))

		r, _ := f.stored(t, "fan-uuid")
		assert.True(t, restoreOf(t, r).IsZero(), "nothing of it is taken")
		assert.Empty(t, told.restored, "nor is what lives in it reset")
	})

	t.Run("and one taken lets go of its id, its record waiting on the restore's other tries still", func(t *testing.T) {
		t.Parallel()

		told := &restorer{}
		f := restoring(t, told, waiting(kindstest.Starting, kindstest.Running, "start"))

		require.NoError(t, f.handler.Handle(ctx, message(t, saying(t, restoredAt))))

		r, _ := f.stored(t, "fan-uuid")
		assert.Equal(t, []string{"command-0"}, r.Restores)
	})

	t.Run("but one heard again once it was taken takes nothing, and resets nothing, nor does one of a restore before the last one taken", func(t *testing.T) {
		t.Parallel()

		told := &restorer{}
		f := restoring(t, told, waiting(kindstest.Starting, kindstest.Running, "start"))

		require.NoError(t, f.handler.Handle(ctx, message(t, saying(t, restoredAt))))

		taken, _ := f.stored(t, "fan-uuid")

		require.NoError(t, f.handler.Handle(ctx, message(t, saying(t, restoredAt))))
		require.NoError(t, f.handler.Handle(ctx, message(t, saying(t, restoredAt.Add(-time.Hour)))))

		after, _ := f.stored(t, "fan-uuid")
		assert.Equal(t, taken.Version, after.Version)
		assert.Len(t, told.restored, 1)
	})

	t.Run("and one its resource waits on no more that failed, or is of a restore before the one its record says, takes nothing, and resets nothing, but lets go of its id", func(t *testing.T) {
		t.Parallel()

		for name, heard := range map[string]kind.ResourceActedOn{
			"failed": func() kind.ResourceActedOn {
				failed := saying(t, restoredAt)
				failed.OK, failed.Status, failed.Reason = false, nil, "the archive is from another engine"

				return failed
			}(),
			"of a restore before the one its record says": saying(t, restoredAt.Add(-time.Hour)),
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				// waiting on nothing any more, its record says a restore,
				// and keeps the id of another try at one.
				fan := kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running)

				var err error
				fan.Status, err = resource.WithRestore(fan.Status, kind.Restore{RestoredAt: restoredAt})
				require.NoError(t, err)

				fan.Restores = []string{"command-0", "command-1"}

				told := &restorer{}
				f := restoring(t, told, fan)

				require.NoError(t, f.handler.Handle(ctx, message(t, heard)))

				r, _ := f.stored(t, "fan-uuid")
				assert.Equal(t, []string{"command-0"}, r.Restores, "its id let go of")
				assert.True(t, restoreOf(t, r).Equal(restoredAt), "the restore its record says, as it was")
				assert.Equal(t, kindstest.Running, kindstest.Typed(r).Status.State)
				assert.Empty(t, told.restored)
			})
		}
	})

	t.Run("what could not be reset is asked for again, and the result is not taken until it is", func(t *testing.T) {
		t.Parallel()

		told := &restorer{failure: errors.New("the database is gone")}
		f := restoring(t, told, waiting(kindstest.Starting, kindstest.Running, "start"))

		assert.ErrorIs(t, f.handler.Handle(ctx, message(t, restored)), told.failure)

		r, _ := f.stored(t, "fan-uuid")
		assert.Equal(t, kindstest.Starting, kindstest.Typed(r).Status.State)
		assert.NotNil(t, r.Pending, "still waiting on it, so it is taken when it comes again")

		told.failure = nil

		require.NoError(t, f.handler.Handle(ctx, message(t, restored)))
		assert.Len(t, told.restored, 1)
	})
}

// TestResourceActedOnHandler_Handle_changed holds the results the graphs of
// roots are told of to a delete carried out, once its resource's record is
// gone, a create that failed, and any result that moved its resource's
// state, once that is written down, and to nothing else.
func TestResourceActedOnHandler_Handle_changed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fan := graph.Ref{Kind: kindstest.Kind, UUID: "fan-uuid"}

	// telling is a handler whose results are told to told.
	telling := func(t *testing.T, told *graphtest.Told, records ...resource.Record) *fixture {
		t.Helper()

		f := newFixture(t, records...)
		f.handler = recordResult.NewResourceActedOnHandler(kindstest.Registry(&kindstest.Fans{}), f.racing, f.waiters, slog.New(slog.DiscardHandler), f.clock.Now, recordResult.WithNotifier(told))

		return f
	}

	for name, tt := range map[string]struct {
		record resource.Record
		result kind.ResourceActedOn
		told   []graph.Ref
	}{
		"a delete carried out is told of, its record gone": {
			record: waiting(kindstest.Deleting, kind.Deleted, "delete"),
			result: kind.ResourceActedOn{ID: "command-1", Kind: kindstest.Kind, UUID: "fan-uuid", Action: "delete", OK: true},
			told:   []graph.Ref{fan},
		},
		"and so is a create that failed": {
			record: waiting(kindstest.Starting, kindstest.Running, "create"),
			result: kind.ResourceActedOn{ID: "command-1", Kind: kindstest.Kind, UUID: "fan-uuid", Action: "create", Reason: "its blades are stuck"},
			told:   []graph.Ref{fan},
		},
		"and a create carried out, which moved it: what waited on it is made": {
			record: waiting(kindstest.Starting, kindstest.Running, "create"),
			result: kind.ResourceActedOn{ID: "command-1", Kind: kindstest.Kind, UUID: "fan-uuid", Action: "create", OK: true, Status: json.RawMessage(`{"state":"running"}`)},
			told:   []graph.Ref{fan},
		},
		"and anything else that moved it, a start that failed": {
			record: waiting(kindstest.Starting, kindstest.Running, "start"),
			result: kind.ResourceActedOn{ID: "command-1", Kind: kindstest.Kind, UUID: "fan-uuid", Action: "start", Reason: "its blades are stuck"},
			told:   []graph.Ref{fan},
		},
		"but not a result that left it where it was": {
			record: waiting(kindstest.Stopping, kindstest.Stopped, "stop"),
			result: kind.ResourceActedOn{ID: "command-1", Kind: kindstest.Kind, UUID: "fan-uuid", Action: "stop", OK: true, Status: json.RawMessage(`{"state":"stopping"}`)},
		},
		"nor a result its resource is not waiting on": {
			record: waiting(kindstest.Deleting, kind.Deleted, "delete"),
			result: kind.ResourceActedOn{ID: "command-9", Kind: kindstest.Kind, UUID: "fan-uuid", Action: "delete", OK: true},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			told := &graphtest.Told{}
			f := telling(t, told, tt.record)

			require.NoError(t, f.handler.Handle(ctx, message(t, tt.result)))

			assert.Equal(t, tt.told, told.Changes())
		})
	}

	t.Run("and what cannot be told is no reason to take the result again", func(t *testing.T) {
		t.Parallel()

		told := &graphtest.Told{Failure: errors.New("the database is gone")}
		f := telling(t, told, waiting(kindstest.Deleting, kind.Deleted, "delete"))

		require.NoError(t, f.handler.Handle(ctx, message(t, kind.ResourceActedOn{ID: "command-1", Kind: kindstest.Kind, UUID: "fan-uuid", Action: "delete", OK: true})))

		_, kept := f.stored(t, "fan-uuid")
		assert.False(t, kept)
	})
}
