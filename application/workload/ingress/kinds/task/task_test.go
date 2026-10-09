package task_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	ingressTasks "github.com/khanzadimahdi/testproject/application/workload/ingress/kinds/task"
	"github.com/khanzadimahdi/testproject/application/workload/ingress/locateResources"
	"github.com/khanzadimahdi/testproject/domain"
	ingressContract "github.com/khanzadimahdi/testproject/domain/workload/ingress"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	infraIngress "github.com/khanzadimahdi/testproject/infrastructure/workload/ingress"
	ingressMemory "github.com/khanzadimahdi/testproject/infrastructure/workload/ingress/memory"
)

// held is what its node's heartbeat says of a task it holds, doing state,
// serving ports under a policy: its VM publishes them only while the policy
// lets anything in.
func held(t *testing.T, uuid string, state kind.State, policy network.Policy, ports ...port.Port) kind.Heartbeat {
	t.Helper()

	run := &taskKind.Run{Name: "request-" + uuid, Slug: "request-" + uuid}
	if policy.VMNetwork().Ingress == vm.AccessAllow {
		run.Ports = ports
	}

	status, err := json.Marshal(taskKind.Status{Status: kind.Status{State: state}, Run: run})
	require.NoError(t, err)

	return kind.Heartbeat{
		Node:     "workload-orchestrator-01",
		At:       time.Now(),
		Observed: kind.Observation{Kind: taskKind.Name, UUID: uuid, Status: status},
	}
}

// hearing is the task kind's ingress strategy, finding tasks where the
// heartbeats it heard, as the ingress hears them, say they are.
func hearing(t *testing.T, heartbeats ...kind.Heartbeat) *ingressTasks.Ingress {
	t.Helper()

	locations := ingressMemory.NewLocations(time.Minute)
	tasks := ingressTasks.New(locations)

	handler := locateResources.NewHeartbeatHandler(locations, map[string]locateResources.Kind{taskKind.Name: tasks}, slog.New(slog.DiscardHandler))

	for _, heartbeat := range heartbeats {
		payload, err := json.Marshal(heartbeat)
		require.NoError(t, err)

		require.NoError(t, handler.Handle(context.Background(), payload))
	}

	return tasks
}

