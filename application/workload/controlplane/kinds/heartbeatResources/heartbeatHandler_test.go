package heartbeatResources_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/heartbeatResources"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/observe"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

// heard is one heartbeat an observer was handed.
type heard struct {
	node     string
	at       time.Time
	instance kind.Observation
}

// recording is an observer that keeps what it is handed, and fails each with
// failure.
type recording struct {
	lock    sync.Mutex
	heard   []heard
	failure error
}

func (r *recording) Heartbeat(_ context.Context, nodeName string, at time.Time, instance kind.Observation) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.heard = append(r.heard, heard{node: nodeName, at: at, instance: instance})

	return r.failure
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

	// what a node says of a fan it holds: running, in its house.
	fan := kind.Observation{
		Kind:   kindstest.Kind,
		UUID:   "fan-uuid",
		Owners: []kind.Reference{{Kind: kindstest.Parent, UUID: kindstest.House}},
		Status: json.RawMessage(`{"state":"running","speed":2}`),
	}

	t.Run("what an instance's heartbeat says is handed on, as its node's word for it at that moment", func(t *testing.T) {
		t.Parallel()

		observer := &recording{}

		require.NoError(t, heartbeatResources.NewHeartbeatHandler(observer, logger).Handle(ctx, message(t, kind.Heartbeat{Node: kindstest.NodeName, At: at, Observed: fan})))

		require.Len(t, observer.heard, 1)
		assert.Equal(t, heard{node: kindstest.NodeName, at: at, instance: fan}, observer.heard[0])
	})

	t.Run("one that says not when is heard now", func(t *testing.T) {
		t.Parallel()

		observer := &recording{}

		require.NoError(t, heartbeatResources.NewHeartbeatHandler(observer, logger).Handle(ctx, message(t, kind.Heartbeat{Node: kindstest.NodeName, Observed: fan})))

		require.Len(t, observer.heard, 1)
		assert.WithinDuration(t, time.Now(), observer.heard[0].at, time.Minute)
	})

	t.Run("one that cannot be read, or that names no node or no kind, is let go of", func(t *testing.T) {
		t.Parallel()

		observer := &recording{}
		handler := heartbeatResources.NewHeartbeatHandler(observer, logger)

		nameless := fan
		nameless.Kind = ""

		assert.NoError(t, handler.Handle(ctx, []byte("{")), "read again, it is as unreadable")
		assert.NoError(t, handler.Handle(ctx, message(t, kind.Heartbeat{At: at, Observed: fan})))
		assert.NoError(t, handler.Handle(ctx, message(t, kind.Heartbeat{Node: kindstest.NodeName, At: at, Observed: nameless})))

		assert.Empty(t, observer.heard)
	})

	t.Run("what it says is taken onto the resource it is, and nothing is taken of what it does not say", func(t *testing.T) {
		t.Parallel()

		resources := resourcesMemory.NewRepository()

		for _, r := range []resource.Record{
			kindstest.AFan("fan-uuid", kindstest.Starting, kindstest.Running),
			kindstest.AFan("unsaid", kindstest.Running, kindstest.Running),
		} {
			_, err := resources.Create(ctx, r)
			require.NoError(t, err)
		}

		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), resources, logger)

		require.NoError(t, heartbeatResources.NewHeartbeatHandler(observer, logger).Handle(ctx, message(t, kind.Heartbeat{Node: kindstest.NodeName, At: at, Observed: fan})))

		taken := stored(t, resources, "fan-uuid")
		assert.Equal(t, kindstest.Running, taken.Status.State, "it arrived")
		assert.Equal(t, 2, taken.Status.Speed)
		assert.Equal(t, at, taken.Status.ObservedAt, "when its node said so")

		unsaid := stored(t, resources, "unsaid")
		assert.Equal(t, kindstest.Running, unsaid.Status.State, "a heartbeat of another says nothing of it: going unheard is the reconcile loop's to make anything of")
		assert.True(t, unsaid.Status.ObservedAt.IsZero())
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

			_, err := resources.Create(ctx, kindstest.AFan("fan-uuid", kindstest.Starting, kindstest.Running))
			require.NoError(t, err)

			observer := observe.NewObserver(tt.registry, resources, logger)

			said := fan
			said.Kind = tt.kind

			require.NoError(t, heartbeatResources.NewHeartbeatHandler(observer, logger).Handle(ctx, message(t, kind.Heartbeat{Node: kindstest.NodeName, At: at, Observed: said})), name)

			untouched := stored(t, resources, "fan-uuid")
			assert.Equal(t, kindstest.Starting, untouched.Status.State, name)
			assert.True(t, untouched.Status.ObservedAt.IsZero(), name)
		}
	})
}

// TestHeartbeatHandler_Handle_hearAgain holds a heartbeat to being failed, to
// be heard again, only when its observer says it is to be: the first look
// into a restored parent, which no later beat says again.
func TestHeartbeatHandler_Handle_hearAgain(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for name, tt := range map[string]struct {
		failure error
		failed  bool
	}{
		"one written down is let go of": {},
		"and so is one that could not be, which the next beat says again": {
			failure: errors.New("the database is gone"),
		},
		"but one to be heard again is failed, and heard again a moment later": {
			failure: fmt.Errorf("%w: the database is gone", kind.ErrHearAgain),
			failed:  true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			observer := &recording{failure: tt.failure}
			if !tt.failed {
				observer.failure = nil
			}

			err := heartbeatResources.NewHeartbeatHandler(observer, slog.New(slog.DiscardHandler)).Handle(ctx, message(t, kind.Heartbeat{Node: kindstest.NodeName, At: kindstest.Moment, Observed: kind.Observation{Kind: kindstest.Kind, UUID: "fan-uuid"}}))

			if tt.failed {
				assert.ErrorIs(t, err, kind.ErrHearAgain)

				var later *domain.Later
				require.ErrorAs(t, err, &later, "a moment later, not at once")
				assert.Positive(t, later.After)

				return
			}

			assert.NoError(t, err)
		})
	}
}
