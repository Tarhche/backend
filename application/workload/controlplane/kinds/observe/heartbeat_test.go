package observe_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/observe"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/graph/graphtest"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

// instance is a fan as its node says it is, in the house fans are in.
func instance(t *testing.T, uuid string, s kindstest.Status) kind.Observation {
	t.Helper()

	return kind.Observation{
		Kind:   kindstest.Kind,
		UUID:   uuid,
		Owners: []kind.Reference{{Kind: kindstest.Parent, UUID: kindstest.House}},
		Status: status(t, s),
	}
}

func stored(t *testing.T, repository resource.Repository, uuid string) (resource.Record, bool) {
	t.Helper()

	r, err := repository.GetOne(context.Background(), kindstest.Kind, uuid)
	if err != nil {
		return resource.Record{}, false
	}

	return r, true
}

func create(t *testing.T, repository resource.Repository, records ...resource.Record) {
	t.Helper()

	for _, r := range records {
		_, err := repository.Create(context.Background(), r)
		require.NoError(t, err)
	}
}

// heardAt is a fan whose node last said what it is doing at a moment.
func heardAt(at time.Time) func(*kindstest.Fan) {
	return func(f *kindstest.Fan) {
		f.Status.ObservedAt = at
	}
}

func TestObserver_Heartbeat(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := slog.New(slog.DiscardHandler)

	t.Run("what a node says of what it holds is taken, one instance at a time, and of nothing else", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()
		create(t, repository,
			kindstest.AFan("running", kindstest.Running, kindstest.Running),
			kindstest.AFan("starting", kindstest.Starting, kindstest.Running),
			kindstest.AFan("stopped", kindstest.Stopped, kindstest.Stopped),
			kindstest.AFan("elsewhere", kindstest.Running, kindstest.Running, func(f *kindstest.Fan) { f.Metadata.Node = "node-2" }),
			kindstest.AFan("unsaid", kindstest.Running, kindstest.Running),
		)

		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger)

		for uuid, s := range map[string]kindstest.Status{
			"running":   observed(kindstest.Running, 3),
			"starting":  observed(kindstest.Running, 1),
			"stopped":   observed(kindstest.Stopped, 0),
			"elsewhere": observed(kindstest.Stopped, 0),
		} {
			observer.Heartbeat(ctx, kindstest.NodeName, later, instance(t, uuid, s))
		}

		running, _ := stored(t, repository, "running")
		assert.Equal(t, 3, kindstest.Typed(running).Status.Speed)
		assert.Equal(t, later, kindstest.Typed(running).Status.ObservedAt)

		starting, _ := stored(t, repository, "starting")
		assert.Equal(t, kindstest.Running, kindstest.Typed(starting).Status.State, "it arrived")

		elsewhere, _ := stored(t, repository, "elsewhere")
		assert.Equal(t, kindstest.Running, kindstest.Typed(elsewhere).Status.State, "another node's word for it is not its node's")
		assert.True(t, kindstest.Typed(elsewhere).Status.ObservedAt.IsZero())

		unsaid, _ := stored(t, repository, "unsaid")
		assert.Equal(t, kindstest.Running, kindstest.Typed(unsaid).Status.State, "what no heartbeat says is not concluded from any of them")
		assert.Equal(t, int64(1), unsaid.Version, "it is not even written")
	})

	t.Run("one whose parent was restored is as it is found once it is heard of, and a look taken before the restore says nothing of it", func(t *testing.T) {
		t.Parallel()

		restored := func(r resource.Record) resource.Record {
			r.Reset = true

			return r
		}

		repository := resourcesMemory.NewRepository()
		create(t, repository,
			restored(kindstest.AFan("found", kindstest.Running, kindstest.Running, heardAt(later))),
			restored(kindstest.AFan("seen-before-the-restore", kindstest.Running, kindstest.Running, heardAt(later))),
		)

		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger)

		observer.Heartbeat(ctx, kindstest.NodeName, later.Add(time.Second), instance(t, "found", observed(kindstest.Stopped, 0)))
		observer.Heartbeat(ctx, kindstest.NodeName, later.Add(-time.Second), instance(t, "seen-before-the-restore", observed(kindstest.Stopped, 0)))

		found, _ := stored(t, repository, "found")
		assert.Equal(t, kindstest.Stopped, kindstest.Typed(found).Status.State, "it is as it was found")
		assert.False(t, found.Reset, "and is reconciled as anything else from now on")

		before, _ := stored(t, repository, "seen-before-the-restore")
		assert.Equal(t, kindstest.Running, kindstest.Typed(before).Status.State)
		assert.True(t, before.Reset, "it is not taken to be on the restored disk")
	})

	t.Run("a heartbeat made before a resource was asked what it is on its way to says nothing of it", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()
		create(t, repository, kindstest.AFan("fan-uuid", kindstest.Starting, kindstest.Running, func(f *kindstest.Fan) { f.Status.Since = later }))

		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger)

		observer.Heartbeat(ctx, kindstest.NodeName, later.Add(-time.Second), instance(t, "fan-uuid", observed(kindstest.Running, 2)))

		fan, _ := stored(t, repository, "fan-uuid")
		assert.Equal(t, kindstest.Starting, kindstest.Typed(fan).Status.State)
		assert.Equal(t, int64(1), fan.Version)

		observer.Heartbeat(ctx, kindstest.NodeName, later.Add(time.Second), instance(t, "fan-uuid", observed(kindstest.Running, 2)))

		fan, _ = stored(t, repository, "fan-uuid")
		assert.Equal(t, kindstest.Running, kindstest.Typed(fan).Status.State, "one made after is")
	})

	t.Run("a kind the control plane does not run, or whose state is not its nodes' to say, is not listened to", func(t *testing.T) {
		t.Parallel()

		// a fan whose state is its record's, as a snapshot's is.
		ledger := kindstest.Descriptor()
		ledger.StateBy = kind.OnControlPlane

		for i := range ledger.Actions {
			if ledger.Actions[i].Name == "state" {
				ledger.Actions[i].Runs = kind.OnControlPlane
			}
		}

		kept := kind.NewRegistry[kind.ControlPlaneBinding]()
		require.NoError(t, kept.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](ledger, &kindstest.Fans{})))

		for name, tt := range map[string]struct {
			registry *kind.Registry[kind.ControlPlaneBinding]
			kind     string
		}{
			"not run":               {registry: kindstest.Registry(&kindstest.Fans{}), kind: "kettle"},
			"not its nodes' to say": {registry: kept, kind: kindstest.Kind},
		} {
			repository := resourcesMemory.NewRepository()
			create(t, repository, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

			said := instance(t, "fan-uuid", observed(kindstest.Stopped, 0))
			said.Kind = tt.kind

			observe.NewObserver(tt.registry, repository, logger).Heartbeat(ctx, kindstest.NodeName, later, said)

			fan, _ := stored(t, repository, "fan-uuid")
			assert.Equal(t, kindstest.Running, kindstest.Typed(fan).Status.State, name)
			assert.Equal(t, int64(1), fan.Version, name)
		}
	})

	t.Run("a heartbeat older than what was last heard is not taken", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()
		create(t, repository, kindstest.AFan("fan-uuid", kindstest.Stopped, kindstest.Stopped, heardAt(later)))

		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger)

		observer.Heartbeat(ctx, kindstest.NodeName, later.Add(-time.Second), instance(t, "fan-uuid", observed(kindstest.Running, 1)))

		fan, _ := stored(t, repository, "fan-uuid")
		assert.Equal(t, kindstest.Stopped, kindstest.Typed(fan).Status.State)
	})

	t.Run("a node saying again what it said is written down now and then, not every beat", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()
		create(t, repository, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger)
		running := instance(t, "fan-uuid", observed(kindstest.Running, 1))

		observer.Heartbeat(ctx, kindstest.NodeName, later, running)

		fan, _ := stored(t, repository, "fan-uuid")
		assert.Equal(t, int64(2), fan.Version, "first heard, it is written")

		observer.Heartbeat(ctx, kindstest.NodeName, later.Add(time.Second), running)

		fan, _ = stored(t, repository, "fan-uuid")
		assert.Equal(t, int64(2), fan.Version, "a second later, the same is not")
		assert.Equal(t, later, kindstest.Typed(fan).Status.ObservedAt)

		observer.Heartbeat(ctx, kindstest.NodeName, later.Add(time.Minute), running)

		fan, _ = stored(t, repository, "fan-uuid")
		assert.Equal(t, int64(3), fan.Version, "a while later, it is, so when it was observed stays roughly true")
		assert.Equal(t, later.Add(time.Minute), kindstest.Typed(fan).Status.ObservedAt)
	})

	t.Run("and as often as it is told to be, so that one its node keeps saying is never taken to be unheard", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()
		create(t, repository, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger, observe.WithRefresh(time.Second))
		running := instance(t, "fan-uuid", observed(kindstest.Running, 1))

		observer.Heartbeat(ctx, kindstest.NodeName, later, running)
		observer.Heartbeat(ctx, kindstest.NodeName, later.Add(500*time.Millisecond), running)

		fan, _ := stored(t, repository, "fan-uuid")
		assert.Equal(t, int64(2), fan.Version)

		observer.Heartbeat(ctx, kindstest.NodeName, later.Add(time.Second), running)

		fan, _ = stored(t, repository, "fan-uuid")
		assert.Equal(t, int64(3), fan.Version)
		assert.Equal(t, later.Add(time.Second), kindstest.Typed(fan).Status.ObservedAt)
	})

	t.Run("a heartbeat crossing a command is taken on what the command left, never over it", func(t *testing.T) {
		t.Parallel()

		memory := resourcesMemory.NewRepository()
		create(t, memory, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		racing := &kindstest.Racing{Repository: memory}

		// a stop is asked for between the heartbeat reading the fan and
		// writing it back.
		racing.Cross(kindstest.Rewrite(memory, func(r *resource.Record) {
			common, _ := r.Common()
			common.State = kindstest.Stopping
			common.Expected = kindstest.Stopped
			_ = r.SetCommon(common)
			r.Pending = &resource.Pending{Action: "stop", IDs: []string{"command-1"}}
		}))

		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), racing, logger)

		observer.Heartbeat(ctx, kindstest.NodeName, later, instance(t, "fan-uuid", observed(kindstest.Running, 2)))

		fan, _ := stored(t, memory, "fan-uuid")

		assert.Equal(t, kindstest.Stopping, kindstest.Typed(fan).Status.State, "the stop is not undone")
		assert.Equal(t, kindstest.Stopped, kindstest.Typed(fan).Status.Expected)
		assert.NotNil(t, fan.Pending, "and is still waited on")
		assert.Equal(t, 2, kindstest.Typed(fan).Status.Speed, "what was observed of it is still taken")
		assert.Equal(t, int64(3), fan.Version)
	})

	t.Run("one that moved to another node in the meantime is that node's to speak for", func(t *testing.T) {
		t.Parallel()

		memory := resourcesMemory.NewRepository()
		create(t, memory, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		racing := &kindstest.Racing{Repository: memory}
		racing.Cross(kindstest.Rewrite(memory, func(r *resource.Record) { r.Metadata.Node = "node-2" }))

		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), racing, logger)

		observer.Heartbeat(ctx, kindstest.NodeName, later, instance(t, "fan-uuid", observed(kindstest.Stopped, 0)))

		fan, _ := stored(t, memory, "fan-uuid")
		assert.Equal(t, kindstest.Running, kindstest.Typed(fan).Status.State)
		assert.Equal(t, "node-2", fan.Metadata.Node)
	})
}

