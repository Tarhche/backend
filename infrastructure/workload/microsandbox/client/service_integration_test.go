//go:build microsandbox_integration

package client_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/danceable/console"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/task/getEndpoint"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/client"
)

// The client against a real workload-microsandbox, running real microVMs: what
// the fake service in this package only promises, done. It needs /dev/kvm
// behind the service, so it runs in a Lima VM in development and in the KVM
// job in CI, and only when asked for:
//
//	go test -tags microsandbox_integration -run TestService -v \
//		./infrastructure/workload/microsandbox/client/
//
// It is configured as an orchestrator is, by the orchestrator's own
// configuration reading the environment: WORKLOAD_MICROSANDBOX_URL, and the
// tunnel's WORKLOAD_TUNNEL_CA_CERT, WORKLOAD_TUNNEL_CERT and
// WORKLOAD_TUNNEL_KEY. Each test works for nodes of its own, named for it, and
// takes away whatever they hold when it is done, so it neither sees nor
// disturbs a real orchestrator's runs.

const (
	busybox = "busybox:1.37"
	nginx   = "nginx:alpine"

	// settle bounds waiting for something a run does on its own time: a job
	// ending, a policy restarting it, a server inside it coming up.
	settle = 2 * time.Minute
)

// realConfigs is the configuration of an orchestrator on microsandbox, read
// from the environment as the orchestrator reads it.
func realConfigs(t *testing.T) *configs.WorkloadOrchestrator {
	t.Helper()

	c := configs.NewWorkloadOrchestrator()

	flagSet := console.NewFlagSet("serve-workload-orchestrator", io.Discard)
	require.NoError(t, flagSet.Struct(c))
	require.NoError(t, flagSet.Parse(nil))

	if len(c.TunnelAuthority) == 0 || len(c.TunnelCertificate) == 0 || len(c.TunnelKey) == 0 {
		t.Fatal("the suite reaches workload-microsandbox as an orchestrator does: set WORKLOAD_MICROSANDBOX_URL, WORKLOAD_TUNNEL_CA_CERT, WORKLOAD_TUNNEL_CERT and WORKLOAD_TUNNEL_KEY")
	}

	c.Runtime = configs.RuntimeMicrosandbox

	return c
}

// realNode is a node of the test's own, and its runtime. Whatever the node
// holds when the test is done is deleted.
func realNode(t *testing.T, c *configs.WorkloadOrchestrator, suffix string) (string, *client.Runtime) {
	t.Helper()

	name := fmt.Sprintf("it-%d-%s", time.Now().UnixNano(), suffix)

	microsandbox, err := client.New(client.Config{
		URL:         c.MicrosandboxURL,
		Node:        name,
		Authority:   c.TunnelAuthority,
		Certificate: c.TunnelCertificate,
		PrivateKey:  c.TunnelKey,
	}, discard())
	require.NoError(t, err)

	runtime := client.NewRuntime(microsandbox)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		held, err := runtime.OnNode(ctx, name)
		if err != nil {
			t.Logf("the runs of %s could not be listed to be taken away: %v", name, err)
		}

		for _, run := range held {
			if err := runtime.Delete(ctx, run.ID); err != nil && !errors.Is(err, domain.ErrNotExists) {
				t.Logf("run %s of %s could not be taken away: %v", run.ID, name, err)
			}
		}

		microsandbox.Close()
	})

	require.NoError(t, runtime.EnsureImage(t.Context(), busybox))

	return name, runtime
}

// job is a busybox job running a shell script, as the code runner asks for one.
func job(node string, name string, script string) *task.Execution {
	return &task.Execution{
		Name:           name,
		TaskUUID:       fmt.Sprintf("%s-%s", node, name),
		TaskName:       name,
		Slug:           name,
		Kind:           task.KindJob,
		NodeName:       node,
		Image:          busybox,
		ResourceLimits: task.ResourceLimits{Cpu: 1, Memory: 128 << 20, Disk: 256 << 20},
		Networks:       network.Attachments(network.PolicyIsolated, "", ""),
		Command:        []string{"sh", "-c", script},
	}
}

