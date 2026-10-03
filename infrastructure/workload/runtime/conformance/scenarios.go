package conformance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

const (
	// service is a service that runs until it is stopped, and stops cleanly
	// when it is asked to once it has said serviceReady.
	service = `trap 'exit 0' TERM; echo ` + serviceReady + `; while true; do sleep 1; done`

	// serviceReady is what service writes once it would stop cleanly.
	serviceReady = "conformance-ready"

	// marker is what a peer serves, so whoever reaches it knows it did.
	marker = "conformance-peer"

	// serve is a peer serving marker on port 80.
	serve = `mkdir -p /www && echo ` + marker + ` > /www/index.html && exec httpd -f -p 80 -h /www`
)

// fetch asks a URL until it answers, for a peer that may still be starting.
func fetch(url string) string {
	return fmt.Sprintf(`for i in $(seq 1 30); do wget -q -T 2 -O- %s && exit 0; sleep 0.5; done; exit 1`, url)
}

// reach asks a URL a few times, giving up on each try quickly. What it reaches
// it reaches within those; what it must not reach it fails to every time.
func reach(url string) string {
	return fmt.Sprintf(`for i in 1 2 3; do wget -q -T 5 -O /dev/null %s && exit 0; sleep 1; done; exit 1`, url)
}

// offer: the class is offered healthy, with exactly what it declares.
func (s *suite) offer(t *testing.T) {
	offer := s.driver.Offer(s.context(t))

	require.True(t, offer.Healthy, "the class is not offered healthy: %s", offer.Reason)
	assert.Equal(t, s.driver.Class(), offer.Class)
	assert.Equal(t, s.driver.Kind().String(), offer.Driver)
	assert.NotEmpty(t, offer.Capabilities.Architectures, "a class runs images for some architecture")

	// what a backend runs images for is where it runs rather than what the
	// class is, so a class that declares none takes what the backend says.
	want := s.declared
	if len(want.Architectures) == 0 {
		want.Architectures = offer.Capabilities.Architectures
	}

	assert.Equal(t, want, offer.Capabilities, "the class offers other capabilities than it declares")
}

// lifecycle: create, start, stop, start again, kill, restart, delete, with
// the states the workload reads them as.
func (s *suite) lifecycle(t *testing.T) {
	ctx := s.context(t)
	tasks := s.driver.Tasks()

	e := s.execution("life", task.KindService, service, func(e *task.Execution) {
		e.OwnerUUID = uuid.Must(uuid.NewV4()).String()
		e.StackUUID = uuid.Must(uuid.NewV4()).String()
		e.Attempt = 2
		e.Interactive = true
		e.TTL = 90 * time.Second
	})

	id := create(t, ctx, s.driver, e)

	created, err := tasks.Inspect(ctx, id)
	require.NoError(t, err)

	assert.Equal(t, id, created.ID)
	assert.Equal(t, task.StatusCreated, created.Status)
	assert.Equal(t, task.Scheduled, task.EvaluateState(created.Status, created.Kind, created.ExitCode))
	describes(t, e, created)

	// limits come back in the units they went in.
	assert.Equal(t, e.ResourceLimits.Cpu, created.ResourceLimits.Cpu)
	assert.Equal(t, e.ResourceLimits.Memory, created.ResourceLimits.Memory)

	require.NoError(t, tasks.Start(ctx, id))
	up := waitFor(t, ctx, s.driver, id, "running", running)
	assert.Equal(t, task.Running, task.EvaluateState(up.Status, up.Kind, up.ExitCode))
	assert.False(t, up.StartedAt.IsZero(), "a running task says when it started")

	// a stop that reaches the program before it has set up how it stops ends
	// it as TERM does, whatever the class: it is asked once it says it is ready.
	says(t, ctx, s.driver, id, serviceReady)

	require.NoError(t, tasks.Stop(ctx, id))
	stopped := waitFor(t, ctx, s.driver, id, "stopped", ended)
	assert.Equal(t, 0, stopped.ExitCode, "a service that stops when it is asked to has not failed")
	assert.Equal(t, task.Stopped, task.EvaluateState(stopped.Status, stopped.Kind, stopped.ExitCode))

	require.NoError(t, tasks.Start(ctx, id))
	waitFor(t, ctx, s.driver, id, "running again", running)

	require.NoError(t, tasks.Kill(ctx, id))
	killed := waitFor(t, ctx, s.driver, id, "killed", ended)
	assert.Equal(t, 137, killed.ExitCode, "a killed task ends as SIGKILL does: 128 + 9")

	require.NoError(t, tasks.Restart(ctx, id))
	waitFor(t, ctx, s.driver, id, "running after a restart", running)

	require.NoError(t, tasks.Delete(ctx, id))

	runs, err := tasks.Of(ctx, e.TaskUUID)
	require.NoError(t, err)
	assert.Empty(t, runs, "a deleted run is not held any more")

	// a command for a run that is gone has already happened, which the use
	// cases take as done rather than as a failure worth trying again.
	assert.ErrorIs(t, tasks.Stop(ctx, id), domain.ErrNotExists)
	assert.ErrorIs(t, tasks.Kill(ctx, id), domain.ErrNotExists)
	assert.ErrorIs(t, tasks.Delete(ctx, id), domain.ErrNotExists)
}

