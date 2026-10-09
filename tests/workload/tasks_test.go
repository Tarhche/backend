package workload_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/code/answerCodeRun"
	"github.com/khanzadimahdi/testproject/application/code/runCode"
	"github.com/khanzadimahdi/testproject/application/code/stop"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/getVMs"
	ingressTasks "github.com/khanzadimahdi/testproject/application/workload/ingress/kinds/task"
	getresourceendpoint "github.com/khanzadimahdi/testproject/application/workload/orchestrator/getResourceEndpoint"
	"github.com/khanzadimahdi/testproject/domain"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/jetstream/produceConsumer"
)

// codeRunner is the blog's code runner in front of the workload: what asks for
// a snippet's task, and what answers its reader from what the workload's
// nodes say, heard as the blog hears it. What a reader is told is kept rather
// than carried down a websocket.
type codeRunner struct {
	run     domain.MessageHandler
	stop    domain.MessageHandler
	replies *messagingMock.RecordingReplyer
}

func (w *workload) codeRunner(t *testing.T) *codeRunner {
	t.Helper()

	logger := slog.New(slog.DiscardHandler)
	replies := &messagingMock.RecordingReplyer{}

	blogMessages, err := produceConsumer.NewProduceConsumer(connect(t, w.natsURL), "blog", logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	require.NoError(t, blogMessages.Consume(ctx, kind.HeartbeatName(taskKind.Name), answerCodeRun.NewHeartbeatHandler(replies, ingressDomain, logger)))
	require.NoError(t, blogMessages.Consume(ctx, kind.ResourceActedOnName, answerCodeRun.NewResourceActedOnHandler(replies, logger)))

	t.Cleanup(func() {
		cancel()
		blogMessages.Wait()
	})

	return &codeRunner{
		run:     runCode.NewRunCodeHandler(w.validator, w.client, replies, logger),
		stop:    stop.NewUseCase(w.client, w.validator, replies, logger),
		replies: replies,
	}
}

// ask asks the code runner for a snippet, as a reader's page does.
func (c *codeRunner) ask(t *testing.T, request runCode.Request) {
	t.Helper()

	payload, err := json.Marshal(request)
	require.NoError(t, err)

	require.NoError(t, c.run.Handle(t.Context(), payload))
}

// answered is the first reply to requestID that says what condition wants,
// and its kind.
func (c *codeRunner) answered(t *testing.T, requestID string, what string, condition func(answerCodeRun.Response, domain.ReplyKind) bool) (answerCodeRun.Response, domain.ReplyKind) {
	t.Helper()

	type answer struct {
		response answerCodeRun.Response
		kind     domain.ReplyKind
	}

	found := eventually(t, what, func(context.Context) (answer, error) {
		for _, reply := range c.replies.Replies() {
			if reply.RequestID != requestID {
				continue
			}

			var response answerCodeRun.Response
			if err := json.Unmarshal(reply.Payload, &response); err != nil {
				return answer{}, err
			}

			if condition(response, reply.Kind) {
				return answer{response: response, kind: reply.Kind}, nil
			}
		}

		return answer{}, errors.New("not answered yet")
	}, func(answer) bool { return true })

	return found.response, found.kind
}

// task is the task named after a request, as the control plane keeps it,
// once condition holds of it.
func (w *workload) task(t *testing.T, name string, what string, condition func(taskKind.Task) bool) taskKind.Task {
	t.Helper()

	return eventually(t, "the task "+what, func(ctx context.Context) (taskKind.Task, error) {
		records, _, err := w.resources.GetAll(ctx, taskKind.Name, resource.Filter{}, 0, 0)
		if err != nil {
			return taskKind.Task{}, err
		}

		for _, record := range records {
			if record.Metadata.Name == name {
				return kind.Decode[taskKind.Spec, taskKind.Status](record.Raw)
			}
		}

		return taskKind.Task{}, domain.ErrNotExists
	}, condition)
}

// taskGone waits for the control plane to have no record of a task any more,
// and its node nothing of it.
func (w *workload) taskGone(t *testing.T, uuid string) {
	t.Helper()

	require.Eventually(t, func() bool {
		_, err := w.resources.GetOne(t.Context(), taskKind.Name, uuid)

		return errors.Is(err, domain.ErrNotExists)
	}, settle, beat, "the task's record was never removed")

	instances, err := w.engine.List(t.Context())
	require.NoError(t, err)

	for _, instance := range instances {
		assert.NotEqual(t, uuid, instance.Labels[vm.LabelTask], "its run is gone from its node")
	}
}

// snippets are what the programs of the snippets these tests run do, by
// their code: a snippet's code is its command's last word.
type snippets struct {
	lock    sync.Mutex
	release map[string]chan struct{}
	ended   map[string]time.Time
}

func newSnippets() *snippets {
	return &snippets{release: make(map[string]chan struct{}), ended: make(map[string]time.Time)}
}

// waiting is a snippet that does not go on until it is released.
func (s *snippets) waiting(code string) chan struct{} {
	s.lock.Lock()
	defer s.lock.Unlock()

	release := make(chan struct{})
	s.release[code] = release

	return release
}

// endedAt is when the program of a snippet that runs until it is stopped was
// stopped.
func (s *snippets) endedAt(code string) (time.Time, bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	at, ended := s.ended[code]

	return at, ended
}

func (s *snippets) run(ctx context.Context, spec vm.Spec, log func(line string)) int {
	code := spec.Command[len(spec.Command)-1]

	s.lock.Lock()
	release := s.release[code]
	s.lock.Unlock()

	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return 0
		}
	}

	switch code {
	case `console.log("hello")`:
		log("hello from e2e")

		return 0

	case "package main":
		log("built, and run")

		return 0

	case "while(true){}":
		// the runner's own timeout ends it, and says so.
		log("still going")
		log("⏰ Execution timed out after 30 seconds")

		return 124
	}

	// it serves, or ignores its timeout, until it is stopped.
	log("listening")
	<-ctx.Done()

	s.lock.Lock()
	s.ended[code] = time.Now()
	s.lock.Unlock()

	return 0
}

