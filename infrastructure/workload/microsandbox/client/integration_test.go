package client_test

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/task/attachTask"
	taskHeartbeat "github.com/khanzadimahdi/testproject/application/workload/orchestrator/task/beatHeart"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/task/getEndpoint"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/task/runTask"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/task/events"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/client"
)

// The client against workload-microsandbox as the api package describes it:
// what it asks of the service for each of the workload's calls, and what it
// makes of the answers. Where it matters what the orchestrator does with an
// answer, the orchestrator's own use cases are run against the client, as the
// orchestrator runs them.

// web is a public service with two ports, as an orchestrator asks for one: it
// sets everything the client carries to the service.
func web() *task.Execution {
	return &task.Execution{
		Name:        "web-xkfqz",
		TaskUUID:    "4f1c2a9e-8d3b-4c7a-9e2f-1a2b3c4d5e6f",
		TaskName:    "web",
		Slug:        "web-xkfqz",
		Kind:        task.KindService,
		NodeName:    orchestrator,
		OwnerUUID:   "8a7b6c5d-4e3f-4a1b-9c8d-7e6f5a4b3c2d",
		Attempt:     2,
		Interactive: true,
		TTL:         time.Hour,

		Image:            "nginx:alpine",
		ResourceLimits:   task.ResourceLimits{Cpu: 0.25, Memory: 256 << 20, Disk: 1 << 30},
		RestartPolicy:    "unless-stopped",
		WorkingDirectory: "/usr/share/nginx/html",
		ExposedPorts:     port.PortSet{80: {}, 443: {}},
		PortBindings:     port.PortMap{80: {{HostIP: "0.0.0.0"}}, 443: {{HostIP: "0.0.0.0"}}},
		Networks:         network.Attachments(network.PolicyPublic, "", ""),
		Environment:      []string{"NGINX_PORT=80", "TZ=UTC"},
		Entrypoint:       []string{"/docker-entrypoint.sh"},
		Command:          []string{"nginx", "-g", "daemon off;"},
	}
}

// snippet is an isolated job, as the code runner asks for one: it sets only
// what every task has.
func snippet() *task.Execution {
	return &task.Execution{
		Name:           "snippet-q7w2e",
		TaskUUID:       "9e8d7c6b-5a4f-4e3d-8c2b-1a0f9e8d7c6b",
		TaskName:       "snippet",
		Slug:           "snippet-q7w2e",
		Kind:           task.KindJob,
		NodeName:       orchestrator,
		Image:          "busybox:1.37",
		ResourceLimits: task.ResourceLimits{Cpu: 1, Memory: 64 << 20, Disk: 64 << 20},
		Networks:       network.Attachments(network.PolicyIsolated, "", ""),
		Command:        []string{"sh", "-c", "echo hello"},
	}
}

// created makes a run of an execution, and says its ID.
func created(t *testing.T, runtime *client.Runtime, execution *task.Execution) string {
	t.Helper()

	id, err := runtime.Create(t.Context(), execution)
	require.NoError(t, err)

	return id
}

// started makes a run of an execution and starts it, and says its ID.
func started(t *testing.T, runtime *client.Runtime, execution *task.Execution) string {
	t.Helper()

	id := created(t, runtime, execution)
	require.NoError(t, runtime.Start(t.Context(), id))

	return id
}

// hostPorts are the host ports a run's guest ports were published on.
func hostPorts(t *testing.T, s *service, id string) map[uint16]uint16 {
	t.Helper()

	run, ok := s.run(id)
	require.True(t, ok)

	published := make(map[uint16]uint16, len(run.Endpoints))
	for _, endpoint := range run.Endpoints {
		published[endpoint.Port] = endpoint.HostPort
	}

	return published
}

func discard() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func accepts() *validator.MockValidator {
	v := &validator.MockValidator{}
	v.On("Validate", mock.Anything).Return(domain.ValidationErrors{})

	return v
}