// describes checks that a run says what it is running, as it was told when it
// was made.
func describes(t *testing.T, want *task.Execution, got task.Execution) {
	t.Helper()

	assert.Equal(t, want.TaskUUID, got.TaskUUID, "task uuid")
	assert.Equal(t, want.TaskName, got.TaskName, "task name")
	assert.Equal(t, want.Slug, got.Slug, "slug")
	assert.Equal(t, want.Kind, got.Kind, "kind")
	assert.Equal(t, want.NodeName, got.NodeName, "node")
	assert.Equal(t, want.OwnerUUID, got.OwnerUUID, "owner")
	assert.Equal(t, want.StackUUID, got.StackUUID, "stack")
	assert.Equal(t, want.Attempt, got.Attempt, "attempt")
	assert.Equal(t, want.Interactive, got.Interactive, "interactive")
	assert.Equal(t, want.TTL, got.TTL, "ttl")
}

// listing: a node finds its runs by node, task and slug, saying what they are
// running, and never another node's on the same backend.
func (s *suite) listing(t *testing.T) {
	ctx := s.context(t)

	e := s.execution("list", task.KindService, service, func(e *task.Execution) {
		e.OwnerUUID = uuid.Must(uuid.NewV4()).String()
		e.Attempt = 1
		e.Interactive = true
		e.TTL = 30 * time.Second
	})
	mine := create(t, ctx, s.driver, e)

	// the same task on another node sharing the backend, as an attempt at it
	// another orchestrator made would be.
	elsewhere := *e
	elsewhere.Name = e.Name + "-elsewhere"
	elsewhere.NodeName = s.otherNode
	theirs := create(t, ctx, s.other, &elsewhere)

	held, err := s.driver.Tasks().OnNode(ctx, s.node)
	require.NoError(t, err)

	index := slices.IndexFunc(held, func(run task.Execution) bool { return run.ID == mine })
	require.GreaterOrEqual(t, index, 0, "a node does not hold the run it made")
	describes(t, e, held[index])
	assert.False(t, slices.ContainsFunc(held, func(run task.Execution) bool { return run.ID == theirs }),
		"a node holds another node's run")

	runs, err := s.driver.Tasks().Of(ctx, e.TaskUUID)
	require.NoError(t, err)
	assert.Equal(t, []string{mine}, ids(runs), "a task's runs on this node")

	runs, err = s.driver.Tasks().BySlug(ctx, e.Slug)
	require.NoError(t, err)
	assert.Equal(t, []string{mine}, ids(runs), "a slug's runs on this node")

	runs, err = s.other.Tasks().Of(ctx, e.TaskUUID)
	require.NoError(t, err)
	assert.Equal(t, []string{theirs}, ids(runs), "a task's runs on the other node")

	held, err = s.other.Tasks().OnNode(ctx, s.otherNode)
	require.NoError(t, err)
	assert.False(t, slices.ContainsFunc(held, func(run task.Execution) bool { return run.ID == mine }),
		"the other node holds this node's run")
}