// TestASnippet runs code-runner snippets end to end: asked for as a reader's
// page asks, admitted by the control plane as a task, run in a VM of its own
// on its node, answered from what the node says, and taken away once it has
// ended.
func TestASnippet(t *testing.T) {
	t.Parallel()

	w := start(t)
	ctx := t.Context()

	programs := newSnippets()
	w.programs.are(programs.run)

	runner := w.codeRunner(t)

	t.Run("a job runs and completes, and its reader is told what it printed and how it ended", func(t *testing.T) {
		release := programs.waiting(`console.log("hello")`)

		runner.ask(t, runCode.Request{ID: "request-hello", Code: `console.log("hello")`, Runner: "nodejs-22.14"})

		running := w.task(t, "request-hello", "running", func(tk taskKind.Task) bool { return tk.Status.State == taskKind.Running })
		assert.Equal(t, task.GuestOwnerUUID, running.Metadata.OwnerUUID)
		assert.Equal(t, nodeName, running.Metadata.Node)
		assert.Equal(t, []string{"--timeout", "30", `console.log("hello")`}, running.Spec.Command)
		assert.Empty(t, runner.replies.Replies(), "a snippet nobody watches is answered once it has ended")

		instance, err := w.engine.Inspect(ctx, running.Status.Run.ID)
		require.NoError(t, err)
		assert.Equal(t, task.GuestOwnerUUID, instance.Labels[vm.LabelOwner])

		spec, err := w.engine.Spec(running.Status.Run.ID)
		require.NoError(t, err)
		assert.Equal(t, vm.Resources{CPUs: 2, Memory: runCode.DefaultMaxMemorySize, Disk: runCode.DefaultMaxDiskSize}, spec.Resources)

		close(release)

		response, replyKind := runner.answered(t, "request-hello", "answered", func(answerCodeRun.Response, domain.ReplyKind) bool { return true })
		assert.Equal(t, domain.ReplyFinal, replyKind)
		assert.Equal(t, answerCodeRun.Response{TaskUUID: running.Metadata.UUID, Name: "request-hello", Logs: []byte("hello from e2e\n"), State: "completed"}, response)

		w.taskGone(t, running.Metadata.UUID)
	})

	t.Run("a job whose program exits with a code of its own failed, and says what it printed", func(t *testing.T) {
		runner.ask(t, runCode.Request{ID: "request-timeout", Code: "while(true){}", Runner: "php-8.4"})

		response, _ := runner.answered(t, "request-timeout", "answered", func(answerCodeRun.Response, domain.ReplyKind) bool { return true })
		assert.Equal(t, "failed", response.State)
		assert.Equal(t, "still going\n⏰ Execution timed out after 30 seconds\n", string(response.Logs))
		assert.Empty(t, response.Error)

		w.taskGone(t, response.TaskUUID)
	})

	t.Run("a Go snippet is given what building it takes", func(t *testing.T) {
		release := programs.waiting("package main")

		runner.ask(t, runCode.Request{ID: "request-go", Code: "package main", Runner: "go-1.24"})

		running := w.task(t, "request-go", "running", func(tk taskKind.Task) bool { return tk.Status.State == taskKind.Running })

		spec, err := w.engine.Spec(running.Status.Run.ID)
		require.NoError(t, err)
		assert.Equal(t, "ghcr.io/tarhche/code-runner:go-1.24-latest", spec.Image)
		assert.Equal(t, vm.Resources{CPUs: 2, Memory: runCode.GoMaxMemorySize, Disk: runCode.GoMaxDiskSize}, spec.Resources)

		close(release)

		w.taskGone(t, running.Metadata.UUID)
	})
}

