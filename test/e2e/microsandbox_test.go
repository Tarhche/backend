//go:build e2e

// Package e2e drives the workload from outside, through the doors a person and
// the blog use, against a stack that is already up. It is built with the e2e
// tag alone, so go test ./... never compiles it, let alone runs it.
//
// This file is the microsandbox runtime's suite. It runs against the
// development stack with every orchestrator in microsandbox mode, which make
// up-microsandbox brings up (make lima-up on a Mac), and make e2e-microsandbox
// runs it. It reaches the control plane's internal API, which asks for no
// token; the ingress; NATS; and the workload-microsandbox service's own API,
// which it is let into as an orchestrator is, with an orchestrator's
// certificate.
//
// What a task does is checked twice where it can be: through the workload, as
// whoever ran the task sees it, and against the service that holds its
// microVM, so a task that ended up a container is caught as well.
//
// Every task is created for an owner made up for the run, which is what keeps
// one run's tasks apart from another's. A terminal is let into a task by its
// owner alone, so the suite signs that owner a token with the development
// PRIVATE_KEY, as the blog would for a person.
//
// The failure drills restart and kill parts of the stack, so they run only with
// -drills (make e2e-microsandbox DRILLS=1). The first needs nothing but the
// service, which is how CI runs it: with -controlplane= on a runner that has
// nothing else.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/auth"
	getstack "github.com/khanzadimahdi/testproject/application/workload/controlplane/stack/getStack"
	runstack "github.com/khanzadimahdi/testproject/application/workload/controlplane/stack/runStack"
	gettask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/getTask"
	gettasklogs "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/getTaskLogs"
	"github.com/khanzadimahdi/testproject/application/workload/spec"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/task/events"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/jetstream/produceConsumer"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

var (
	drills = flag.Bool("drills", false, "also run the failure drills, which restart and kill parts of the stack")

	controlPlaneURL = flag.String("controlplane", envOr("E2E_CONTROLPLANE_URL", "http://localhost:8020"),
		"the control plane's internal API; empty leaves out whatever needs it")
	ingressURL = flag.String("ingress", envOr("E2E_INGRESS_URL", "http://localhost:8030"),
		"the workload ingress")
	ingressDomain = flag.String("ingress-domain", envOr("WORKLOAD_INGRESS_DOMAIN", "workload.localhost"),
		"the domain the ingress serves the tasks' ports under")
	natsURL = flag.String("nats", envOr("E2E_NATS_URL", "nats://localhost:4222"),
		"NATS, where jobs are asked for and heartbeats are heard")
	serviceURL = flag.String("microsandbox", envOr("E2E_MICROSANDBOX_URL", "https://localhost:8443"),
		"the workload-microsandbox service's API")
	containerName = flag.String("container", "workload-microsandbox",
		"the service's container, which the drills restart, and kill things inside")
	recreateCommand = flag.String("recreate",
		"docker compose --profile microsandbox --env-file .env --env-file .env.microsandbox up --detach --no-deps --force-recreate workload-microsandbox",
		"how drill 2 recreates the service's container, run from the repository's root")
)

const (
	// a pull, a boot (about a second, longer nested in Lima) and the
	// heartbeats that report it
	startTimeout = 3 * time.Minute

	// a stop, a kill or a restart reaching the task, and a heartbeat saying so
	changeTimeout = 2 * time.Minute

	// a request finding its way through the ingress, or a line into the log
	reachTimeout = time.Minute

	// heartbeats come once a second, so asking more often learns nothing
	pollInterval = time.Second
)

// TestMicrosandbox is §10.3 of the plan: a job, a service through every door it
// has, and the specs a microVM cannot honour.
func TestMicrosandbox(t *testing.T) {
	s := newStack(t)
	if s.controlPlane == nil {
		t.Skip("this suite drives the workload through its control plane: give -controlplane")
	}

	t.Run("a job that exits 3 fails and keeps its log", s.job)
	t.Run("a service is served, logged, reached and commanded", s.serviceLifecycle)
	t.Run("what a microVM cannot be is refused with the reason", s.refusals)
}

// TestMicrosandboxDrills is §10.4 of the plan. Each drill restarts or breaks
// something under runs of its own, so they run one at a time.
func TestMicrosandboxDrills(t *testing.T) {
	if !*drills {
		t.Skip("the drills restart and kill parts of the stack: run them with -drills")
	}

	s := newStack(t)

	t.Run("1_restart_the_service", s.drillRestart(func(t *testing.T) {
		docker(t, "restart", *containerName)
	}))

	t.Run("2_recreate_the_container", s.drillRestart(func(t *testing.T) {
		shell(t, *recreateCommand)
	}))

	t.Run("3_a_vm_dies", s.drillLostVM)
	t.Run("4_the_memory_budget_runs_out", s.drillCapacity)
	t.Run("5_a_guest_runs_out_of_memory", s.drillGuestOutOfMemory)
	t.Run("6_an_orchestrator_restarts", s.drillRestartOrchestrator)
}

// stack is the running stack, as the suite reaches it.
type stack struct {
	// controlPlane is nil when -controlplane is empty.
	controlPlane *controlPlane
	service      *service

	// id tells this run's names from another's, and owner is whose its
	// tasks are.
	id    string
	owner string
}

func newStack(t *testing.T) *stack {
	t.Helper()

	s := &stack{
		service: newService(t),
		id:      strings.ReplaceAll(uuid.Must(uuid.NewV4()).String(), "-", "")[:8],
		owner:   uuid.Must(uuid.NewV4()).String(),
	}

	if len(*controlPlaneURL) > 0 {
		s.controlPlane = &controlPlane{
			url:  strings.TrimSuffix(*controlPlaneURL, "/"),
			http: &http.Client{Timeout: 30 * time.Second},
		}
	}

	s.service.waitReady(t)

	return s
}

