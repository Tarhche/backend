//go:build firecracker && linux

package firecracker

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/runner/machine"
	"github.com/khanzadimahdi/testproject/domain/runner/network"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/initrd"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/layout"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/vmm"
)

// These run real machines: they need /dev/kvm, firecracker, mke2fs, a kernel
// at RUNNER_TEST_KERNEL, and a registry to pull alpine from. The launcher's
// half runs in the test, as whoever runs it, and its tasks are on no network,
// so none of it needs root.
//
//	RUNNER_TEST_KERNEL=/path/to/vmlinux go test -tags firecracker ./infrastructure/runner/firecracker/

const testOwner = "runner-orchestrator-test"

// localLauncher is the launcher's half, in the test: machines' processes, and
// no networks at all.
type localLauncher struct {
	vmm *vmm.VMM
}

func (l *localLauncher) Launch(ctx context.Context, spec machine.Spec) (machine.Machine, error) {
	if len(spec.Taps) > 0 {
		return machine.Machine{}, errors.New("there are no networks here")
	}

	return l.vmm.Spawn(ctx, spec)
}

func (l *localLauncher) Terminate(ctx context.Context, id string) error {
	return l.vmm.Kill(ctx, id)
}

func (l *localLauncher) Machines(ctx context.Context, owner string) ([]machine.Machine, error) {
	all, err := l.vmm.List(ctx)
	if err != nil {
		return nil, err
	}

	var owned []machine.Machine
	for _, m := range all {
		if m.Owner == owner {
			owned = append(owned, m)
		}
	}

	return owned, nil
}

func (l *localLauncher) EnsureNetwork(context.Context, string, string, bool) (machine.Network, error) {
	return machine.Network{}, errors.New("there are no networks here")
}

func (l *localLauncher) RemoveNetwork(context.Context, string, string) error {
	return nil
}

// harness is a state directory with everything a machine boots from in it.
type harness struct {
	state    string
	config   Config
	launcher *localLauncher
	logger   *slog.Logger
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	kernel := os.Getenv("RUNNER_TEST_KERNEL")
	if len(kernel) == 0 {
		t.Skip("RUNNER_TEST_KERNEL names no kernel to boot")
	}

	firecrackerBinary, err := exec.LookPath("firecracker")
	if err != nil {
		t.Skip("there is no firecracker to run machines with")
	}

	// short on purpose: a socket's path is at most 108 bytes.
	state, err := os.MkdirTemp("", "rt")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(state) })

	require.NoError(t, layout.Prepare(state, kernel, os.Getuid(), os.Getgid()))

	agent := filepath.Join(state, "runner-guest")
	build := exec.Command("go", "build", "-o", agent, "./cmd/runner-guest")
	build.Dir = moduleRoot(t)
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	output, err := build.CombinedOutput()
	require.NoError(t, err, string(output))

	initramfs, err := initrd.Ensure(layout.Boot(state), agent)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if testing.Verbose() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}

	// every machine runs as whoever runs the test: nothing here can be anybody
	// else.
	machines, err := vmm.New(vmm.Config{StateDir: state, FirecrackerBinary: firecrackerBinary}, logger)
	require.NoError(t, err)

	return &harness{
		state: state,
		config: Config{
			Owner:    testOwner,
			StateDir: state,
			Kernel:   layout.Kernel(state),
			Initrd:   initramfs,
			UID:      os.Getuid(),
			GID:      os.Getgid(),
		},
		launcher: &localLauncher{vmm: machines},
		logger:   logger,
	}
}

func (h *harness) runtime(t *testing.T) *Runtime {
	t.Helper()

	r, _, err := New(h.config, h.launcher, h.logger)
	require.NoError(t, err)

	return r
}

func moduleRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, "the module's root is not above here")
		dir = parent
	}
}

// execution is a task on no network, the way runTask hands one over.
func execution(name string, kind task.Kind, command ...string) *task.Execution {
	return &task.Execution{
		Name:           name,
		TaskUUID:       name + "-uuid",
		TaskName:       name,
		Slug:           name,
		Kind:           kind,
		NodeName:       testOwner,
		Image:          "alpine:3.20",
		Command:        command,
		Networks:       network.Attachments(network.PolicyNone, "", ""),
		ResourceLimits: task.ResourceLimits{Cpu: 1, Memory: 128 << 20, Disk: 128 << 20},
	}
}

// waitFor waits for a machine to be what until says, and says what it was.
func waitFor(t *testing.T, r *Runtime, id string, until func(task.Execution) bool) task.Execution {
	t.Helper()

	deadline := time.Now().Add(60 * time.Second)

	for {
		inspected, err := r.Inspect(context.Background(), id)
		require.NoError(t, err)

		if until(inspected) {
			return inspected
		}

		require.True(t, time.Now().Before(deadline), "the machine never got there: %+v", inspected)
		time.Sleep(50 * time.Millisecond)
	}
}

