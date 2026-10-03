package vmhost

import (
	"bufio"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	vmMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/vm"
)

func TestEngine_Logs(t *testing.T) {
	t.Parallel()

	collect := func(t *testing.T, w *world, id string, after uint64, since time.Time) []string {
		t.Helper()

		var lines []string
		require.NoError(t, w.engine.Logs(w.ctx, id, after, since, false, func(line vm.LogLine) error {
			lines = append(lines, line.Content)

			return nil
		}))

		return lines
	}

	t.Run("lines after a number, or from a time, are read without what came before", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		id := w.run(spec("service"))
		agent := w.agent(id)

		agent.Write(guest.StreamStdout, "one")
		agent.Write(guest.StreamStdout, "two")
		require.Eventually(t, func() bool { return len(w.output(id)) == 2 }, 5*time.Second, 5*time.Millisecond)

		middle := time.Now().UTC()
		time.Sleep(5 * time.Millisecond)

		agent.Write(guest.StreamStderr, "three")
		require.Eventually(t, func() bool { return len(w.output(id)) == 3 }, 5*time.Second, 5*time.Millisecond)

		assert.Equal(t, []string{"two", "three"}, collect(t, w, id, 1, time.Time{}))
		assert.Equal(t, []string{"three"}, collect(t, w, id, 0, middle))
		assert.Empty(t, collect(t, w, id, 3, time.Time{}))
	})

	t.Run("following hands what comes as it comes, and ends with the task", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		id := w.run(spec("service"))
		agent := w.agent(id)

		followed := make(chan string, 16)
		finished := make(chan error, 1)

		go func() {
			finished <- w.engine.Logs(w.ctx, id, 0, time.Time{}, true, func(line vm.LogLine) error {
				followed <- line.Content

				return nil
			})
		}()

		agent.Write(guest.StreamStdout, "first")
		assert.Equal(t, "first", receive(t, followed))

		agent.Write(guest.StreamStdout, "second")
		assert.Equal(t, "second", receive(t, followed))

		agent.Write(guest.StreamStdout, "last")
		agent.Exit(0)

		assert.Equal(t, "last", receive(t, followed))

		select {
		case err := <-finished:
			assert.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("following never ended")
		}
	})

	t.Run("following ends when the reader goes, or refuses a line", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		id := w.run(spec("service"))
		w.agent(id).Write(guest.StreamStdout, "line")

		refused := errors.New("the reader went away")

		err := w.engine.Logs(w.ctx, id, 0, time.Time{}, true, func(vm.LogLine) error { return refused })
		assert.ErrorIs(t, err, refused)

		ctx, cancel := context.WithTimeout(w.ctx, 50*time.Millisecond)
		defer cancel()

		assert.NoError(t, w.engine.Logs(ctx, id, 1, time.Time{}, true, func(vm.LogLine) error { return nil }))
	})

	t.Run("a vm that is not there has no output", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		err := w.engine.Logs(w.ctx, "0123456789abcdef", 0, time.Time{}, false, func(vm.LogLine) error { return nil })
		assert.ErrorIs(t, err, vm.ErrNotFound)
	})
}

func receive(t *testing.T, lines <-chan string) string {
	t.Helper()

	select {
	case line := <-lines:
		return line
	case <-time.After(5 * time.Second):
		t.Fatal("nothing came")

		return ""
	}
}

func TestEngine_Exec(t *testing.T) {
	t.Parallel()

	t.Run("a command runs beside the task, its input and output travelling as frames", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		id := w.run(spec("service"))

		execID, conn, err := w.engine.Exec(w.ctx, id, guest.Exec{Process: guest.Process{Args: []string{"/bin/sh"}}, TTY: true})
		require.NoError(t, err)
		defer conn.Close()

		assert.NotEmpty(t, execID)

		require.NoError(t, guest.WriteFrame(conn, guest.FrameStdin, []byte("echo 42\n")))

		frame, err := guest.ReadFrame(conn)
		require.NoError(t, err)
		assert.Equal(t, guest.FrameStdout, frame.Type)
		assert.Equal(t, "echo 42\n", string(frame.Payload))

		require.NoError(t, guest.WriteFrame(conn, guest.FrameResize, guest.ResizePayload(24, 80)))

		frame, err = guest.ReadFrame(conn)
		require.NoError(t, err)
		assert.Equal(t, "24 80", string(frame.Payload))

		ended, err := w.engine.EndExec(w.ctx, id, execID, guest.EndExec{Grace: time.Second})
		require.NoError(t, err)
		assert.True(t, ended.Signalled)
	})

	t.Run("a vm that does not run runs no command, and has none left to end", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		id := w.create(spec("created"))

		_, _, err := w.engine.Exec(w.ctx, id, guest.Exec{Process: guest.Process{Args: []string{"/bin/sh"}}})
		assert.ErrorIs(t, err, vm.ErrNotRunning)

		ended, err := w.engine.EndExec(w.ctx, id, "exec-1", guest.EndExec{})
		require.NoError(t, err)
		assert.False(t, ended.Signalled)

		_, _, err = w.engine.Exec(w.ctx, "0123456789abcdef", guest.Exec{})
		assert.ErrorIs(t, err, vm.ErrNotFound)
	})

	t.Run("an agent's refusal for a task that does not run is the vm's", func(t *testing.T) {
		t.Parallel()

		assert.ErrorIs(t, guestError(guest.ErrNotRunning), vm.ErrNotRunning)
		assert.ErrorIs(t, guestError(guest.ErrNotRunning), guest.ErrNotRunning)

		other := errors.New("something else")
		assert.Equal(t, other, guestError(other))
	})
}

