package reconcileResources_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/observe"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/reconcileResources"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/recordResult"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/waiters"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	nodesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/nodes"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

// now is when the passes in these tests are made: an hour after the fans
// were made, with node-1 heard a moment ago, node-3 twenty seconds ago, after
// it went quiet, and node-2 long ago.
var now = kindstest.Moment.Add(time.Hour)

func config() reconcileResources.Config {
	return reconcileResources.Config{
		Batch:               2,
		NodeSilentAfter:     30 * time.Second,
		ResourceSilentAfter: 30 * time.Second,
		Patience:            5 * time.Minute,
		Backoff:             15 * time.Second,
		MaxBackoff:          15 * time.Minute,
	}
}

type fixture struct {
	resources *resourcesMemory.Repository
	nodes     *nodesMemory.Repository
	producer  *messagingMock.Recorder
	fans      *kindstest.Fans
	houses    houses
	clock     *kindstest.Clock
	useCase   *reconcileResources.UseCase
}

func newFixture(t *testing.T, records ...resource.Record) *fixture {
	t.Helper()

	f := &fixture{
		resources: resourcesMemory.NewRepository(),
		nodes: nodesMemory.NewRepository(
			node.Node{Name: kindstest.NodeName, LastHeartbeatAt: now.Add(-time.Second)},
			node.Node{Name: "node-2", LastHeartbeatAt: now.Add(-time.Hour)},
			node.Node{Name: "node-3", LastHeartbeatAt: now.Add(-20 * time.Second)},
		),
		producer: &messagingMock.Recorder{},
		fans:     &kindstest.Fans{Node: kindstest.NodeName},
		houses:   houses{},
		clock:    kindstest.NewClock(),
	}

	for _, r := range records {
		_, err := f.resources.Create(context.Background(), r)
		require.NoError(t, err)
	}

	f.clock.Advance(time.Hour)

	logger := slog.New(slog.DiscardHandler)
	registry := kindstest.Registry(f.fans)

	dispatcher := dispatch.New(f.resources, f.producer, waiters.New(), f.clock.Now)
	observer := observe.NewObserver(registry, f.resources, logger, observe.WithParents(f.houses))
	f.useCase = reconcileResources.NewUseCase(registry, f.resources, f.nodes, dispatcher, observer, logger, config())

	return f
}

// houses are the houses fans live in that are closed, and what they are: a
// fan in one cannot be looked at, as a container in a VM that is stopped.
type houses map[string]kind.State

func (h houses) Down(_ context.Context, parent kind.Reference) (kind.State, error) {
	if parent.Kind != kindstest.Parent {
		return "", nil
	}

	return h[parent.UUID], nil
}

func (f *fixture) sent(t *testing.T) []kind.ActOnResource {
	t.Helper()

	commands, err := messagingMock.Produced[kind.ActOnResource](f.producer, kind.ActOnResourceName)
	require.NoError(t, err)

	return commands
}

func (f *fixture) stored(t *testing.T, uuid string) (resource.Record, bool) {
	t.Helper()

	r, err := f.resources.GetOne(context.Background(), kindstest.Kind, uuid)
	if err != nil {
		return resource.Record{}, false
	}

	return r, true
}

// fan is a record of a fan for these tests: doing state, expected to be
// expected, on node, heard of a moment ago, as changes say otherwise.
func fan(state kind.State, expected kind.State, onNode string, changes ...func(*resource.Record)) resource.Record {
	r := kindstest.AFan("fan-uuid", state, expected, func(f *kindstest.Fan) {
		f.Metadata.Node = onNode
	})

	heard(time.Second)(&r)

	for _, change := range changes {
		change(&r)
	}

	return r
}

// heard is a record whose node last said what it is doing ago.
func heard(ago time.Duration) func(*resource.Record) {
	return func(r *resource.Record) {
		common, _ := r.Common()
		common.ObservedAt = now.Add(-ago)
		_ = r.SetCommon(common)
	}
}