// job is what the code runner asks for: it runs once, the heartbeat that says
// it ended carries everything it printed, and then the control plane takes it
// away. So that is where its log is looked for.
func (s *stack) job(t *testing.T) {
	t.Parallel()

	connection, err := nats.Connect(*natsURL, nats.Name("e2e"))
	require.NoError(t, err)
	t.Cleanup(connection.Close)

	name := "e2e-job-" + s.id
	printed := "e2e-job-printed-" + s.id

	// heartbeats are heard as they are published, before anything acts on
	// them, and so before the job is taken away
	beats := make(chan events.Heartbeat, 1024)
	subscription, err := connection.Subscribe(events.HeartbeatName, func(message *nats.Msg) {
		var beat events.Heartbeat
		if json.Unmarshal(message.Data, &beat) != nil || beat.Name != name {
			return
		}

		select {
		case beats <- beat:
		default:
		}
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = subscription.Unsubscribe() })
	require.NoError(t, connection.Flush())

	// asked for as the code runner asks: a message, not a request
	producer, err := produceConsumer.NewProduceConsumer(connection, "e2e", slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	noRetries := 0
	payload, err := json.Marshal(events.TaskRunRequested{
		Name:  name,
		Kind:  string(task.KindJob),
		Image: "busybox:latest",
		// long enough to be seen running, so its run can be followed to its end
		Command:        []string{"sh", "-c", "echo " + printed + "; sleep 3; exit 3"},
		MaxRetries:     &noRetries,
		ResourceLimits: events.ResourceLimits{Cpu: 1, Memory: 128 << 20, Disk: 512 << 20},
		OwnerUUID:      s.owner,
	})
	require.NoError(t, err)
	require.NoError(t, producer.Produce(t.Context(), events.TaskRunRequestedName, payload))

	explain := func() string { return s.controlPlane.explain(s.owner, name) }

	running := receive(t, beats, startTimeout, "the job running", explain, func(beat events.Heartbeat) bool {
		return beat.State == int(task.Running) || task.IsTerminalState(task.State(beat.State))
	})
	require.Equalf(t, int(task.Running), running.State, "the job ended before it was seen running: %s", explain())

	// a microVM, followed to its end and read before the control plane takes
	// the job, and its run with it, away
	run := s.service.runOf(t, running.NodeName, running.UUID)

	ctx, cancel := context.WithTimeout(t.Context(), changeTimeout)
	_, err = s.service.logs(ctx, run.ID, true)
	cancel()
	require.NoError(t, err)

	if ended, found, err := s.service.run(t.Context(), run.ID); err == nil && found {
		require.Equal(t, api.StateExited, ended.State)
		require.Equal(t, 3, ended.ExitCode, "the run's exit code")
	} else {
		t.Logf("the job was taken away before its run could be read (%v); what its last heartbeat says still holds", err)
	}

	ended := receive(t, beats, changeTimeout, "the job ending", explain, func(beat events.Heartbeat) bool {
		return task.IsTerminalState(task.State(beat.State))
	})
	require.Equal(t, int(task.Failed), ended.State, "a job that exits 3 has failed")
	require.Contains(t, string(ended.Logs), printed, "the heartbeat that says it ended carries its log")
}

// serviceLifecycle is a service through every door it has: its port through
// the ingress, its log, a terminal, and every command the dashboard gives it.
func (s *stack) serviceLifecycle(t *testing.T) {
	t.Parallel()

	created := s.controlPlane.runTask(t, taskRequest{
		Name:      "e2e-nginx-" + s.id,
		OwnerUUID: s.owner,
		Service: spec.Service{
			Image:   "nginx:alpine",
			Ports:   spec.Ports{{Task: 80}},
			Restart: "always",
			Deploy:  limits(128 << 20),
		},
	})

	running := s.controlPlane.waitForState(t, created.UUID, "running", startTimeout, serves(80))
	node := running.NodeName

	// a microVM, not a container, and the one the task asked for
	run := s.service.runOf(t, node, created.UUID)
	require.Equal(t, api.StateRunning, run.State)
	require.Equal(t, "always", run.RestartPolicy)
	require.Equal(t, uint64(128<<20), run.Memory)
	require.NotZero(t, hostPort(run, 80), "port 80 is published")

	// served through the ingress under its slug, and logged as it is
	host := running.Slug + "." + *ingressDomain
	visit := "e2e-visit-" + s.id
	get(t, host, "/?"+visit, "Welcome to nginx")
	s.controlPlane.waitForLog(t, created.UUID, visit)

	// a terminal, through the ingress, as its owner
	s.terminal(t, created.UUID, "echo e2e-$((1+1))", "e2e-2")

	// stop: the main process is asked to end, and the VM goes after it
	s.controlPlane.command(t, created.UUID, "stop")
	s.controlPlane.waitForState(t, created.UUID, "stopped", changeTimeout, nil)
	stopped := s.service.waitForRunOf(t, node, created.UUID, changeTimeout, "the run exited", exited)

	// start: a stopped service is asked for again, on the node that holds it
	s.controlPlane.command(t, created.UUID, "restart")
	s.controlPlane.waitForState(t, created.UUID, "running", startTimeout, serves(80))
	started := s.service.waitForRunOf(t, node, created.UUID, startTimeout, "the run started again", startedAfter(stopped.StartedAt))
	get(t, host, "/", "Welcome to nginx")

	// restart: a running one goes down and comes back
	s.controlPlane.command(t, created.UUID, "restart")
	s.service.waitForRunOf(t, node, created.UUID, startTimeout, "the run restarted", startedAfter(started.StartedAt))
	s.controlPlane.waitForState(t, created.UUID, "running", changeTimeout, serves(80))

	// kill: SIGKILL, which docker numbers 137
	s.controlPlane.command(t, created.UUID, "kill")
	s.controlPlane.waitForState(t, created.UUID, "stopped", changeTimeout, nil)
	killed := s.service.waitForRunOf(t, node, created.UUID, changeTimeout, "the run was killed", exited)
	require.Equal(t, 137, killed.ExitCode)

	// delete: the task goes, and its microVM with it
	s.controlPlane.deleteTask(t, created.UUID)
	s.controlPlane.waitForGone(t, created.UUID)
	waitFor(t, changeTimeout, "the run deleted", func() ([]api.Run, bool, error) {
		runs, err := s.service.runs(t.Context(), node, created.UUID)
		return runs, err == nil && len(runs) == 0, err
	})
}

// refusals are the specs §7 of the plan refuses on microsandbox: each task
// fails on its node, and says why.
func (s *stack) refusals(t *testing.T) {
	t.Parallel()

	sleeper := func() spec.Service {
		return spec.Service{
			Image:   "busybox:latest",
			Command: spec.StringOrSlice{"sleep", "3600"},
			Deploy:  limits(64 << 20),
		}
	}

	readOnly := sleeper()
	readOnly.ReadOnly = true

	noNetwork := sleeper()
	noNetwork.NetworkMode = "none"

	for _, c := range []struct {
		name    string
		task    string
		service spec.Service
		reason  string
	}{
		{name: "a read-only root", task: "e2e-read-only-", service: readOnly, reason: "read-only root"},
		{name: "no network interface", task: "e2e-no-network-", service: noNetwork, reason: "no network interface"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			created := s.controlPlane.runTask(t, taskRequest{
				Name:      c.task + s.id,
				OwnerUUID: s.owner,
				Service:   c.service,
			})

			failed := s.controlPlane.waitForState(t, created.UUID, "failed", changeTimeout, hasReason)
			require.Contains(t, failed.Reason, c.reason)
		})
	}

	t.Run("a stack", func(t *testing.T) {
		t.Parallel()

		created := s.controlPlane.runStack(t, runstack.Request{
			Stack: spec.Stack{
				Name:     "e2e-stack-" + s.id,
				Services: map[string]spec.Service{"web": sleeper()},
			},
			OwnerUUID: s.owner,
		})
		require.Len(t, created.Services, 1)

		failed := s.controlPlane.waitForState(t, created.Services[0].UUID, "failed", changeTimeout, hasReason)
		require.Contains(t, failed.Reason, "cannot run a stack")
	})
}