// TestObserver_Unheard holds a node's silence about a resource, once it has
// gone on for long enough, to being taken as the resource being gone from the
// node, the same way for every kind: missing, waiting on its parent when the
// parent is down, or forgotten when the parent was restored since it was last
// heard of; and to being taken by the guards anything its node says is.
func TestObserver_Unheard(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := slog.New(slog.DiscardHandler)

	// when the node, beating on, has long said nothing of what it holds.
	silence := later.Add(time.Minute)

	unhoused := func(f *kindstest.Fan) { f.Metadata.Owners, f.Metadata.Refs = nil, nil }
	inHouse := func(house string) func(f *kindstest.Fan) {
		return func(f *kindstest.Fan) { f.Metadata.Refs = []kind.Reference{{Kind: kindstest.Parent, UUID: house}} }
	}

	// keptInHouse is a fan kept before refs: the house it is in is among its
	// owners.
	keptInHouse := func(house string) func(f *kindstest.Fan) {
		return func(f *kindstest.Fan) {
			f.Metadata.Owners, f.Metadata.Refs = []kind.Reference{{Kind: kindstest.Parent, UUID: house}}, nil
		}
	}

	for name, tt := range map[string]struct {
		record resource.Record
		houses parents
		gone   bool
		state  kind.State
		reason string
		since  time.Time
	}{
		"what its node has long said nothing of is missing from its house": {
			record: kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running, heardAt(later)),
			state:  kind.Missing, reason: "its house has none of it", since: silence,
		},
		"and one that lives in nothing from its node": {
			record: kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running, heardAt(later), unhoused),
			state:  kind.Missing, since: silence,
		},
		"one on its way out is gone, record and all": {
			record: kindstest.AFan("fan-uuid", kindstest.Deleting, kind.Deleted, heardAt(later)),
			gone:   true,
		},
		"one on its way somewhere is as its machine takes it: not there yet": {
			record: kindstest.AFan("fan-uuid", kindstest.Starting, kindstest.Running, heardAt(later)),
			state:  kindstest.Starting, since: kindstest.Moment,
		},
		"one in a house that is down waits on it, as the house is": {
			record: kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running, heardAt(later), inHouse("house-3")),
			houses: parents{"house-3": "closed"},
			state:  kind.Waiting, reason: "its house is closed", since: silence,
		},
		"whatever it was doing, rather than missing": {
			record: kindstest.AFan("fan-uuid", kindstest.Stopped, kindstest.Stopped, heardAt(later), inHouse("house-3")),
			houses: parents{"house-3": "closed"},
			state:  kind.Waiting, reason: "its house is closed", since: silence,
		},
		"and so does one kept before refs, its house among its owners": {
			record: kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running, heardAt(later), keptInHouse("house-3")),
			houses: parents{"house-3": "closed"},
			state:  kind.Waiting, reason: "its house is closed", since: silence,
		},
		"but what is in flight in it is its command's to end": {
			record: kindstest.AFan("fan-uuid", kindstest.Starting, kindstest.Running, heardAt(later), inHouse("house-3")),
			houses: parents{"house-3": "closed"},
			state:  kindstest.Starting, since: kindstest.Moment,
		},
		"one in a house nobody says is down is missing from it": {
			record: kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running, heardAt(later), inHouse("house-4")),
			houses: parents{"house-3": "closed"},
			state:  kind.Missing, reason: "its house has none of it", since: silence,
		},
		"and with nobody to say what houses are doing, a house is taken to be open": {
			record: kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running, heardAt(later), inHouse("house-3")),
			state:  kind.Missing, reason: "its house has none of it", since: silence,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			repository := resourcesMemory.NewRepository()
			create(t, repository, tt.record)

			var options []observe.Option
			if tt.houses != nil {
				options = append(options, observe.WithParents(tt.houses))
			}

			r, _ := stored(t, repository, "fan-uuid")

			require.NoError(t, observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger, options...).Unheard(ctx, kindstest.Descriptor(), r, silence))

			kept, there := stored(t, repository, "fan-uuid")

			if tt.gone {
				assert.False(t, there)

				return
			}

			require.True(t, there)

			fan := kindstest.Typed(kept)
			assert.Equal(t, tt.state, fan.Status.State, "state")
			assert.Equal(t, tt.reason, fan.Status.Reason, "reason")
			assert.Equal(t, tt.since, fan.Status.Since, "since")
			assert.Equal(t, silence, fan.Status.ObservedAt, "its node's silence said so then")
			assert.Equal(t, kindstest.Typed(tt.record).Status.Expected, fan.Status.Expected, "what is expected is never its node's to say")
		})
	}

	t.Run("one whose parent was restored since it was last heard of is forgotten, rather than made again", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()

		r := kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running, heardAt(later))
		r.Reset = true
		create(t, repository, r)

		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger)

		require.NoError(t, observer.Unheard(ctx, kindstest.Descriptor(), r, later))

		_, there := stored(t, repository, "fan-uuid")
		require.True(t, there, "silence up to the restore itself says nothing of it")

		require.NoError(t, observer.Unheard(ctx, kindstest.Descriptor(), r, silence))

		_, there = stored(t, repository, "fan-uuid")
		assert.False(t, there, "the restored house does not have it")
	})

	t.Run("one whose restored parent says when what lives in it was looked into is forgotten only once its node went on without a word of it since that look", func(t *testing.T) {
		t.Parallel()

		resetAt := later.Truncate(time.Millisecond)

		// reset is the fan reset for the house's restore, at later, and last
		// heard of then.
		reset := func(t *testing.T, repository *resourcesMemory.Repository) resource.Record {
			t.Helper()

			r := kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running, heardAt(later))
			r.Reset, r.ResetAt = true, resetAt
			create(t, repository, r)

			kept, _ := stored(t, repository, "fan-uuid")

			return kept
		}

		for name, tt := range map[string]struct {
			looked looks
		}{
			"not looked into since its restore at all, its dockerd slow to come up": {
				looked: looks{kindstest.House: {}},
			},
			"nor since the restore it was reset for, though looked into after one before": {
				looked: looks{kindstest.House: later.Add(-time.Hour)},
			},
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				repository := resourcesMemory.NewRepository()
				r := reset(t, repository)

				observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger, observe.WithParents(tt.looked))

				for _, beat := range []time.Time{silence, silence.Add(time.Hour)} {
					require.NoError(t, observer.Unheard(ctx, kindstest.Descriptor(), r, beat))

					kept, there := stored(t, repository, "fan-uuid")
					require.True(t, there, "its silence says nothing before the look")
					assert.Equal(t, r.Version, kept.Version, "nothing of it is written")
				}
			})
		}

		t.Run("and once it was looked into since, its silence counts from the look, which it is taken to have been heard of as of", func(t *testing.T) {
			t.Parallel()

			repository := resourcesMemory.NewRepository()
			r := reset(t, repository)

			look := silence.Add(time.Minute)
			observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger, observe.WithParents(looks{kindstest.House: look}))

			require.NoError(t, observer.Unheard(ctx, kindstest.Descriptor(), r, look.Add(time.Hour)))

			kept, there := stored(t, repository, "fan-uuid")
			require.True(t, there, "silence since the restore, before the look, says nothing")
			assert.True(t, kept.Reset, "it is reset still")
			assert.Equal(t, look, kindstest.Typed(kept).Status.ObservedAt, "heard of as of the look, which its silence counts from")

			require.NoError(t, observer.Unheard(ctx, kindstest.Descriptor(), kept, look.Add(time.Hour)))

			_, there = stored(t, repository, "fan-uuid")
			assert.False(t, there, "once its node went on without a word of it since the look, the restored house does not have it")
		})
	})

	t.Run("silence older than what was last heard of it, or than the command it is on its way to, says nothing", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()
		create(t, repository,
			kindstest.AFan("heard-since", kindstest.Running, kindstest.Running, heardAt(silence.Add(time.Second))),
			kindstest.AFan("asked-since", kindstest.Deleting, kind.Deleted, heardAt(later), func(f *kindstest.Fan) { f.Status.Since = silence.Add(time.Second) }),
		)

		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger)

		for _, uuid := range []string{"heard-since", "asked-since"} {
			r, _ := stored(t, repository, uuid)
			require.NoError(t, observer.Unheard(ctx, kindstest.Descriptor(), r, silence))

			kept, there := stored(t, repository, uuid)
			require.True(t, there, uuid)
			assert.Equal(t, int64(1), kept.Version, uuid)
		}
	})

	t.Run("and a word of it heard as late as the silence, which crossed it, breaks it", func(t *testing.T) {
		t.Parallel()

		memory := resourcesMemory.NewRepository()
		create(t, memory, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running, heardAt(later)))

		racing := &kindstest.Racing{Repository: memory}

		// its node's heartbeat of the very beat the silence is judged at is
		// heard between the silence reading the fan and writing it back.
		racing.Cross(kindstest.Rewrite(memory, func(r *resource.Record) {
			common, _ := r.Common()
			common.ObservedAt = silence
			_ = r.SetCommon(common)
		}))

		r, _ := stored(t, memory, "fan-uuid")

		require.NoError(t, observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), racing, logger).Unheard(ctx, kindstest.Descriptor(), r, silence))

		fan, _ := stored(t, memory, "fan-uuid")
		assert.Equal(t, kindstest.Running, kindstest.Typed(fan).Status.State, "it was heard, so it is not gone")
	})
}