// unheard is a record nothing was ever heard of.
func unheard(r *resource.Record) {
	common, _ := r.Common()
	common.ObservedAt = time.Time{}
	_ = r.SetCommon(common)
}

func pending(action string, sentAgo time.Duration, attempts int) func(*resource.Record) {
	return func(r *resource.Record) {
		r.Pending = &resource.Pending{Action: action, IDs: []string{"command-1"}, SentAt: now.Add(-sentAgo)}
		r.Attempts = attempts
		r.TriedAt = now.Add(-sentAgo)
	}
}

func tried(attempts int, ago time.Duration) func(*resource.Record) {
	return func(r *resource.Record) {
		r.Attempts = attempts
		r.TriedAt = now.Add(-ago)
	}
}

func since(ago time.Duration) func(*resource.Record) {
	return func(r *resource.Record) {
		common, _ := r.Common()
		common.Since = now.Add(-ago)
		_ = r.SetCommon(common)
	}
}

// reset is a record whose parent was restored from a snapshot since its node
// last saw it there.
func reset(r *resource.Record) {
	r.Reset = true
}

func expiring(at time.Time) func(*resource.Record) {
	return func(r *resource.Record) {
		r.Metadata.Lifetime = time.Hour
		r.Metadata.ExpiresAt = at
	}
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for name, tt := range map[string]struct {
		record   resource.Record
		intents  func(kindstest.Fan) []kind.Intent
		gone     bool
		state    kind.State
		expected kind.State
		reason   string
		sent     []string
		attempt  int
	}{
		// silence
		"a running resource whose node fell silent is failed as lost, and still expected running": {
			record: fan(kindstest.Running, kindstest.Running, "node-2"),
			state:  kind.Failed, expected: kindstest.Running, reason: reconcileResources.ReasonNodeLost,
		},
		"and so is one on its way somewhere, which is not getting there": {
			record: fan(kindstest.Starting, kindstest.Running, "node-2", pending("start", time.Hour, 1)),
			state:  kind.Failed, expected: kindstest.Running, reason: reconcileResources.ReasonNodeLost,
		},
		"and one on a node nobody ever heard of": {
			record: fan(kindstest.Running, kindstest.Running, "node-9"),
			state:  kind.Failed, expected: kindstest.Running, reason: reconcileResources.ReasonNodeLost,
		},
		"but one that ended stays as it ended": {
			record: fan(kindstest.Stopped, kindstest.Stopped, "node-2"),
			state:  kindstest.Stopped, expected: kindstest.Stopped,
		},
		"and one expected deleted on a silent node is forgotten: its delete waits for the node": {
			record: fan(kindstest.Deleting, kind.Deleted, "node-2", pending("delete", time.Hour, 1)),
			gone:   true,
		},
		"once its delete was sent, which a silent node can be sent": {
			record: fan(kind.Failed, kind.Deleted, "node-2"),
			state:  kindstest.Deleting, expected: kind.Deleted, sent: []string{"delete"},
		},
		"and one expected deleted on its way somewhere else is not getting there": {
			record: fan(kindstest.Starting, kind.Deleted, "node-2", pending("start", time.Minute, 1)),
			state:  kind.Failed, expected: kind.Deleted, reason: reconcileResources.ReasonNodeLost,
		},

		// lifetimes
		"one whose lifetime is over is deleted": {
			record: fan(kindstest.Running, kindstest.Running, kindstest.NodeName, expiring(now.Add(-time.Second))),
			state:  kindstest.Deleting, expected: kind.Deleted, sent: []string{"delete"},
		},
		"even on a silent node, where its delete waits for the node": {
			record: fan(kindstest.Running, kindstest.Running, "node-2", expiring(now.Add(-time.Second))),
			state:  kindstest.Deleting, expected: kind.Deleted, sent: []string{"delete"},
		},
		"but not before": {
			record: fan(kindstest.Running, kindstest.Running, kindstest.NodeName, expiring(now.Add(time.Second))),
			state:  kindstest.Running, expected: kindstest.Running,
		},

		// in flight
		"one in flight is waited on while its command may yet be answered": {
			record: fan(kindstest.Starting, kindstest.Running, kindstest.NodeName, pending("start", time.Minute, 1)),
			state:  kindstest.Starting, expected: kindstest.Running,
		},
		"and asked again once it has gone unanswered for longer than it takes": {
			record: fan(kindstest.Starting, kindstest.Running, kindstest.NodeName, pending("start", 10*time.Minute, 1)),
			state:  kindstest.Starting, expected: kindstest.Running, sent: []string{"start"}, attempt: 1,
		},
		"for longer each time it was": {
			record: fan(kindstest.Starting, kindstest.Running, kindstest.NodeName, pending("start", 10*time.Minute, 7)),
			state:  kindstest.Starting, expected: kindstest.Running,
		},
		"one in flight with nothing to wait on is given time": {
			record: fan(kindstest.Pending, kindstest.Running, kindstest.NodeName, since(time.Minute)),
			state:  kindstest.Pending, expected: kindstest.Running,
		},
		"and failed when nothing comes of it": {
			record: fan(kindstest.Pending, kindstest.Running, kindstest.NodeName, since(10*time.Minute)),
			state:  kind.Failed, expected: kindstest.Running, reason: reconcileResources.ReasonStuck,
		},

		// a restored parent
		"one whose parent was restored waits for the next look inside it, rather than being made again": {
			record: fan(kindstest.Stopped, kindstest.Running, kindstest.NodeName, reset),
			state:  kindstest.Stopped, expected: kindstest.Running,
		},

		// backoff
		"one tried for already is left alone a while": {
			record: fan(kind.Failed, kindstest.Running, kindstest.NodeName, tried(1, 5*time.Second)),
			state:  kind.Failed, expected: kindstest.Running,
		},
		"and tried for again after": {
			record: fan(kind.Failed, kindstest.Running, kindstest.NodeName, tried(1, 20*time.Second)),
			state:  kindstest.Starting, expected: kindstest.Running, sent: []string{"start"}, attempt: 1,
		},
		"a while that doubles with every try": {
			record: fan(kind.Failed, kindstest.Running, kindstest.NodeName, tried(3, 50*time.Second)),
			state:  kind.Failed, expected: kindstest.Running,
		},

		// deleting
		"one expected deleted is asked for its delete": {
			record: fan(kind.Failed, kind.Deleted, kindstest.NodeName),
			state:  kindstest.Deleting, expected: kind.Deleted, sent: []string{"delete"},
		},
		"and one that got there already is forgotten": {
			record: fan(kind.Deleted, kind.Deleted, kindstest.NodeName),
			gone:   true,
		},

		// the kind's decisions
		"one not doing what it is expected to is asked for what its kind says would make it": {
			record: fan(kindstest.Stopped, kindstest.Running, kindstest.NodeName),
			state:  kindstest.Starting, expected: kindstest.Running, sent: []string{"start"},
		},
		"whichever way round": {
			record: fan(kindstest.Running, kindstest.Stopped, kindstest.NodeName),
			state:  kindstest.Stopping, expected: kindstest.Stopped, sent: []string{"stop"},
		},
		"one doing what it is expected to is asked nothing": {
			record: fan(kindstest.Running, kindstest.Running, kindstest.NodeName),
			state:  kindstest.Running, expected: kindstest.Running,
		},
		"what its state does not allow is not asked": {
			record:  fan(kindstest.Stopped, kindstest.Stopped, kindstest.NodeName),
			intents: func(kindstest.Fan) []kind.Intent { return []kind.Intent{{Action: "stop"}} },
			state:   kindstest.Stopped, expected: kindstest.Stopped,
		},
		"what runs in the control plane is carried out in place, and what follows it asked": {
			record: fan(kindstest.Stopped, kindstest.Running, kindstest.NodeName),
			intents: func(kindstest.Fan) []kind.Intent {
				return []kind.Intent{
					{Action: "rename", Payload: kindstest.RenamePayload{Name: "hall"}},
					{Action: "start"},
				}
			},
			state: kindstest.Starting, expected: kindstest.Running, sent: []string{"start"},
		},
		"and only one command for its node at a time": {
			record: fan(kindstest.Stopped, kindstest.Running, kindstest.NodeName),
			intents: func(kindstest.Fan) []kind.Intent {
				return []kind.Intent{{Action: "start"}, {Action: "delete"}}
			},
			state: kindstest.Starting, expected: kindstest.Running, sent: []string{"start"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, tt.record)
			f.fans.Intents = tt.intents

			require.NoError(t, f.useCase.Execute(ctx))

			stored, kept := f.stored(t, "fan-uuid")

			var actions []string
			for _, command := range f.sent(t) {
				actions = append(actions, command.Action)

				assert.Equal(t, tt.attempt, command.Attempt, "attempt")
				assert.Equal(t, stored.Metadata.Node, command.Node, "it is addressed to the node holding it")
			}

			assert.Equal(t, tt.sent, actions, "sent")

			if tt.gone {
				assert.False(t, kept, "it is gone")

				return
			}

			require.True(t, kept)

			fan := kindstest.Typed(stored)
			assert.Equal(t, tt.state, fan.Status.State, "state")
			assert.Equal(t, tt.expected, fan.Status.Expected, "expected")
			assert.Equal(t, tt.reason, fan.Status.Reason, "reason")

			if len(tt.sent) > 0 {
				require.NotNil(t, stored.Pending, "what was sent is what it waits on")
				assert.Contains(t, stored.Pending.IDs, f.sent(t)[0].ID)
			}
		})
	}

	t.Run("a resource asked again is sent the same command under an ID of its own, which answers it as well", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, fan(kindstest.Starting, kindstest.Running, kindstest.NodeName, pending("start", 10*time.Minute, 1)))

		require.NoError(t, f.useCase.Execute(ctx))

		stored, _ := f.stored(t, "fan-uuid")
		sent := f.sent(t)

		require.Len(t, sent, 1)
		assert.Equal(t, []string{"command-1", sent[0].ID}, stored.Pending.IDs)
		assert.Equal(t, now, stored.Pending.SentAt)
		assert.Equal(t, 2, stored.Attempts)
	})

	t.Run("the tries it took are forgotten once it has stayed what it is expected to be for as long as the last wait", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t,
			kindstest.AFan("settled", kindstest.Running, kindstest.Running),
			kindstest.AFan("just-back", kindstest.Running, kindstest.Running),
		)

		for _, uuid := range []string{"settled", "just-back"} {
			r, _ := f.stored(t, uuid)
			heard(time.Second)(&r)
			_, err := f.resources.Update(ctx, r)
			require.NoError(t, err)
		}

		settled, _ := f.stored(t, "settled")
		tried(3, time.Hour)(&settled)
		since(time.Minute)(&settled)
		_, err := f.resources.Update(ctx, settled)
		require.NoError(t, err)

		justBack, _ := f.stored(t, "just-back")
		tried(3, time.Hour)(&justBack)
		since(30 * time.Second)(&justBack)
		_, err = f.resources.Update(ctx, justBack)
		require.NoError(t, err)

		require.NoError(t, f.useCase.Execute(ctx))

		settled, _ = f.stored(t, "settled")
		assert.Equal(t, 0, settled.Attempts, "a minute is as long as the third wait")

		justBack, _ = f.stored(t, "just-back")
		assert.Equal(t, 3, justBack.Attempts, "half a minute is not")
		assert.Empty(t, f.sent(t), "neither is asked anything")
	})

	t.Run("so one that falls over as soon as it is brought back is brought back less and less often", func(t *testing.T) {
		t.Parallel()

		// brought back by its third try half a minute ago, and stopped again
		// since: the fourth waits a minute after the third, as long again as
		// the third waited after the second.
		f := newFixture(t, fan(kindstest.Stopped, kindstest.Running, kindstest.NodeName, tried(3, 30*time.Second), since(10*time.Second)))

		require.NoError(t, f.useCase.Execute(ctx))
		assert.Empty(t, f.sent(t))

		g := newFixture(t, fan(kindstest.Stopped, kindstest.Running, kindstest.NodeName, tried(3, time.Minute), since(10*time.Second)))

		require.NoError(t, g.useCase.Execute(ctx))
		require.Len(t, g.sent(t), 1)
		assert.Equal(t, 3, g.sent(t)[0].Attempt)
	})

	t.Run("a pass covers every resource, a batch at a time", func(t *testing.T) {
		t.Parallel()

		var records []resource.Record
		for i := range 5 {
			record := kindstest.AFan(fmt.Sprintf("fan-%d", i), kindstest.Stopped, kindstest.Running)
			heard(time.Second)(&record)

			records = append(records, record)
		}

		f := newFixture(t, records...)

		require.NoError(t, f.useCase.Execute(ctx))

		assert.Len(t, f.sent(t), 5)
	})

	t.Run("one the kind could not decide about is no reason to leave the rest", func(t *testing.T) {
		t.Parallel()

		thinking := kindstest.AFan("fan-1", kindstest.Stopped, kindstest.Running)
		heard(time.Second)(&thinking)

		f := newFixture(t,
			thinking,
			kindstest.AFan("fan-2", kindstest.Running, kindstest.Running, func(f *kindstest.Fan) { f.Metadata.Node = "node-2" }),
		)
		f.fans.Failure = errors.New("the kind cannot think")

		require.NoError(t, f.useCase.Execute(ctx))

		lost, _ := f.stored(t, "fan-2")
		assert.Equal(t, kind.Failed, kindstest.Typed(lost).Status.State, "the silent one is failed all the same")
		assert.Empty(t, f.sent(t))
	})

	t.Run("with no kind registered, a pass does nothing at all", func(t *testing.T) {
		t.Parallel()

		nodes := nodesMemory.NewRepository()
		nodes.Fail = errors.New("the database is gone")

		resources := resourcesMemory.NewRepository()
		dispatcher := dispatch.New(resources, &messagingMock.Recorder{}, nil, nil)

		useCase := reconcileResources.NewUseCase(kind.NewRegistry[kind.ControlPlaneBinding](), resources, nodes, dispatcher, nil, slog.New(slog.DiscardHandler), config())

		assert.NoError(t, useCase.Execute(ctx))
	})

	t.Run("a pass that cannot tell which nodes are alive fails", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-1", kindstest.Running, kindstest.Running))
		f.nodes.Fail = errors.New("the database is gone")

		assert.ErrorIs(t, f.useCase.Execute(ctx), f.nodes.Fail)
	})

	t.Run("and one that cannot read the resources says so", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-1", kindstest.Running, kindstest.Running))
		f.resources.Fail = errors.New("the database is gone")

		assert.ErrorIs(t, f.useCase.Execute(ctx), f.resources.Fail)
	})
}