// drillRestart is drills 1 and 2: the service goes away under its runs, by
// restart or by recreation, and comes back. A run whose policy brings it back is
// back on its host port with its journal intact, one whose policy does not
// stays ended and says why, and a task of the control plane's comes back through
// it. A recreated container pulls nothing again: the image cache is on the
// volume.
func (s *stack) drillRestart(restart func(t *testing.T)) func(t *testing.T) {
	return func(t *testing.T) {
		always := s.service.startRun(t, s.runSpec(t, "always", "nginx:alpine", nil, 128<<20, "always", 80))
		once := s.service.startRun(t, s.runSpec(t, "once", "busybox:latest", []string{"sleep", "3600"}, 64<<20, "no"))

		port := hostPort(always, 80)
		require.NotZero(t, port, "the always run publishes port 80")

		const ready = "ready for start up"
		s.service.waitForLog(t, always.ID, ready, 1)

		var managed gettask.Response
		if s.controlPlane != nil {
			var created gettask.Response
			// the control plane's own: retried as a service is, and so asked
			// for again once it has stopped without being asked to
			created = s.controlPlane.runTask(t, taskRequest{
				Name:      "e2e-drill-" + s.id,
				OwnerUUID: s.owner,
				Service: spec.Service{
					Image:   "busybox:latest",
					Command: spec.StringOrSlice{"sleep", "3600"},
					Restart: "no",
					Deploy: spec.Deploy{Resources: spec.Resources{Limits: spec.Limits{
						CPUs: 1, Memory: 64 << 20, Disk: 512 << 20,
					}}},
				},
			})
			managed = s.controlPlane.waitForState(t, created.UUID, "running", startTimeout, nil)
		}

		restartedAt := time.Now()
		restart(t)
		s.service.waitReady(t)

		back := s.service.waitForRun(t, always.ID, startTimeout, "the always run back", startedAfter(restartedAt))
		require.Equal(t, port, hostPort(back, 80), "back on its host port")
		s.service.waitForLog(t, always.ID, ready, 2)

		ended := s.service.waitForRun(t, once.ID, changeTimeout, "the run that is not brought back", exited)
		require.Equal(t, "service_restarted", ended.Error)

		if s.controlPlane != nil {
			s.controlPlane.waitForState(t, managed.UUID, "running", startTimeout, nil)
			s.service.waitForRunOf(t, managed.NodeName, managed.UUID, startTimeout, "the control plane's run back", startedAfter(restartedAt))
		}
	}
}

