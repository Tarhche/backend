package observe_test

import (
	"context"
	"encoding/json"
	"log/slog"
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

// report is what a node says of the fans it holds.
func report(t *testing.T, unseen []string, instances map[string]kindstest.Status) map[string]kind.Report[json.RawMessage] {
	t.Helper()

	r := kind.Report[json.RawMessage]{Instances: []kind.Observation{}, Unseen: unseen}
	for uuid, s := range instances {
		r.Instances = append(r.Instances, kind.Observation{Kind: kindstest.Kind, UUID: uuid, Status: status(t, s)})
	}

	return map[string]kind.Report[json.RawMessage]{kindstest.Kind: r}
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

		observer.Heartbeat(ctx, kindstest.NodeName, later, report(t, nil, map[string]kindstest.Status{
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

		observer.Heartbeat(ctx, kindstest.NodeName, later, report(t, []string{"house-2"}, map[string]kindstest.Status{}))

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

	t.Run("a kind that sent no report, or that the control plane does not run, says nothing", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()
		create(t, repository, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger)

		observer.Heartbeat(ctx, kindstest.NodeName, later, map[string]kind.Report[json.RawMessage]{
			"kettle": {Instances: []kind.Observation{}},
		})

		fan, _ := stored(t, repository, "fan-uuid")
		assert.Equal(t, kindstest.Running, kindstest.Typed(fan).Status.State)
		assert.Equal(t, int64(1), fan.Version)
	})

	t.Run("a report older than what was last heard is not taken", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()
		create(t, repository, kindstest.AFan("fan-uuid", kindstest.Stopped, kindstest.Stopped, func(f *kindstest.Fan) { f.Status.ObservedAt = later }))

		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger)

		observer.Heartbeat(ctx, kindstest.NodeName, later.Add(-time.Second), report(t, nil, map[string]kindstest.Status{"fan-uuid": observed(kindstest.Running, 1)}))

		fan, _ := stored(t, repository, "fan-uuid")
		assert.Equal(t, kindstest.Stopped, kindstest.Typed(fan).Status.State)
	})

	t.Run("a node saying again what it said is written down now and then, not every beat", func(t *testing.T) {
		t.Parallel()

		repository := resourcesMemory.NewRepository()
		create(t, repository, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), repository, logger)
		running := report(t, nil, map[string]kindstest.Status{"fan-uuid": observed(kindstest.Running, 1)})

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

		observer.Heartbeat(ctx, kindstest.NodeName, later, report(t, nil, map[string]kindstest.Status{"fan-uuid": observed(kindstest.Running, 2)}))

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

		observer.Heartbeat(ctx, kindstest.NodeName, later, report(t, nil, map[string]kindstest.Status{"fan-uuid": observed(kindstest.Stopped, 0)}))

		fan, _ := stored(t, memory, "fan-uuid")
		assert.Equal(t, kindstest.Running, kindstest.Typed(fan).Status.State)
		assert.Equal(t, "node-2", fan.Metadata.Node)
	})
}