func TestRuntime_Lifecycle(t *testing.T) {
	t.Parallel()

	s := newService(t)
	runtime := s.runtime(t, orchestrator)
	ctx := t.Context()

	execution := web()
	id := created(t, runtime, execution)

	t.Run("an execution is asked for as the run it describes", func(t *testing.T) {
		asked := s.asked(api.RouteCreateRun)
		require.Len(t, asked, 1)

		var spec api.RunSpec
		require.NoError(t, json.Unmarshal(asked[0].body, &spec))

		assert.Equal(t, api.RunSpec{
			Node:          orchestrator,
			Name:          "web-xkfqz",
			Image:         "nginx:alpine",
			Entrypoint:    []string{"/docker-entrypoint.sh"},
			Command:       []string{"nginx", "-g", "daemon off;"},
			Environment:   []string{"NGINX_PORT=80", "TZ=UTC"},
			WorkingDir:    "/usr/share/nginx/html",
			CPU:           0.25,
			Memory:        256 << 20,
			Disk:          1 << 30,
			Network:       api.NetworkPublic,
			Ports:         []uint16{80, 443},
			RestartPolicy: "unless-stopped",
			Task: api.Task{
				UUID:        "4f1c2a9e-8d3b-4c7a-9e2f-1a2b3c4d5e6f",
				Name:        "web",
				Slug:        "web-xkfqz",
				Kind:        "service",
				Owner:       "8a7b6c5d-4e3f-4a1b-9c8d-7e6f5a4b3c2d",
				Attempt:     2,
				Interactive: true,
				TTLSeconds:  3600,
			},
		}, spec)
	})

	t.Run("what went in comes back out, and a run that was never started has no ports", func(t *testing.T) {
		run, ok := s.run(id)
		require.True(t, ok)

		want := *execution
		want.ID = id
		want.Status = task.StatusCreated
		want.CreatedAt = run.CreatedAt
		want.ExposedPorts = port.PortSet{}
		want.PortBindings = port.PortMap{}

		inspected, err := runtime.Inspect(ctx, id)
		require.NoError(t, err)

		assert.Equal(t, want, inspected)
		assert.Equal(t, task.Scheduled, task.EvaluateState(inspected.Status, inspected.Kind, inspected.ExitCode))
	})

	t.Run("a running run reports its ports as docker reports a container's", func(t *testing.T) {
		require.NoError(t, runtime.Start(ctx, id))

		published := hostPorts(t, s, id)
		require.Len(t, published, 2)

		inspected, err := runtime.Inspect(ctx, id)
		require.NoError(t, err)

		run, _ := s.run(id)

		assert.Equal(t, task.StatusRunning, inspected.Status)
		assert.Equal(t, run.StartedAt, inspected.StartedAt)
		assert.Equal(t, port.PortSet{80: {}, 443: {}}, inspected.ExposedPorts)
		assert.Equal(t, port.PortMap{
			80:  {{HostIP: "0.0.0.0", HostPort: port.Port(published[80])}},
			443: {{HostIP: "0.0.0.0", HostPort: port.Port(published[443])}},
		}, inspected.PortBindings)
		assert.Equal(t, task.Running, task.EvaluateState(inspected.Status, inspected.Kind, inspected.ExitCode))
	})

	t.Run("starting a running run changes nothing", func(t *testing.T) {
		before := hostPorts(t, s, id)

		require.NoError(t, runtime.Start(ctx, id))

		assert.Equal(t, before, hostPorts(t, s, id))
	})

	t.Run("a stop gives the main process docker's ten seconds, and ends what it published", func(t *testing.T) {
		require.NoError(t, runtime.Stop(ctx, id))

		asked := s.asked(api.RouteStopRun)
		require.Len(t, asked, 1)
		assert.JSONEq(t, `{"timeout_seconds":10}`, string(asked[0].body))

		inspected, err := runtime.Inspect(ctx, id)
		require.NoError(t, err)

		assert.Equal(t, task.StatusExited, inspected.Status)
		assert.Equal(t, stoppedBySignal, inspected.ExitCode)
		assert.Empty(t, inspected.ExposedPorts)
		assert.Empty(t, inspected.PortBindings)
		assert.Equal(t, task.Stopped, task.EvaluateState(inspected.Status, inspected.Kind, inspected.ExitCode))
	})

	t.Run("stopping a run that has ended changes nothing", func(t *testing.T) {
		require.NoError(t, runtime.Stop(ctx, id))

		inspected, err := runtime.Inspect(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, stoppedBySignal, inspected.ExitCode)
	})

	t.Run("a run that ended starts again", func(t *testing.T) {
		require.NoError(t, runtime.Start(ctx, id))

		inspected, err := runtime.Inspect(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, task.StatusRunning, inspected.Status)
		assert.Zero(t, inspected.ExitCode)
	})

	t.Run("a kill is a kill", func(t *testing.T) {
		require.NoError(t, runtime.Kill(ctx, id))

		asked := s.asked(api.RouteKillRun)
		require.Len(t, asked, 1)
		assert.Empty(t, asked[0].body)

		inspected, err := runtime.Inspect(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, task.StatusExited, inspected.Status)
		assert.Equal(t, killed, inspected.ExitCode)
	})

	t.Run("a restart is the same run again, and not one of its policy's restarts", func(t *testing.T) {
		require.NoError(t, runtime.Restart(ctx, id))

		asked := s.asked(api.RouteRestartRun)
		require.Len(t, asked, 1)
		assert.JSONEq(t, `{"timeout_seconds":10}`, string(asked[0].body))

		inspected, err := runtime.Inspect(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, id, inspected.ID)
		assert.Equal(t, task.StatusRunning, inspected.Status)
		assert.Zero(t, inspected.RestartCount)
	})

	t.Run("a deleted run is gone, and deleting it again says so", func(t *testing.T) {
		require.NoError(t, runtime.Delete(ctx, id))

		_, err := runtime.Inspect(ctx, id)
		assert.ErrorIs(t, err, domain.ErrNotExists)

		assert.ErrorIs(t, runtime.Delete(ctx, id), domain.ErrNotExists)
	})
}

func TestRuntime_Listing(t *testing.T) {
	t.Parallel()

	s := newService(t)
	ctx := t.Context()

	runtime := s.runtime(t, orchestrator)
	neighbours := s.runtime(t, neighbour)

	webID := started(t, runtime, web())
	snippetID := created(t, runtime, snippet())

	// another node's run of the same task, as a task rescheduled elsewhere
	// leaves behind.
	elsewhere := web()
	elsewhere.NodeName = neighbour
	elsewhereID := created(t, neighbours, elsewhere)

	ids := func(executions []task.Execution) []string {
		listed := make([]string, len(executions))
		for i, e := range executions {
			listed[i] = e.ID
		}

		return listed
	}

	t.Run("a node holds its own runs, the latest first, as docker lists containers", func(t *testing.T) {
		held, err := runtime.OnNode(ctx, orchestrator)
		require.NoError(t, err)

		assert.Equal(t, []string{snippetID, webID}, ids(held))

		assert.Equal(t, task.StatusRunning, held[1].Status)
		assert.Equal(t, task.KindService, held[1].Kind)
		assert.Equal(t, "web-xkfqz", held[1].Slug)
		assert.NotEmpty(t, held[1].PortBindings, "a heartbeat reads the ports off the listing")

		theirs, err := runtime.OnNode(ctx, neighbour)
		require.NoError(t, err)
		assert.Equal(t, []string{elsewhereID}, ids(theirs))
	})

	t.Run("a task's runs and a slug's are looked for among the node's own", func(t *testing.T) {
		of, err := runtime.Of(ctx, web().TaskUUID)
		require.NoError(t, err)
		assert.Equal(t, []string{webID}, ids(of))

		bySlug, err := runtime.BySlug(ctx, "web-xkfqz")
		require.NoError(t, err)
		assert.Equal(t, []string{webID}, ids(bySlug))

		theirs, err := neighbours.Of(ctx, web().TaskUUID)
		require.NoError(t, err)
		assert.Equal(t, []string{elsewhereID}, ids(theirs))

		asked := s.asked(api.RouteListRuns)
		assert.Equal(t, []string{orchestrator}, asked[len(asked)-3].query[api.QueryNode])
		assert.Equal(t, []string{web().TaskUUID}, asked[len(asked)-3].query[api.QueryTask])
		assert.Equal(t, []string{"web-xkfqz"}, asked[len(asked)-2].query[api.QuerySlug])
	})

	t.Run("what nobody is asked about is answered with nothing, and nobody is asked", func(t *testing.T) {
		before := len(s.asked(api.RouteListRuns))

		for _, list := range []func() ([]task.Execution, error){
			func() ([]task.Execution, error) { return runtime.OnNode(ctx, "") },
			func() ([]task.Execution, error) { return runtime.Of(ctx, "") },
			func() ([]task.Execution, error) { return runtime.BySlug(ctx, "") },
		} {
			listed, err := list()
			require.NoError(t, err)
			assert.Equal(t, []task.Execution{}, listed)
		}

		assert.Len(t, s.asked(api.RouteListRuns), before)
	})

	t.Run("a task that has no runs has an empty list, not an error", func(t *testing.T) {
		listed, err := runtime.Of(ctx, "00000000-0000-0000-0000-000000000000")
		require.NoError(t, err)
		assert.Equal(t, []task.Execution{}, listed)
	})
}