// drillLostVM is drill 3: a microVM dies under its run. The run ends as a lost
// VM, and its policy decides what then.
func (s *stack) drillLostVM(t *testing.T) {
	always := s.service.startRun(t, s.runSpec(t, "lost-always", "busybox:latest", []string{"sleep", "3600"}, 64<<20, "always"))
	once := s.service.startRun(t, s.runSpec(t, "lost-once", "busybox:latest", []string{"sleep", "3600"}, 64<<20, "no"))

	for _, run := range []api.Run{always, once} {
		docker(t, "exec", *containerName, "pkill", "-9", "-f", "msb.*wk-"+run.ID)
	}

	lost := s.service.waitForRun(t, once.ID, changeTimeout, "the run with no policy lost", exited)
	require.Equal(t, 137, lost.ExitCode)
	require.Equal(t, "vm_lost", lost.Error)

	s.service.waitForRun(t, always.ID, startTimeout, "the always run brought back by its policy", func(run api.Run) bool {
		return run.State == api.StateRunning && run.RestartCount > 0
	})
}

// drillCapacity is drill 4: runs of 1 GiB until the memory budget is spent.
// The next is refused as capacity, and the ones already running carry on.
func (s *stack) drillCapacity(t *testing.T) {
	var started []api.Run

	for i := range 64 {
		created := s.service.createRun(t, s.runSpec(t, fmt.Sprintf("budget-%d", i), "busybox:latest", []string{"sleep", "3600"}, 1<<30, "no"))

		ctx, cancel := context.WithTimeout(t.Context(), startTimeout)
		run, failure, err := s.service.start(ctx, created.ID)
		cancel()
		require.NoError(t, err)

		if failure != nil {
			require.Equal(t, api.CodeCapacity, failure.Code, failure.Message)
			require.NotEmpty(t, started, "not even one run of 1 GiB fits the budget")
			t.Logf("%d runs of 1 GiB fit, and the next was refused: %s", len(started), failure.Message)

			for _, run := range started {
				s.service.waitForRun(t, run.ID, changeTimeout, "a run admitted before the budget ran out", running)
			}

			return
		}

		started = append(started, run)
	}

	t.Fatalf("64 runs of 1 GiB and the budget never ran out")
}

// drillGuestOutOfMemory is drill 5: a guest touches more memory than it has.
// Its kernel kills the process, the run says so, and nothing around it notices.
//
// It reads 96 MiB into one buffer in a guest of 128 MiB, which keeps about 24
// MiB for its own kernel. Asking for twice its memory at once would not do: the
// guest refuses an allocation bigger than all of its RAM before anything is
// touched, and the process merely exits 1.
func (s *stack) drillGuestOutOfMemory(t *testing.T) {
	neighbour := s.service.startRun(t, s.runSpec(t, "neighbour", "busybox:latest", []string{"sleep", "3600"}, 64<<20, "no"))
	hog := s.service.startRun(t, s.runSpec(t, "hog", "busybox:latest",
		[]string{"dd", "if=/dev/zero", "of=/dev/null", "bs=96M", "count=1"}, 128<<20, "no"))

	killed := s.service.waitForRun(t, hog.ID, changeTimeout, "the hog killed", exited)
	require.Equal(t, 137, killed.ExitCode)
	require.Equal(t, "killed", killed.Error)

	s.service.waitForRun(t, neighbour.ID, reachTimeout, "its neighbour untouched", running)
	s.service.waitReady(t)
}

// drillRestartOrchestrator is drill 6: an orchestrator is restarted. Its runs
// belong to the service, not to it, so none of them notices.
func (s *stack) drillRestartOrchestrator(t *testing.T) {
	if s.controlPlane == nil {
		t.Skip("needs the control plane, which gives an orchestrator a task: give -controlplane")
	}

	created := s.controlPlane.runTask(t, taskRequest{
		Name:      "e2e-steady-" + s.id,
		OwnerUUID: s.owner,
		Service: spec.Service{
			Image:   "busybox:latest",
			Command: spec.StringOrSlice{"sleep", "3600"},
			Restart: "always",
			Deploy:  limits(64 << 20),
		},
	})
	running := s.controlPlane.waitForState(t, created.UUID, "running", startTimeout, nil)
	before := s.service.runOf(t, running.NodeName, created.UUID)

	// the orchestrator's container, by the compose service it is, which is
	// named after it
	container := docker(t, "ps", "--quiet", "--filter", "label=com.docker.compose.service="+running.NodeName)
	require.NotEmpty(t, container, "no container for %s", running.NodeName)
	docker(t, "restart", container)

	waitFor(t, startTimeout, running.NodeName+" healthy again", func() (string, bool, error) {
		health, err := dockerOutput(t.Context(), "inspect", "--format", "{{.State.Health.Status}}", container)
		return health, err == nil && health == "healthy", err
	})

	// a few heartbeats from the new orchestrator, and the run is the one it was
	time.Sleep(5 * pollInterval)
	s.controlPlane.waitForState(t, created.UUID, "running", reachTimeout, nil)

	after, found, err := s.service.run(t.Context(), before.ID)
	require.NoError(t, err)
	require.True(t, found, "the run is still there")
	require.Equal(t, api.StateRunning, after.State)
	require.True(t, before.StartedAt.Equal(after.StartedAt), "and was never restarted")
	require.Equal(t, before.RestartCount, after.RestartCount)
}

// taskRequest is POST /api/tasks/run's body: a task in a compose service's
// shape, and whose it is.
type taskRequest struct {
	Name      string       `json:"name"`
	OwnerUUID string       `json:"owner_uuid"`
	Service   spec.Service `json:"service"`
}

// limits is what every task here asks for: one CPU, the memory given, room
// on disk, and no second attempt, so a failure says so at once.
func limits(memory int64) spec.Deploy {
	noRetries := 0

	return spec.Deploy{
		Resources: spec.Resources{Limits: spec.Limits{
			CPUs:   1,
			Memory: spec.ByteSize(memory),
			Disk:   512 << 20,
		}},
		RestartPolicy: spec.RestartPolicy{MaxAttempts: &noRetries},
	}
}

