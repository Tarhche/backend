package resourceResult_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/resourceResult"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/waiters"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

type fixture struct {
	memory  *resourcesMemory.Repository
	racing  *kindstest.Racing
	waiters *waiters.Waiters
	clock   *kindstest.Clock
	handler *resourceResult.Result
}

func newFixture(t *testing.T, records ...resource.Record) *fixture {
	t.Helper()

	f := &fixture{memory: resourcesMemory.NewRepository(), waiters: waiters.New(), clock: kindstest.NewClock()}
	f.racing = &kindstest.Racing{Repository: f.memory}

	for _, r := range records {
		_, err := f.memory.Create(context.Background(), r)
		require.NoError(t, err)
	}

	f.handler = resourceResult.NewResult(kindstest.Registry(&kindstest.Fans{}), f.racing, f.waiters, slog.New(slog.DiscardHandler), f.clock.Now)

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

func message(t *testing.T, result kind.Result) []byte {
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

func TestResult_Handle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	at := kindstest.Moment.Add(time.Minute)

	started := kind.Result{ID: "command-1", Kind: kindstest.Kind, UUID: "fan-uuid", Action: "start", Node: kindstest.NodeName, Attempt: 1, OK: true, Status: json.RawMessage(`{"state":"running","speed":2}`), Output: "started", At: at}

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
		assert.Equal(t, 0, stored.Attempts, "it is what it is expected to be")
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

		failed := kind.Result{ID: "command-1", Kind: kindstest.Kind, UUID: "fan-uuid", Action: "start", OK: false, Reason: "the blades are stuck"}

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

		require.NoError(t, f.handler.Handle(ctx, message(t, kind.Result{ID: "command-1", Kind: kindstest.Kind, UUID: "fan-uuid", Action: "delete", OK: true})))

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
