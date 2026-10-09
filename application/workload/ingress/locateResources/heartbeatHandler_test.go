package locateResources_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ingressTasks "github.com/khanzadimahdi/testproject/application/workload/ingress/kinds/task"
	ingressVMs "github.com/khanzadimahdi/testproject/application/workload/ingress/kinds/vm"
	"github.com/khanzadimahdi/testproject/application/workload/ingress/locateResources"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/ingress"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	ingressMemory "github.com/khanzadimahdi/testproject/infrastructure/workload/ingress/memory"
)

func TestHeartbeatHandler_Handle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := slog.New(slog.DiscardHandler)
	at := time.Now().Add(-time.Second).UTC()

	// what a node says of a lamp it holds: lit, under a slug, letting the
	// ingress in to a port.
	lamp := kind.Observation{
		Kind:   "lamp",
		UUID:   "lamp-uuid",
		Status: json.RawMessage(`{"state":"lit","slug":"desk-xkfqz","ports":[80]}`),
	}

	t.Run("where an instance's heartbeat says it is, is written down, as its node's word for it at that moment", func(t *testing.T) {
		t.Parallel()

		locations := ingressMemory.NewLocations(time.Minute)

		require.NoError(t, locateResources.NewHeartbeatHandler(locations, lampsOnly, logger).Handle(ctx, message(t, kind.Heartbeat{Node: nodeName, At: at, Observed: lamp})))

		heard, err := locations.BySlug(ctx, "lamp", "desk-xkfqz")
		require.NoError(t, err)
		assert.Equal(t, ingress.Heard{Kind: "lamp", UUID: "lamp-uuid", Slug: "desk-xkfqz", Node: nodeName, State: lit, Ports: []port.Port{80}, At: at}, heard)
	})

	t.Run("one that says not when is heard now", func(t *testing.T) {
		t.Parallel()

		locations := &recording{}

		require.NoError(t, locateResources.NewHeartbeatHandler(locations, lampsOnly, logger).Handle(ctx, message(t, kind.Heartbeat{Node: nodeName, Observed: lamp})))

		require.Len(t, locations.heard, 1)
		assert.WithinDuration(t, time.Now(), locations.heard[0].At, time.Minute)
	})

	t.Run("one that cannot be read, that names no node or no kind, or whose status cannot be read, is let go of", func(t *testing.T) {
		t.Parallel()

		locations := &recording{}
		handler := locateResources.NewHeartbeatHandler(locations, lampsOnly, logger)

		nameless := lamp
		nameless.Kind = ""

		unreadable := lamp
		unreadable.Status = json.RawMessage(`{"slug":"desk-xkfqz"}`)

		assert.NoError(t, handler.Handle(ctx, []byte("{")), "read again, it is as unreadable")
		assert.NoError(t, handler.Handle(ctx, message(t, kind.Heartbeat{At: at, Observed: lamp})))
		assert.NoError(t, handler.Handle(ctx, message(t, kind.Heartbeat{Node: nodeName, At: at, Observed: nameless})))
		assert.NoError(t, handler.Handle(ctx, message(t, kind.Heartbeat{Node: nodeName, At: at, Observed: unreadable})))

		assert.Empty(t, locations.heard)
	})

	t.Run("and a kind the ingress does not route to is not listened to, nor an instance nobody keeps a record of", func(t *testing.T) {
		t.Parallel()

		locations := &recording{}
		handler := locateResources.NewHeartbeatHandler(locations, lampsOnly, logger)

		kettle := lamp
		kettle.Kind = "kettle"

		unrecorded := lamp
		unrecorded.UUID = ""

		assert.NoError(t, handler.Handle(ctx, message(t, kind.Heartbeat{Node: nodeName, At: at, Observed: kettle})))
		assert.NoError(t, handler.Handle(ctx, message(t, kind.Heartbeat{Node: nodeName, At: at, Observed: unrecorded})))

		assert.Empty(t, locations.heard)
	})

	t.Run("a task and a vm are found where their nodes' heartbeats say they are, as their strategies read them", func(t *testing.T) {
		t.Parallel()

		locations := ingressMemory.NewLocations(time.Minute)
		tasks, vms := ingressTasks.New(locations), ingressVMs.New(locations)

		handler := locateResources.NewHeartbeatHandler(locations, map[string]locateResources.Kind{taskKind.Name: tasks, vmKind.Name: vms}, logger)

		require.NoError(t, handler.Handle(ctx, message(t, kind.Heartbeat{Node: nodeName, At: at, Observed: kind.Observation{
			Kind: taskKind.Name,
			UUID: "task-uuid",
			Status: status(t, taskKind.Status{
				Status: kind.Status{State: taskKind.Running},
				Run:    &taskKind.Run{Slug: "request-xkfqz", Ports: []port.Port{3000}, Endpoints: []taskKind.Endpoint{{Port: 3000, Address: "vmhost:20000"}}},
			}),
		}})))

		require.NoError(t, handler.Handle(ctx, message(t, kind.Heartbeat{Node: "workload-orchestrator-02", At: at, Observed: kind.Observation{
			Kind: vmKind.Name,
			UUID: "vm-uuid",
			Status: status(t, vmKind.Status{
				Status:    kind.Status{State: vmKind.Running},
				Slug:      "box-xkfqz",
				Endpoints: []vmKind.Endpoint{{Port: 8080, Address: "vmhost:20001"}, {Port: 80, Address: "vmhost:20000"}},
			}),
		}})))

		task, err := tasks.BySlug(ctx, "request-xkfqz")
		require.NoError(t, err)
		assert.Equal(t, kind.Location{UUID: "task-uuid", Node: nodeName, Ports: []port.Port{3000}}, task)

		vm, err := vms.BySlug(ctx, "box-xkfqz")
		require.NoError(t, err)
		assert.Equal(t, kind.Location{UUID: "vm-uuid", Node: "workload-orchestrator-02", Ports: []port.Port{80, 8080}}, vm)

		_, err = vms.BySlug(ctx, "request-xkfqz")
		assert.ErrorIs(t, err, domain.ErrNotExists, "a task's slug names no vm")
	})
}