// serves says a running task's port has come up where the ingress can reach it.
func serves(port uint) func(gettask.Response) bool {
	return func(t gettask.Response) bool {
		for _, endpoint := range t.Endpoints {
			if endpoint.TaskPort == port {
				return true
			}
		}

		return false
	}
}

func hasReason(t gettask.Response) bool {
	return len(t.Reason) > 0
}

// controlPlane is the control plane's internal API, which the dashboard and the
// blog reach through the blog, and which asks for no token of its own.
type controlPlane struct {
	url  string
	http *http.Client
}

func (c *controlPlane) request(ctx context.Context, method, path string, body any) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}

		reader = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, c.url+path, reader)
	if err != nil {
		return 0, nil, err
	}

	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.http.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()

	payload, err := io.ReadAll(response.Body)

	return response.StatusCode, payload, err
}

// runTask runs a task, and deletes it once the test is over.
func (c *controlPlane) runTask(t *testing.T, request taskRequest) gettask.Response {
	t.Helper()

	status, payload, err := c.request(t.Context(), http.MethodPost, "/api/tasks/run", request)
	require.NoError(t, err)
	require.Equalf(t, http.StatusCreated, status, "running %s: %s", request.Name, payload)

	var created gettask.Response
	require.NoError(t, json.Unmarshal(payload, &created))

	t.Cleanup(func() { c.forget("/api/tasks/" + url.PathEscape(created.UUID) + "?force=true") })

	return created
}

// runStack runs a stack, and deletes it once the test is over.
func (c *controlPlane) runStack(t *testing.T, request runstack.Request) getstack.Response {
	t.Helper()

	status, payload, err := c.request(t.Context(), http.MethodPost, "/api/stacks/run", request)
	require.NoError(t, err)
	require.Equalf(t, http.StatusCreated, status, "running %s: %s", request.Name, payload)

	var created getstack.Response
	require.NoError(t, json.Unmarshal(payload, &created))

	t.Cleanup(func() { c.forget("/api/stacks/" + url.PathEscape(created.UUID)) })

	return created
}

// forget deletes what a test made, if it is still there. The test's own
// context is over by then.
func (c *controlPlane) forget(path string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, _, _ = c.request(ctx, http.MethodDelete, path, nil)
}

func (c *controlPlane) task(ctx context.Context, uuid string) (gettask.Response, bool, error) {
	var current gettask.Response

	status, payload, err := c.request(ctx, http.MethodGet, "/api/tasks/"+url.PathEscape(uuid), nil)
	switch {
	case err != nil:
		return current, false, err
	case status == http.StatusNotFound:
		return current, false, nil
	case status != http.StatusOK:
		return current, false, fmt.Errorf("%d: %s", status, payload)
	}

	return current, true, json.Unmarshal(payload, &current)
}

// command is one of the commands the dashboard gives: stop, kill or restart.
func (c *controlPlane) command(t *testing.T, uuid, command string) {
	t.Helper()

	status, payload, err := c.request(t.Context(), http.MethodPost, "/api/tasks/"+url.PathEscape(uuid)+"/"+command, nil)
	require.NoError(t, err)
	require.Equalf(t, http.StatusAccepted, status, "%s: %s", command, payload)
}

func (c *controlPlane) deleteTask(t *testing.T, uuid string) {
	t.Helper()

	status, payload, err := c.request(t.Context(), http.MethodDelete, "/api/tasks/"+url.PathEscape(uuid)+"?force=true", nil)
	require.NoError(t, err)
	require.Equalf(t, http.StatusNoContent, status, "delete: %s", payload)
}

// waitForState waits for a task to be in a state, and for anything else asked
// of it besides. A task worth no second attempt that fails while something else
// is awaited will not come back, so that ends the wait at once.
func (c *controlPlane) waitForState(t *testing.T, uuid, state string, timeout time.Duration, also func(gettask.Response) bool) gettask.Response {
	t.Helper()

	return waitFor(t, timeout, "task "+uuid+" "+state, func() (gettask.Response, bool, error) {
		current, found, err := c.task(t.Context(), uuid)
		switch {
		case err != nil:
			return current, false, err
		case !found:
			return current, false, fmt.Errorf("no such task")
		case current.CurrentState == "failed" && state != "failed" && current.MaxRetries == 0:
			t.Fatalf("task %s failed while it was to be %s: %s", uuid, state, current.Reason)
		}

		return current, current.CurrentState == state && (also == nil || also(current)), nil
	})
}

func (c *controlPlane) waitForGone(t *testing.T, uuid string) {
	t.Helper()

	waitFor(t, changeTimeout, "task "+uuid+" gone", func() (gettask.Response, bool, error) {
		current, found, err := c.task(t.Context(), uuid)
		return current, err == nil && !found, err
	})
}

// waitForLog waits for a line containing text in a task's log, which the node
// ships to the control plane line by line.
func (c *controlPlane) waitForLog(t *testing.T, uuid, text string) {
	t.Helper()

	waitFor(t, reachTimeout, "a line with "+text+" in the log of task "+uuid, func() (int, bool, error) {
		status, payload, err := c.request(t.Context(), http.MethodGet, "/api/tasks/"+url.PathEscape(uuid)+"/logs", nil)
		if err != nil {
			return 0, false, err
		}

		if status != http.StatusOK {
			return 0, false, fmt.Errorf("%d: %s", status, payload)
		}

		var logs gettasklogs.Response
		if err := json.Unmarshal(payload, &logs); err != nil {
			return 0, false, err
		}

		for _, line := range logs.Items {
			if strings.Contains(line.Content, text) {
				return len(logs.Items), true, nil
			}
		}

		return len(logs.Items), false, nil
	})
}