// until inspects a run until it is what is wanted, and fails the test if it
// never becomes that.
func until(t *testing.T, runtime *client.Runtime, id string, what string, wanted func(task.Execution) bool) task.Execution {
	t.Helper()

	deadline := time.Now().Add(settle)

	for {
		inspected, err := runtime.Inspect(t.Context(), id)
		require.NoError(t, err)

		if wanted(inspected) {
			return inspected
		}

		if time.Now().After(deadline) {
			t.Fatalf("run %s never %s: it is %+v", id, what, inspected)
		}

		time.Sleep(250 * time.Millisecond)
	}
}

func exited(e task.Execution) bool {
	return e.Status == task.StatusExited
}

// logged waits until a run's log holds text, and fails the test if it never
// does.
func logged(t *testing.T, runtime *client.Runtime, id string, text string) {
	t.Helper()

	deadline := time.Now().Add(settle)

	for {
		var log bytes.Buffer
		require.NoError(t, runtime.Logs(t.Context(), id, &log))

		if strings.Contains(log.String(), text) {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("run %s never logged %q: its log is %q", id, text, log.String())
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func TestService_Lifecycle(t *testing.T) {
	c := realConfigs(t)
	node, runtime := realNode(t, c, "lifecycle")
	ctx := t.Context()

	// a service that finishes cleanly when it is asked to stop, once it has
	// said it will.
	execution := job(node, "lifecycle", `trap "exit 0" TERM; echo trapped; while true; do sleep 1; done`)
	execution.Kind = task.KindService

	id, err := runtime.Create(ctx, execution)
	require.NoError(t, err)

	inspected, err := runtime.Inspect(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, task.StatusCreated, inspected.Status)

	require.NoError(t, runtime.Start(ctx, id))
	inspected, err = runtime.Inspect(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, task.StatusRunning, inspected.Status)
	assert.False(t, inspected.StartedAt.IsZero())

	// started is the process running, not its shell having set the trap
	// yet: a stop sent before that ends it with 143, as it would on docker.
	logged(t, runtime, id, "trapped")

	require.NoError(t, runtime.Stop(ctx, id))
	inspected, err = runtime.Inspect(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, task.StatusExited, inspected.Status)
	assert.Equal(t, 0, inspected.ExitCode, "the TERM trap exits cleanly")

	require.NoError(t, runtime.Start(ctx, id))
	inspected, err = runtime.Inspect(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, task.StatusRunning, inspected.Status)

	require.NoError(t, runtime.Kill(ctx, id))
	inspected, err = runtime.Inspect(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, task.StatusExited, inspected.Status)
	assert.Equal(t, 137, inspected.ExitCode)

	require.NoError(t, runtime.Restart(ctx, id))
	inspected, err = runtime.Inspect(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, task.StatusRunning, inspected.Status)
	assert.Zero(t, inspected.RestartCount, "a restart somebody asked for is not the policy's")

	require.NoError(t, runtime.Delete(ctx, id))
	_, err = runtime.Inspect(ctx, id)
	assert.ErrorIs(t, err, domain.ErrNotExists)
}

func TestService_Listing(t *testing.T) {
	c := realConfigs(t)
	nodeA, runtimeA := realNode(t, c, "a")
	nodeB, runtimeB := realNode(t, c, "b")
	ctx := t.Context()

	first := job(nodeA, "first", "true")
	second := job(nodeA, "second", "true")

	// the same task on the other node, as a task rescheduled elsewhere leaves.
	elsewhere := job(nodeB, "elsewhere", "true")
	elsewhere.TaskUUID = first.TaskUUID

	firstID, err := runtimeA.Create(ctx, first)
	require.NoError(t, err)
	secondID, err := runtimeA.Create(ctx, second)
	require.NoError(t, err)
	elsewhereID, err := runtimeB.Create(ctx, elsewhere)
	require.NoError(t, err)

	ids := func(executions []task.Execution, err error) []string {
		require.NoError(t, err)

		listed := make([]string, len(executions))
		for i, e := range executions {
			listed[i] = e.ID
		}

		return listed
	}

	assert.Equal(t, []string{secondID, firstID}, ids(runtimeA.OnNode(ctx, nodeA)))
	assert.Equal(t, []string{elsewhereID}, ids(runtimeB.OnNode(ctx, nodeB)))
	assert.Equal(t, []string{firstID}, ids(runtimeA.Of(ctx, first.TaskUUID)))
	assert.Equal(t, []string{elsewhereID}, ids(runtimeB.Of(ctx, first.TaskUUID)))
	assert.Equal(t, []string{secondID}, ids(runtimeA.BySlug(ctx, "second")))
	assert.Empty(t, ids(runtimeB.BySlug(ctx, "second")))
}

func TestService_ExitCodes(t *testing.T) {
	c := realConfigs(t)
	node, runtime := realNode(t, c, "exits")
	ctx := t.Context()

	for _, code := range []int{0, 1, 3} {
		id, err := runtime.Create(ctx, job(node, fmt.Sprintf("exit-%d", code), fmt.Sprintf("exit %d", code)))
		require.NoError(t, err)
		require.NoError(t, runtime.Start(ctx, id))

		ended := until(t, runtime, id, "exited", exited)
		assert.Equal(t, code, ended.ExitCode)
	}

	id, err := runtime.Create(ctx, job(node, "killed", "sleep 600"))
	require.NoError(t, err)
	require.NoError(t, runtime.Start(ctx, id))
	require.NoError(t, runtime.Kill(ctx, id))

	ended := until(t, runtime, id, "exited", exited)
	assert.Equal(t, 137, ended.ExitCode)
	assert.Equal(t, task.Completed, task.EvaluateState(ended.Status, ended.Kind, ended.ExitCode), "a job that was cut short did not fail")
}

func TestService_Logs(t *testing.T) {
	c := realConfigs(t)
	node, runtime := realNode(t, c, "logs")
	ctx := t.Context()

	id, err := runtime.Create(ctx, job(node, "counting", `i=1; while [ $i -le 100 ]; do echo line-$i; i=$((i+1)); done; echo done >&2`))
	require.NoError(t, err)
	require.NoError(t, runtime.Start(ctx, id))
	until(t, runtime, id, "exited", exited)

	var whole bytes.Buffer
	require.NoError(t, runtime.Logs(ctx, id, &whole))
	assert.Equal(t, 101, strings.Count(whole.String(), "\n"))
	assert.True(t, strings.HasPrefix(whole.String(), "line-1\nline-2\n"))

	var lines []task.LogLine
	require.NoError(t, runtime.StreamLogs(ctx, id, time.Time{}, func(line task.LogLine) error {
		lines = append(lines, line)

		return nil
	}))
	require.Len(t, lines, 101)

	// lines keep their order within a stream. Between stdout and stderr
	// nothing promises one, here as on docker: the guest reads the two apart.
	var stdout, stderr []string
	for _, line := range lines {
		if line.Stream == task.StreamStderr {
			stderr = append(stderr, line.Content)
		} else {
			stdout = append(stdout, line.Content)
		}
	}

	assert.Equal(t, []string{"done"}, stderr)
	require.Len(t, stdout, 100)
	for i, content := range stdout {
		assert.Equal(t, fmt.Sprintf("line-%d", i+1), content)
	}

	for i := 1; i < len(lines); i++ {
		assert.True(t, lines[i].At.After(lines[i-1].At), "every line is later than the one before")
	}

	// picked up again from a line it already has: that line again, and
	// everything after it, nothing lost.
	var resumed []task.LogLine
	require.NoError(t, runtime.StreamLogs(ctx, id, lines[49].At, func(line task.LogLine) error {
		resumed = append(resumed, line)

		return nil
	}))
	assert.Equal(t, lines[49:], resumed)
}

func TestService_Exec(t *testing.T) {
	c := realConfigs(t)
	node, runtime := realNode(t, c, "exec")
	ctx := t.Context()

	service := job(node, "terminal", "while true; do sleep 1; done")
	service.Kind = task.KindService

	id, err := runtime.Create(ctx, service)
	require.NoError(t, err)
	require.NoError(t, runtime.Start(ctx, id))

	session, err := runtime.Exec(ctx, id, task.ExecOptions{Command: []string{"/bin/sh"}, TTY: true})
	require.NoError(t, err)

	require.NoError(t, session.Resize(ctx, 40, 120))

	_, err = session.Write([]byte("stty size; sleep 1001 &\n"))
	require.NoError(t, err)

	readUntil(t, session, "40 120")

	require.NoError(t, session.Close())

	ending, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	require.NoError(t, session.End(ending))

	// what the terminal started went with it.
	sweep, err := runtime.Exec(ctx, id, task.ExecOptions{Command: []string{"sh", "-c", "ps | grep -c '[s]leep 1001' || true"}})
	require.NoError(t, err)
	defer sweep.Close()

	left, err := io.ReadAll(sweep)
	require.NoError(t, err)
	assert.Equal(t, "0", strings.TrimSpace(string(left)))
}

// readUntil reads a session until what it has read holds want.
func readUntil(t *testing.T, session io.Reader, want string) {
	t.Helper()

	var read bytes.Buffer

	done := make(chan error, 1)
	go func() {
		buffer := make([]byte, 1024)
		for !strings.Contains(read.String(), want) {
			n, err := session.Read(buffer)
			read.Write(buffer[:n])

			if err != nil {
				done <- err

				return
			}
		}

		done <- nil
	}()

	select {
	case err := <-done:
		require.NoError(t, err, "read so far: %q", read.String())
	case <-time.After(settle):
		t.Fatalf("%q never came", want)
	}
}

func TestService_Limits(t *testing.T) {
	c := realConfigs(t)
	node, runtime := realNode(t, c, "limits")
	ctx := t.Context()

	t.Run("memory past the limit is a kill", func(t *testing.T) {
		// one buffer of 96 MiB, filled at once, in a guest of 128 MiB whose
		// kernel keeps about 24 of them; dd is the main process, so what the
		// guest kills is the run. Memory taken a little at a time can leave a
		// guest reclaiming for minutes before anything is killed, and a buffer
		// larger than all of the guest's memory is refused before anything is
		// touched, so the process merely exits 1.
		hungry := job(node, "hungry", `exec dd if=/dev/zero of=/dev/null bs=96M count=1`)

		id, err := runtime.Create(ctx, hungry)
		require.NoError(t, err)
		require.NoError(t, runtime.Start(ctx, id))

		ended := until(t, runtime, id, "exited", exited)
		assert.Equal(t, 137, ended.ExitCode)
	})

	t.Run("disk past the limit fails", func(t *testing.T) {
		filling := job(node, "filling", `dd if=/dev/zero of=/fill bs=1M count=512`)
		filling.ResourceLimits.Disk = 64 << 20

		id, err := runtime.Create(ctx, filling)
		require.NoError(t, err)
		require.NoError(t, runtime.Start(ctx, id))

		ended := until(t, runtime, id, "exited", exited)
		assert.NotZero(t, ended.ExitCode)

		var log bytes.Buffer
		require.NoError(t, runtime.Logs(ctx, id, &log))
		assert.Contains(t, log.String(), "No space left on device")
	})
}

func TestService_Ports(t *testing.T) {
	c := realConfigs(t)
	node, runtime := realNode(t, c, "ports")
	ctx := t.Context()

	require.NoError(t, runtime.EnsureImage(ctx, nginx))

	web := &task.Execution{
		Name:           "it-nginx",
		TaskUUID:       node + "-nginx",
		TaskName:       "nginx",
		Slug:           "it-nginx",
		Kind:           task.KindService,
		NodeName:       node,
		Image:          nginx,
		ResourceLimits: task.ResourceLimits{Cpu: 1, Memory: 128 << 20, Disk: 256 << 20},
		Networks:       network.Attachments(network.PolicyIsolated, "", ""),
		ExposedPorts:   port.PortSet{80: {}},
		PortBindings:   port.PortMap{80: {{HostIP: "0.0.0.0"}}},
	}

	id, err := runtime.Create(ctx, web)
	require.NoError(t, err)
	require.NoError(t, runtime.Start(ctx, id))

	endpoint, err := getEndpoint.NewUseCase(runtime, c.PortsHost()).Execute(ctx, &getEndpoint.Request{Slug: "it-nginx"})
	require.NoError(t, err)

	visitor := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	deadline := time.Now().Add(settle)

	for {
		response, err := visitor.Get("http://" + endpoint.Address() + "/")
		if err == nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()

			assert.Equal(t, http.StatusOK, response.StatusCode)
			assert.Contains(t, string(body), "nginx")

			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("nginx was never reached at %s: %v", endpoint.Address(), err)
		}

		time.Sleep(500 * time.Millisecond)
	}
}

func TestService_RestartPolicies(t *testing.T) {
	c := realConfigs(t)
	node, runtime := realNode(t, c, "policies")
	ctx := t.Context()

	start := func(t *testing.T, name string, policy string, script string) string {
		t.Helper()

		execution := job(node, name, script)
		execution.Kind = task.KindService
		execution.RestartPolicy = policy

		id, err := runtime.Create(ctx, execution)
		require.NoError(t, err)
		require.NoError(t, runtime.Start(ctx, id))

		return id
	}

	t.Run("no stays down", func(t *testing.T) {
		id := start(t, "policy-no", "no", "exit 1")

		ended := until(t, runtime, id, "exited", exited)
		time.Sleep(3 * time.Second)

		after, err := runtime.Inspect(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, task.StatusExited, after.Status)
		assert.Equal(t, ended.StartedAt, after.StartedAt)
		assert.Zero(t, after.RestartCount)
	})

	t.Run("on-failure gives up after as many restarts as it was given", func(t *testing.T) {
		id := start(t, "policy-on-failure", "on-failure:2", "exit 1")

		ended := until(t, runtime, id, "restarted twice and stopped", func(e task.Execution) bool {
			return exited(e) && e.RestartCount == 2
		})
		assert.Equal(t, 1, ended.ExitCode)
	})

	t.Run("on-failure leaves a success alone", func(t *testing.T) {
		id := start(t, "policy-on-success", "on-failure", "exit 0")

		until(t, runtime, id, "exited", exited)
		time.Sleep(3 * time.Second)

		after, err := runtime.Inspect(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, task.StatusExited, after.Status)
		assert.Zero(t, after.RestartCount)
	})

	for _, policy := range []string{"always", "unless-stopped"} {
		t.Run(policy+" comes back until it is stopped", func(t *testing.T) {
			id := start(t, "policy-"+policy, policy, "sleep 1; exit 0")

			until(t, runtime, id, "restarted", func(e task.Execution) bool { return e.RestartCount >= 2 })

			require.NoError(t, runtime.Stop(ctx, id))
			stopped := until(t, runtime, id, "exited", exited)

			time.Sleep(3 * time.Second)

			after, err := runtime.Inspect(ctx, id)
			require.NoError(t, err)
			assert.Equal(t, task.StatusExited, after.Status, "a stop somebody asked for is not undone")
			assert.Equal(t, stopped.RestartCount, after.RestartCount)
		})
	}
}