func TestRuntime_Refusals(t *testing.T) {
	t.Parallel()

	// what microsandbox cannot run, each with the reason the task is given,
	// word for word.
	refusals := []struct {
		name   string
		modify func(*task.Execution)
		reason string
	}{
		{
			name: "a service of a stack",
			modify: func(e *task.Execution) {
				e.StackUUID = "c3d2e1f0-a9b8-4c7d-8e6f-5a4b3c2d1e0f"
				e.Networks = network.Attachments(network.PolicyIsolated, "shop", "api")
			},
			reason: "microsandbox cannot run a stack: its microVMs share no network",
		},
		{
			name:   "a public service of a stack",
			modify: func(e *task.Execution) { e.Networks = network.Attachments(network.PolicyPublic, "shop", "api") },
			reason: "microsandbox cannot run a stack: its microVMs share no network",
		},
		{
			name:   "a read-only root",
			modify: func(e *task.Execution) { e.ReadOnly = true },
			reason: "microsandbox cannot give a task a read-only root",
		},
		{
			name:   "no network at all",
			modify: func(e *task.Execution) { e.Networks = network.Attachments(network.PolicyNone, "", "") },
			reason: "microsandbox cannot run a task with no network interface",
		},
		{
			name:   "nothing to be on, which docker takes as no network",
			modify: func(e *task.Execution) { e.Networks = nil },
			reason: "microsandbox cannot run a task with no network interface",
		},
	}

	for _, tt := range refusals {
		t.Run(tt.name+" is refused before the service is asked", func(t *testing.T) {
			t.Parallel()

			s := newService(t)

			execution := snippet()
			tt.modify(execution)

			_, err := s.runtime(t, orchestrator).Create(t.Context(), execution)

			assert.EqualError(t, err, tt.reason)
			assert.Empty(t, s.routesAsked())
		})
	}

	t.Run("a stack's network is refused, and there is never one to remove", func(t *testing.T) {
		t.Parallel()

		networks := client.NewNetworkManager()

		assert.EqualError(t, networks.EnsureStackNetwork(t.Context(), "shop"), "microsandbox cannot run a stack: its microVMs share no network")
		assert.ErrorIs(t, networks.EnsureStackNetwork(t.Context(), "shop"), client.ErrStack)
		assert.NoError(t, networks.RemoveStackNetwork(t.Context(), "shop"))
		assert.NoError(t, networks.EnsureIsolatedNetwork(t.Context()))
	})

	// what the orchestrator does with a refusal: it reports the task as
	// failed, with the refusal as its reason.
	scheduled := []struct {
		name      string
		scheduled events.TaskScheduled
		reason    string
	}{
		{
			name:      "a stack's service",
			scheduled: events.TaskScheduled{StackUUID: "c3d2e1f0-a9b8-4c7d-8e6f-5a4b3c2d1e0f", StackSlug: "shop", ServiceName: "api"},
			reason:    "microsandbox cannot run a stack: its microVMs share no network",
		},
		{
			name:      "a read-only task",
			scheduled: events.TaskScheduled{ReadOnly: true},
			reason:    "microsandbox cannot give a task a read-only root",
		},
		{
			name:      "a task with no network",
			scheduled: events.TaskScheduled{NetworkPolicy: network.PolicyNone},
			reason:    "microsandbox cannot run a task with no network interface",
		},
	}

	for _, tt := range scheduled {
		t.Run(tt.name+" fails with the refusal as its reason", func(t *testing.T) {
			t.Parallel()

			s := newService(t)

			message := tt.scheduled
			message.UUID = "5b6c7d8e-9f0a-4b1c-8d2e-3f4a5b6c7d8e"
			message.Name = "a-request-id"
			message.Slug = "refused-x1y2z"
			message.Image = "busybox:1.37"
			message.NominatedNode = orchestrator
			message.ResourceLimits = events.ResourceLimits{Cpu: 1, Memory: 64 << 20, Disk: 64 << 20}

			payload, err := json.Marshal(message)
			require.NoError(t, err)

			var producer messagingMock.MockProduceConsumer
			producer.On("Produce", mock.Anything, events.TaskFailedName, mock.Anything).Return(nil).Once()
			defer producer.AssertExpectations(t)

			useCase := runTask.NewUseCase(s.runtime(t, orchestrator), client.NewNetworkManager(), accepts(), orchestrator)
			require.NoError(t, runTask.NewTaskScheduled(useCase, &producer, orchestrator, discard()).Handle(t.Context(), payload))

			var failed events.TaskFailed
			require.NoError(t, json.Unmarshal(producer.Calls[0].Arguments.Get(2).([]byte), &failed))

			assert.Equal(t, tt.reason, failed.Reason)
			assert.Empty(t, s.asked(api.RouteCreateRun), "nothing reaches the service that it would only have to refuse")
		})
	}
}