func TestIngress_ByUUID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ingress := hearing(t,
		held(t, "01", taskKind.Running, network.PolicyIsolated, 3000),
		held(t, "02", taskKind.Completed, network.PolicyNone),
	)

	t.Run("a task's terminal is carried to the node holding it", func(t *testing.T) {
		t.Parallel()

		location, err := ingress.ByUUID(ctx, "01")
		require.NoError(t, err)
		assert.Equal(t, kind.Location{UUID: "01", Node: "workload-orchestrator-01", Ports: []port.Port{3000}}, location)
	})

	t.Run("whatever it is doing: its node says whether it can be opened", func(t *testing.T) {
		t.Parallel()

		location, err := ingress.ByUUID(ctx, "02")
		require.NoError(t, err)
		assert.Equal(t, kind.Location{UUID: "02", Node: "workload-orchestrator-01"}, location)
	})

	t.Run("one no node has said anything of lately is not there", func(t *testing.T) {
		t.Parallel()

		_, err := ingress.ByUUID(ctx, "09")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("where it is that cannot be said is said", func(t *testing.T) {
		t.Parallel()

		var locations infraIngress.MockLocations
		locations.On("ByUUID", mock.Anything, taskKind.Name, "01").Once().Return(ingressContract.Heard{}, errors.New("the locations are gone"))
		defer locations.AssertExpectations(t)

		_, err := ingressTasks.New(&locations).ByUUID(ctx, "01")
		require.Error(t, err)
		assert.NotErrorIs(t, err, domain.ErrNotExists)
	})
}

func TestIngress_BySlug(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ingress := hearing(t,
		held(t, "01", taskKind.Running, network.PolicyPublic, 3000, 8080),
		held(t, "02", taskKind.Scheduled, network.PolicyIsolated, 3000),
		held(t, "03", taskKind.Running, network.PolicyNone, 3000),
		held(t, "04", taskKind.Running, ""),
	)

	t.Run("a running task's ports are reached on the node holding it", func(t *testing.T) {
		t.Parallel()

		location, err := ingress.BySlug(ctx, "request-01")
		require.NoError(t, err)
		assert.Equal(t, kind.Location{UUID: "01", Node: "workload-orchestrator-01", Ports: []port.Port{3000, 8080}}, location)
	})

	t.Run("one that is not running cannot be reached now, and says which ports it would let in", func(t *testing.T) {
		t.Parallel()

		location, err := ingress.BySlug(ctx, "request-02")
		assert.ErrorIs(t, err, kind.ErrUnreachable)
		assert.ErrorContains(t, err, "the task is not running")
		assert.Equal(t, []port.Port{3000}, location.Ports)
	})

	t.Run("one with no network lets nothing in, and one with no policy is isolated", func(t *testing.T) {
		t.Parallel()

		location, err := ingress.BySlug(ctx, "request-03")
		require.NoError(t, err)
		assert.Empty(t, location.Ports)

		location, err = ingress.BySlug(ctx, "request-04")
		require.NoError(t, err)
		assert.Equal(t, "04", location.UUID)
	})

	t.Run("a slug no task has is not there, for the next kind to be asked", func(t *testing.T) {
		t.Parallel()

		_, err := ingress.BySlug(ctx, "request-09")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("where it is that cannot be said is said", func(t *testing.T) {
		t.Parallel()

		var locations infraIngress.MockLocations
		locations.On("BySlug", mock.Anything, taskKind.Name, "request-01").Once().Return(ingressContract.Heard{}, errors.New("the locations are gone"))
		defer locations.AssertExpectations(t)

		_, err := ingressTasks.New(&locations).BySlug(ctx, "request-01")
		require.Error(t, err)
		assert.NotErrorIs(t, err, domain.ErrNotExists)
	})
}

func TestIngress_Read(t *testing.T) {
	t.Parallel()

	tasks := ingressTasks.New(ingressMemory.NewLocations(time.Minute))

	t.Run("a task is where its run says: under the slug it was made with, on the ports its vm publishes, whether or not they are up", func(t *testing.T) {
		t.Parallel()

		heard, err := tasks.Read(json.RawMessage(`{"state":"completed","run":{"slug":"request-01","ports":[3000],"exit_code":0}}`))
		require.NoError(t, err)
		assert.Equal(t, ingressContract.Heard{Slug: "request-01", State: taskKind.Completed, Ports: []port.Port{3000}}, heard)
	})

	t.Run("one no node has run is reached under no slug, and on no port", func(t *testing.T) {
		t.Parallel()

		heard, err := tasks.Read(json.RawMessage(`{"state":"failed","reason":"no room"}`))
		require.NoError(t, err)
		assert.Equal(t, ingressContract.Heard{State: taskKind.Failed}, heard)
	})

	t.Run("a status that cannot be read is said", func(t *testing.T) {
		t.Parallel()

		_, err := tasks.Read(json.RawMessage(`{"state":`))
		assert.Error(t, err)
	})
}

func TestIngress_Allowed(t *testing.T) {
	t.Parallel()

	tasks := ingressTasks.New(ingressMemory.NewLocations(time.Minute))

	// allowed are the ports a task as the control plane recorded it lets in.
	allowed := func(t *testing.T, spec taskKind.Spec) []port.Port {
		t.Helper()

		raw, err := kind.Encode(taskKind.Task{Kind: taskKind.Name, Metadata: kind.Metadata{UUID: "01"}, Spec: spec})
		require.NoError(t, err)

		ports, err := tasks.Allowed(raw)
		require.NoError(t, err)

		return ports
	}

	t.Run("a task lets in the ports it serves while its policy lets anything in, and one with no policy is isolated", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, []port.Port{3000, 8080}, allowed(t, taskKind.Spec{Ports: []port.Port{3000, 8080}, NetworkPolicy: network.PolicyPublic}))
		assert.Equal(t, []port.Port{3000}, allowed(t, taskKind.Spec{Ports: []port.Port{3000}}))
	})

	t.Run("and none under a policy that lets nothing in", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, allowed(t, taskKind.Spec{Ports: []port.Port{3000}, NetworkPolicy: network.PolicyNone}))
	})

	t.Run("one that cannot be read is said", func(t *testing.T) {
		t.Parallel()

		_, err := tasks.Allowed(kind.Raw{Kind: taskKind.Name, Spec: json.RawMessage(`{"ports":`)})
		assert.Error(t, err)
	})

	t.Run("it is the task kind, reached while it runs", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, taskKind.Name, tasks.Descriptor().Name)
		assert.Equal(t, taskKind.Running, tasks.Running())
	})
}