func ids(runs []task.Execution) []string {
	result := make([]string, len(runs))
	for i := range runs {
		result[i] = runs[i].ID
	}

	return result
}

// exitCodes: what a job returns is what it is read as.
func (s *suite) exitCodes(t *testing.T) {
	for _, tt := range []struct {
		name    string
		command string
		code    int
		state   task.State
	}{
		{name: "a job that succeeds completes", command: "exit 0", code: 0, state: task.Completed},
		{name: "a job that fails has failed", command: "exit 1", code: 1, state: task.Failed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			finished, _ := job(t, s.context(t), s.driver, s.execution("exit", task.KindJob, tt.command))

			assert.Equal(t, tt.code, finished.ExitCode)
			assert.Equal(t, tt.state, task.EvaluateState(finished.Status, finished.Kind, finished.ExitCode))
		})
	}

	t.Run("a job ended from outside was cut short, not failed", func(t *testing.T) {
		t.Parallel()

		ctx := s.context(t)

		id := started(t, ctx, s.driver, s.execution("killed", task.KindJob, "sleep 300"))
		waitFor(t, ctx, s.driver, id, "running", running)

		require.NoError(t, s.driver.Tasks().Kill(ctx, id))
		killed := waitFor(t, ctx, s.driver, id, "killed", ended)

		assert.Equal(t, 137, killed.ExitCode)
		assert.Equal(t, task.Completed, task.EvaluateState(killed.Status, killed.Kind, killed.ExitCode))
	})
}

// errEnough is a follower that has read as much as it wanted to.
var errEnough = errors.New("read enough")

// logs: a run's output is read whole, and followed from where a follower left
// off with nothing lost and nothing told twice that cannot be told apart.
func (s *suite) logs(t *testing.T) {
	ctx := s.context(t)
	tasks := s.driver.Tasks()

	const lines = 40

	id := started(t, ctx, s.driver, s.execution("logs", task.KindJob,
		fmt.Sprintf(`for i in $(seq 1 %d); do echo "line $i"; sleep 0.05; done`, lines)))

	want := make([]string, lines)
	for i := range want {
		want[i] = fmt.Sprintf("line %d", i+1)
	}

	// a follower that stops part of the way through, as one does when its
	// node is redeployed.
	first := make([]task.LogLine, 0, lines)
	err := tasks.StreamLogs(ctx, id, time.Time{}, func(line task.LogLine) error {
		first = append(first, line)

		if len(first) == lines/3 {
			return errEnough
		}

		return nil
	})
	if err != nil {
		require.ErrorIs(t, err, errEnough)
	}

	require.Len(t, first, lines/3)

	// and one that picks up from the last line it has, as a log shipper does.
	// The stream ends once the job has, and all of its output has been read.
	second := make([]task.LogLine, 0, lines)
	require.NoError(t, tasks.StreamLogs(ctx, id, first[len(first)-1].At, func(line task.LogLine) error {
		second = append(second, line)

		return nil
	}))

	// a line read twice is the same line, at the same moment, which is what
	// the control plane stores once.
	merged := make([]task.LogLine, 0, lines)
	for _, line := range append(first, second...) {
		if !slices.ContainsFunc(merged, func(kept task.LogLine) bool { return same(kept, line) }) {
			merged = append(merged, line)
		}
	}

	read := make([]string, len(merged))
	for i, line := range merged {
		read[i] = line.Content
		assert.Equal(t, task.StreamStdout, line.Stream)
		assert.False(t, line.At.IsZero(), "a line says when it was written")
	}

	assert.Equal(t, want, read, "lines were lost or told apart from themselves across a reconnect")

	waitFor(t, ctx, s.driver, id, "ended", ended)
	assert.Equal(t, want, strings.Split(strings.TrimSpace(output(t, ctx, s.driver, id)), "\n"))
}