// explain says what the control plane makes of an owner's task, found by name:
// the most there is to go on when a task never got as far as a heartbeat.
func (c *controlPlane) explain(owner, name string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, payload, err := c.request(ctx, http.MethodGet, "/api/tasks?owner="+url.QueryEscape(owner), nil)
	if err != nil {
		return "the control plane could not be asked: " + err.Error()
	}

	var listing struct {
		Items []gettask.Response `json:"items"`
	}
	if err := json.Unmarshal(payload, &listing); err != nil {
		return "the control plane answered " + string(payload)
	}

	for _, item := range listing.Items {
		if item.Name == name {
			return fmt.Sprintf("the control plane has it %s on %q (%s)", item.CurrentState, item.NodeName, item.Reason)
		}
	}

	return "the control plane has no such task"
}

// terminal opens a shell in a task through the ingress, as the dashboard does,
// types a line into it, and waits for what the line should print.
func (s *stack) terminal(t *testing.T, uuid, line, want string) {
	t.Helper()

	endpoint := "ws" + strings.TrimPrefix(strings.TrimSuffix(*ingressURL, "/"), "http") + "/tasks/" + url.PathEscape(uuid) + "/attach"
	header := http.Header{"Authorization": {"Bearer " + s.token(t)}}

	connection := waitFor(t, reachTimeout, "a terminal in task "+uuid, func() (*websocket.Conn, bool, error) {
		connection, response, err := websocket.DefaultDialer.DialContext(t.Context(), endpoint, header)
		if err != nil && response != nil {
			err = fmt.Errorf("%w (%s)", err, response.Status)
		}

		return connection, err == nil, err
	})
	defer connection.Close()

	done := make(chan struct{})
	defer close(done)

	output := make(chan []byte)
	go func() {
		defer close(output)

		for {
			kind, data, err := connection.ReadMessage()
			if err != nil {
				return
			}

			if kind != websocket.BinaryMessage {
				continue
			}

			select {
			case output <- data:
			case <-done:
				return
			}
		}
	}()

	require.NoError(t, connection.WriteMessage(websocket.BinaryMessage, []byte(line+"\n")))

	// what is typed comes back as it was typed, so only the shell's answer
	// can hold what is wanted
	var printed strings.Builder
	deadline := time.After(reachTimeout)

	for !strings.Contains(printed.String(), want) {
		select {
		case data, open := <-output:
			if !open {
				t.Fatalf("the terminal closed before it printed %q; it printed %q", want, printed.String())
			}

			printed.Write(data)
		case <-deadline:
			t.Fatalf("the terminal did not print %q; it printed %q", want, printed.String())
		}
	}

	_ = connection.WriteMessage(websocket.BinaryMessage, []byte("exit\n"))
}

// token is an access token for the suite's owner, signed with the development
// key the blog signs with, which is the one the orchestrators verify against.
func (s *stack) token(t *testing.T) string {
	t.Helper()

	key := pemFromEnv("PRIVATE_KEY")
	if len(key) == 0 {
		t.Fatal("a terminal is opened with its owner's token: set PRIVATE_KEY to the development key, as make e2e-microsandbox does from .env")
	}

	privateKey, err := ecdsa.ParsePrivateKey([]byte(key))
	require.NoError(t, err)

	now := time.Now()

	claims := jwt.NewClaimsBuilder()
	claims.SetSubject(s.owner)
	claims.SetAudience([]string{auth.AccessToken})
	claims.SetIssuedAt(now)
	claims.SetExpirationTime(now.Add(15 * time.Minute))

	token, err := jwt.NewJWT(privateKey, nil).Generate(t.Context(), claims.Build())
	require.NoError(t, err)

	return token
}

// get asks the ingress for a page of a task's, by its hostname as a browser
// would, until the page says what it should.
func get(t *testing.T, host, path, want string) {
	t.Helper()

	client := &http.Client{Timeout: 10 * time.Second}

	waitFor(t, reachTimeout, "http://"+host+path, func() (string, bool, error) {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, strings.TrimSuffix(*ingressURL, "/")+path, nil)
		if err != nil {
			return "", false, err
		}

		request.Host = host

		response, err := client.Do(request)
		if err != nil {
			return "", false, err
		}
		defer response.Body.Close()

		body, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		answer := fmt.Sprintf("%s: %.200s", response.Status, body)

		return answer, err == nil && response.StatusCode == http.StatusOK && strings.Contains(string(body), want), err
	})
}

// service is the workload-microsandbox service's API, which the suite is let
// into as an orchestrator is: with an orchestrator's certificate, under the
// tunnel's authority.
type service struct {
	url  string
	http *http.Client
}

func newService(t *testing.T) *service {
	t.Helper()

	authority := pemFromEnv("WORKLOAD_TUNNEL_CA_CERT")
	ownCertificate := pemFromEnv("WORKLOAD_TUNNEL_CERT")
	privateKey := pemFromEnv("WORKLOAD_TUNNEL_KEY")

	if len(authority) == 0 || len(ownCertificate) == 0 || len(privateKey) == 0 {
		t.Fatal("the service's API is reached as an orchestrator: set WORKLOAD_TUNNEL_CA_CERT, WORKLOAD_TUNNEL_CERT and WORKLOAD_TUNNEL_KEY, as make e2e-microsandbox does from .env")
	}

	address, err := url.Parse(*serviceURL)
	require.NoError(t, err)

	config, err := certificate.ClientTLSConfig(certificate.Credentials{
		Authority:   authority,
		Certificate: ownCertificate,
		PrivateKey:  privateKey,
		ServerName:  address.Hostname(),
	})
	require.NoError(t, err)

	// no timeout of the client's own: a log followed to its end takes as long
	// as the run does, so every call carries a context instead
	return &service{
		url:  strings.TrimSuffix(*serviceURL, "/"),
		http: &http.Client{Transport: &http.Transport{TLSClientConfig: config}},
	}
}