func TestRuntime_Errors(t *testing.T) {
	t.Parallel()

	t.Run("a run the service does not have is one that does not exist", func(t *testing.T) {
		t.Parallel()

		s := newService(t)
		runtime := s.runtime(t, orchestrator)
		ctx := t.Context()

		const missing = "0000000000000000000000000000dead"

		calls := map[string]func() error{
			"start":   func() error { return runtime.Start(ctx, missing) },
			"stop":    func() error { return runtime.Stop(ctx, missing) },
			"restart": func() error { return runtime.Restart(ctx, missing) },
			"kill":    func() error { return runtime.Kill(ctx, missing) },
			"delete":  func() error { return runtime.Delete(ctx, missing) },
			"inspect": func() error { _, err := runtime.Inspect(ctx, missing); return err },
			"stats":   func() error { _, err := runtime.Stats(ctx, missing); return err },
			"logs":    func() error { return runtime.Logs(ctx, missing, io.Discard) },
			"stream logs": func() error {
				return runtime.StreamLogs(ctx, missing, time.Time{}, func(task.LogLine) error { return nil })
			},
			"exec": func() error {
				_, err := runtime.Exec(ctx, missing, task.ExecOptions{Command: []string{"sh"}})
				return err
			},
		}

		for name, call := range calls {
			assert.ErrorIs(t, call(), domain.ErrNotExists, name)
		}
	})

	t.Run("a run is asked for by its ID, and with none is not asked for at all", func(t *testing.T) {
		t.Parallel()

		s := newService(t)

		err := s.runtime(t, orchestrator).Start(t.Context(), "")

		assert.ErrorContains(t, err, "with no id")
		assert.Empty(t, s.routesAsked())
	})

	t.Run("anything else the service refuses is passed on with its own message", func(t *testing.T) {
		t.Parallel()

		refusals := []struct {
			route   string
			status  int
			code    string
			message string
			call    func(*client.Runtime, string) error
		}{
			{
				route: api.RouteStartRun, status: http.StatusConflict, code: api.CodeCapacity,
				message: "the node's memory budget cannot take 1.06 GiB more",
				call:    func(r *client.Runtime, id string) error { return r.Start(context.Background(), id) },
			},
			{
				route: api.RouteStartRun, status: http.StatusBadGateway, code: api.CodePullFailed,
				message: "nginx:alpine could not be pulled: manifest unknown",
				call:    func(r *client.Runtime, id string) error { return r.Start(context.Background(), id) },
			},
			{
				route: api.RouteStopRun, status: http.StatusServiceUnavailable, code: api.CodeUnavailable,
				message: "the service is adopting its runs again",
				call:    func(r *client.Runtime, id string) error { return r.Stop(context.Background(), id) },
			},
			{
				route: api.RoutePullImage, status: http.StatusBadRequest, code: api.CodeNotSupported,
				message: "nginx:alpine has no variant for arm64",
				call:    func(r *client.Runtime, _ string) error { return r.EnsureImage(context.Background(), "nginx:alpine") },
			},
			{
				route: api.RouteCreateRun, status: http.StatusBadRequest, code: api.CodeInvalid,
				message: "an environment entry cannot hold a tab",
				call: func(r *client.Runtime, _ string) error {
					_, err := r.Create(context.Background(), snippet())
					return err
				},
			},
		}

		for _, tt := range refusals {
			s := newService(t)
			runtime := s.runtime(t, orchestrator)
			id := created(t, runtime, web())

			s.fail(tt.route, failure{status: tt.status, code: tt.code, message: tt.message})

			err := tt.call(runtime, id)

			assert.EqualError(t, err, tt.message)

			var refusal *api.Error
			require.ErrorAs(t, err, &refusal)
			assert.Equal(t, tt.code, refusal.Code)
		}
	})

	t.Run("a name the node already uses is refused, and the orchestrator takes the run that is there", func(t *testing.T) {
		t.Parallel()

		s := newService(t)
		runtime := s.runtime(t, orchestrator)

		existing := created(t, runtime, web())

		_, err := runtime.Create(t.Context(), web())

		var refusal *api.Error
		require.ErrorAs(t, err, &refusal)
		assert.Equal(t, api.CodeNameInUse, refusal.Code)

		// the same task asked for twice, as a redelivered message asks for it.
		payload, err := json.Marshal(events.TaskScheduled{
			UUID:           web().TaskUUID,
			Name:           "web",
			Slug:           "web-xkfqz",
			Kind:           string(task.KindService),
			Image:          "nginx:alpine",
			ExposedPorts:   []port.Port{80, 443},
			NetworkPolicy:  network.PolicyPublic,
			ResourceLimits: events.ResourceLimits{Cpu: 0.25, Memory: 256 << 20, Disk: 1 << 30},
			NominatedNode:  orchestrator,
			Attempt:        2,
		})
		require.NoError(t, err)

		var producer messagingMock.MockProduceConsumer
		defer producer.AssertExpectations(t)

		useCase := runTask.NewUseCase(runtime, client.NewNetworkManager(), accepts(), orchestrator)
		require.NoError(t, runTask.NewTaskScheduled(useCase, &producer, orchestrator, discard()).Handle(t.Context(), payload))

		run, ok := s.run(existing)
		require.True(t, ok)
		assert.Equal(t, api.StateRunning, run.State, "the run that was there is the one started")
	})

	t.Run("an answer that is not the service's own is passed on as what it is", func(t *testing.T) {
		t.Parallel()

		s := newService(t)
		runtime := s.runtime(t, orchestrator)
		id := created(t, runtime, web())

		s.fail(api.RouteGetRun, failure{status: http.StatusBadGateway, message: "upstream went away"})

		_, err := runtime.Inspect(t.Context(), id)

		assert.EqualError(t, err, "workload-microsandbox answered 502 Bad Gateway: upstream went away")

		var refusal *api.Error
		require.ErrorAs(t, err, &refusal)
		assert.Equal(t, api.CodeInternal, refusal.Code)
	})

	t.Run("a service that hangs up is one that cannot be reached", func(t *testing.T) {
		t.Parallel()

		s := newService(t)
		runtime := s.runtime(t, orchestrator)
		id := created(t, runtime, web())

		s.fail(api.RouteStartRun, failure{drop: true})

		assert.ErrorIs(t, runtime.Start(t.Context(), id), client.ErrUnreachable)
	})

	t.Run("a service that is not there is one that cannot be reached, and says where it was looked for", func(t *testing.T) {
		t.Parallel()

		s := newService(t)
		runtime := s.runtime(t, orchestrator)
		s.close()

		_, err := runtime.OnNode(t.Context(), orchestrator)

		assert.ErrorIs(t, err, client.ErrUnreachable)
		assert.ErrorContains(t, err, s.server.URL)
	})
}