// TestObserver_changed holds the observer to telling the graphs a resource is
// in that it changed once it went missing, or its record went, and of nothing
// else it writes down: what needs it is made again at once.
func TestObserver_changed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := slog.New(slog.DiscardHandler)

	// when the node, beating on, has long said nothing of what it holds.
	silence := later.Add(time.Minute)
	fan := graph.Ref{Kind: kindstest.Kind, UUID: "fan-uuid"}

	t.Run("what its node's silence says is missing is told of, once it is written down", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()
		create(t, repository, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running, heardAt(later)))

		told := &graphtest.Told{}
		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger, observe.WithNotifier(told))

		r, _ := stored(t, repository, "fan-uuid")
		require.NoError(t, observer.Unheard(ctx, kindstest.Descriptor(), r, silence))

		assert.Equal(t, []graph.Ref{fan}, told.Changes())

		missing, _ := stored(t, repository, "fan-uuid")
		require.NoError(t, observer.Unheard(ctx, kindstest.Descriptor(), missing, silence.Add(time.Minute)))

		assert.Len(t, told.Changes(), 1, "and not again while it is missing still")
	})

	t.Run("and so is what its node says is missing", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()
		create(t, repository, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		told := &graphtest.Told{}
		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger, observe.WithNotifier(told))

		observer.Heartbeat(ctx, kindstest.NodeName, later, instance(t, "fan-uuid", observed(kind.Missing, 0)))

		assert.Equal(t, []graph.Ref{fan}, told.Changes())
	})

	t.Run("and what is gone, record and all", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()
		create(t, repository, kindstest.AFan("fan-uuid", kindstest.Deleting, kind.Deleted, heardAt(later)))

		told := &graphtest.Told{}
		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger, observe.WithNotifier(told))

		r, _ := stored(t, repository, "fan-uuid")
		require.NoError(t, observer.Unheard(ctx, kindstest.Descriptor(), r, silence))

		_, there := stored(t, repository, "fan-uuid")
		assert.False(t, there)
		assert.Equal(t, []graph.Ref{fan}, told.Changes())
	})

	t.Run("and what its restored parent does not have, which is forgotten", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()

		r := kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running, heardAt(later))
		r.Reset = true
		create(t, repository, r)

		told := &graphtest.Told{}
		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger, observe.WithNotifier(told))

		require.NoError(t, observer.Unheard(ctx, kindstest.Descriptor(), r, silence))

		assert.Equal(t, []graph.Ref{fan}, told.Changes())
	})

	t.Run("and so is any other change of its state, its parent going down among it", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()
		create(t, repository,
			kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running),
			kindstest.AFan("in-a-closed-house", kindstest.Running, kindstest.Running, heardAt(later), func(f *kindstest.Fan) {
				f.Metadata.Refs = []kind.Reference{{Kind: kindstest.Parent, UUID: "house-3"}}
			}),
		)

		told := &graphtest.Told{}
		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger, observe.WithNotifier(told), observe.WithParents(parents{"house-3": "closed"}))

		observer.Heartbeat(ctx, kindstest.NodeName, later, instance(t, "fan-uuid", observed(kindstest.Stopped, 0)))

		closed, _ := stored(t, repository, "in-a-closed-house")
		require.NoError(t, observer.Unheard(ctx, kindstest.Descriptor(), closed, silence))

		waiting, _ := stored(t, repository, "in-a-closed-house")
		require.Equal(t, kind.Waiting, kindstest.Typed(waiting).Status.State)

		assert.Equal(t, []graph.Ref{fan, {Kind: kindstest.Kind, UUID: "in-a-closed-house"}}, told.Changes())
	})

	t.Run("but not what its node says again, however often it beats, nor what changes beside its state", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()
		create(t, repository, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		told := &graphtest.Told{}
		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger, observe.WithNotifier(told), observe.WithRefresh(time.Nanosecond))

		for i := range 3 {
			observer.Heartbeat(ctx, kindstest.NodeName, later.Add(time.Duration(i)*time.Second), instance(t, "fan-uuid", observed(kindstest.Running, 1)))
		}

		observer.Heartbeat(ctx, kindstest.NodeName, later.Add(time.Minute), instance(t, "fan-uuid", observed(kindstest.Running, 3)))

		r, _ := stored(t, repository, "fan-uuid")
		require.Equal(t, 3, kindstest.Typed(r).Status.Speed, "what changed beside its state is written down")

		assert.Empty(t, told.Changes())
	})

	t.Run("and what cannot be told is no reason to fail what was written down", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()
		create(t, repository, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running, heardAt(later)))

		told := &graphtest.Told{Failure: errors.New("the database is gone")}
		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger, observe.WithNotifier(told))

		r, _ := stored(t, repository, "fan-uuid")
		require.NoError(t, observer.Unheard(ctx, kindstest.Descriptor(), r, silence))

		missing, _ := stored(t, repository, "fan-uuid")
		assert.Equal(t, kind.Missing, kindstest.Typed(missing).Status.State)
	})
}