// route is one of the contract's routes, as a method and a path with its
// wildcards filled in, in order.
func route(pattern string, wildcards ...string) (string, string) {
	method, path, _ := strings.Cut(pattern, " ")

	for _, value := range wildcards {
		start, end := strings.Index(path, "{"), strings.Index(path, "}")
		path = path[:start] + url.PathEscape(value) + path[end+1:]
	}

	return method, path
}

// call makes a request of the contract's, and decodes the answer into out, or
// into the error the service answered with.
func (s *service) call(ctx context.Context, pattern string, wildcards []string, query url.Values, body, out any) (*api.Error, error) {
	method, path := route(pattern, wildcards...)

	endpoint := s.url + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}

		reader = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, err
	}

	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := s.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	if response.StatusCode >= http.StatusBadRequest {
		var failure api.ErrorResponse
		if json.Unmarshal(payload, &failure) != nil || len(failure.Error.Code) == 0 {
			failure.Error = api.Error{Code: api.CodeInternal, Message: fmt.Sprintf("%s: %s", response.Status, payload)}
		}

		return &failure.Error, nil
	}

	if out == nil || len(payload) == 0 {
		return nil, nil
	}

	return nil, json.Unmarshal(payload, out)
}

// waitReady waits for the service to have checked the runtime and adopted its
// runs, which is when it starts taking commands.
func (s *service) waitReady(t *testing.T) {
	t.Helper()

	waitFor(t, startTimeout, "the service ready", func() (api.Info, bool, error) {
		var info api.Info
		failure, err := s.call(t.Context(), api.RouteInfo, nil, nil, nil, &info)
		if failure != nil {
			return info, false, failure
		}

		return info, err == nil && info.Ready, err
	})
}

func (s *service) run(ctx context.Context, id string) (api.Run, bool, error) {
	var run api.Run

	failure, err := s.call(ctx, api.RouteGetRun, []string{id}, nil, nil, &run)
	switch {
	case err != nil:
		return run, false, err
	case failure != nil && failure.Code == api.CodeNotFound:
		return run, false, nil
	case failure != nil:
		return run, false, failure
	}

	return run, true, nil
}

// runs are a task's runs on a node: its attempts, each one a microVM.
func (s *service) runs(ctx context.Context, node, task string) ([]api.Run, error) {
	var list api.RunList

	failure, err := s.call(ctx, api.RouteListRuns, nil, url.Values{api.QueryNode: {node}, api.QueryTask: {task}}, nil, &list)
	if failure != nil {
		return nil, failure
	}

	return list.Runs, err
}

// runOf is a task's latest run on a node, once there is one.
func (s *service) runOf(t *testing.T, node, task string) api.Run {
	t.Helper()

	return s.waitForRunOf(t, node, task, reachTimeout, "a run of task "+task+" on "+node, func(api.Run) bool { return true })
}

// waitForRunOf waits for a task's latest run on a node to be what is wanted.
// It asks for the task's runs rather than one run, because a node may make a
// new run of a task where it could have started the one it had.
func (s *service) waitForRunOf(t *testing.T, node, task string, timeout time.Duration, what string, want func(api.Run) bool) api.Run {
	t.Helper()

	return waitFor(t, timeout, what, func() (api.Run, bool, error) {
		runs, err := s.runs(t.Context(), node, task)
		if err != nil || len(runs) == 0 {
			return api.Run{}, false, err
		}

		latest := runs[0]
		for _, run := range runs[1:] {
			if run.CreatedAt.After(latest.CreatedAt) {
				latest = run
			}
		}

		return latest, want(latest), nil
	})
}

func (s *service) waitForRun(t *testing.T, id string, timeout time.Duration, what string, want func(api.Run) bool) api.Run {
	t.Helper()

	return waitFor(t, timeout, what, func() (api.Run, bool, error) {
		run, found, err := s.run(t.Context(), id)
		if err == nil && !found {
			err = fmt.Errorf("no run %s", id)
		}

		return run, err == nil && want(run), err
	})
}

// logs reads a run's journal from its start; following, it stays open until the
// run has ended and every line it wrote has been sent.
func (s *service) logs(ctx context.Context, id string, follow bool) ([]api.LogLine, error) {
	method, path := route(api.RouteRunLogs, id)

	endpoint := s.url + path
	if follow {
		endpoint += "?" + url.Values{api.QueryFollow: {"true"}}.Encode()
	}

	request, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, err
	}

	response, err := s.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)

		return nil, fmt.Errorf("%s: %s", response.Status, payload)
	}

	var lines []api.LogLine

	decoder := json.NewDecoder(response.Body)
	for {
		var line api.LogLine
		if err := decoder.Decode(&line); err == io.EOF {
			return lines, nil
		} else if err != nil {
			return lines, err
		}

		lines = append(lines, line)
	}
}

// waitForLog waits for a run's journal to hold at least count lines with text.
func (s *service) waitForLog(t *testing.T, id, text string, count int) {
	t.Helper()

	waitFor(t, reachTimeout, fmt.Sprintf("%d lines with %q in run %s's journal", count, text, id), func() (int, bool, error) {
		lines, err := s.logs(t.Context(), id, false)

		found := 0
		for _, line := range lines {
			if strings.Contains(line.Content, text) {
				found++
			}
		}

		return found, err == nil && found >= count, err
	})
}