// TestUseCase_Execute_unheard holds the loop to taking a resource its node
// has gone on beating without a word of for long enough to be gone from the
// node, as the node's silence about it is taken: missing, waiting on its
// parent when the parent is not running, or forgotten when the parent was
// restored since it was last heard of. No heartbeat says what a node does not
// hold, so nothing else says it.
func TestUseCase_Execute_unheard(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for name, tt := range map[string]struct {
		record   resource.Record
		closed   bool
		gone     bool
		state    kind.State
		expected kind.State
		reason   string
		sent     []string
	}{
		"one its node has gone on beating without a word of for longer than it may is missing": {
			record: fan(kindstest.Running, kindstest.Running, kindstest.NodeName, heard(40*time.Second)),
			state:  kind.Missing, expected: kindstest.Running, reason: "its house has none of it",
		},
		"and is asked for what it is missing at the next pass, not this one": {
			record: fan(kindstest.Stopped, kindstest.Running, kindstest.NodeName, heard(40*time.Second)),
			state:  kind.Missing, expected: kindstest.Running, reason: "its house has none of it",
		},
		"one heard of lately is as it was heard": {
			record: fan(kindstest.Running, kindstest.Running, kindstest.NodeName, heard(20*time.Second)),
			state:  kindstest.Running, expected: kindstest.Running,
		},
		"and so is one whose node, alive, has not beaten for as long since it was last heard of: a node gone quiet says nothing of what it holds": {
			record: fan(kindstest.Running, kindstest.Running, "node-3", heard(40*time.Second)),
			state:  kindstest.Running, expected: kindstest.Running,
		},
		"one on a node that fell silent is lost with it, not missing": {
			record: fan(kindstest.Running, kindstest.Running, "node-2", heard(2*time.Hour)),
			state:  kind.Failed, expected: kindstest.Running, reason: reconcileResources.ReasonNodeLost,
		},
		"one in a house that is not open waits on it, rather than being missing": {
			record: fan(kindstest.Running, kindstest.Running, kindstest.NodeName, heard(40*time.Second)),
			closed: true,
			state:  kind.Waiting, expected: kindstest.Running, reason: "its house is closed",
		},
		"one whose house was restored since it was last heard of is forgotten, not made again": {
			record: fan(kindstest.Running, kindstest.Running, kindstest.NodeName, heard(40*time.Second), reset),
			gone:   true,
		},
		"but only once it has gone unheard for long enough": {
			record: fan(kindstest.Running, kindstest.Running, kindstest.NodeName, heard(20*time.Second), reset),
			state:  kindstest.Running, expected: kindstest.Running,
		},
		"one never heard of yet, made a moment ago, has as long as any other to be heard of": {
			record: fan(kindstest.Running, kindstest.Running, kindstest.NodeName, unheard, since(10*time.Second)),
			state:  kindstest.Running, expected: kindstest.Running,
		},
		"as has one moved a moment ago by what it was asked": {
			record: fan(kindstest.Stopped, kindstest.Stopped, kindstest.NodeName, heard(40*time.Second), since(10*time.Second)),
			state:  kindstest.Stopped, expected: kindstest.Stopped,
		},
		"but one never heard of since it was made long ago is missing": {
			record: fan(kindstest.Running, kindstest.Running, kindstest.NodeName, unheard, since(40*time.Second)),
			state:  kind.Missing, expected: kindstest.Running, reason: "its house has none of it",
		},
		"and one in flight is its command's, however long its node says nothing of it": {
			record: fan(kindstest.Starting, kindstest.Running, kindstest.NodeName, heard(time.Hour), pending("start", time.Minute, 1)),
			state:  kindstest.Starting, expected: kindstest.Running,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, tt.record)
			if tt.closed {
				f.houses[kindstest.House] = "closed"
			}

			require.NoError(t, f.useCase.Execute(ctx))

			stored, kept := f.stored(t, "fan-uuid")

			if tt.gone {
				assert.False(t, kept, "it is gone, record and all")

				return
			}

			require.True(t, kept)

			fan := kindstest.Typed(stored)
			assert.Equal(t, tt.state, fan.Status.State, "state")
			assert.Equal(t, tt.expected, fan.Status.Expected, "expected")
			assert.Equal(t, tt.reason, fan.Status.Reason, "reason")
			assert.Empty(t, f.sent(t), "nothing is asked of it in the pass that takes it to be gone")
		})
	}

	t.Run("it is taken to be gone as of its node's last beat, and asked for what it is missing at the next pass", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, fan(kindstest.Running, kindstest.Running, kindstest.NodeName, heard(40*time.Second)))

		require.NoError(t, f.useCase.Execute(ctx))

		missing, _ := f.stored(t, "fan-uuid")
		assert.Equal(t, kind.Missing, kindstest.Typed(missing).Status.State)
		assert.Equal(t, now.Add(-time.Second), kindstest.Typed(missing).Status.ObservedAt, "its node's last beat, in its node's own time")

		require.NoError(t, f.useCase.Execute(ctx))

		var actions []string
		for _, command := range f.sent(t) {
			actions = append(actions, command.Action)
		}

		assert.Equal(t, []string{"create"}, actions)
	})
}