func TestEngine_Dial(t *testing.T) {
	t.Parallel()

	t.Run("a port the task is reached on is dialled through its agent, byte for byte", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		id := w.run(spec("web"))

		conn, err := w.engine.Dial(w.ctx, id, 80)
		require.NoError(t, err)
		defer conn.Close()

		_, err = conn.Write([]byte("GET / HTTP/1.1\r\n"))
		require.NoError(t, err)

		echoed, err := bufio.NewReader(conn).ReadString('\n')
		require.NoError(t, err)
		assert.Equal(t, "GET / HTTP/1.1\r\n", echoed)
	})

	t.Run("a port the task is not reached on is refused", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		id := w.run(spec("web"))

		_, err := w.engine.Dial(w.ctx, id, 8080)
		assert.ErrorIs(t, err, vm.ErrInvalid)

		offline := spec("offline")
		offline.Networks = nil

		_, err = w.engine.Dial(w.ctx, w.run(offline), 80)
		assert.ErrorIs(t, err, vm.ErrInvalid, "a vm on no network is reached on no port")
	})

	t.Run("a vm that does not run is not dialled", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		id := w.run(spec("web"))
		require.NoError(t, w.engine.Stop(w.ctx, id, time.Second))

		_, err := w.engine.Dial(w.ctx, id, 80)
		assert.ErrorIs(t, err, vm.ErrNotRunning)
	})
}

func TestEngine_Stats(t *testing.T) {
	t.Parallel()

	t.Run("memory is the guest's, and the rest is the guest's where the host counts nothing", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		id := w.run(spec("web"))
		w.agent(id).SetStats(guest.Stats{PIDs: 3, CPUPercent: 12.5, MemoryUsage: 64 << 20, MemoryLimit: 250 << 20, NetworkInput: 10, BlockOutput: 20})

		stats, err := w.engine.Stats(w.ctx, id)
		require.NoError(t, err)

		assert.Equal(t, vm.Stats{
			PIDs:         3,
			CPUPercent:   12.5,
			MemoryUsage:  64 << 20,
			MemoryLimit:  256 << 20,
			NetworkInput: 10,
			BlockOutput:  20,
		}, stats)
	})

	t.Run("where the host counts, cpu is measured between two askings, and network and disk are the host's", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		var usage vmMock.MockUsageReader
		w.engine.usage = &usage

		id := w.run(spec("web"))
		w.agent(id).SetStats(guest.Stats{PIDs: 3, CPUPercent: 99, MemoryUsage: 64 << 20})

		usage.On("Usage", mock.Anything, mock.MatchedBy(func(machine vm.Machine) bool { return machine.ID == id }), []string{w.vm(id).Interfaces[0].Device}).
			Once().Return(vm.Usage{CPU: time.Second, NetworkInput: 100, NetworkOutput: 200, BlockInput: 300, BlockOutput: 400}, nil)

		first, err := w.engine.Stats(w.ctx, id)
		require.NoError(t, err)
		assert.Equal(t, 99.0, first.CPUPercent, "the first asking has nothing to measure against")
		assert.Equal(t, uint64(100), first.NetworkInput)
		assert.Equal(t, uint64(400), first.BlockOutput)

		usage.On("Usage", mock.Anything, mock.Anything, mock.Anything).
			Once().Return(vm.Usage{CPU: time.Second + 50*time.Millisecond, NetworkInput: 150}, nil)

		time.Sleep(100 * time.Millisecond)

		second, err := w.engine.Stats(w.ctx, id)
		require.NoError(t, err)
		assert.InDelta(t, 50, second.CPUPercent, 25, "50 ms of cpu in about 100 ms")
		assert.Equal(t, uint64(150), second.NetworkInput)
		assert.Equal(t, uint64(64<<20), second.MemoryUsage)
	})

	t.Run("a vm that does not run uses nothing", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		stats, err := w.engine.Stats(w.ctx, w.create(spec("web")))
		require.NoError(t, err)
		assert.Equal(t, vm.Stats{}, stats)
	})
}