func TestClient_Version(t *testing.T) {
	t.Parallel()

	t.Run("a service of another major version is refused before it is asked anything", func(t *testing.T) {
		t.Parallel()

		s := newService(t)
		s.speak("2")

		runtime := s.runtime(t, orchestrator)

		_, err := runtime.Create(t.Context(), web())

		assert.ErrorIs(t, err, client.ErrIncompatible)
		assert.EqualError(t, err, `workload-microsandbox speaks another version of its API: it speaks version "2", and this orchestrator version "1"`)
		assert.Empty(t, s.routesAsked())
	})

	t.Run("the version is asked once, however much else is", func(t *testing.T) {
		t.Parallel()

		s := newService(t)
		runtime := s.runtime(t, orchestrator)
		nodes := client.NewNodeManager(s.client(t, orchestrator))

		for range 3 {
			_, err := runtime.OnNode(t.Context(), orchestrator)
			require.NoError(t, err)
		}

		_, err := nodes.Stats(t.Context(), orchestrator)
		require.NoError(t, err)

		// one client each.
		assert.Len(t, s.asked(api.RouteInfo), 2)
	})

	t.Run("a service that was lost is asked its version again, since it may have come back as another", func(t *testing.T) {
		t.Parallel()

		s := newService(t)
		runtime := s.runtime(t, orchestrator)

		_, err := runtime.OnNode(t.Context(), orchestrator)
		require.NoError(t, err)

		s.fail(api.RouteListRuns, failure{drop: true})

		_, err = runtime.OnNode(t.Context(), orchestrator)
		require.ErrorIs(t, err, client.ErrUnreachable)

		s.recover(api.RouteListRuns)
		s.speak("2")

		_, err = runtime.OnNode(t.Context(), orchestrator)
		assert.ErrorIs(t, err, client.ErrIncompatible)
		assert.Len(t, s.asked(api.RouteInfo), 2)
	})

	t.Run("a refusal is not losing the service", func(t *testing.T) {
		t.Parallel()

		s := newService(t)
		runtime := s.runtime(t, orchestrator)

		_, err := runtime.Inspect(t.Context(), "0000000000000000000000000000dead")
		require.ErrorIs(t, err, domain.ErrNotExists)

		_, err = runtime.Inspect(t.Context(), "0000000000000000000000000000beef")
		require.ErrorIs(t, err, domain.ErrNotExists)

		assert.Len(t, s.asked(api.RouteInfo), 1)
	})
}

func TestClient_MutualTLS(t *testing.T) {
	t.Parallel()

	t.Run("the service answers no client its authority did not sign for", func(t *testing.T) {
		t.Parallel()

		s := newService(t)
		stranger := newAuthority(t)
		certificatePEM, keyPEM := stranger.orchestratorCertificate(t, orchestrator)

		c, err := client.New(client.Config{
			URL:         s.server.URL,
			Node:        orchestrator,
			Authority:   s.authority.pem,
			Certificate: certificatePEM,
			PrivateKey:  keyPEM,
		}, discard())
		require.NoError(t, err)

		_, err = client.NewRuntime(c).OnNode(t.Context(), orchestrator)

		assert.ErrorIs(t, err, client.ErrUnreachable)
		assert.Empty(t, s.asked(api.RouteInfo))
	})

	t.Run("nor a certificate that was signed for serving rather than for connecting", func(t *testing.T) {
		t.Parallel()

		s := newService(t)
		certificatePEM, keyPEM := s.authority.serviceCertificate(t)

		c, err := client.New(client.Config{
			URL:         s.server.URL,
			Node:        orchestrator,
			Authority:   s.authority.pem,
			Certificate: certificatePEM,
			PrivateKey:  keyPEM,
		}, discard())
		require.NoError(t, err)

		_, err = client.NewRuntime(c).OnNode(t.Context(), orchestrator)

		assert.ErrorIs(t, err, client.ErrUnreachable)
		assert.Empty(t, s.asked(api.RouteInfo))
	})

	t.Run("the client hands its certificate to no service its own authority did not sign", func(t *testing.T) {
		t.Parallel()

		s := newService(t)
		stranger := newAuthority(t)
		certificatePEM, keyPEM := stranger.orchestratorCertificate(t, orchestrator)

		c, err := client.New(client.Config{
			URL:         s.server.URL,
			Node:        orchestrator,
			Authority:   stranger.pem,
			Certificate: certificatePEM,
			PrivateKey:  keyPEM,
		}, discard())
		require.NoError(t, err)

		_, err = client.NewRuntime(c).OnNode(t.Context(), orchestrator)

		assert.ErrorIs(t, err, client.ErrUnreachable)
		assert.ErrorContains(t, err, "certificate")
	})

	t.Run("nor to one whose certificate does not answer for the host it was reached at", func(t *testing.T) {
		t.Parallel()

		// signed by the right authority, but for its own name alone: anything
		// the authority signed for anybody would do otherwise, another
		// orchestrator's server certificate included.
		s := newServiceAnsweringFor(t)
		certificatePEM, keyPEM := s.authority.orchestratorCertificate(t, orchestrator)

		c, err := client.New(client.Config{
			URL:         s.server.URL,
			Node:        orchestrator,
			Authority:   s.authority.pem,
			Certificate: certificatePEM,
			PrivateKey:  keyPEM,
		}, discard())
		require.NoError(t, err)

		_, err = client.NewRuntime(c).OnNode(t.Context(), orchestrator)

		assert.ErrorIs(t, err, client.ErrUnreachable)

		var mismatch x509.HostnameError
		assert.ErrorAs(t, err, &mismatch)
		assert.Empty(t, s.asked(api.RouteInfo))
	})

	t.Run("and it is reached by name as well as by address", func(t *testing.T) {
		t.Parallel()

		s := newService(t)
		certificatePEM, keyPEM := s.authority.orchestratorCertificate(t, orchestrator)

		_, servicePort, _ := strings.Cut(strings.TrimPrefix(s.server.URL, "https://"), ":")

		c, err := client.New(client.Config{
			URL:         "https://localhost:" + servicePort,
			Node:        orchestrator,
			Authority:   s.authority.pem,
			Certificate: certificatePEM,
			PrivateKey:  keyPEM,
		}, discard())
		require.NoError(t, err)

		info, err := c.Info(t.Context())
		require.NoError(t, err)
		assert.Equal(t, api.Version, info.APIVersion)
	})
}