// same reports whether two lines are one line, read twice.
func same(a task.LogLine, b task.LogLine) bool {
	return a.Stream == b.Stream && a.Content == b.Content && a.At.Equal(b.At)
}

// exec: a terminal in a running task echoes, takes its size, and ending it
// ends what it started.
func (s *suite) exec(t *testing.T) {
	if !s.declared.TTY {
		t.Skip("the class opens no terminal")
	}

	ctx := s.context(t)
	tasks := s.driver.Tasks()

	id := started(t, ctx, s.driver, s.execution("exec", task.KindService, service))
	waitFor(t, ctx, s.driver, id, "running", running)

	session, err := tasks.Exec(ctx, id, task.ExecOptions{Command: []string{"sh"}, TTY: true})
	require.NoError(t, err)
	defer session.Close()

	screen := watch(session)

	eventually(t, ctx, 10*time.Second, "the terminal takes a size", func() (bool, string) {
		err := session.Resize(ctx, 40, 100)

		return err == nil, fmt.Sprint(err)
	})

	// done-42 is not what was typed, so seeing it is seeing the shell answer.
	_, err = io.WriteString(session, "stty size; echo done-$((40+2))\n")
	require.NoError(t, err)

	eventually(t, ctx, 30*time.Second, "the terminal answers", func() (bool, string) {
		return strings.Contains(screen.String(), "done-42"), screen.String()
	})
	assert.Contains(t, screen.String(), "40 100", "the terminal is not the size it was told it is")

	// something the session starts, which has to end with it although nobody
	// is attached to it any more. A process of its own: busybox's shell runs
	// a sleep inside itself, where ps would never see it.
	_, err = io.WriteString(session, "sh -c 'sleep 4321'\n")
	require.NoError(t, err)

	eventually(t, ctx, 30*time.Second, "the session's command runs", func() (bool, string) {
		listed, err := processes(ctx, tasks, id)

		return err == nil && strings.Contains(listed, "sleep 4321"), listed
	})

	require.NoError(t, session.Close())

	ending, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	require.NoError(t, session.End(ending))

	eventually(t, ctx, 30*time.Second, "what the session started ends with it", func() (bool, string) {
		listed, err := processes(ctx, tasks, id)

		return err == nil && !strings.Contains(listed, "sleep 4321"), listed
	})
}

// processes is what is running in a run, as ps says.
func processes(ctx context.Context, tasks task.Runtime, id string) (string, error) {
	session, err := tasks.Exec(ctx, id, task.ExecOptions{Command: []string{"ps"}})
	if err != nil {
		return "", err
	}
	defer session.Close()

	listed := watch(session)

	select {
	case <-listed.done:
	case <-time.After(10 * time.Second):
		return listed.String(), errors.New("ps did not finish")
	}

	return listed.String(), nil
}

// screen is what a session has written so far.
type screen struct {
	mu     sync.Mutex
	buffer bytes.Buffer
	done   chan struct{}
}

// watch reads a session until it ends.
func watch(r io.Reader) *screen {
	s := &screen{done: make(chan struct{})}

	go func() {
		defer close(s.done)

		chunk := make([]byte, 4096)
		for {
			n, err := r.Read(chunk)

			s.mu.Lock()
			s.buffer.Write(chunk[:n])
			s.mu.Unlock()

			if err != nil {
				return
			}
		}
	}()

	return s
}

func (s *screen) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.buffer.String()
}

// eventually asks until ready says yes, in the test's own goroutine, and fails
// with the last thing it was told when it never does.
func eventually(t *testing.T, ctx context.Context, within time.Duration, what string, ready func() (bool, string)) {
	t.Helper()

	deadline := time.Now().Add(within)

	for {
		ok, last := ready()
		if ok {
			return
		}

		if time.Now().After(deadline) || ctx.Err() != nil {
			t.Fatalf("%s: never did; last seen: %q", what, last)
		}

		time.Sleep(250 * time.Millisecond)
	}
}