// createRun creates a run of the drills' own, and deletes it once the test is
// over.
func (s *service) createRun(t *testing.T, spec api.RunSpec) api.Run {
	t.Helper()

	var created api.Run
	failure, err := s.call(t.Context(), api.RouteCreateRun, nil, nil, spec, &created)
	require.NoError(t, err)
	require.Nilf(t, failure, "creating %s: %v", spec.Name, failure)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		_, _ = s.call(ctx, api.RouteDeleteRun, []string{created.ID}, nil, nil, nil)
	})

	return created
}

func (s *service) start(ctx context.Context, id string) (api.Run, *api.Error, error) {
	var started api.Run
	failure, err := s.call(ctx, api.RouteStartRun, []string{id}, nil, nil, &started)

	return started, failure, err
}

// startRun creates a run and starts it, which pulls, boots and starts its main
// process before it answers.
func (s *service) startRun(t *testing.T, spec api.RunSpec) api.Run {
	t.Helper()

	created := s.createRun(t, spec)

	ctx, cancel := context.WithTimeout(t.Context(), startTimeout)
	defer cancel()

	started, failure, err := s.start(ctx, created.ID)
	require.NoError(t, err)
	require.Nilf(t, failure, "starting %s: %v", spec.Name, failure)

	return started
}

// nodeName is the node the drills' runs are made on: one no orchestrator is,
// so nothing but the drill ever touches them.
func (s *stack) nodeName() string {
	return "e2e-drills-" + s.id
}

// runSpec is a run of the drills' own, isolated, since nothing in it needs the
// internet. Its name is unique on the node, as a run's has to be, however often
// a drill asks for one by the same name.
func (s *stack) runSpec(t *testing.T, name, image string, command []string, memory uint64, policy string, ports ...uint16) api.RunSpec {
	t.Helper()

	name += "-" + strings.ReplaceAll(uuid.Must(uuid.NewV4()).String(), "-", "")[:8]

	return api.RunSpec{
		Node:          s.nodeName(),
		Name:          name,
		Image:         image,
		Command:       command,
		CPU:           1,
		Memory:        memory,
		Disk:          512 << 20,
		Network:       api.NetworkIsolated,
		Ports:         ports,
		RestartPolicy: policy,
		Task: api.Task{
			UUID: uuid.Must(uuid.NewV4()).String(),
			Name: name,
			Slug: name,
			Kind: string(task.KindService),
		},
	}
}

func hostPort(run api.Run, port uint16) uint16 {
	for _, endpoint := range run.Endpoints {
		if endpoint.Port == port {
			return endpoint.HostPort
		}
	}

	return 0
}

func running(run api.Run) bool {
	return run.State == api.StateRunning
}

func exited(run api.Run) bool {
	return run.State == api.StateExited
}

func startedAfter(moment time.Time) func(api.Run) bool {
	return func(run api.Run) bool {
		return run.State == api.StateRunning && run.StartedAt.After(moment)
	}
}

// waitFor asks until observe says it is done, and fails the test with what it
// saw last once time is up.
func waitFor[T any](t *testing.T, timeout time.Duration, what string, observe func() (T, bool, error)) T {
	t.Helper()

	deadline := time.Now().Add(timeout)

	for {
		seen, done, err := observe()
		if done && err == nil {
			return seen
		}

		if time.Now().After(deadline) {
			t.Fatalf("%s: not within %s; last seen %+v, error %v", what, timeout, seen, err)
		}

		select {
		case <-t.Context().Done():
			t.Fatalf("%s: %v", what, t.Context().Err())
		case <-time.After(pollInterval):
		}
	}
}

// receive waits for a heartbeat that is what is wanted, and fails the test with
// what explain has to say once time is up.
func receive(t *testing.T, beats <-chan events.Heartbeat, timeout time.Duration, what string, explain func() string, want func(events.Heartbeat) bool) events.Heartbeat {
	t.Helper()

	deadline := time.After(timeout)

	for {
		select {
		case beat := <-beats:
			if want(beat) {
				return beat
			}
		case <-deadline:
			t.Fatalf("%s: no heartbeat said so within %s; %s", what, timeout, explain())
		}
	}
}

// docker runs the docker command, against whichever daemon DOCKER_HOST names,
// and fails the test if it fails.
func docker(t *testing.T, arguments ...string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), startTimeout)
	defer cancel()

	output, err := dockerOutput(ctx, arguments...)
	require.NoErrorf(t, err, "docker %s: %s", strings.Join(arguments, " "), output)

	return output
}

func dockerOutput(ctx context.Context, arguments ...string) (string, error) {
	output, err := exec.CommandContext(ctx, "docker", arguments...).CombinedOutput()

	return strings.TrimSpace(string(output)), err
}

// shell runs a command line from the repository's root, where compose finds
// its files.
func shell(t *testing.T, line string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), startTimeout)
	defer cancel()

	command := exec.CommandContext(ctx, "sh", "-c", line)
	command.Dir = filepath.Join("..", "..")

	output, err := command.CombinedOutput()
	require.NoErrorf(t, err, "%s: %s", line, output)
}

// pemFromEnv reads a PEM from the environment as .env writes one, with its
// line breaks as \n: compose turns those back into line breaks, and a shell
// that sources .env does not.
func pemFromEnv(name string) string {
	return strings.ReplaceAll(os.Getenv(name), `\n`, "\n")
}

func envOr(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}

	return fallback
}