func TestRuntime_Ports(t *testing.T) {
	t.Parallel()

	s := newService(t)
	runtime := s.runtime(t, orchestrator)
	ctx := t.Context()

	id := started(t, runtime, web())
	published := hostPorts(t, s, id)

	// the orchestrator's own configuration, which says where a task's port is
	// reached.
	orchestratorConfigs := configs.NewWorkloadOrchestrator()
	orchestratorConfigs.Name = orchestrator
	orchestratorConfigs.Runtime = configs.RuntimeMicrosandbox
	orchestratorConfigs.MicrosandboxURL = s.server.URL
	orchestratorConfigs.AdvertiseHost = "docker"

	t.Run("a heartbeat reports the ports that came up", func(t *testing.T) {
		var producer messagingMock.MockProduceConsumer
		producer.On("Produce", mock.Anything, events.HeartbeatName, mock.Anything).Return(nil).Once()
		defer producer.AssertExpectations(t)

		require.NoError(t, taskHeartbeat.NewUseCase(runtime, &producer, orchestrator, discard()).Execute(ctx))

		var heartbeat events.Heartbeat
		require.NoError(t, json.Unmarshal(producer.Calls[0].Arguments.Get(2).([]byte), &heartbeat))

		assert.Equal(t, id, heartbeat.ExecutionID)
		assert.Equal(t, int(task.Running), heartbeat.State)
		assert.Equal(t, []events.Endpoint{
			{TaskPort: 80, HostPort: port.Port(published[80])},
			{TaskPort: 443, HostPort: port.Port(published[443])},
		}, heartbeat.Endpoints)

		// counted from when the run started, which only inspecting it says.
		run, _ := s.run(id)
		assert.WithinDuration(t, run.StartedAt.Add(time.Hour), heartbeat.Deadline, 0)
	})

	t.Run("a port is reached at the service's host, on the port the service published it on", func(t *testing.T) {
		endpoints := getEndpoint.NewUseCase(runtime, orchestratorConfigs.PortsHost())

		// a connection of its own each time, so none is left open behind the
		// test.
		visitor := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}

		for asked, guest := range map[port.Port]uint16{443: 443, 0: 80} {
			endpoint, err := endpoints.Execute(ctx, &getEndpoint.Request{Slug: "web-xkfqz", Port: asked})
			require.NoError(t, err)

			assert.Equal(t, "127.0.0.1", endpoint.Host)
			assert.Equal(t, port.Port(published[guest]), endpoint.Port)

			response, err := visitor.Get("http://" + endpoint.Address() + "/")
			require.NoError(t, err)

			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			require.NoError(t, err)

			assert.Equal(t, "web-xkfqz answers on its port "+strconv.Itoa(int(guest)), string(body))
		}
	})

	t.Run("a stopped task has no port to reach", func(t *testing.T) {
		require.NoError(t, runtime.Stop(ctx, id))

		_, err := getEndpoint.NewUseCase(runtime, orchestratorConfigs.PortsHost()).
			Execute(ctx, &getEndpoint.Request{Slug: "web-xkfqz"})

		assert.ErrorIs(t, err, getEndpoint.ErrNotRunning)
	})
}