// limits: what a task is limited to holds.
func (s *suite) limits(t *testing.T) {
	t.Run("memory past the limit is not given", func(t *testing.T) {
		t.Parallel()

		// one process taking 96 MiB at once, limited to 16; a class that gives
		// every task more than that still gives it less than 96, as a
		// microVM's least does. It is one process on purpose:
		// several that run one program, as a pipeline of busybox applets is,
		// can keep the kernel reading that program's pages back in, and so
		// reclaiming rather than ending anything, for minutes, whatever the
		// class. That is the kernel's, and not what this asks.
		finished, out := job(t, s.context(t), s.driver, s.execution("memory", task.KindJob,
			`dd if=/dev/zero of=/dev/null bs=100663296 count=1 && echo survived`, func(e *task.Execution) {
				e.ResourceLimits.Memory = 16 << 20
			}))

		assert.NotContains(t, out, "survived", "a task was given memory past its limit")
		assert.Equal(t, 137, finished.ExitCode, "a task past its memory limit is killed")
	})

	t.Run("a cpu limit is held as it was asked for", func(t *testing.T) {
		t.Parallel()

		ctx := s.context(t)

		id := create(t, ctx, s.driver, s.execution("cpu", task.KindJob, "true", func(e *task.Execution) {
			e.ResourceLimits.Cpu = 0.25
		}))

		inspected, err := s.driver.Tasks().Inspect(ctx, id)
		require.NoError(t, err)

		assert.Equal(t, 0.25, inspected.ResourceLimits.Cpu)
	})

	t.Run("disk past the limit is not given", func(t *testing.T) {
		if !s.declared.DiskLimit {
			t.Skip("the class does not hold a task to its disk limit")
		}

		t.Parallel()

		finished, out := job(t, s.context(t), s.driver, s.execution("disk", task.KindJob,
			`dd if=/dev/zero of=/fill bs=1048576 count=96 && echo filled`, func(e *task.Execution) {
				e.ResourceLimits.Disk = 64 << 20
			}))

		assert.NotContains(t, out, "filled", "a task was given disk past its limit")
		assert.NotEqual(t, 0, finished.ExitCode)
	})
}

// readOnly: a task with a read-only root cannot write to it, and one without
// can. Whether /tmp and /run are still there to write to on a read-only root
// differs between classes (ScratchOnReadOnlyRoot), and is held to what the
// class says.
func (s *suite) readOnly(t *testing.T) {
	if !s.declared.ReadOnlyRoot {
		t.Skip("the class cannot keep a task's root from being written to")
	}

	ctx := s.context(t)

	finished, _ := job(t, ctx, s.driver, s.execution("readonly", task.KindJob, "touch /probe", func(e *task.Execution) {
		e.ReadOnly = true
	}))

	assert.NotEqual(t, 0, finished.ExitCode, "a task wrote to a root it was not to write to")
	assert.True(t, finished.ReadOnly)

	scratch, out := job(t, ctx, s.driver, s.execution("scratch", task.KindJob, "touch /tmp/probe /run/probe", func(e *task.Execution) {
		e.ReadOnly = true
	}))

	if s.options.scratchOnReadOnlyRoot {
		assert.Equal(t, 0, scratch.ExitCode, "a task on a read-only root of a class that keeps /tmp and /run writable could not write there: %s", out)
	} else {
		assert.NotEqual(t, 0, scratch.ExitCode, "a task on a read-only root wrote to /tmp or /run, which its class does not say it may")
	}

	writable, _ := job(t, ctx, s.driver, s.execution("writable", task.KindJob, "touch /probe"))
	assert.Equal(t, 0, writable.ExitCode, "a task could not write to its own root")
}