// TestAJobPastItsTTL holds a job that ignores its own timeout to being
// stopped once it has run for its ttl, counted from when its run started,
// and its reader to being told it ended.
func TestAJobPastItsTTL(t *testing.T) {
	t.Parallel()

	w := start(t)
	ctx := t.Context()

	programs := newSnippets()
	w.programs.are(programs.run)

	runner := w.codeRunner(t)

	none := 0

	asked, err := w.client.RunTask(ctx, task.GuestOwnerUUID, workloadControlPlane.TaskRequest{
		Name: "request-forever",
		Spec: taskKind.Spec{
			Kind:       task.KindJob,
			Image:      "ghcr.io/tarhche/code-runner:nodejs-22.14-latest",
			Command:    []string{"--timeout", "30", "trap('SIGTERM')"},
			TTL:        time.Second,
			Limits:     taskKind.Limits{CPU: 2, Memory: runCode.DefaultMaxMemorySize, Disk: runCode.DefaultMaxDiskSize},
			MaxRetries: &none,
		},
	})
	require.NoError(t, err)

	running := w.task(t, "request-forever", "running", func(tk taskKind.Task) bool {
		return tk.Status.State == taskKind.Running && tk.Status.Run != nil && !tk.Status.Run.StartedAt.IsZero()
	})
	assert.Equal(t, running.Status.Run.StartedAt.Add(time.Second), running.Status.Run.Deadline, "its ttl after it started")

	response, _ := runner.answered(t, "request-forever", "answered", func(answerCodeRun.Response, domain.ReplyKind) bool { return true })
	assert.Equal(t, "completed", response.State, "a job cut short completed")
	assert.Equal(t, "listening\n", string(response.Logs))

	stoppedAt, stopped := programs.endedAt("trap('SIGTERM')")
	require.True(t, stopped, "its program was stopped")
	assert.GreaterOrEqual(t, stoppedAt.Sub(running.Status.Run.StartedAt), time.Second, "no sooner than its ttl after it started")

	w.taskGone(t, asked.Metadata.UUID)
}

