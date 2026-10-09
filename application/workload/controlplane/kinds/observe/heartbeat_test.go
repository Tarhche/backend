package observe_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/observe"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

// report is what a node says of the fans it holds, having read the house
// they are in unless it is among the unseen.
func report(t *testing.T, unseen []string, instances map[string]kindstest.Status) kind.Report[json.RawMessage] {
	t.Helper()

	r := kind.Report[json.RawMessage]{Instances: []kind.Observation{}, Unseen: unseen}
	if !slices.Contains(unseen, kindstest.House) {
		r.Read = []string{kindstest.House}
	}

	for uuid, s := range instances {
		r.Instances = append(r.Instances, kind.Observation{Kind: kindstest.Kind, UUID: uuid, Status: status(t, s)})
	}

	return r
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

func TestObserver_Heartbeat(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := slog.New(slog.DiscardHandler)

	inAnotherHouse := func(f *kindstest.Fan) {
		f.Metadata.Owners = []kind.Reference{{Kind: kindstest.Parent, UUID: "house-2"}}
	}

	t.Run("what a node reports of what it holds is taken, and of nothing else", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()
		create(t, repository,
			kindstest.AFan("running", kindstest.Running, kindstest.Running),
			kindstest.AFan("starting", kindstest.Starting, kindstest.Running),
			kindstest.AFan("stopped", kindstest.Stopped, kindstest.Stopped),
			kindstest.AFan("elsewhere", kindstest.Running, kindstest.Running, func(f *kindstest.Fan) { f.Metadata.Node = "node-2" }),
		)

		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger)

		observer.Heartbeat(ctx, kindstest.NodeName, kindstest.Kind, later, report(t, nil, map[string]kindstest.Status{
			"running":   observed(kindstest.Running, 3),
			"starting":  observed(kindstest.Running, 1),
			"stopped":   observed(kindstest.Stopped, 0),
			"elsewhere": observed(kindstest.Stopped, 0),
		}))

		running, _ := stored(t, repository, "running")
		assert.Equal(t, 3, kindstest.Typed(running).Status.Speed)
		assert.Equal(t, later, kindstest.Typed(running).Status.ObservedAt)

		starting, _ := stored(t, repository, "starting")
		assert.Equal(t, kindstest.Running, kindstest.Typed(starting).Status.State, "it arrived")

		elsewhere, _ := stored(t, repository, "elsewhere")
		assert.Equal(t, kindstest.Running, kindstest.Typed(elsewhere).Status.State, "another node's word for it is not its node's")
		assert.True(t, kindstest.Typed(elsewhere).Status.ObservedAt.IsZero())
	})

	t.Run("what a report could see and does not list is gone from the node, but what it could not see is not", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()
		create(t, repository,
			kindstest.AFan("lost", kindstest.Running, kindstest.Running),
			kindstest.AFan("deleting", kindstest.Deleting, kind.Deleted),
			kindstest.AFan("on-its-way", kindstest.Starting, kindstest.Running),
			kindstest.AFan("unseen", kindstest.Running, kindstest.Running, inAnotherHouse),
		)

		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger)

		observer.Heartbeat(ctx, kindstest.NodeName, kindstest.Kind, later, report(t, []string{"house-2"}, map[string]kindstest.Status{}))

		lost, _ := stored(t, repository, "lost")
		assert.Equal(t, kind.Missing, kindstest.Typed(lost).Status.State)

		_, kept := stored(t, repository, "deleting")
		assert.False(t, kept, "one on its way out that is gone is deleted, record and all")

		onItsWay, _ := stored(t, repository, "on-its-way")
		assert.Equal(t, kindstest.Starting, kindstest.Typed(onItsWay).Status.State, "one being made is not there yet")

		unseen, _ := stored(t, repository, "unseen")
		assert.Equal(t, kindstest.Running, kindstest.Typed(unseen).Status.State, "a house that did not answer says nothing of the fans in it")
		assert.True(t, kindstest.Typed(unseen).Status.ObservedAt.IsZero())
		assert.Equal(t, int64(1), unseen.Version, "it is not even written")
	})

	t.Run("what lives in a parent its node did not look into waits on the parent, as the parent is", func(t *testing.T) {
		t.Parallel()

		inHouse := func(house string) func(f *kindstest.Fan) {
			return func(f *kindstest.Fan) { f.Metadata.Owners = []kind.Reference{{Kind: kindstest.Parent, UUID: house}} }
		}

		repository := resourcesMemory.NewRepository()
		create(t, repository,
			kindstest.AFan("in-a-closed-house", kindstest.Running, kindstest.Running, inHouse("house-3")),
			kindstest.AFan("in-an-open-house", kindstest.Running, kindstest.Running, inHouse("house-4")),
			kindstest.AFan("stopped-in-a-closed-house", kindstest.Stopped, kindstest.Stopped, inHouse("house-3")),
			kindstest.AFan("on-its-way-in-a-closed-house", kindstest.Starting, kindstest.Running, inHouse("house-3")),
		)

		houses := parents{"house-3": "closed"}
		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger, observe.WithParents(houses))

		observer.Heartbeat(ctx, kindstest.NodeName, kindstest.Kind, later, report(t, nil, map[string]kindstest.Status{}))

		closed, _ := stored(t, repository, "in-a-closed-house")
		assert.Equal(t, kind.Waiting, kindstest.Typed(closed).Status.State)
		assert.Equal(t, "its house is closed", kindstest.Typed(closed).Status.Reason)
		assert.Equal(t, kindstest.Running, kindstest.Typed(closed).Status.Expected)

		stoppedInIt, _ := stored(t, repository, "stopped-in-a-closed-house")
		assert.Equal(t, kind.Waiting, kindstest.Typed(stoppedInIt).Status.State, "whatever it was doing")

		onItsWay, _ := stored(t, repository, "on-its-way-in-a-closed-house")
		assert.Equal(t, kindstest.Starting, kindstest.Typed(onItsWay).Status.State, "what is in flight is its command's to end")

		open, _ := stored(t, repository, "in-an-open-house")
		assert.Equal(t, kindstest.Running, kindstest.Typed(open).Status.State, "a parent that is not down, as far as anybody knows, says nothing")
		assert.Equal(t, int64(1), open.Version)
	})

	t.Run("and with nobody to say what parents are doing, nothing is concluded of what is in them", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()
		create(t, repository, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running, inAnotherHouse))

		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger)

		observer.Heartbeat(ctx, kindstest.NodeName, kindstest.Kind, later, report(t, nil, map[string]kindstest.Status{}))

		fan, _ := stored(t, repository, "fan-uuid")
		assert.Equal(t, kindstest.Running, kindstest.Typed(fan).Status.State)
		assert.Equal(t, int64(1), fan.Version)
	})

	t.Run("one missing from a parent it was read in says so", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()
		create(t, repository, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger).
			Heartbeat(ctx, kindstest.NodeName, kindstest.Kind, later, report(t, nil, map[string]kindstest.Status{}))

		fan, _ := stored(t, repository, "fan-uuid")
		assert.Equal(t, kind.Missing, kindstest.Typed(fan).Status.State)
		assert.Equal(t, "its house has none of it", kindstest.Typed(fan).Status.Reason)
	})

	t.Run("one whose parent was restored is what the restored parent holds: forgotten when it is not there", func(t *testing.T) {
		t.Parallel()

		reset := func(r resource.Record) resource.Record {
			r.Reset = true

			return r
		}

		repository := resourcesMemory.NewRepository()
		create(t, repository,
			reset(kindstest.AFan("gone-with-the-restore", kindstest.Running, kindstest.Running)),
			reset(kindstest.AFan("kept-by-the-restore", kindstest.Running, kindstest.Running)),
			reset(kindstest.AFan("not-seen-yet", kindstest.Running, kindstest.Running, inAnotherHouse)),
		)

		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger)

		observer.Heartbeat(ctx, kindstest.NodeName, kindstest.Kind, later, report(t, []string{"house-2"}, map[string]kindstest.Status{
			"kept-by-the-restore": observed(kindstest.Stopped, 0),
		}))

		_, kept := stored(t, repository, "gone-with-the-restore")
		assert.False(t, kept, "the restored house does not have it, so it is not made again")

		found, _ := stored(t, repository, "kept-by-the-restore")
		assert.Equal(t, kindstest.Stopped, kindstest.Typed(found).Status.State, "it is as it was found")
		assert.False(t, found.Reset, "and is reconciled as anything else from now on")

		notSeen, _ := stored(t, repository, "not-seen-yet")
		assert.True(t, notSeen.Reset, "a house not read yet says nothing of it")
	})

	t.Run("and a look taken before its parent was restored says nothing of it either way", func(t *testing.T) {
		t.Parallel()

		restoredAt := func(r resource.Record) resource.Record {
			r.Reset = true

			common, err := r.Common()
			require.NoError(t, err)

			common.ObservedAt = later

			require.NoError(t, r.SetCommon(common))

			return r
		}

		repository := resourcesMemory.NewRepository()
		create(t, repository,
			restoredAt(kindstest.AFan("made-since-the-snapshot", kindstest.Running, kindstest.Running)),
			restoredAt(kindstest.AFan("deleted-since-the-snapshot", kind.Missing, kindstest.Running)),
		)

		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger)

		// what the house held just before it was restored.
		observer.Heartbeat(ctx, kindstest.NodeName, kindstest.Kind, later.Add(-time.Second), report(t, nil, map[string]kindstest.Status{
			"made-since-the-snapshot": observed(kindstest.Running, 1),
		}))

		made, _ := stored(t, repository, "made-since-the-snapshot")
		assert.True(t, made.Reset, "it is not taken to be on the restored disk")

		deleted, kept := stored(t, repository, "deleted-since-the-snapshot")
		require.True(t, kept, "nor not to be")
		assert.True(t, deleted.Reset)
	})

	t.Run("a report made before a resource was asked what it is on its way to says nothing of it", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()
		create(t, repository, kindstest.AFan("fan-uuid", kindstest.Starting, kindstest.Running, func(f *kindstest.Fan) { f.Status.Since = later }))

		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger)

		observer.Heartbeat(ctx, kindstest.NodeName, kindstest.Kind, later.Add(-time.Second), report(t, nil, map[string]kindstest.Status{"fan-uuid": observed(kindstest.Running, 2)}))

		fan, _ := stored(t, repository, "fan-uuid")
		assert.Equal(t, kindstest.Starting, kindstest.Typed(fan).Status.State)
		assert.Equal(t, int64(1), fan.Version)

		observer.Heartbeat(ctx, kindstest.NodeName, kindstest.Kind, later.Add(time.Second), report(t, nil, map[string]kindstest.Status{"fan-uuid": observed(kindstest.Running, 2)}))

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

			observe.NewObserver(tt.registry, repository, logger).Heartbeat(ctx, kindstest.NodeName, tt.kind, later, report(t, nil, map[string]kindstest.Status{}))

			fan, _ := stored(t, repository, "fan-uuid")
			assert.Equal(t, kindstest.Running, kindstest.Typed(fan).Status.State, name)
			assert.Equal(t, int64(1), fan.Version, name)
		}
	})

	t.Run("a report older than what was last heard is not taken", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()
		create(t, repository, kindstest.AFan("fan-uuid", kindstest.Stopped, kindstest.Stopped, func(f *kindstest.Fan) { f.Status.ObservedAt = later }))

		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger)

		observer.Heartbeat(ctx, kindstest.NodeName, kindstest.Kind, later.Add(-time.Second), report(t, nil, map[string]kindstest.Status{"fan-uuid": observed(kindstest.Running, 1)}))

		fan, _ := stored(t, repository, "fan-uuid")
		assert.Equal(t, kindstest.Stopped, kindstest.Typed(fan).Status.State)
	})

	t.Run("a node saying again what it said is written down now and then, not every beat", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()
		create(t, repository, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger)
		running := report(t, nil, map[string]kindstest.Status{"fan-uuid": observed(kindstest.Running, 1)})

		observer.Heartbeat(ctx, kindstest.NodeName, kindstest.Kind, later, running)

		fan, _ := stored(t, repository, "fan-uuid")
		assert.Equal(t, int64(2), fan.Version, "first heard, it is written")

		observer.Heartbeat(ctx, kindstest.NodeName, kindstest.Kind, later.Add(time.Second), running)

		fan, _ = stored(t, repository, "fan-uuid")
		assert.Equal(t, int64(2), fan.Version, "a second later, the same is not")
		assert.Equal(t, later, kindstest.Typed(fan).Status.ObservedAt)

		observer.Heartbeat(ctx, kindstest.NodeName, kindstest.Kind, later.Add(time.Minute), running)

		fan, _ = stored(t, repository, "fan-uuid")
		assert.Equal(t, int64(3), fan.Version, "a while later, it is, so when it was observed stays roughly true")
		assert.Equal(t, later.Add(time.Minute), kindstest.Typed(fan).Status.ObservedAt)
	})

	t.Run("a report crossing a command is taken on what the command left, never over it", func(t *testing.T) {
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

		observer.Heartbeat(ctx, kindstest.NodeName, kindstest.Kind, later, report(t, nil, map[string]kindstest.Status{"fan-uuid": observed(kindstest.Running, 2)}))

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

		observer.Heartbeat(ctx, kindstest.NodeName, kindstest.Kind, later, report(t, nil, map[string]kindstest.Status{"fan-uuid": observed(kindstest.Stopped, 0)}))

		fan, _ := stored(t, memory, "fan-uuid")
		assert.Equal(t, kindstest.Running, kindstest.Typed(fan).Status.State)
		assert.Equal(t, "node-2", fan.Metadata.Node)
	})
}