// parents are houses, and the state of those that are down.
type parents map[string]kind.State

var _ observe.Parents = parents{}

// Looked says nothing of when what lives in a house was looked into: a
// house is never restored.
func (p parents) Looked(context.Context, kind.Reference) (time.Time, bool, error) {
	return time.Time{}, false, nil
}

func (p parents) Down(_ context.Context, parent kind.Reference) (kind.State, error) {
	if parent.Kind != kindstest.Parent {
		return "", nil
	}

	return p[parent.UUID], nil
}

// looks are houses, each restored and looked into since as a moment says,
// none when it was not looked into since.
type looks map[string]time.Time

var _ observe.Parents = looks{}

func (l looks) Down(context.Context, kind.Reference) (kind.State, error) {
	return "", nil
}

func (l looks) Looked(_ context.Context, parent kind.Reference) (time.Time, bool, error) {
	looked, says := l[parent.UUID]
	if parent.Kind != kindstest.Parent {
		return time.Time{}, false, nil
	}

	return looked, says, nil
}

// orphans are told of what a node holds that nobody keeps a record of, and
// as of when, and of what it holds that somebody does.
type orphans struct {
	told []string
	at   []time.Time
	kept []string
}

func (o *orphans) Orphaned(_ context.Context, d kind.Descriptor, nodeName string, uuid string, at time.Time) error {
	o.told = append(o.told, d.Name+" "+uuid+" on "+nodeName)
	o.at = append(o.at, at)

	return nil
}