func TestEngine_Hosts(t *testing.T) {
	t.Parallel()

	w := newWorld(t, nil)

	_, err := w.fabric.EnsureNetwork(w.ctx, network.StackNetworkName("shop"), false)
	require.NoError(t, err)

	service := func(name string) vm.Spec {
		s := spec("shop-" + name)
		s.Networks = []vm.Attachment{{Network: network.StackNetworkName("shop"), Aliases: []string{name}}}

		return s
	}

	api := w.run(service("api"))
	db := w.run(service("db"))

	address := func(id string) string {
		v := w.vm(id)
		require.NotEmpty(t, v.Interfaces)

		ip, _, _ := cutAddress(v.Interfaces[0].Address)

		return ip
	}

	// the database came second: it was told about the api when it booted,
	// and the api was told about it once it ran.
	assert.Equal(t, []guest.Host{{Address: address(api), Names: []string{"api", "shop-api"}}}, w.agent(db).Configured().Hosts)
	assert.Equal(t, []guest.Host{{Address: address(db), Names: []string{"db", "shop-db"}}}, w.agent(api).Hosts())

	// a neighbour that leaves is forgotten.
	require.NoError(t, w.engine.Stop(w.ctx, db, time.Second))
	assert.Empty(t, w.agent(api).Hosts())
}

func cutAddress(address string) (string, string, bool) {
	for i := range address {
		if address[i] == '/' {
			return address[:i], address[i+1:], true
		}
	}

	return address, "", false
}

func TestEngine_Images(t *testing.T) {
	t.Parallel()

	w := newWorld(t, nil)

	image, err := w.engine.PrepareImage(w.ctx, "nginx:alpine")
	require.NoError(t, err)
	assert.Equal(t, "nginx:alpine", image.Reference)

	id := w.create(spec("web"))
	booted := w.vm(id).ImageDigest

	assert.ErrorIs(t, w.engine.DeleteImage(w.ctx, booted), vm.ErrConflict, "an image a vm boots stays")
	assert.ErrorIs(t, w.engine.DeleteImage(w.ctx, "sha256:missing"), vm.ErrNotFound)
	require.NoError(t, w.engine.DeleteImage(w.ctx, image.Digest))

	images, err := w.engine.Images(w.ctx)
	require.NoError(t, err)
	require.Len(t, images, 1)
	assert.Equal(t, booted, images[0].Digest)
}

func TestEngine_Networks(t *testing.T) {
	t.Parallel()

	w := newWorld(t, nil)

	stack := network.StackNetworkName("shop")

	made, err := w.engine.EnsureNetwork(w.ctx, stack, false)
	require.NoError(t, err)
	assert.Equal(t, stack, made.Name)

	_, err = w.engine.EnsureNetwork(w.ctx, stack, true)
	assert.ErrorIs(t, err, vm.ErrConflict, "whether a network routes out is said when it is made")

	s := spec("shop-api")
	s.Networks = []vm.Attachment{{Network: stack}}
	id := w.run(s)

	assert.ErrorIs(t, w.engine.RemoveNetwork(w.ctx, stack), vm.ErrNetworkInUse)
	assert.ErrorIs(t, w.engine.RemoveNetwork(w.ctx, vm.PublicNetwork), vm.ErrConflict)

	require.NoError(t, w.engine.Stop(w.ctx, id, time.Second))
	require.NoError(t, w.engine.RemoveNetwork(w.ctx, stack))
}

// the reader of a VM's output never sees a line twice, nor misses one, when
// the VM's own writer is closed under it.
func TestEngine_LogsAfterDelete(t *testing.T) {
	t.Parallel()

	w := newWorld(t, nil)

	id := w.run(spec("service"))
	w.agent(id).Write(guest.StreamStdout, "line")
	require.Eventually(t, func() bool { return len(w.output(id)) == 1 }, 5*time.Second, 5*time.Millisecond)

	finished := make(chan error, 1)
	go func() {
		finished <- w.engine.Logs(w.ctx, id, 1, time.Time{}, true, func(vm.LogLine) error { return nil })
	}()

	time.Sleep(20 * time.Millisecond)
	require.NoError(t, w.engine.Delete(w.ctx, id))

	select {
	case err := <-finished:
		assert.True(t, err == nil || errors.Is(err, io.EOF), "%v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("following a deleted vm never ended")
	}
}