// parents are houses, and the state of those that are down.
type parents map[string]kind.State

var _ observe.Parents = parents{}

func (p parents) Down(_ context.Context, parent kind.Reference) (kind.State, error) {
	if parent.Kind != kindstest.Parent {
		return "", nil
	}

	return p[parent.UUID], nil
}

// orphans are told of what a node holds that nobody keeps a record of.
type orphans struct {
	told []string
}

func (o *orphans) Orphaned(_ context.Context, d kind.Descriptor, nodeName string, uuid string) error {
	o.told = append(o.told, d.Name+" "+uuid+" on "+nodeName)

	return nil
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

	unhoused := func(f *kindstest.Fan) { f.Metadata.Owners = nil }

	t.Run("what a node holds that nobody keeps a record of is an orphan", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()
		create(t, repository,
			kindstest.AFan("kept", kindstest.Running, kindstest.Running, unhoused),
			kindstest.AFan("kept-elsewhere", kindstest.Running, kindstest.Running, unhoused, func(f *kindstest.Fan) { f.Metadata.Node = "node-2" }),
		)

		told := &orphans{}
		observer := observe.NewObserver(homeless(), repository, logger, observe.WithOrphans(told))

		observer.Heartbeat(ctx, kindstest.NodeName, kindstest.Kind, kindstest.Moment, report(t, nil, map[string]kindstest.Status{
			"kept":           {Status: kind.Status{State: kindstest.Running}},
			"kept-elsewhere": {Status: kind.Status{State: kindstest.Running}},
			"orphan":         {Status: kind.Status{State: kindstest.Running}},
		}))

		assert.Equal(t, []string{"fan orphan on " + kindstest.NodeName}, told.told, "one kept elsewhere is its own node's to speak for")
	})

	t.Run("what lives in a parent is never an orphan of its own", func(t *testing.T) {
		t.Parallel()

		told := &orphans{}
		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), resourcesMemory.NewRepository(), logger, observe.WithOrphans(told))

		observer.Heartbeat(ctx, kindstest.NodeName, kindstest.Kind, kindstest.Moment, report(t, nil, map[string]kindstest.Status{
			"orphan": {Status: kind.Status{State: kindstest.Running}},
		}))

		assert.Empty(t, told.told)
	})

	t.Run("and an observer given no orphans tells nobody", func(t *testing.T) {
		t.Parallel()

		observer := observe.NewObserver(homeless(), resourcesMemory.NewRepository(), logger)

		assert.NotPanics(t, func() {
			observer.Heartbeat(ctx, kindstest.NodeName, kindstest.Kind, kindstest.Moment, report(t, nil, map[string]kindstest.Status{
				"orphan": {Status: kind.Status{State: kindstest.Running}},
			}))
		})
	})
}