func (o *orphans) Kept(_ context.Context, d kind.Descriptor, nodeName string, uuid string) {
	o.kept = append(o.kept, d.Name+" "+uuid+" on "+nodeName)
}

func TestObserver_Heartbeat_orphans(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := slog.New(slog.DiscardHandler)

	// homeless are fans that live in no house, as a VM lives in nothing.
	homeless := func() *kind.Registry[kind.ControlPlaneBinding] {
		d := kindstest.Descriptor()
		d.Parent, d.OnParent = "", kind.ParentRules{}

		registry := kind.NewRegistry[kind.ControlPlaneBinding]()
		if err := registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](d, &kindstest.Fans{})); err != nil {
			panic(err)
		}

		return registry
	}

	unhoused := func(f *kindstest.Fan) { f.Metadata.Owners, f.Metadata.Refs = nil, nil }

	running := func(uuid string) kind.Observation {
		return kind.Observation{Kind: kindstest.Kind, UUID: uuid, Status: status(t, kindstest.Status{Status: kind.Status{State: kindstest.Running}})}
	}

	t.Run("what a node holds that nobody keeps a record of is an orphan", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()
		create(t, repository,
			kindstest.AFan("kept", kindstest.Running, kindstest.Running, unhoused),
			kindstest.AFan("kept-elsewhere", kindstest.Running, kindstest.Running, unhoused, func(f *kindstest.Fan) { f.Metadata.Node = "node-2" }),
		)

		told := &orphans{}
		observer := observe.NewObserver(homeless(), repository, logger, observe.WithOrphans(told))

		for _, uuid := range []string{"kept", "kept-elsewhere", "orphan", ""} {
			observer.Heartbeat(ctx, kindstest.NodeName, kindstest.Moment, running(uuid))
		}

		assert.Equal(t, []string{"fan orphan on " + kindstest.NodeName}, told.told, "one kept elsewhere is its own node's to speak for, and one that says no uuid names nothing to tell of")
		assert.Equal(t, []time.Time{kindstest.Moment}, told.at, "as of the heartbeat its node took")
		assert.Equal(t, []string{"fan kept on " + kindstest.NodeName, "fan kept-elsewhere on " + kindstest.NodeName}, told.kept, "and of what has a record, which no orphan is")
	})

	t.Run("what lives in a parent is never an orphan of its own", func(t *testing.T) {
		t.Parallel()

		told := &orphans{}
		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), resourcesMemory.NewRepository(), logger, observe.WithOrphans(told))

		observer.Heartbeat(ctx, kindstest.NodeName, kindstest.Moment, running("orphan"))

		assert.Empty(t, told.told)
		assert.Empty(t, told.kept)
	})

	t.Run("and an observer given no orphans tells nobody", func(t *testing.T) {
		t.Parallel()

		observer := observe.NewObserver(homeless(), resourcesMemory.NewRepository(), logger)

		assert.NotPanics(t, func() {
			observer.Heartbeat(ctx, kindstest.NodeName, kindstest.Moment, running("orphan"))
		})
	})
}

