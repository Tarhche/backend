package microvm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/driver"
	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// newDriver builds the firecracker class's driver for orchestrator 01, on a
// fake vmhost.
func newDriver(t *testing.T, f *fakeVMHost) *Driver {
	t.Helper()

	d, err := New(t.Context(), orchestrator, driver.Spec{
		Class:    runtime.Firecracker,
		Kind:     driver.KindMicroVM,
		Endpoint: f.endpoint,
		Options:  url.Values{},
	}, slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	t.Cleanup(func() { _ = d.Close() })

	return d
}

// asked is what the driver asked vmhost for, but for what it asks every second
// to keep its offer fresh.
func asked(f *fakeVMHost) []string {
	return slices.DeleteFunc(f.requests(), func(request string) bool {
		return request == "GET "+vm.PathInfo
	})
}

// ours and theirs are VMs held for orchestrator 01, and for another.
func ours(f *fakeVMHost, taskUUID string, slug string) vm.VM {
	return running(f, map[string]string{"node.name": orchestrator, "task.uuid": taskUUID, "task.slug": slug, "task.kind": "service"}, 80)
}

func theirs(f *fakeVMHost, taskUUID string, slug string) vm.VM {
	return running(f, map[string]string{"node.name": "workload-orchestrator-02", "task.uuid": taskUUID, "task.slug": slug, "task.kind": "service"}, 80)
}

func ids(executions []task.Execution) []string {
	found := make([]string, len(executions))
	for i := range executions {
		found[i] = executions[i].ID
	}

	return found
}

func TestRuntime(t *testing.T) {
	t.Run("a run is made as a vm, and read back as the run it was asked to be", func(t *testing.T) {
		f := newFakeVMHost(t)
		tasks := newDriver(t, f).Tasks()

		run := service()

		id, err := tasks.Create(t.Context(), &run)
		require.NoError(t, err)
		assert.True(t, vm.IsID(id))

		held, found := f.held(id)
		require.True(t, found)
		assert.Equal(t, orchestrator, held.Spec.Labels["node.name"])
		assert.Equal(t, "shop-api-xkfqz", held.Spec.Hostname)

		inspected, err := tasks.Inspect(t.Context(), id)
		require.NoError(t, err)

		assert.Equal(t, id, inspected.ID)
		assert.Equal(t, runtime.Firecracker, inspected.Runtime)
		assert.Equal(t, task.StatusCreated, inspected.Status)
		assert.Equal(t, run.TaskUUID, inspected.TaskUUID)
		assert.Equal(t, run.Networks, inspected.Networks)
		assert.Equal(t, run.ResourceLimits, inspected.ResourceLimits)
	})

	t.Run("a vm vmhost has no room for is refused as such, so another node may run it", func(t *testing.T) {
		f := newFakeVMHost(t)
		f.room = 128 << 20

		run := service()

		_, err := newDriver(t, f).Tasks().Create(t.Context(), &run)

		assert.ErrorIs(t, err, vm.ErrCapacity)
	})

	t.Run("a run of the same name as one held already is a conflict, as docker's would be", func(t *testing.T) {
		f := newFakeVMHost(t)
		tasks := newDriver(t, f).Tasks()

		run := service()

		_, err := tasks.Create(t.Context(), &run)
		require.NoError(t, err)

		_, err = tasks.Create(t.Context(), &run)
		assert.ErrorIs(t, err, vm.ErrConflict)
	})

	t.Run("a node lists its own vms, and no other node's", func(t *testing.T) {
		f := newFakeVMHost(t)
		tasks := newDriver(t, f).Tasks()

		mine := ours(f, "u-1", "nginx-xkfqz")
		theirs(f, "u-1", "nginx-xkfqz")
		other := ours(f, "u-2", "redis-bcdef")

		held, err := tasks.OnNode(t.Context(), orchestrator)
		require.NoError(t, err)
		assert.Equal(t, []string{mine.ID, other.ID}, ids(held))

		of, err := tasks.Of(t.Context(), "u-1")
		require.NoError(t, err)
		assert.Equal(t, []string{mine.ID}, ids(of))

		bySlug, err := tasks.BySlug(t.Context(), "nginx-xkfqz")
		require.NoError(t, err)
		assert.Equal(t, []string{mine.ID}, ids(bySlug))

		elsewhere, err := tasks.OnNode(t.Context(), "workload-orchestrator-02")
		require.NoError(t, err)
		assert.Empty(t, elsewhere, "another node's vms are not this driver's to list")

		assert.Equal(t, []string{
			"GET /v1/vms?label=node.name%3Dworkload-orchestrator-01",
			"GET /v1/vms?label=node.name%3Dworkload-orchestrator-01&label=task.uuid%3Du-1",
			"GET /v1/vms?label=node.name%3Dworkload-orchestrator-01&label=task.slug%3Dnginx-xkfqz",
		}, asked(f))
	})

	t.Run("a vm vmhost hands back that is not this node's is not handed on", func(t *testing.T) {
		f := newFakeVMHost(t)
		f.unfiltered = true

		mine := ours(f, "u-1", "nginx-xkfqz")
		theirs(f, "u-1", "nginx-xkfqz")

		held, err := newDriver(t, f).Tasks().Of(t.Context(), "u-1")

		require.NoError(t, err)
		assert.Equal(t, []string{mine.ID}, ids(held))
	})

	t.Run("a listing carries what a run returned and how to reach it", func(t *testing.T) {
		f := newFakeVMHost(t)

		mine := ours(f, "u-1", "nginx-xkfqz")

		held, err := newDriver(t, f).Tasks().BySlug(t.Context(), "nginx-xkfqz")
		require.NoError(t, err)
		require.Len(t, held, 1)

		assert.Equal(t, mine.ID, held[0].ID)
		assert.Equal(t, runtime.Firecracker, held[0].Runtime)
		assert.Equal(t, task.StatusRunning, held[0].Status)
		assert.Equal(t, []port.Port{80}, held[0].Endpoints)
		assert.Empty(t, held[0].PortBindings, "a vm's ports are published nowhere")
	})

	t.Run("a vm's life is lived through vmhost", func(t *testing.T) {
		f := newFakeVMHost(t)
		tasks := newDriver(t, f).Tasks()

		run := service()

		id, err := tasks.Create(t.Context(), &run)
		require.NoError(t, err)

		require.NoError(t, tasks.Start(t.Context(), id))

		held, _ := f.held(id)
		assert.Equal(t, vm.StateRunning, held.State)

		require.NoError(t, tasks.Stop(t.Context(), id))

		held, _ = f.held(id)
		assert.Equal(t, vm.StateExited, held.State)
		assert.Equal(t, "stopped within 10s", held.Reason, "the task is given the time docker gives a container")

		require.NoError(t, tasks.Restart(t.Context(), id))
		require.NoError(t, tasks.Kill(t.Context(), id))

		inspected, err := tasks.Inspect(t.Context(), id)
		require.NoError(t, err)
		assert.Equal(t, 137, inspected.ExitCode)
		assert.Equal(t, uint(1), inspected.RestartCount)

		require.NoError(t, tasks.Delete(t.Context(), id))

		_, found := f.held(id)
		assert.False(t, found)

		assert.ErrorIs(t, tasks.Delete(t.Context(), id), domain.ErrNotExists, "a vm that is gone is already what was asked for")
	})

	t.Run("another node's vm is not this node's to act on, even by its id", func(t *testing.T) {
		f := newFakeVMHost(t)
		tasks := newDriver(t, f).Tasks()

		other := theirs(f, "u-1", "nginx-xkfqz")

		for name, act := range map[string]func(context.Context, string) error{
			"start":   tasks.Start,
			"stop":    tasks.Stop,
			"restart": tasks.Restart,
			"kill":    tasks.Kill,
			"delete":  tasks.Delete,
		} {
			assert.ErrorIs(t, act(t.Context(), other.ID), domain.ErrNotExists, name)
		}

		_, err := tasks.Inspect(t.Context(), other.ID)
		assert.ErrorIs(t, err, domain.ErrNotExists)

		_, err = tasks.Exec(t.Context(), other.ID, task.ExecOptions{Command: []string{"sh"}})
		assert.ErrorIs(t, err, domain.ErrNotExists)

		for _, request := range asked(f) {
			assert.True(t, strings.HasPrefix(request, "GET "), "nothing but asking about it: %s", request)
		}

		held, _ := f.held(other.ID)
		assert.Equal(t, vm.StateRunning, held.State)
	})

	t.Run("an image is made ready through vmhost", func(t *testing.T) {
		f := newFakeVMHost(t)

		require.NoError(t, newDriver(t, f).Tasks().EnsureImage(t.Context(), "nginx:alpine"))

		assert.Equal(t, []string{"POST /v1/images/prepare"}, asked(f))
	})

	t.Run("an image that cannot be made into a disk says so", func(t *testing.T) {
		f := newFakeVMHost(t)
		f.refuseRoute(vm.RoutePrepareImage, fmt.Errorf("%w: amd64-only:1 has no variant for arm64", vm.ErrImage))

		err := newDriver(t, f).Tasks().EnsureImage(t.Context(), "amd64-only:1")

		assert.ErrorIs(t, err, vm.ErrImage)
	})

	t.Run("what a vm uses is what vmhost says, and nothing for one that does not run", func(t *testing.T) {
		f := newFakeVMHost(t)
		tasks := newDriver(t, f).Tasks()

		up := ours(f, "u-1", "nginx-xkfqz")
		f.stats[up.ID] = vm.Stats{PIDs: 4, CPUPercent: 50, MemoryUsage: 64 << 20, MemoryLimit: 256 << 20, NetworkInput: 10, BlockOutput: 20}

		down := f.hold(vm.VM{State: vm.StateExited, Spec: vm.Spec{Labels: map[string]string{"node.name": orchestrator}}})

		used, err := tasks.Stats(t.Context(), up.ID)
		require.NoError(t, err)
		assert.Equal(t, task.Stats{PIDs: 4, CPUPercent: 50, MemoryUsage: 64 << 20, MemoryLimit: 256 << 20, MemoryPercent: 25, NetworkInput: 10, BlockOutput: 20}, used)

		used, err = tasks.Stats(t.Context(), down.ID)
		require.NoError(t, err)
		assert.Equal(t, task.Stats{}, used)
	})

	t.Run("a vm's whole output is written out a line at a time", func(t *testing.T) {
		f := newFakeVMHost(t)

		held := ours(f, "u-1", "nginx-xkfqz")
		f.say(held.ID, vm.LogLine{Seq: 1, Stream: guest.StreamStdout, Content: "one"})
		f.say(held.ID, vm.LogLine{Seq: 2, Stream: guest.StreamStderr, Content: "two"})

		var written bytes.Buffer
		require.NoError(t, newDriver(t, f).Tasks().Logs(t.Context(), held.ID, &written))

		assert.Equal(t, "one\ntwo\n", written.String())
	})

	t.Run("following a vm's output carries each line from the time asked for, and stops without failing when asked to", func(t *testing.T) {
		f := newFakeVMHost(t)
		tasks := newDriver(t, f).Tasks()

		held := ours(f, "u-1", "nginx-xkfqz")
		at := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

		f.say(held.ID, vm.LogLine{Seq: 1, Stream: guest.StreamStdout, At: at, Content: "before"})
		f.say(held.ID, vm.LogLine{Seq: 2, Stream: guest.StreamStderr, At: at.Add(time.Second), Content: "since"})

		ctx, cancel := context.WithCancel(t.Context())

		lines := make(chan task.LogLine, 4)
		done := make(chan error, 1)

		go func() {
			done <- tasks.StreamLogs(ctx, held.ID, at.Add(time.Second), func(line task.LogLine) error {
				lines <- line

				return nil
			})
		}()

		assert.Equal(t, task.LogLine{Stream: task.StreamStderr, Content: "since", At: at.Add(time.Second)}, <-lines)

		require.Eventually(t, func() bool { return f.following(held.ID) }, 5*time.Second, 10*time.Millisecond)
		f.say(held.ID, vm.LogLine{Seq: 3, Stream: guest.StreamStdout, At: at.Add(2 * time.Second), Content: "live"})

		assert.Equal(t, task.LogLine{Stream: task.StreamStdout, Content: "live", At: at.Add(2 * time.Second)}, <-lines)

		cancel()

		assert.NoError(t, <-done, "a caller that stops listening is not a failure")
	})

	t.Run("a follow vmhost ends, as it does once a vm's task has ended, ends", func(t *testing.T) {
		f := newFakeVMHost(t)

		held := ours(f, "u-1", "nginx-xkfqz")

		done := make(chan error, 1)

		go func() {
			done <- newDriver(t, f).Tasks().StreamLogs(t.Context(), held.ID, time.Time{}, func(task.LogLine) error { return nil })
		}()

		require.Eventually(t, func() bool { return f.following(held.ID) }, 5*time.Second, 10*time.Millisecond)
		f.quiet(held.ID)

		assert.NoError(t, <-done)
	})

	t.Run("a line the follower refuses ends the follow with its reason", func(t *testing.T) {
		f := newFakeVMHost(t)

		held := ours(f, "u-1", "nginx-xkfqz")
		f.say(held.ID, vm.LogLine{Seq: 1, Content: "one"})

		refused := errors.New("the batch is full")

		err := newDriver(t, f).Tasks().StreamLogs(t.Context(), held.ID, time.Time{}, func(task.LogLine) error { return refused })

		assert.ErrorIs(t, err, refused)
	})

	t.Run("a command runs in a vm as a session: input in, output out, a terminal resized, and ended once left", func(t *testing.T) {
		f := newFakeVMHost(t)

		held := ours(f, "u-1", "nginx-xkfqz")

		session, err := newDriver(t, f).Tasks().Exec(t.Context(), held.ID, task.ExecOptions{
			Command: []string{"/bin/sh"},
			TTY:     true,
			Env:     []string{"TERM=xterm-256color"},
			WorkDir: "/srv",
		})
		require.NoError(t, err)
		defer session.Close()

		assert.Equal(t, []guest.Exec{{
			Process: guest.Process{Args: []string{"/bin/sh"}, Env: []string{"TERM=xterm-256color"}, WorkingDir: "/srv"},
			TTY:     true,
		}}, f.execs)

		read := func() string {
			buffer := make([]byte, 64)

			n, err := session.Read(buffer)
			require.NoError(t, err)

			return string(buffer[:n])
		}

		_, err = session.Write([]byte("ls\n"))
		require.NoError(t, err)
		assert.Equal(t, "ls\n", read())

		require.NoError(t, session.Resize(t.Context(), 24, 80))
		assert.Equal(t, "24x80", read())

		_, err = session.Write([]byte("exit\n"))
		require.NoError(t, err)

		_, err = session.Read(make([]byte, 8))
		assert.ErrorIs(t, err, io.EOF, "a command that ended has nothing more to say")

		require.NoError(t, session.Close())
		require.NoError(t, session.Close(), "closing twice is closing once")

		require.NoError(t, session.End(t.Context()))
		assert.Equal(t, guest.EndExec{Grace: terminationGrace, KillGrace: killGrace}, f.ends[held.ID+"/e1"])
	})

	t.Run("closing a session releases whoever is reading it", func(t *testing.T) {
		f := newFakeVMHost(t)

		held := ours(f, "u-1", "nginx-xkfqz")

		session, err := newDriver(t, f).Tasks().Exec(t.Context(), held.ID, task.ExecOptions{Command: []string{"sh"}})
		require.NoError(t, err)

		reading := make(chan error, 1)

		go func() {
			_, err := session.Read(make([]byte, 8))
			reading <- err
		}()

		require.NoError(t, session.Close())

		select {
		case err := <-reading:
			assert.Error(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("a reader parked in Read was not released")
		}
	})

	t.Run("a vm's port is dialled through vmhost", func(t *testing.T) {
		f := newFakeVMHost(t)

		upstream, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer upstream.Close()

		go func() {
			conn, err := upstream.Accept()
			if err != nil {
				return
			}
			defer conn.Close()

			_, _ = io.WriteString(conn, "HTTP/1.1 204 No Content\r\n\r\n")
		}()

		f.dialTo = upstream.Addr().String()
		held := ours(f, "u-1", "nginx-xkfqz")

		dialer, ok := newDriver(t, f).Tasks().(task.Dialer)
		require.True(t, ok, "the microvm driver reaches its runs itself")

		conn, err := dialer.DialContext(t.Context(), held.ID, 80)
		require.NoError(t, err)
		defer conn.Close()

		answer, err := io.ReadAll(conn)
		require.NoError(t, err)
		assert.Equal(t, "HTTP/1.1 204 No Content\r\n\r\n", string(answer))

		assert.Equal(t, []string{"POST /v1/vms/" + held.ID + "/dial?port=80"}, asked(f))
	})

	t.Run("something that is not a port is not dialled at all", func(t *testing.T) {
		f := newFakeVMHost(t)

		held := ours(f, "u-1", "nginx-xkfqz")
		dialer := newDriver(t, f).Tasks().(task.Dialer)

		for _, p := range []port.Port{0, 70000} {
			_, err := dialer.DialContext(t.Context(), held.ID, p)
			assert.ErrorIs(t, err, vm.ErrInvalid, p)
		}

		assert.Empty(t, asked(f))
	})

	t.Run("a vm that is not running cannot be dialled", func(t *testing.T) {
		f := newFakeVMHost(t)

		stopped := f.hold(vm.VM{State: vm.StateExited, Spec: vm.Spec{Labels: map[string]string{"node.name": orchestrator}}})

		_, err := newDriver(t, f).Tasks().(task.Dialer).DialContext(t.Context(), stopped.ID, 80)

		assert.ErrorIs(t, err, vm.ErrNotRunning)
	})
}

func TestNetworks(t *testing.T) {
	t.Run("the shared isolated network, and a stack's own, are made without a way out", func(t *testing.T) {
		f := newFakeVMHost(t)
		networks := newDriver(t, f).Networks()

		require.NoError(t, networks.EnsureIsolatedNetwork(t.Context()))
		require.NoError(t, networks.EnsureStackNetwork(t.Context(), "shop-xkfqz"))

		assert.False(t, f.networks[network.IsolatedNetworkName].Masquerade)
		assert.False(t, f.networks["workload-stack-shop-xkfqz"].Masquerade)

		assert.Equal(t, []string{
			"PUT /v1/networks/workload-isolated",
			"PUT /v1/networks/workload-stack-shop-xkfqz",
		}, asked(f))
	})

	t.Run("a stack with a long name has a network all the same, made and removed under one name", func(t *testing.T) {
		f := newFakeVMHost(t)
		networks := newDriver(t, f).Networks()

		slug := strings.Repeat("a", 57) + "-xkfqz"

		require.NoError(t, networks.EnsureStackNetwork(t.Context(), slug))
		require.Contains(t, f.networks, networkName(network.StackNetworkName(slug)))

		require.NoError(t, networks.RemoveStackNetwork(t.Context(), slug))
		assert.NotContains(t, f.networks, networkName(network.StackNetworkName(slug)))
	})

	t.Run("a stack's network still holding vms is waited for", func(t *testing.T) {
		f := newFakeVMHost(t)
		d := newDriver(t, f)
		d.networks.detachInterval = time.Millisecond

		require.NoError(t, d.Networks().EnsureStackNetwork(t.Context(), "shop-xkfqz"))
		f.inUse["workload-stack-shop-xkfqz"] = 2

		require.NoError(t, d.Networks().RemoveStackNetwork(t.Context(), "shop-xkfqz"))

		assert.NotContains(t, f.networks, "workload-stack-shop-xkfqz")
		assert.Equal(t, 3, strings.Count(strings.Join(asked(f), "\n"), "DELETE /v1/networks/workload-stack-shop-xkfqz"))
	})

	t.Run("a stack's network that never comes free is left, and said so", func(t *testing.T) {
		f := newFakeVMHost(t)
		d := newDriver(t, f)
		d.networks.detachInterval = time.Millisecond
		d.networks.detachTimeout = 20 * time.Millisecond

		require.NoError(t, d.Networks().EnsureStackNetwork(t.Context(), "shop-xkfqz"))
		f.refuseRoute(vm.RouteRemoveNetwork, vm.ErrNetworkInUse)

		err := d.Networks().RemoveStackNetwork(t.Context(), "shop-xkfqz")

		assert.ErrorIs(t, err, vm.ErrNetworkInUse)
	})

	t.Run("a stack's network that is not there is the outcome asked for", func(t *testing.T) {
		f := newFakeVMHost(t)

		assert.NoError(t, newDriver(t, f).Networks().RemoveStackNetwork(t.Context(), "shop-xkfqz"))
	})
}

func TestNode(t *testing.T) {
	t.Run("a node uses what its running vms use together", func(t *testing.T) {
		f := newFakeVMHost(t)

		first := ours(f, "u-1", "nginx-xkfqz")
		second := ours(f, "u-2", "redis-bcdef")
		elsewhere := theirs(f, "u-3", "postgres-cdefg")
		f.hold(vm.VM{State: vm.StateExited, Spec: vm.Spec{Labels: map[string]string{"node.name": orchestrator}}})

		f.stats[first.ID] = vm.Stats{PIDs: 2, CPUPercent: 10, MemoryUsage: 64 << 20, MemoryLimit: 256 << 20}
		f.stats[second.ID] = vm.Stats{PIDs: 3, CPUPercent: 30, MemoryUsage: 192 << 20, MemoryLimit: 256 << 20, NetworkOutput: 5}
		f.stats[elsewhere.ID] = vm.Stats{PIDs: 100}

		used, err := newDriver(t, f).Node().Stats(t.Context(), orchestrator)

		require.NoError(t, err)
		assert.Equal(t, node.Stats{
			PIDs:          5,
			CPUPercent:    40,
			MemoryUsage:   256 << 20,
			MemoryLimit:   512 << 20,
			MemoryPercent: 50,
			NetworkOutput: 5,
		}, used)
	})

	t.Run("another node's vms are not counted at all", func(t *testing.T) {
		f := newFakeVMHost(t)

		theirs(f, "u-1", "nginx-xkfqz")

		used, err := newDriver(t, f).Node().Stats(t.Context(), "workload-orchestrator-02")

		require.NoError(t, err)
		assert.Equal(t, node.Stats{}, used)
		assert.Empty(t, asked(f))
	})

	t.Run("a vm that stopped, or went, since it was listed is not counted, and the rest still are", func(t *testing.T) {
		f := newFakeVMHost(t)

		stopped := ours(f, "u-1", "nginx-xkfqz")
		gone := ours(f, "u-2", "redis-bcdef")
		counted := ours(f, "u-3", "postgres-cdefg")

		// listed running, and then not, by the time they are asked about.
		f.statsRefused[stopped.ID] = vm.ErrNotRunning
		f.statsRefused[gone.ID] = vm.ErrNotFound
		f.stats[counted.ID] = vm.Stats{PIDs: 7}

		used, err := newDriver(t, f).Node().Stats(t.Context(), orchestrator)

		require.NoError(t, err)
		assert.Equal(t, uint64(7), used.PIDs)
	})

	t.Run("a vmhost failing to say what one vm uses fails the node's stats", func(t *testing.T) {
		f := newFakeVMHost(t)

		broken := ours(f, "u-1", "nginx-xkfqz")
		f.statsRefused[broken.ID] = errors.New("the agent did not answer")

		_, err := newDriver(t, f).Node().Stats(t.Context(), orchestrator)

		assert.ErrorContains(t, err, "the agent did not answer")
	})
}