// TestALiveSnippet runs a snippet somebody watches, which serves a port: it
// is followed as it runs, its port is found where the ingress looks for it,
// its terminal opens for anybody, it is among anybody's VMs and nobody's own,
// and it is taken away when its reader stops it.
func TestALiveSnippet(t *testing.T) {
	t.Parallel()

	w := start(t)
	ctx := t.Context()

	programs := newSnippets()
	w.programs.are(programs.run)

	runner := w.codeRunner(t)

	code := `require("http").createServer((q, s) => s.end("served")).listen(3000)`

	runner.ask(t, runCode.Request{ID: "request-live", Code: code, Runner: "nodejs-22.14", Ports: []port.Port{3000}, Terminal: true})

	followed, replyKind := runner.answered(t, "request-live", "followed while it runs", func(response answerCodeRun.Response, _ domain.ReplyKind) bool {
		return response.State == "running" && len(response.Endpoints) > 0
	})
	assert.Equal(t, domain.ReplyChunk, replyKind, "a snippet somebody watches is told as it goes")

	running := w.task(t, "request-live", "running", func(tk taskKind.Task) bool { return tk.Status.State == taskKind.Running })
	slug := running.Metadata.Slug

	t.Run("its reader is told where it is reached, and when it will be stopped", func(t *testing.T) {
		assert.Equal(t, []answerCodeRun.Endpoint{{TaskPort: 3000, URL: "http://" + slug + "-3000." + ingressDomain}}, followed.Endpoints)
		require.NotNil(t, followed.Deadline)
		assert.Equal(t, running.Status.Run.StartedAt.Add(runCode.LiveTTL), *followed.Deadline)
		assert.Equal(t, running.Metadata.UUID, followed.TaskUUID)
	})

	t.Run("its port is found on its node, by its slug, as the ingress finds it", func(t *testing.T) {
		location, err := ingressTasks.New(w.resources).BySlug(ctx, slug)
		require.NoError(t, err)
		assert.Equal(t, nodeName, location.Node)
		assert.Equal(t, []port.Port{3000}, location.Ports)

		endpoint, err := getresourceendpoint.NewUseCase(w.kinds).Execute(ctx, &getresourceendpoint.Request{Kind: taskKind.Name, Slug: slug, Port: 3000})
		require.NoError(t, err)
		assert.Equal(t, port.Port(3000), endpoint.Port)

		instance, err := w.engine.Inspect(ctx, running.Status.Run.ID)
		require.NoError(t, err)
		require.Len(t, instance.Endpoints, 1)
		assert.Equal(t, instance.Endpoints[0].Address, endpoint.Address, "where its engine published it")
	})

	t.Run("its terminal opens for anybody, signed in or not", func(t *testing.T) {
		binding, runs := w.kinds.Lookup(taskKind.Name)
		require.True(t, runs)

		session, err := binding.Attach(ctx, taskKind.ActionAttach, running.Metadata.UUID, "")
		require.NoError(t, err)
		defer session.Close()

		said, err := io.ReadAll(session.Stdout())
		require.NoError(t, err)
		assert.Contains(t, string(said), "docker: not found", "the shell ran in its vm, which has no docker")
	})

	t.Run("it is among anybody's vms, as the guest's, managed by the code runner, and in nobody's own", func(t *testing.T) {
		listed := getVMs.NewUseCase(w.client, w.validator, w.owners, ingressDomain)

		everybody, err := listed.Execute(ctx, &getVMs.Request{})
		require.NoError(t, err)

		index := slices.IndexFunc(everybody.Items, func(v presenter.VM) bool { return v.UUID == running.Metadata.UUID })
		require.GreaterOrEqual(t, index, 0, "the run is listed")

		shown := everybody.Items[index]
		assert.Equal(t, vm.ManagedByCodeRunner, shown.ManagedBy)
		assert.Equal(t, task.GuestOwnerUUID, shown.OwnerUUID)
		require.NotNil(t, shown.Owner)
		assert.Equal(t, task.GuestOwnerUUID, shown.Owner.UUID)
		assert.Equal(t, "running", shown.State)
		assert.Equal(t, string(vm.KindMachine), shown.Kind)

		own, err := listed.Execute(ctx, &getVMs.Request{OwnerUUID: ownerUUID})
		require.NoError(t, err)
		assert.False(t, slices.ContainsFunc(own.Items, func(v presenter.VM) bool { return v.UUID == running.Metadata.UUID }))
	})

	t.Run("stopped by its reader, it is taken away", func(t *testing.T) {
		payload, err := json.Marshal(stop.Request{ID: "request-stop", TaskUUID: running.Metadata.UUID})
		require.NoError(t, err)

		require.NoError(t, runner.stop.Handle(ctx, payload))

		replies := runner.replies.Replies()
		stopped := slices.IndexFunc(replies, func(reply domain.Reply) bool { return reply.RequestID == "request-stop" })
		require.GreaterOrEqual(t, stopped, 0)
		assert.JSONEq(t, `{}`, string(replies[stopped].Payload))

		w.taskGone(t, running.Metadata.UUID)

		_, err = getresourceendpoint.NewUseCase(w.kinds).Execute(ctx, &getresourceendpoint.Request{Kind: taskKind.Name, Slug: slug, Port: 3000})
		assert.ErrorIs(t, err, domain.ErrNotExists, "its port is served no more")
	})
}