func TestRuntime_Logs(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, time.October, 4, 9, 30, 2, 0, time.UTC)

	// lines a nanosecond apart, as the service stamps lines read in the same
	// instant: both streams, and an empty line.
	lines := []api.LogLine{
		{Stream: api.StreamStdout, At: at, Content: "first"},
		{Stream: api.StreamStderr, At: at.Add(1), Content: "second"},
		{Stream: api.StreamStdout, At: at.Add(2), Content: ""},
		{Stream: api.StreamStdout, At: at.Add(3), Content: "third"},
	}

	t.Run("a job's log is written whole, a line at a time, both streams in the order they came", func(t *testing.T) {
		t.Parallel()

		s := newService(t)
		runtime := s.runtime(t, orchestrator)
		id := started(t, runtime, snippet())
		s.write(id, lines...)

		var log bytes.Buffer
		require.NoError(t, runtime.Logs(t.Context(), id, &log))

		assert.Equal(t, "first\nsecond\n\nthird\n", log.String())

		asked := s.asked(api.RouteRunLogs)
		require.Len(t, asked, 1)
		assert.Empty(t, asked[0].query, "the whole log, and no following it")
	})

	t.Run("a log followed from a line on gets that line, what came after, and what comes next, until the run ends", func(t *testing.T) {
		t.Parallel()

		s := newService(t)
		runtime := s.runtime(t, orchestrator)
		id := started(t, runtime, snippet())
		s.write(id, lines...)

		var (
			lock     sync.Mutex
			received []task.LogLine
		)

		arrived := make(chan struct{}, 16)
		done := make(chan error, 1)

		go func() {
			done <- runtime.StreamLogs(t.Context(), id, at.Add(1), func(line task.LogLine) error {
				lock.Lock()
				received = append(received, line)
				lock.Unlock()

				arrived <- struct{}{}

				return nil
			})
		}()

		for range 3 {
			<-arrived
		}

		s.write(id, api.LogLine{Stream: api.StreamStdout, At: at.Add(time.Second), Content: "fourth"})
		<-arrived

		s.exit(id, 0)
		require.NoError(t, <-done)

		lock.Lock()
		defer lock.Unlock()

		assert.Equal(t, []task.LogLine{
			{Stream: task.StreamStderr, At: at.Add(1), Content: "second"},
			{Stream: task.StreamStdout, At: at.Add(2), Content: ""},
			{Stream: task.StreamStdout, At: at.Add(3), Content: "third"},
			{Stream: task.StreamStdout, At: at.Add(time.Second), Content: "fourth"},
		}, received)

		asked := s.asked(api.RouteRunLogs)
		require.Len(t, asked, 1)
		assert.Equal(t, []string{"2026-10-04T09:30:02.000000001Z"}, asked[0].query[api.QuerySince])
		assert.Equal(t, []string{"true"}, asked[0].query[api.QueryFollow])
	})

	t.Run("a time in another zone is the same instant", func(t *testing.T) {
		t.Parallel()

		s := newService(t)
		runtime := s.runtime(t, orchestrator)
		id := created(t, runtime, snippet())

		tehran := time.FixedZone("IRST", 3*60*60+30*60)
		require.NoError(t, runtime.StreamLogs(t.Context(), id, at.In(tehran), func(task.LogLine) error { return nil }))

		asked := s.asked(api.RouteRunLogs)
		require.Len(t, asked, 1)
		assert.Equal(t, []string{"2026-10-04T09:30:02Z"}, asked[0].query[api.QuerySince])
	})

	t.Run("a line refused is the end of the stream, and the refusal is what comes back", func(t *testing.T) {
		t.Parallel()

		s := newService(t)
		runtime := s.runtime(t, orchestrator)
		id := started(t, runtime, snippet())
		s.write(id, lines...)

		refused := errors.New("the control plane is not taking lines")

		var count int
		err := runtime.StreamLogs(t.Context(), id, time.Time{}, func(task.LogLine) error {
			count++

			return refused
		})

		assert.ErrorIs(t, err, refused)
		assert.Equal(t, 1, count)
	})

	t.Run("a caller that stops listening has not failed", func(t *testing.T) {
		t.Parallel()

		s := newService(t)
		runtime := s.runtime(t, orchestrator)
		id := started(t, runtime, snippet())
		s.write(id, lines[0])

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)

		go func() {
			done <- runtime.StreamLogs(ctx, id, time.Time{}, func(task.LogLine) error {
				cancel()

				return nil
			})
		}()

		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(10 * time.Second):
			t.Fatal("following a log did not stop when its caller did")
		}
	})

	t.Run("a log that cannot be followed is an error to try again after", func(t *testing.T) {
		t.Parallel()

		s := newService(t)
		runtime := s.runtime(t, orchestrator)
		id := started(t, runtime, snippet())

		s.fail(api.RouteRunLogs, failure{drop: true})

		err := runtime.StreamLogs(t.Context(), id, time.Time{}, func(task.LogLine) error { return nil })

		assert.ErrorIs(t, err, client.ErrUnreachable)
	})
}

func TestRuntime_Exec(t *testing.T) {
	t.Parallel()

	// read reads from a session until it has want, or gives up.
	read := func(t *testing.T, session io.Reader, want string) string {
		t.Helper()

		got := make([]byte, 0, len(want))
		buffer := make([]byte, 64)

		for len(got) < len(want) {
			n, err := session.Read(buffer)
			got = append(got, buffer[:n]...)

			if err != nil {
				break
			}
		}

		return string(got)
	}

	t.Run("a terminal carries input, output and its size, and outlives its stream until it is ended", func(t *testing.T) {
		t.Parallel()

		s := newService(t)
		runtime := s.runtime(t, orchestrator)
		ctx := t.Context()
		id := started(t, runtime, web())

		session, err := runtime.Exec(ctx, id, task.ExecOptions{
			Command: []string{"/bin/sh"},
			TTY:     true,
			Env:     []string{"TERM=xterm-256color"},
			WorkDir: "/srv",
		})
		require.NoError(t, err)

		opened, ok := s.session("exec-1")
		require.True(t, ok)
		assert.Equal(t, api.ExecRequest{
			Command: []string{"/bin/sh"},
			TTY:     true,
			Env:     []string{"TERM=xterm-256color"},
			WorkDir: "/srv",
		}, opened.request)

		_, err = session.Write([]byte("echo hi\n"))
		require.NoError(t, err)
		assert.Equal(t, "echo hi\n", read(t, session, "echo hi\n"))

		require.NoError(t, session.Resize(ctx, 40, 120))
		assert.Equal(t, "40 120\n", read(t, session, "40 120\n"))

		resized, _ := s.session("exec-1")
		assert.Equal(t, uint16(40), resized.rows)
		assert.Equal(t, uint16(120), resized.cols)

		require.NoError(t, session.Close())
		require.NoError(t, session.Close(), "closing twice is closing once")

		n, err := session.Read(make([]byte, 8))
		assert.Zero(t, n)
		assert.ErrorIs(t, err, io.EOF, "a closed session has nothing more to read")

		still, _ := s.session("exec-1")
		assert.False(t, still.ended, "closing the stream leaves the command running")

		require.NoError(t, session.End(ctx))

		ended, _ := s.session("exec-1")
		assert.True(t, ended.ended)

		asked := s.asked(api.RouteEndExec)
		require.Len(t, asked, 1)

		require.NoError(t, session.End(ctx), "a command that is no longer there has been ended")
	})

	t.Run("the attach use case opens a shell, as the terminal asks for one", func(t *testing.T) {
		t.Parallel()

		s := newService(t)
		runtime := s.runtime(t, orchestrator)
		started(t, runtime, web())

		session, validationErrors, err := attachTask.NewUseCase(runtime, accepts()).Execute(t.Context(), &attachTask.Request{
			UUID:      web().TaskUUID,
			TTY:       true,
			OwnerUUID: web().OwnerUUID,
		})
		require.NoError(t, err)
		require.Empty(t, validationErrors)
		defer session.Close()

		opened, ok := s.session("exec-1")
		require.True(t, ok)
		assert.Equal(t, []string{"/bin/sh"}, opened.request.Command)
		assert.True(t, opened.request.TTY)

		_, _, err = attachTask.NewUseCase(runtime, accepts()).Execute(t.Context(), &attachTask.Request{
			UUID:      web().TaskUUID,
			TTY:       true,
			OwnerUUID: "somebody-else",
		})
		assert.ErrorIs(t, err, domain.ErrNotExists, "a task is opened for its owner alone")
	})

	t.Run("a command's output is read to its exit, both streams of it", func(t *testing.T) {
		t.Parallel()

		s := newService(t)
		runtime := s.runtime(t, orchestrator)
		id := started(t, runtime, web())

		session, err := runtime.Exec(t.Context(), id, task.ExecOptions{Command: []string{"exit", "3"}})
		require.NoError(t, err)
		defer session.Close()

		output, err := io.ReadAll(session)
		require.NoError(t, err)
		assert.Equal(t, "leaving\nwith 3\n", string(output))

		n, err := session.Read(make([]byte, 8))
		assert.Zero(t, n)
		assert.ErrorIs(t, err, io.EOF)
	})

	t.Run("a command the service cannot start is refused with its reason", func(t *testing.T) {
		t.Parallel()

		s := newService(t)
		runtime := s.runtime(t, orchestrator)
		id := started(t, runtime, web())

		_, err := runtime.Exec(t.Context(), id, task.ExecOptions{})

		assert.EqualError(t, err, "a command has to be named")

		var refusal *api.Error
		require.ErrorAs(t, err, &refusal)
		assert.Equal(t, api.CodeInvalid, refusal.Code)
	})

	t.Run("a run that is not running runs no command", func(t *testing.T) {
		t.Parallel()

		s := newService(t)
		runtime := s.runtime(t, orchestrator)
		id := created(t, runtime, web())

		_, err := runtime.Exec(t.Context(), id, task.ExecOptions{Command: []string{"/bin/sh"}, TTY: true})

		var refusal *api.Error
		require.ErrorAs(t, err, &refusal)
		assert.Equal(t, api.CodeNotRunning, refusal.Code)
	})
}