// TestObserver_Heartbeat_witness holds a kind that hears its heartbeats to
// being told of every instance, and of whether a record of its is it, once
// what one says of a record is written down: what nobody keeps a record of is
// there for it, and is not the framework's to make anything of.
func TestObserver_Heartbeat_witness(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	repository := resourcesMemory.NewRepository()
	create(t, repository, kindstest.AFan("running", kindstest.Running, kindstest.Running))

	witness := &kindstest.Witnessing{Fans: &kindstest.Fans{}, Records: repository}

	registry := kind.NewRegistry[kind.ControlPlaneBinding]()
	require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](kindstest.Descriptor(), witness)))

	observer := observe.NewObserver(registry, repository, slog.New(slog.DiscardHandler))

	recorded := instance(t, "running", observed(kindstest.Running, 2))
	unrecorded := instance(t, "", observed(kindstest.Running, 1))

	observer.Heartbeat(ctx, kindstest.NodeName, later, recorded)

	running, _ := stored(t, repository, "running")
	assert.Equal(t, 2, kindstest.Typed(running).Status.Speed, "what it says of a record is written down as ever")

	observer.Heartbeat(ctx, kindstest.NodeName, later, unrecorded)

	assert.Equal(t, []kindstest.Heard{
		{Node: kindstest.NodeName, Instance: recorded, Kept: true, At: later},
		{Node: kindstest.NodeName, Instance: unrecorded, At: later},
	}, witness.Heard(), "each as it was said, and whether a record is it")

	all, _, err := repository.GetAll(ctx, kindstest.Kind, resource.Filter{}, 0, 0)
	require.NoError(t, err)
	assert.Len(t, all, 1, "and nothing is kept of what has none")

	t.Run("what it cannot take in fails nothing, but for what it asks to be heard again", func(t *testing.T) {
		t.Parallel()

		for name, tt := range map[string]struct {
			fails  error
			failed bool
		}{
			"what it could not take in, which the next beat says again, fails nothing": {
				fails: errors.New("the database is gone"),
			},
			"what it asks to be heard again for, which no later beat says again, fails the heartbeat": {
				fails:  fmt.Errorf("%w: the database is gone", kind.ErrHearAgain),
				failed: true,
			},
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				repository := resourcesMemory.NewRepository()
				failing := &kindstest.Witnessing{Fans: &kindstest.Fans{}, Records: repository, Fails: tt.fails}

				registry := kind.NewRegistry[kind.ControlPlaneBinding]()
				require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](kindstest.Descriptor(), failing)))

				err := observe.NewObserver(registry, repository, slog.New(slog.DiscardHandler)).Heartbeat(ctx, kindstest.NodeName, later, unrecorded)
				if tt.failed {
					assert.ErrorIs(t, err, kind.ErrHearAgain)

					return
				}

				assert.NoError(t, err)
			})
		}
	})
}