// networks: each policy reaches what it is meant to, and nothing else.
func (s *suite) networks(t *testing.T) {
	t.Run("none has no network at all", func(t *testing.T) {
		if !s.declared.SupportsNetworkPolicy(network.PolicyNone) {
			t.Skip("the class does not take a task off every network")
		}

		t.Parallel()

		// the interfaces that are up, and every route there is. A kernel puts
		// tunnel devices it was built with in every network namespace, down
		// and carrying nothing, so what counts is what is up and routes.
		finished, out := job(t, s.context(t), s.driver, s.execution("none", task.KindJob,
			`ifconfig | grep -oE '^[^ ]+'; echo ---; tail -n +2 /proc/net/route`,
			on(network.PolicyNone, "", "")))

		assert.Equal(t, 0, finished.ExitCode, out)
		assert.Equal(t, []string{"lo", "---"}, strings.Fields(out), "a task with no network has an interface up, or a route")
	})

	t.Run("isolated reaches its peers and not the internet", func(t *testing.T) {
		if !s.declared.SupportsNetworkPolicy(network.PolicyIsolated) {
			t.Skip("the class has no isolated network")
		}

		t.Parallel()

		ctx := s.context(t)

		peer := s.execution("peer", task.KindService, serve, on(network.PolicyIsolated, "", ""))
		waitFor(t, ctx, s.driver, started(t, ctx, s.driver, peer), "running", running)

		finished, out := job(t, ctx, s.driver, s.execution("client", task.KindJob, fetch("http://"+peer.Name+"/"),
			on(network.PolicyIsolated, "", "")))
		assert.Equal(t, 0, finished.ExitCode, "an isolated task cannot reach its peer: %s", out)
		assert.Contains(t, out, marker)

		finished, _ = job(t, ctx, s.driver, s.execution("isolated", task.KindJob, reach(s.internet()),
			on(network.PolicyIsolated, "", "")))
		assert.NotEqual(t, 0, finished.ExitCode, "an isolated task reached the internet")
	})

	t.Run("public reaches the internet, and nothing it must not", func(t *testing.T) {
		if !s.declared.SupportsNetworkPolicy(network.PolicyPublic) {
			t.Skip("the class has no public network")
		}

		t.Parallel()

		ctx := s.context(t)

		if len(s.options.internet) > 0 {
			finished, out := job(t, ctx, s.driver, s.execution("public", task.KindJob, reach(s.options.internet),
				on(network.PolicyPublic, "", "")))
			assert.Equal(t, 0, finished.ExitCode, "a public task cannot reach the internet: %s", out)
		}

		for _, address := range s.options.forbidden {
			finished, _ := job(t, ctx, s.driver, s.execution("forbidden", task.KindJob, reach("http://"+address+"/"),
				on(network.PolicyPublic, "", "")))
			assert.NotEqual(t, 0, finished.ExitCode, "a public task reached %s", address)
		}
	})

	t.Run("a stack's services reach each other by name", func(t *testing.T) {
		if !s.declared.StackNetworks {
			t.Skip("the class gives a stack no network of its own")
		}

		t.Parallel()

		ctx := s.context(t)

		stack := "cf-stack-" + random()[:5]
		stackUUID := uuid.Must(uuid.NewV4()).String()

		require.NoError(t, s.driver.Networks().EnsureStackNetwork(ctx, stack))

		// registered first, so it runs last: after the services are gone.
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			if err := s.driver.Networks().RemoveStackNetwork(ctx, stack); err != nil && !errors.Is(err, domain.ErrNotExists) {
				t.Logf("could not take away the %s stack's network: %v", stack, err)
			}
		})

		inStack := func(e *task.Execution) { e.StackUUID = stackUUID }

		api := s.execution("api", task.KindService, serve, on(network.PolicyIsolated, stack, "api"), inStack)
		waitFor(t, ctx, s.driver, started(t, ctx, s.driver, api), "running", running)

		finished, out := job(t, ctx, s.driver, s.execution("web", task.KindJob, fetch("http://api/"),
			on(network.PolicyIsolated, stack, "web"), inStack))
		assert.Equal(t, 0, finished.ExitCode, "a service cannot reach its stack's api by name: %s", out)
		assert.Contains(t, out, marker)
	})
}