func ended(e task.Execution) bool {
	return e.Status == task.StatusExited || e.Status == task.StatusDead
}

func logsOf(t *testing.T, r *Runtime, id string) string {
	t.Helper()

	var logs strings.Builder
	require.NoError(t, r.Logs(context.Background(), id, &logs))

	return logs.String()
}

func TestRuntime(t *testing.T) {
	h := newHarness(t)
	r := h.runtime(t)
	defer r.Close(context.Background())

	ctx := context.Background()
	require.NoError(t, r.EnsureImage(ctx, "alpine:3.20"))

	t.Run("a job runs in a machine of its own, and says what it wrote and what it returned", func(t *testing.T) {
		id, err := r.Create(ctx, execution("job", task.KindJob, "/bin/sh", "-c", "echo hello from a microvm; echo and from its stderr >&2; exit 3"))
		require.NoError(t, err)

		started := time.Now()
		require.NoError(t, r.Start(ctx, id))

		finished := waitFor(t, r, id, ended)
		t.Logf("the job ran and ended in %s", time.Since(started))

		assert.Equal(t, task.StatusExited, finished.Status)
		assert.Equal(t, 3, finished.ExitCode)
		assert.Equal(t, "hello from a microvm\nand from its stderr\n", logsOf(t, r, id))

		var streamed []task.LogLine
		require.NoError(t, r.StreamLogs(ctx, id, time.Time{}, func(line task.LogLine) error {
			streamed = append(streamed, line)

			return nil
		}))
		require.Len(t, streamed, 2)
		assert.Equal(t, task.StreamStderr, streamed[1].Stream)

		held, err := h.launcher.Machines(ctx, testOwner)
		require.NoError(t, err)
		assert.Empty(t, held, "a machine whose task ended is let go")

		require.NoError(t, r.Delete(ctx, id))

		_, err = r.Inspect(ctx, id)
		assert.Error(t, err)
	})

	t.Run("a task that fails is started again as often as its policy says, inside its machine", func(t *testing.T) {
		failing := execution("failing", task.KindService, "/bin/sh", "-c", "echo attempt; exit 1")
		failing.RestartPolicy = "on-failure:2"

		id, err := r.Create(ctx, failing)
		require.NoError(t, err)
		require.NoError(t, r.Start(ctx, id))

		finished := waitFor(t, r, id, ended)

		assert.Equal(t, uint(2), finished.RestartCount)
		assert.Equal(t, 1, finished.ExitCode)
		assert.Equal(t, "attempt\nattempt\nattempt\n", logsOf(t, r, id))

		require.NoError(t, r.Delete(ctx, id))
	})

	t.Run("a service runs until it is stopped, and a terminal can be opened in it meanwhile", func(t *testing.T) {
		id, err := r.Create(ctx, execution("service", task.KindService, "/bin/sh", "-c", "echo up; exec sleep 1000"))
		require.NoError(t, err)
		require.NoError(t, r.Start(ctx, id))

		running := waitFor(t, r, id, func(e task.Execution) bool { return e.Status == task.StatusRunning })
		assert.False(t, running.StartedAt.IsZero())

		stats, err := r.Stats(ctx, id)
		require.NoError(t, err)
		assert.NotZero(t, stats.PIDs)
		assert.NotZero(t, stats.MemoryLimit)

		session, err := r.Exec(ctx, id, task.ExecOptions{Command: []string{"/bin/sh"}, TTY: true})
		require.NoError(t, err)

		require.NoError(t, session.Resize(ctx, 24, 80))

		_, err = session.Write([]byte("echo $((6*7)) > /tmp/answer; cat /tmp/answer; stty size\n"))
		require.NoError(t, err)

		answer := make(chan string, 1)
		go func() {
			reader := bufio.NewReader(session)
			var seen strings.Builder

			for {
				line, err := reader.ReadString('\n')
				seen.WriteString(line)

				if strings.Contains(seen.String(), "24 80") || err != nil {
					answer <- seen.String()

					return
				}
			}
		}()

		select {
		case seen := <-answer:
			assert.Contains(t, seen, "42")
			assert.Contains(t, seen, "24 80")
		case <-time.After(10 * time.Second):
			t.Fatal("the terminal never answered")
		}

		require.NoError(t, session.Close())

		endCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		require.NoError(t, session.End(endCtx))

		require.NoError(t, r.Stop(ctx, id))

		stopped := waitFor(t, r, id, ended)
		assert.Equal(t, task.StatusExited, stopped.Status)
		assert.Equal(t, 143, stopped.ExitCode, "sleep ends on the TERM a stop sends")
		assert.Contains(t, logsOf(t, r, id), "up\n")

		// started again from the same disks, and never taken for ended while it
		// boots: what it wrote is still there.
		statuses := watchedWhile(t, r, id, r.Start)
		assert.Contains(t, statuses, task.StatusRestarting, "a stopped machine booting again says so")
		for _, status := range statuses {
			assert.Contains(t, []task.Status{task.StatusRunning, task.StatusRestarting}, status, "a stopped machine starting again reported %d", status)
		}

		check, err := r.Exec(ctx, id, task.ExecOptions{Command: []string{"cat", "/tmp/answer"}})
		require.NoError(t, err)

		kept, err := io.ReadAll(check)
		require.NoError(t, err)
		assert.Equal(t, "42\n", string(kept))
		require.NoError(t, check.Close())

		// a restart is never an end, to anybody watching it, whether the
		// machine was running...
		for _, status := range watchedWhile(t, r, id, r.Restart) {
			assert.Contains(t, []task.Status{task.StatusRunning, task.StatusRestarting}, status, "a restarting machine reported %d", status)
		}

		// ...or had been stopped, and has a machine to boot before it runs.
		require.NoError(t, r.Stop(ctx, id))
		waitFor(t, r, id, ended)

		statuses = watchedWhile(t, r, id, r.Restart)
		assert.Contains(t, statuses, task.StatusRestarting, "a stopped machine booting again says so")
		for _, status := range statuses {
			assert.Contains(t, []task.Status{task.StatusRunning, task.StatusRestarting}, status, "a stopped machine restarting reported %d", status)
		}

		require.NoError(t, r.Kill(ctx, id))

		killed := waitFor(t, r, id, ended)
		assert.Equal(t, 137, killed.ExitCode)

		require.NoError(t, r.Delete(ctx, id))
	})

	t.Run("a read-only task can change nothing of its root", func(t *testing.T) {
		readOnly := execution("read-only", task.KindJob, "/bin/sh", "-c", "touch /changed")
		readOnly.ReadOnly = true

		id, err := r.Create(ctx, readOnly)
		require.NoError(t, err)
		require.NoError(t, r.Start(ctx, id))

		finished := waitFor(t, r, id, ended)
		assert.NotEqual(t, 0, finished.ExitCode)
		assert.Contains(t, logsOf(t, r, id), "Read-only file system")

		require.NoError(t, r.Delete(ctx, id))
	})

	t.Run("a command the image does not have is a task that could not start", func(t *testing.T) {
		id, err := r.Create(ctx, execution("missing", task.KindJob, "no-such-command"))
		require.NoError(t, err)

		assert.Error(t, r.Start(ctx, id))

		finished, err := r.Inspect(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, 127, finished.ExitCode)

		require.NoError(t, r.Delete(ctx, id))
	})
}

