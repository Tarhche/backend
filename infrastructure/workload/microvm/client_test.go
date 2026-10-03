package microvm

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// client is a client of a fake vmhost.
func client(t *testing.T, f *fakeVMHost) *Client {
	t.Helper()

	c, err := NewClient(f.endpoint)
	require.NoError(t, err)
	t.Cleanup(c.Close)

	return c
}

// running puts a running VM in vmhost.
func running(f *fakeVMHost, labels map[string]string, ports ...uint16) vm.VM {
	return f.hold(vm.VM{
		State:     vm.StateRunning,
		StartedAt: time.Now().UTC(),
		Spec: vm.Spec{
			Name:         "nginx-xkfqz",
			Image:        "nginx:alpine",
			Labels:       labels,
			Networks:     []vm.Attachment{{Network: "workload-isolated"}},
			ExposedPorts: ports,
		},
		Interfaces: []vm.Interface{{Network: "workload-isolated", Address: "10.250.1.2/24"}},
	})
}

func TestNewClient(t *testing.T) {
	t.Parallel()

	for _, endpoint := range []string{vm.DefaultEndpoint, "unix:///tmp/vmhost.sock"} {
		_, err := NewClient(endpoint)
		assert.NoError(t, err, endpoint)
	}

	for _, endpoint := range []string{"", "tcp://vmhost:2375", "unix://", "unix://run/vmhost.sock", "/run/vmhost.sock", "%zz"} {
		_, err := NewClient(endpoint)
		assert.Error(t, err, endpoint)
	}
}