// TestUseCase_Execute_heardOfByItsResult holds a resource just acted on to
// having as long as any other to be heard of, counted from when the result of
// what it was asked said what it is: a command's result is a word of it.
func TestUseCase_Execute_heardOfByItsResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// asked to start an hour ago, and never heard of since.
	f := newFixture(t, fan(kindstest.Starting, kindstest.Running, kindstest.NodeName, unheard, since(time.Hour), pending("start", time.Hour, 1)))

	status, err := json.Marshal(kindstest.Status{Status: kind.Status{State: kindstest.Running}, Speed: 2})
	require.NoError(t, err)

	answeredAt := now.Add(-10 * time.Second)

	result, err := json.Marshal(kind.ResourceActedOn{ID: "command-1", Kind: kindstest.Kind, UUID: "fan-uuid", Action: "start", Node: kindstest.NodeName, OK: true, Status: status, At: answeredAt})
	require.NoError(t, err)

	require.NoError(t, recordResult.NewResourceActedOnHandler(kindstest.Registry(f.fans), f.resources, nil, slog.New(slog.DiscardHandler), f.clock.Now).Handle(ctx, result))

	answered, _ := f.stored(t, "fan-uuid")
	assert.Equal(t, answeredAt, kindstest.Typed(answered).Status.ObservedAt, "what its result said is a word of it")

	require.NoError(t, f.useCase.Execute(ctx))

	running, _ := f.stored(t, "fan-uuid")
	assert.Equal(t, kindstest.Running, kindstest.Typed(running).Status.State, "it is not taken to be gone before it could be heard of")

	// half a minute on, its node has beaten on without a word of it.
	f.clock.Advance(30 * time.Second)

	beating, err := f.nodes.GetOne(ctx, kindstest.NodeName)
	require.NoError(t, err)

	beating.LastHeartbeatAt = f.clock.Now().Add(-time.Second)
	_, err = f.nodes.Save(ctx, &beating)
	require.NoError(t, err)

	require.NoError(t, f.useCase.Execute(ctx))

	missing, _ := f.stored(t, "fan-uuid")
	assert.Equal(t, kind.Missing, kindstest.Typed(missing).Status.State, "and once it has gone unheard for long enough since, it is gone")
}