// watchedWhile does something to a machine, and says every status it read as
// while that was done.
func watchedWhile(t *testing.T, r *Runtime, id string, do func(context.Context, string) error) []task.Status {
	t.Helper()

	done := make(chan error, 1)
	go func() { done <- do(context.Background(), id) }()

	// what it read as before what was asked took hold is not what is watched.
	time.Sleep(100 * time.Millisecond)

	var statuses []task.Status

	for {
		if inspected, err := r.Inspect(context.Background(), id); err == nil {
			statuses = append(statuses, inspected.Status)
		}

		select {
		case err := <-done:
			require.NoError(t, err)

			return statuses
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestRuntimeTakesItsMachinesBack(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	first := h.runtime(t)

	id, err := first.Create(ctx, execution("long", task.KindService, "/bin/sh", "-c", "echo before; sleep 2; echo after; exec sleep 1000"))
	require.NoError(t, err)
	require.NoError(t, first.Start(ctx, id))
	waitFor(t, first, id, func(e task.Execution) bool { return e.Status == task.StatusRunning })

	// the orchestrator goes away; the machine does not.
	require.NoError(t, first.Close(ctx))

	held, err := h.launcher.Machines(ctx, testOwner)
	require.NoError(t, err)
	require.Len(t, held, 1)
	assert.True(t, held[0].Running)

	second := h.runtime(t)
	defer second.Close(ctx)

	waitFor(t, second, id, func(e task.Execution) bool { return second.keeper(id) != nil })

	// what the task wrote while nobody was looking is kept all the same.
	deadline := time.Now().Add(20 * time.Second)
	for !strings.Contains(logsOf(t, second, id), "after") {
		require.True(t, time.Now().Before(deadline), "what was written meanwhile never arrived: %q", logsOf(t, second, id))
		time.Sleep(100 * time.Millisecond)
	}

	assert.Equal(t, "before\nafter\n", logsOf(t, second, id))

	require.NoError(t, second.Stop(ctx, id))
	waitFor(t, second, id, ended)
	require.NoError(t, second.Delete(ctx, id))

	held, err = h.launcher.Machines(ctx, testOwner)
	require.NoError(t, err)
	assert.Empty(t, held)
}
