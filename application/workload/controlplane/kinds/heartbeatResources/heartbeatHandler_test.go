package heartbeatResources_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/heartbeatResources"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/observe"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

// heard is one heartbeat an observer was handed.
type heard struct {
	node   string
	kind   string
	at     time.Time
	report kind.Report[json.RawMessage]
}

// recording is an observer that keeps what it is handed.
type recording struct {
	lock  sync.Mutex
	heard []heard
}

func (r *recording) Heartbeat(_ context.Context, nodeName string, kindName string, at time.Time, report kind.Report[json.RawMessage]) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.heard = append(r.heard, heard{node: nodeName, kind: kindName, at: at, report: report})
}

func message(t *testing.T, heartbeat kind.Heartbeat) []byte {
	t.Helper()

	payload, err := json.Marshal(heartbeat)
	require.NoError(t, err)

	return payload
}

func stored(t *testing.T, repository resource.Repository, uuid string) kindstest.Fan {
	t.Helper()

	r, err := repository.GetOne(context.Background(), kindstest.Kind, uuid)
	require.NoError(t, err)

	return kindstest.Typed(r)
}

func TestHeartbeatHandler_Handle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := slog.New(slog.DiscardHandler)
	at := kindstest.Moment.Add(time.Minute)

	// what a node says of the fans it holds: one running, in the house it
	// read.
	fans := kind.Report[json.RawMessage]{
		Instances: []kind.Observation{{
			Kind:   kindstest.Kind,
			UUID:   "fan-uuid",
			Owners: []kind.Reference{{Kind: kindstest.Parent, UUID: kindstest.House}},
			Status: json.RawMessage(`{"state":"running","speed":2}`),
		}},
		Read: []string{kindstest.House},
	}

	t.Run("what a kind's heartbeat says is handed on, as its node's word for that kind at that moment", func(t *testing.T) {
		t.Parallel()

		observer := &recording{}

		require.NoError(t, heartbeatResources.NewHeartbeatHandler(observer, logger).Handle(ctx, message(t, kind.Heartbeat{Node: kindstest.NodeName, Kind: kindstest.Kind, At: at, Report: fans})))

		require.Len(t, observer.heard, 1)
		assert.Equal(t, heard{node: kindstest.NodeName, kind: kindstest.Kind, at: at, report: fans}, observer.heard[0])
	})

	t.Run("one that says not when is heard now", func(t *testing.T) {
		t.Parallel()

		observer := &recording{}

		require.NoError(t, heartbeatResources.NewHeartbeatHandler(observer, logger).Handle(ctx, message(t, kind.Heartbeat{Node: kindstest.NodeName, Kind: kindstest.Kind, Report: fans})))

		require.Len(t, observer.heard, 1)
		assert.WithinDuration(t, time.Now(), observer.heard[0].at, time.Minute)
	})

	t.Run("one that cannot be read, or that names no node or no kind, is let go of", func(t *testing.T) {
		t.Parallel()

		observer := &recording{}
		handler := heartbeatResources.NewHeartbeatHandler(observer, logger)

		assert.NoError(t, handler.Handle(ctx, []byte("{")), "read again, it is as unreadable")
		assert.NoError(t, handler.Handle(ctx, message(t, kind.Heartbeat{Kind: kindstest.Kind, At: at, Report: fans})))
		assert.NoError(t, handler.Handle(ctx, message(t, kind.Heartbeat{Node: kindstest.NodeName, At: at, Report: fans})))

		assert.Empty(t, observer.heard)
	})

	t.Run("what it says is taken onto the resources of its kind its node holds", func(t *testing.T) {
		t.Parallel()

		resources := resourcesMemory.NewRepository()

		for _, r := range []resource.Record{
			kindstest.AFan("fan-uuid", kindstest.Starting, kindstest.Running),
			kindstest.AFan("lost", kindstest.Running, kindstest.Running),
		} {
			_, err := resources.Create(ctx, r)
			require.NoError(t, err)
		}

		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), resources, logger)

		require.NoError(t, heartbeatResources.NewHeartbeatHandler(observer, logger).Handle(ctx, message(t, kind.Heartbeat{Node: kindstest.NodeName, Kind: kindstest.Kind, At: at, Report: fans})))

		fan := stored(t, resources, "fan-uuid")
		assert.Equal(t, kindstest.Running, fan.Status.State, "it arrived")
		assert.Equal(t, 2, fan.Status.Speed)
		assert.Equal(t, at, fan.Status.ObservedAt, "when its node said so")

		assert.Equal(t, kind.Missing, stored(t, resources, "lost").Status.State, "what it leaves out of the house it read is gone from it")
	})

	t.Run("and a kind the control plane does not run, or whose state is not its nodes' to say, is not listened to", func(t *testing.T) {
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
			resources := resourcesMemory.NewRepository()

			_, err := resources.Create(ctx, kindstest.AFan("lost", kindstest.Running, kindstest.Running))
			require.NoError(t, err)

			observer := observe.NewObserver(tt.registry, resources, logger)

			require.NoError(t, heartbeatResources.NewHeartbeatHandler(observer, logger).Handle(ctx, message(t, kind.Heartbeat{Node: kindstest.NodeName, Kind: tt.kind, At: at, Report: fans})), name)

			lost := stored(t, resources, "lost")
			assert.Equal(t, kindstest.Running, lost.Status.State, name)
			assert.True(t, lost.Status.ObservedAt.IsZero(), name)
		}
	})
}