// internet is somewhere on the internet to fail to reach, which an isolated
// task must fail to reach even where nothing could.
func (s *suite) internet() string {
	if len(s.options.internet) > 0 {
		return s.options.internet
	}

	return defaultInternet
}

// ports: an exposed port is up while its task runs, and reached through the
// driver when the class dials its runs.
func (s *suite) ports(t *testing.T) {
	policy := network.PolicyIsolated
	if !s.declared.SupportsNetworkPolicy(policy) {
		policy = network.PolicyPublic
	}

	if !s.declared.SupportsNetworkPolicy(policy) {
		t.Skip("the class has no network a port can be exposed on")
	}

	ctx := s.context(t)

	web := s.execution("ports", task.KindService, serve, on(policy, "", ""), func(e *task.Execution) {
		e.ExposedPorts = port.PortSet{80: {}}
		e.PortBindings = port.PortMap{80: {{HostIP: "0.0.0.0"}}}
	})

	id := started(t, ctx, s.driver, web)
	waitFor(t, ctx, s.driver, id, "running with port 80 up", func(e task.Execution) bool {
		return running(e) && slices.Contains(e.Endpoints, 80)
	})

	held, err := s.driver.Tasks().BySlug(ctx, web.Slug)
	require.NoError(t, err)
	require.Len(t, held, 1)
	assert.Contains(t, held[0].Endpoints, port.Port(80), "a listing says which ports are up too")

	dialer, ok := s.driver.Tasks().(task.Dialer)
	if !ok {
		t.Log("the class publishes its runs' ports rather than dialling them; where they are published is its node's to reach")

		return
	}

	eventually(t, ctx, 30*time.Second, "port 80 answers through the driver", func() (bool, string) {
		answer, err := get(ctx, dialer, id, 80)
		if err != nil {
			return false, err.Error()
		}

		return strings.Contains(answer, marker), answer
	})
}

// get asks for / on a run's port through a dialer, over plain HTTP.
func get(ctx context.Context, dialer task.Dialer, id string, p port.Port) (string, error) {
	conn, err := dialer.DialContext(ctx, id, p)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return "", err
	}

	if _, err := io.WriteString(conn, "GET / HTTP/1.0\r\nHost: conformance\r\n\r\n"); err != nil {
		return "", err
	}

	answer, err := io.ReadAll(conn)

	return string(answer), err
}

// restartPolicies: a run is started again by the policies the class declares,
// and left alone without one.
func (s *suite) restartPolicies(t *testing.T) {
	for _, tt := range []struct {
		policy   string
		command  string
		restarts bool
	}{
		{policy: "no", command: "exit 1", restarts: false},
		{policy: "on-failure", command: "exit 1", restarts: true},
		{policy: "always", command: "exit 0", restarts: true},
		{policy: "unless-stopped", command: "exit 0", restarts: true},
	} {
		t.Run(tt.policy, func(t *testing.T) {
			if !s.declared.SupportsRestartPolicy(tt.policy) {
				t.Skip("the class does not apply this policy")
			}

			t.Parallel()

			ctx := s.context(t)

			id := started(t, ctx, s.driver, s.execution("restart", task.KindJob, tt.command, func(e *task.Execution) {
				e.RestartPolicy = tt.policy
			}))

			if tt.restarts {
				waitFor(t, ctx, s.driver, id, "restarted", func(e task.Execution) bool { return e.RestartCount >= 1 })

				return
			}

			waitFor(t, ctx, s.driver, id, "ended", ended)

			// a moment for a restart that should not come.
			time.Sleep(2 * time.Second)

			after, err := s.driver.Tasks().Inspect(ctx, id)
			require.NoError(t, err)

			assert.True(t, after.Status.Ended(), "a run with no restart policy came back")
			assert.Equal(t, uint(0), after.RestartCount)
		})
	}
}