func TestClient(t *testing.T) {
	t.Run("info is what vmhost says about itself", func(t *testing.T) {
		f := newFakeVMHost(t)

		info, err := client(t, f).Info(t.Context())

		require.NoError(t, err)
		assert.Equal(t, f.info, info)
		assert.Equal(t, []string{"GET /v1/info"}, f.requests())
	})

	t.Run("an image is asked for by its reference and said back as a disk", func(t *testing.T) {
		f := newFakeVMHost(t)

		prepared, err := client(t, f).PrepareImage(t.Context(), "nginx:alpine")

		require.NoError(t, err)
		assert.Equal(t, "nginx:alpine", prepared.Reference)
		assert.True(t, strings.HasPrefix(prepared.Digest, "sha256:"))
		assert.Equal(t, []string{"POST /v1/images/prepare"}, f.requests())
	})

	t.Run("images are listed, and one is let go of by its digest", func(t *testing.T) {
		f := newFakeVMHost(t)
		c := client(t, f)

		prepared, err := c.PrepareImage(t.Context(), "alpine:3")
		require.NoError(t, err)

		images, err := c.Images(t.Context())
		require.NoError(t, err)
		assert.Equal(t, []vm.Image{prepared}, images)

		require.NoError(t, c.DeleteImage(t.Context(), prepared.Digest))
		assert.ErrorIs(t, c.DeleteImage(t.Context(), prepared.Digest), vm.ErrNotFound)

		assert.Contains(t, f.requests(), "DELETE /v1/images/"+prepared.Digest)
	})

	t.Run("a network is asked for with whether it routes out, and said back", func(t *testing.T) {
		f := newFakeVMHost(t)
		c := client(t, f)

		made, err := c.EnsureNetwork(t.Context(), "workload-isolated", false)
		require.NoError(t, err)
		assert.Equal(t, "workload-isolated", made.Name)
		assert.False(t, made.Masquerade)

		_, err = c.EnsureNetwork(t.Context(), "workload-isolated", true)
		assert.ErrorIs(t, err, vm.ErrConflict, "a network routes out or not from when it is made")

		require.NoError(t, c.RemoveNetwork(t.Context(), "workload-isolated"))
		assert.ErrorIs(t, c.RemoveNetwork(t.Context(), "workload-isolated"), vm.ErrNotFound)

		assert.Equal(t, []string{
			"PUT /v1/networks/workload-isolated",
			"PUT /v1/networks/workload-isolated",
			"DELETE /v1/networks/workload-isolated",
			"DELETE /v1/networks/workload-isolated",
		}, f.requests())
	})

	t.Run("a network that still holds vms says so", func(t *testing.T) {
		f := newFakeVMHost(t)
		f.inUse["workload-stack-shop"] = 1

		err := client(t, f).RemoveNetwork(t.Context(), "workload-stack-shop")

		assert.ErrorIs(t, err, vm.ErrNetworkInUse)
	})

	t.Run("a vm is made from a spec, and called what vmhost calls it", func(t *testing.T) {
		f := newFakeVMHost(t)
		c := client(t, f)

		spec := vm.Spec{Name: "nginx-xkfqz", Image: "nginx:alpine", Labels: map[string]string{"node.name": "workload-orchestrator-01"}}

		id, err := c.Create(t.Context(), spec)
		require.NoError(t, err)
		assert.True(t, vm.IsID(id))

		made, err := c.VM(t.Context(), id)
		require.NoError(t, err)
		assert.Equal(t, spec, made.Spec)
		assert.Equal(t, vm.StateCreated, made.State)
	})

	t.Run("a vm vmhost has no room for is refused as such, after a 409", func(t *testing.T) {
		f := newFakeVMHost(t)
		f.room = 256 << 20

		_, err := client(t, f).Create(t.Context(), vm.Spec{Name: "big", Image: "alpine", Resources: vm.Resources{Memory: 1 << 30}})

		assert.ErrorIs(t, err, vm.ErrCapacity)
		assert.ErrorContains(t, err, "1073741824 bytes asked for")
	})

	t.Run("a name another vm answers to is a conflict", func(t *testing.T) {
		f := newFakeVMHost(t)
		c := client(t, f)

		_, err := c.Create(t.Context(), vm.Spec{Name: "nginx-xkfqz", Image: "nginx"})
		require.NoError(t, err)

		_, err = c.Create(t.Context(), vm.Spec{Name: "nginx-xkfqz", Image: "nginx"})
		assert.ErrorIs(t, err, vm.ErrConflict)
	})

	t.Run("vms are listed by every label filter given", func(t *testing.T) {
		f := newFakeVMHost(t)

		ours := running(f, map[string]string{"node.name": "workload-orchestrator-01", "task.uuid": "u-1"})
		running(f, map[string]string{"node.name": "workload-orchestrator-02", "task.uuid": "u-1"})
		running(f, map[string]string{"node.name": "workload-orchestrator-01", "task.uuid": "u-2"})

		listed, err := client(t, f).VMs(t.Context(), "node.name=workload-orchestrator-01", "task.uuid=u-1")

		require.NoError(t, err)
		assert.Equal(t, []vm.VM{ours}, listed)
		assert.Equal(t, []string{"GET /v1/vms?label=node.name%3Dworkload-orchestrator-01&label=task.uuid%3Du-1"}, f.requests())
	})

	t.Run("a listing is read whether it comes as a list or wrapped in an object", func(t *testing.T) {
		f := newFakeVMHost(t)
		f.wrapped = true

		held := running(f, map[string]string{"node.name": "workload-orchestrator-01"})

		listed, err := client(t, f).VMs(t.Context())

		require.NoError(t, err)
		assert.Equal(t, []vm.VM{held}, listed)
	})

	t.Run("a vm that is not there is not there in the domain's words too", func(t *testing.T) {
		f := newFakeVMHost(t)

		_, err := client(t, f).VM(t.Context(), "0123456789abcdef")

		assert.ErrorIs(t, err, vm.ErrNotFound)
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("a vm's life is asked for on its routes", func(t *testing.T) {
		f := newFakeVMHost(t)
		c := client(t, f)

		id, err := c.Create(t.Context(), vm.Spec{Name: "nginx-xkfqz", Image: "nginx"})
		require.NoError(t, err)

		require.NoError(t, c.Start(t.Context(), id))
		require.NoError(t, c.Stop(t.Context(), id, 10*time.Second))
		require.NoError(t, c.Restart(t.Context(), id))
		require.NoError(t, c.Kill(t.Context(), id))
		require.NoError(t, c.Delete(t.Context(), id))

		assert.Equal(t, []string{
			"POST /v1/vms",
			"POST /v1/vms/" + id + "/start",
			"POST /v1/vms/" + id + "/stop?timeout=10s",
			"POST /v1/vms/" + id + "/restart",
			"POST /v1/vms/" + id + "/kill",
			"DELETE /v1/vms/" + id,
		}, f.requests())

		assert.ErrorIs(t, c.Delete(t.Context(), id), vm.ErrNotFound)
	})

	t.Run("an id that cannot be a vm's is refused without asking", func(t *testing.T) {
		f := newFakeVMHost(t)
		c := client(t, f)

		for _, id := range []string{"", "../../v1/info", "firecracker:0123456789abcdef", "0123456789ABCDEF"} {
			assert.ErrorIs(t, c.Start(t.Context(), id), vm.ErrNotFound, id)
			assert.ErrorIs(t, c.Delete(t.Context(), id), vm.ErrNotFound, id)
		}

		assert.Empty(t, f.requests())
	})

	t.Run("what a vm used is said back", func(t *testing.T) {
		f := newFakeVMHost(t)

		held := running(f, nil)
		f.stats[held.ID] = vm.Stats{PIDs: 3, CPUPercent: 12.5, MemoryUsage: 64 << 20, MemoryLimit: 256 << 20}

		stats, err := client(t, f).Stats(t.Context(), held.ID)

		require.NoError(t, err)
		assert.Equal(t, f.stats[held.ID], stats)
	})

	t.Run("a vm's output is read line by line, from where the reader left off", func(t *testing.T) {
		f := newFakeVMHost(t)

		held := running(f, nil)
		at := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

		for i, content := range []string{"one", "two", "three"} {
			f.say(held.ID, vm.LogLine{Seq: uint64(i + 1), Stream: guest.StreamStdout, At: at.Add(time.Duration(i) * time.Second), Content: content})
		}

		c := client(t, f)

		var read []string
		collect := func(line vm.LogLine) error {
			read = append(read, line.Content)

			return nil
		}

		require.NoError(t, c.Logs(t.Context(), held.ID, 1, time.Time{}, false, collect))
		assert.Equal(t, []string{"two", "three"}, read, "after the line numbered 1")

		read = nil

		require.NoError(t, c.Logs(t.Context(), held.ID, 0, at.Add(2*time.Second), false, collect))
		assert.Equal(t, []string{"three"}, read, "from the time asked for")

		assert.Equal(t, []string{
			"GET /v1/vms/" + held.ID + "/logs?after=1",
			"GET /v1/vms/" + held.ID + "/logs?since=2026-10-02T12%3A00%3A02Z",
		}, f.requests())
	})

	t.Run("following a vm's output goes on until it is stopped", func(t *testing.T) {
		f := newFakeVMHost(t)

		held := running(f, nil)
		f.say(held.ID, vm.LogLine{Seq: 1, Stream: guest.StreamStdout, At: time.Now(), Content: "before"})

		ctx, cancel := context.WithCancel(t.Context())

		lines := make(chan string, 4)
		done := make(chan error, 1)

		go func() {
			done <- client(t, f).Logs(ctx, held.ID, 0, time.Time{}, true, func(line vm.LogLine) error {
				lines <- line.Content

				return nil
			})
		}()

		assert.Equal(t, "before", <-lines)

		require.Eventually(t, func() bool { return f.following(held.ID) }, 5*time.Second, 10*time.Millisecond)
		f.say(held.ID, vm.LogLine{Seq: 2, Stream: guest.StreamStderr, At: time.Now(), Content: "after"})

		assert.Equal(t, "after", <-lines)

		cancel()

		assert.ErrorIs(t, <-done, context.Canceled)
		assert.Contains(t, f.requests(), "GET /v1/vms/"+held.ID+"/logs?follow=1")
	})

	t.Run("a line the reader refuses ends the reading with its reason", func(t *testing.T) {
		f := newFakeVMHost(t)

		held := running(f, nil)
		f.say(held.ID, vm.LogLine{Seq: 1, Content: "one"})
		f.say(held.ID, vm.LogLine{Seq: 2, Content: "two"})

		refused := errors.New("no more")

		var read int
		err := client(t, f).Logs(t.Context(), held.ID, 0, time.Time{}, false, func(vm.LogLine) error {
			read++

			return refused
		})

		assert.ErrorIs(t, err, refused)
		assert.Equal(t, 1, read)
	})

	t.Run("a command's input and output travel as frames, under the name vmhost gave it", func(t *testing.T) {
		f := newFakeVMHost(t)
		c := client(t, f)

		held := running(f, nil)

		exec := guest.Exec{Process: guest.Process{Args: []string{"/bin/sh"}, Env: []string{"TERM=xterm"}}, TTY: true}

		execID, conn, err := c.Exec(t.Context(), held.ID, exec)
		require.NoError(t, err)
		defer conn.Close()

		assert.Equal(t, "e1", execID)
		assert.Equal(t, []guest.Exec{exec}, f.execs)

		require.NoError(t, guest.WriteFrame(conn, guest.FrameStdin, []byte("hello")))

		frame, err := guest.ReadFrame(conn)
		require.NoError(t, err)
		assert.Equal(t, guest.Frame{Type: guest.FrameStdout, Payload: []byte("hello")}, frame)

		ended, err := c.EndExec(t.Context(), held.ID, execID, guest.EndExec{Grace: time.Second, KillGrace: 2 * time.Second})
		require.NoError(t, err)
		assert.True(t, ended.Signalled)
		assert.Equal(t, guest.EndExec{Grace: time.Second, KillGrace: 2 * time.Second}, f.ends[held.ID+"/e1"])
	})

	t.Run("a command in a vm that is not running is refused as such", func(t *testing.T) {
		f := newFakeVMHost(t)

		held := f.hold(vm.VM{State: vm.StateExited})

		_, _, err := client(t, f).Exec(t.Context(), held.ID, guest.Exec{Process: guest.Process{Args: []string{"sh"}}})

		assert.ErrorIs(t, err, vm.ErrNotRunning)
	})

	t.Run("a connection to a vm's port carries raw bytes both ways", func(t *testing.T) {
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

			said, _ := io.ReadAll(conn)
			_, _ = conn.Write(append([]byte("you said: "), said...))
		}()

		f.dialTo = upstream.Addr().String()
		held := running(f, nil, 80)

		conn, err := client(t, f).Dial(t.Context(), held.ID, 80)
		require.NoError(t, err)
		defer conn.Close()

		_, err = conn.Write([]byte("ping"))
		require.NoError(t, err)

		// the half-close is how the far end learns there is nothing more.
		require.NoError(t, conn.(interface{ CloseWrite() error }).CloseWrite())

		answered, err := io.ReadAll(conn)
		require.NoError(t, err)
		assert.Equal(t, "you said: ping", string(answered))

		assert.Contains(t, f.requests(), "POST /v1/vms/"+held.ID+"/dial?port=80")
	})

	t.Run("a vmhost that is not there cannot run anything", func(t *testing.T) {
		c, err := NewClient("unix://" + filepath.Join(os.TempDir(), "no-vmhost-here.sock"))
		require.NoError(t, err)

		_, err = c.Info(t.Context())
		assert.ErrorIs(t, err, vm.ErrUnavailable)

		_, err = c.Dial(t.Context(), "0123456789abcdef", 80)
		assert.ErrorIs(t, err, vm.ErrUnavailable)

		err = c.Logs(t.Context(), "0123456789abcdef", 0, time.Time{}, true, func(vm.LogLine) error { return nil })
		assert.ErrorIs(t, err, vm.ErrUnavailable)
	})

	t.Run("an answer that is none of vmhost's errors says what it was", func(t *testing.T) {
		c := rawVMHost(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "404 page not found", http.StatusNotFound)
		})

		_, err := c.VM(t.Context(), "0123456789abcdef")

		assert.ErrorContains(t, err, "404 Not Found: 404 page not found")
		assert.NotErrorIs(t, err, vm.ErrNotFound, "a route that is not there is not a vm that is not there")
	})

	t.Run("an answer larger than any vmhost gives is refused", func(t *testing.T) {
		c := rawVMHost(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"version":"`+strings.Repeat("x", maxAnswer)+`"}`)
		})

		_, err := c.Info(t.Context())

		assert.ErrorContains(t, err, "more than")
	})

	t.Run("a vm called something that cannot name one is not taken", func(t *testing.T) {
		c := rawVMHost(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"../info"}`)
		})

		_, err := c.Create(t.Context(), vm.Spec{Name: "a", Image: "b"})

		assert.ErrorContains(t, err, "cannot name one")
	})

	t.Run("a stream vmhost will not switch to is not taken", func(t *testing.T) {
		c := rawVMHost(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		_, err := c.Dial(t.Context(), "0123456789abcdef", 80)

		assert.ErrorContains(t, err, "rather than switching to "+vm.UpgradeDial)
	})

	t.Run("a command vmhost does not name is not taken, since it could never be ended", func(t *testing.T) {
		// a vmhost that switches without naming the command
		c := rawVMHost(t, func(w http.ResponseWriter, r *http.Request) {
			conn, buffered, err := http.NewResponseController(w).Hijack()
			if err != nil {
				return
			}
			defer conn.Close()

			_, _ = buffered.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: " + vm.UpgradeExec + "\r\n\r\n")
			_ = buffered.Flush()
		})

		_, _, err := c.Exec(t.Context(), "0123456789abcdef", guest.Exec{Process: guest.Process{Args: []string{"sh"}}})

		assert.ErrorContains(t, err, "without saying what it calls it")
	})
}

// rawVMHost is a vmhost that answers everything with handler, for answers the
// contract does not have.
func rawVMHost(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	dir, err := os.MkdirTemp("", "vmhost")
	require.NoError(t, err)

	socket := filepath.Join(dir, "vmhost.sock")

	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)

	server := httptest.NewUnstartedServer(handler)
	_ = server.Listener.Close()
	server.Listener = listener
	server.Start()

	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()
		_ = os.RemoveAll(dir)
	})

	c, err := NewClient("unix://" + socket)
	require.NoError(t, err)
	t.Cleanup(c.Close)

	return c
}