// TestObserver_Heartbeat_witness holds a kind that hears its reports to being
// told the whole of each, once what it says of the records is written down:
// what nobody keeps a record of is there for it, and is not the framework's
// to make anything of.
func TestObserver_Heartbeat_witness(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	repository := resourcesMemory.NewRepository()
	create(t, repository, kindstest.AFan("running", kindstest.Running, kindstest.Running))

	witness := &kindstest.Witnessing{Fans: &kindstest.Fans{}, Records: repository}

	registry := kind.NewRegistry[kind.ControlPlaneBinding]()
	require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](kindstest.Descriptor(), witness)))

	observer := observe.NewObserver(registry, repository, slog.New(slog.DiscardHandler))

	unrecorded := report(t, nil, map[string]kindstest.Status{"running": observed(kindstest.Running, 2)})
	unrecorded.Instances = append(unrecorded.Instances, kind.Observation{
		Kind:   kindstest.Kind,
		Owners: []kind.Reference{{Kind: kindstest.Parent, UUID: kindstest.House}},
		Status: status(t, observed(kindstest.Running, 1)),
	})

	observer.Heartbeat(ctx, kindstest.NodeName, kindstest.Kind, later, unrecorded)

	heard := witness.Heard()
	require.Len(t, heard, 1)

	assert.Equal(t, kindstest.NodeName, heard[0].Node)
	assert.Equal(t, later, heard[0].At)
	assert.Equal(t, unrecorded, heard[0].Report, "all of it, what has no record among it")

	running, _ := stored(t, repository, "running")
	assert.Equal(t, 2, kindstest.Typed(running).Status.Speed, "what it says of a record is written down as ever")

	all, _, err := repository.GetAll(ctx, kindstest.Kind, resource.Filter{}, 0, 0)
	require.NoError(t, err)
	assert.Len(t, all, 1, "and nothing is kept of what has none")
}