// TestUseCase_Execute_stateKeptInTheControlPlane holds a kind whose state
// the control plane keeps, as a snapshot's, to its node speaking only for the
// command it was sent: a resource waiting on that command is lost with the
// node, and one at rest is not, however long its node has been silent.
func TestUseCase_Execute_stateKeptInTheControlPlane(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// a fan whose state is its record's, as a snapshot's is.
	ledger := kindstest.Descriptor()
	ledger.StateBy = kind.OnControlPlane

	for i := range ledger.Actions {
		if ledger.Actions[i].Name == "state" {
			ledger.Actions[i].Runs = kind.OnControlPlane
		}
	}

	for name, tt := range map[string]struct {
		record resource.Record
		state  kind.State
		reason string
	}{
		"one waiting on a command its silent node will not answer is lost with it": {
			record: fan(kindstest.Starting, kindstest.Running, "node-2", pending("start", time.Minute, 1)),
			state:  kind.Failed, reason: reconcileResources.ReasonNodeLost,
		},
		"and one at rest is not: the node does not speak for it": {
			record: fan(kindstest.Running, kindstest.Running, "node-2"),
			state:  kindstest.Running,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			resources := resourcesMemory.NewRepository()

			_, err := resources.Create(ctx, tt.record)
			require.NoError(t, err)

			registry := kind.NewRegistry[kind.ControlPlaneBinding]()
			require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](ledger, &kindstest.Fans{Node: kindstest.NodeName})))

			nodes := nodesMemory.NewRepository(
				node.Node{Name: kindstest.NodeName, LastHeartbeatAt: now.Add(-time.Second)},
				node.Node{Name: "node-2", LastHeartbeatAt: now.Add(-time.Hour)},
			)

			clock := kindstest.NewClock()
			clock.Advance(time.Hour)

			producer := &messagingMock.Recorder{}
			dispatcher := dispatch.New(resources, producer, waiters.New(), clock.Now)

			observer := observe.NewObserver(registry, resources, slog.New(slog.DiscardHandler))

			require.NoError(t, reconcileResources.NewUseCase(registry, resources, nodes, dispatcher, observer, slog.New(slog.DiscardHandler), config()).Execute(ctx))

			stored, err := resources.GetOne(ctx, kindstest.Kind, "fan-uuid")
			require.NoError(t, err)

			fan := kindstest.Typed(stored)
			assert.Equal(t, tt.state, fan.Status.State, "state")
			assert.Equal(t, tt.reason, fan.Status.Reason, "reason")
			assert.Equal(t, kindstest.Running, fan.Status.Expected, "it is still to be what it was")
			assert.Empty(t, producer.Messages(), "nothing is asked of a silent node")
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	c := reconcileResources.DefaultConfig()

	assert.Positive(t, c.Batch)
	assert.Equal(t, 30*time.Second, c.NodeSilentAfter, "the silence a VM's node is taken to be lost after, as it always was")
	assert.Equal(t, 30*time.Second, c.ResourceSilentAfter, "and what its node holds to be gone after, while the node beats")
	assert.Greater(t, c.MaxBackoff, c.Backoff)
	assert.Greater(t, c.Patience, c.Backoff)
}