func TestRuntime_Stats(t *testing.T) {
	t.Parallel()

	s := newService(t)
	runtime := s.runtime(t, orchestrator)
	nodes := client.NewNodeManager(s.client(t, orchestrator))
	ctx := t.Context()

	webID := started(t, runtime, web())
	snippetID := started(t, runtime, snippet())

	elsewhere := web()
	elsewhere.NodeName = neighbour
	elsewhereID := started(t, s.runtime(t, neighbour), elsewhere)

	s.measure(webID, api.Stats{CPUPercent: 12.5, MemoryUsage: 64 << 20, MemoryLimit: 256 << 20, NetworkInput: 1000, NetworkOutput: 2000, BlockInput: 3000, BlockOutput: 4000})
	s.measure(snippetID, api.Stats{CPUPercent: 50, MemoryUsage: 32 << 20, MemoryLimit: 64 << 20, NetworkInput: 1, NetworkOutput: 2, BlockInput: 3, BlockOutput: 4})
	s.measure(elsewhereID, api.Stats{CPUPercent: 99, MemoryUsage: 1 << 30, MemoryLimit: 1 << 30})

	t.Run("a run's VM is what it uses", func(t *testing.T) {
		stats, err := runtime.Stats(ctx, webID)
		require.NoError(t, err)

		assert.Equal(t, task.Stats{
			CPUPercent:    12.5,
			MemoryUsage:   64 << 20,
			MemoryLimit:   256 << 20,
			MemoryPercent: 25,
			NetworkInput:  1000,
			NetworkOutput: 2000,
			BlockInput:    3000,
			BlockOutput:   4000,
		}, stats)
	})

	t.Run("a node's runs use what they use between them", func(t *testing.T) {
		stats, err := nodes.Stats(ctx, orchestrator)
		require.NoError(t, err)

		assert.Equal(t, node.Stats{
			CPUPercent:    62.5,
			MemoryUsage:   96 << 20,
			MemoryLimit:   320 << 20,
			MemoryPercent: 30,
			NetworkInput:  1001,
			NetworkOutput: 2002,
			BlockInput:    3003,
			BlockOutput:   4004,
		}, stats)

		asked := s.asked(api.RouteNodeStats)
		require.Len(t, asked, 1)
		assert.Equal(t, []string{orchestrator}, asked[0].query[api.QueryNode])
	})

	t.Run("a run that is not running uses nothing", func(t *testing.T) {
		require.NoError(t, runtime.Stop(ctx, snippetID))

		stats, err := runtime.Stats(ctx, snippetID)
		require.NoError(t, err)
		assert.Equal(t, task.Stats{}, stats)
	})

	t.Run("a node that is not named holds nothing, and nobody is asked", func(t *testing.T) {
		before := len(s.asked(api.RouteNodeStats))

		stats, err := nodes.Stats(ctx, "")
		require.NoError(t, err)
		assert.Equal(t, node.Stats{}, stats)

		assert.Len(t, s.asked(api.RouteNodeStats), before)
	})
}

func TestRuntime_EnsureImage(t *testing.T) {
	t.Parallel()

	s := newService(t)
	runtime := s.runtime(t, orchestrator)

	require.NoError(t, runtime.EnsureImage(t.Context(), "nginx:alpine"))
	require.NoError(t, runtime.EnsureImage(t.Context(), "nginx:alpine"), "an image that is there is no work")

	asked := s.asked(api.RoutePullImage)
	require.Len(t, asked, 2)
	assert.JSONEq(t, `{"reference":"nginx:alpine"}`, string(asked[0].body))
	assert.Equal(t, []string{"nginx:alpine"}, s.images())
}