// TestUseCase_Execute_timeouts holds the loop to waiting on a command its
// node may take long over, an image pulled first or a disk streamed to the
// bucket, for as long as the node gives it, and then its patience, before it
// asks for it again: asked again sooner, it would hold another of the node's
// command slots, waiting behind the first for the resource's lock.
func TestUseCase_Execute_timeouts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// a fan whose start may pull an image first, which its node gives 13
	// minutes, as nodes give a pull by default.
	d := kindstest.Descriptor()
	for i := range d.Actions {
		if d.Actions[i].Name == "start" {
			d.Actions[i].Timeout = kind.TimeoutPull
		}
	}

	settings := config()
	settings.Timeouts = map[kind.Timeout]time.Duration{kind.TimeoutPull: 13 * time.Minute}

	for name, tt := range map[string]struct {
		record resource.Record
		sent   []string
	}{
		"a command given a timeout is waited on past the loop's patience": {
			record: fan(kindstest.Starting, kindstest.Running, kindstest.NodeName, pending("start", 10*time.Minute, 1)),
		},
		"for as long as its node gives it, and then that patience": {
			record: fan(kindstest.Starting, kindstest.Running, kindstest.NodeName, pending("start", 17*time.Minute, 1)),
		},
		"and is asked again once that has passed": {
			record: fan(kindstest.Starting, kindstest.Running, kindstest.NodeName, pending("start", 19*time.Minute, 1)),
			sent:   []string{"start"},
		},
		"while one given none is asked again after the patience alone": {
			record: fan(kindstest.Stopping, kindstest.Stopped, kindstest.NodeName, pending("stop", 10*time.Minute, 1)),
			sent:   []string{"stop"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, tt.record)

			registry := kind.NewRegistry[kind.ControlPlaneBinding]()
			require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](d, f.fans)))

			clock := kindstest.NewClock()
			clock.Advance(time.Hour)

			dispatcher := dispatch.New(f.resources, f.producer, waiters.New(), clock.Now)
			observer := observe.NewObserver(registry, f.resources, slog.New(slog.DiscardHandler))
			useCase := reconcileResources.NewUseCase(registry, f.resources, f.nodes, dispatcher, observer, slog.New(slog.DiscardHandler), settings)

			require.NoError(t, useCase.Execute(ctx))

			var actions []string
			for _, command := range f.sent(t) {
				actions = append(actions, command.Action)
			}

			assert.Equal(t, tt.sent, actions)
		})
	}
}
