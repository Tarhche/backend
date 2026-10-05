package vmhost_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/docker/docker/api"
	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	infraDocker "github.com/khanzadimahdi/testproject/infrastructure/workload/docker"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/vmhost"
)

// what the test VMs run.
var (
	// cat says back what it reads.
	cat = func(ctx context.Context, _ string, _ vm.ExecOptions, stdin io.Reader, stdout io.Writer, _ io.Writer) int {
		if _, err := io.Copy(stdout, stdin); err != nil {
			return 1
		}

		return 0
	}

	// shout reads all of its input, says it louder, complains on its errors
	// and exits with 3.
	shout = func(ctx context.Context, _ string, _ vm.ExecOptions, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
		said, err := io.ReadAll(stdin)
		if err != nil {
			return 1
		}

		_, _ = stdout.Write(bytes.ToUpper(said))
		_, _ = io.WriteString(stderr, "that was loud\n")

		return 3
	}

	// sleep runs until it is ended.
	sleep = func(ctx context.Context, _ string, _ vm.ExecOptions, _ io.Reader, _ io.Writer, _ io.Writer) int {
		<-ctx.Done()

		return 137
	}

	// exit ends at once, reading nothing.
	exit = func(context.Context, string, vm.ExecOptions, io.Reader, io.Writer, io.Writer) int {
		return 0
	}
)

// running is a vmhost holding a running VM named "vm-1", whose commands are
// these.
func running(t *testing.T, run commands) (*vmhost.Client, *memory.Engine, *sessions) {
	t.Helper()

	engine := newEngine(memory.WithExec(run.exec))
	recorded := &sessions{Engine: engine}
	client := serve(t, recorded).client

	_, err := engine.Create(t.Context(), machine("vm-1"))
	require.NoError(t, err)

	return client, engine, recorded
}

// output reads both of a session's streams to their ends, at once, as
// whoever runs a command has to.
func output(t *testing.T, session vm.ExecSession) (string, string) {
	t.Helper()

	var (
		stdout, stderr []byte
		read           sync.WaitGroup
	)

	read.Go(func() {
		var err error
		stdout, err = io.ReadAll(session.Stdout())
		assert.NoError(t, err)
	})

	read.Go(func() {
		var err error
		stderr, err = io.ReadAll(session.Stderr())
		assert.NoError(t, err)
	})

	read.Wait()

	return string(stdout), string(stderr)
}

func TestClient_Exec(t *testing.T) {
	t.Parallel()

	client, engine, recorded := running(t, commands{"shout": shout})

	session, err := client.Exec(t.Context(), "vm-1", vm.ExecOptions{
		Command:    []string{"shout", "--loudly"},
		Env:        []string{"LOUD=1"},
		WorkingDir: "/srv",
	})
	require.NoError(t, err)
	defer session.Close()

	_, err = io.WriteString(session.Stdin(), "hello, ")
	require.NoError(t, err)
	_, err = io.WriteString(session.Stdin(), "world")
	require.NoError(t, err)
	require.NoError(t, session.Stdin().Close(), "closing the input is the end of it")

	stdout, stderr := output(t, session)

	assert.Equal(t, "HELLO, WORLD", stdout)
	assert.Equal(t, "that was loud\n", stderr)

	exitCode, err := session.Wait(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 3, exitCode, "the exit code, exactly")

	assert.Equal(t, vm.ExecOptions{
		Command:    []string{"shout", "--loudly"},
		Env:        []string{"LOUD=1"},
		WorkingDir: "/srv",
	}, recorded.asked(t), "the command runs as it was asked to")

	_, err = session.Stdin().Write([]byte("more"))
	assert.ErrorIs(t, err, io.ErrClosedPipe, "an input that was closed stays closed")

	require.NoError(t, session.Close())
	eventually(t, "the vmhost lets go of the session", func() bool { return engine.Sessions("vm-1") == 0 })
}

func TestClient_Exec_TTY(t *testing.T) {
	t.Parallel()

	// a shell, as far as a terminal can tell: it says its prompt, says back
	// what it is given, and complains on its errors, which under a terminal
	// is where everything else goes.
	shell := func(ctx context.Context, _ string, options vm.ExecOptions, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
		if !options.TTY {
			return 2
		}

		_, _ = io.WriteString(stdout, "$ ")

		lines := bufio.NewScanner(stdin)
		for lines.Scan() {
			if lines.Text() == "exit" {
				return 0
			}

			_, _ = fmt.Fprintf(stdout, "%s\r\n$ ", lines.Text())
			_, _ = io.WriteString(stderr, "(err)")
		}

		return 1
	}

	client, _, recorded := running(t, commands{"/bin/sh": shell})

	session, err := client.Exec(t.Context(), "vm-1", vm.ExecOptions{Command: []string{"/bin/sh"}, TTY: true, Rows: 24, Cols: 80})
	require.NoError(t, err)
	defer session.Close()

	opened := recorded.last(t)

	rows, cols := opened.Size()
	assert.Equal(t, []uint{24, 80}, []uint{rows, cols}, "the terminal is as big as it was asked to be")

	require.NoError(t, session.Resize(t.Context(), 50, 160))
	eventually(t, "the terminal is resized", func() bool {
		rows, cols := opened.Size()

		return rows == 50 && cols == 160
	})

	_, err = io.WriteString(session.Stdin(), "echo hi\nexit\n")
	require.NoError(t, err)

	stdout, stderr := output(t, session)

	assert.Equal(t, "$ echo hi\r\n$ (err)", stdout, "under a terminal everything is on its output")
	assert.Empty(t, stderr, "and its errors are an empty stream")

	exitCode, err := session.Wait(t.Context())
	require.NoError(t, err)
	assert.Zero(t, exitCode)
}

// TestClient_Exec_Concurrent holds sessions to being each their own: many at
// once, each says back only what it was told.
func TestClient_Exec_Concurrent(t *testing.T) {
	t.Parallel()

	client, engine, _ := running(t, commands{"cat": cat})

	var sessions sync.WaitGroup

	for n := range 32 {
		sessions.Go(func() {
			said := strings.Repeat(fmt.Sprintf("session %d; ", n), 1000)

			session, err := client.Exec(t.Context(), "vm-1", vm.ExecOptions{Command: []string{"cat"}})
			if !assert.NoError(t, err) {
				return
			}
			defer session.Close()

			go func() {
				_, _ = io.WriteString(session.Stdin(), said)
				_ = session.Stdin().Close()
			}()

			stdout, stderr := output(t, session)
			assert.Equal(t, said, stdout)
			assert.Empty(t, stderr)

			exitCode, err := session.Wait(t.Context())
			assert.NoError(t, err)
			assert.Zero(t, exitCode)
		})
	}

	sessions.Wait()

	eventually(t, "every session is let go of", func() bool { return engine.Sessions("vm-1") == 0 })
}

// TestClient_Exec_FlowControl moves far more than a window through a session
// both ways at once, and holds a stream nobody reads to holding up only
// itself.
func TestClient_Exec_FlowControl(t *testing.T) {
	t.Parallel()

	t.Run("megabytes each way at once", func(t *testing.T) {
		t.Parallel()

		client, _, _ := running(t, commands{"cat": cat})

		said := make([]byte, 8<<20)
		_, err := rand.Read(said)
		require.NoError(t, err)

		session, err := client.Exec(t.Context(), "vm-1", vm.ExecOptions{Command: []string{"cat"}})
		require.NoError(t, err)
		defer session.Close()

		go func() {
			_, _ = session.Stdin().Write(said)
			_ = session.Stdin().Close()
		}()

		stdout, _ := output(t, session)
		assert.True(t, bytes.Equal(said, []byte(stdout)), "every byte arrives, in order")

		exitCode, err := session.Wait(t.Context())
		require.NoError(t, err)
		assert.Zero(t, exitCode)
	})

	t.Run("a stream read last does not hold up the one read first", func(t *testing.T) {
		t.Parallel()

		// errors first, then far more output than any buffer holds: through
		// a pipe the output would wait for somebody to read the errors.
		noisy := func(_ context.Context, _ string, _ vm.ExecOptions, _ io.Reader, stdout io.Writer, stderr io.Writer) int {
			_, _ = stderr.Write(bytes.Repeat([]byte{'e'}, 100<<10))
			_, _ = stdout.Write(bytes.Repeat([]byte{'o'}, 4<<20))

			return 0
		}

		client, _, _ := running(t, commands{"noisy": noisy})

		session, err := client.Exec(t.Context(), "vm-1", vm.ExecOptions{Command: []string{"noisy"}})
		require.NoError(t, err)
		defer session.Close()

		stdout, err := io.ReadAll(session.Stdout())
		require.NoError(t, err)
		assert.Len(t, stdout, 4<<20)

		stderr, err := io.ReadAll(session.Stderr())
		require.NoError(t, err)
		assert.Len(t, stderr, 100<<10)

		exitCode, err := session.Wait(t.Context())
		require.NoError(t, err)
		assert.Zero(t, exitCode)
	})

	t.Run("a command that does not read holds up its writer, and nothing else", func(t *testing.T) {
		t.Parallel()

		client, engine, _ := running(t, commands{"sleep": sleep})

		session, err := client.Exec(t.Context(), "vm-1", vm.ExecOptions{Command: []string{"sleep"}})
		require.NoError(t, err)

		wrote := make(chan error, 1)
		go func() {
			_, err := session.Stdin().Write(make([]byte, 4<<20))
			wrote <- err
		}()

		select {
		case <-wrote:
			t.Fatal("megabytes were taken by a command that reads nothing")
		case <-time.After(100 * time.Millisecond):
		}

		require.NoError(t, session.Resize(t.Context(), 10, 10), "the session still listens while its input waits")
		require.NoError(t, session.Close())

		assert.Error(t, <-wrote, "the waiting writer is let go once the session closes")
		eventually(t, "the command is ended", func() bool { return engine.Sessions("vm-1") == 0 })
	})
}

func TestClient_Exec_Ending(t *testing.T) {
	t.Parallel()

	t.Run("closing a session ends its command", func(t *testing.T) {
		t.Parallel()

		ended := make(chan struct{})
		sleeper := func(ctx context.Context, id string, options vm.ExecOptions, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
			defer close(ended)

			return sleep(ctx, id, options, stdin, stdout, stderr)
		}

		client, engine, _ := running(t, commands{"sleep": sleeper})

		session, err := client.Exec(t.Context(), "vm-1", vm.ExecOptions{Command: []string{"sleep"}})
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()

		_, err = session.Wait(ctx)
		assert.ErrorIs(t, err, context.DeadlineExceeded, "waiting is given up on with its context, and the command goes on")

		require.NoError(t, session.Close())
		require.NoError(t, session.Close(), "closing twice is closing once")

		select {
		case <-ended:
		case <-time.After(settle):
			t.Fatal("the command outlived its session")
		}

		eventually(t, "the vmhost lets go of the session", func() bool { return engine.Sessions("vm-1") == 0 })

		_, err = session.Wait(t.Context())
		assert.Error(t, err, "a session closed before its command ended has no exit to tell")

		_, err = session.Stdout().Read(make([]byte, 1))
		assert.Error(t, err)

		assert.Error(t, session.Resize(t.Context(), 1, 1))
	})

	t.Run("a command that ended takes no more input", func(t *testing.T) {
		t.Parallel()

		client, _, _ := running(t, commands{"exit": exit})

		session, err := client.Exec(t.Context(), "vm-1", vm.ExecOptions{Command: []string{"exit"}})
		require.NoError(t, err)
		defer session.Close()

		exitCode, err := session.Wait(t.Context())
		require.NoError(t, err)
		assert.Zero(t, exitCode)

		_, err = session.Stdin().Write([]byte("too late"))
		assert.ErrorIs(t, err, io.ErrClosedPipe)

		stdout, stderr := output(t, session)
		assert.Empty(t, stdout)
		assert.Empty(t, stderr)
	})

	t.Run("a vm that stops ends what runs in it", func(t *testing.T) {
		t.Parallel()

		client, engine, _ := running(t, commands{"sleep": sleep})

		session, err := client.Exec(t.Context(), "vm-1", vm.ExecOptions{Command: []string{"sleep"}})
		require.NoError(t, err)
		defer session.Close()

		require.NoError(t, client.Stop(t.Context(), "vm-1"))

		ctx, cancel := context.WithTimeout(t.Context(), settle)
		defer cancel()

		exitCode, err := session.Wait(ctx)
		require.NoError(t, err)
		assert.Equal(t, 137, exitCode)

		eventually(t, "the vmhost lets go of the session", func() bool { return engine.Sessions("vm-1") == 0 })
	})

	t.Run("a vmhost that goes away ends its sessions", func(t *testing.T) {
		t.Parallel()

		engine := newEngine(memory.WithExec(commands{"sleep": sleep}.exec))
		h := serve(t, engine)

		_, err := engine.Create(t.Context(), machine("vm-1"))
		require.NoError(t, err)

		session, err := h.client.Exec(t.Context(), "vm-1", vm.ExecOptions{Command: []string{"sleep"}})
		require.NoError(t, err)
		defer session.Close()

		require.NoError(t, h.server.Close())

		ctx, cancel := context.WithTimeout(t.Context(), settle)
		defer cancel()

		_, err = session.Wait(ctx)
		assert.ErrorIs(t, err, io.ErrUnexpectedEOF, "it says its connection ended rather than how a command it never saw end ended")

		_, err = io.ReadAll(session.Stdout())
		assert.ErrorIs(t, err, io.ErrUnexpectedEOF)

		eventually(t, "the command is ended", func() bool { return engine.Sessions("vm-1") == 0 })
	})
}

// dialStdio is what a node carries a connection to a Docker VM's dockerd
// over, which here connects to address instead.
func dialStdio(address string) memory.ExecFunc {
	return func(ctx context.Context, _ string, _ vm.ExecOptions, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
		connection, err := net.Dial("tcp", address)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)

			return 1
		}
		defer connection.Close()

		go func() {
			_, _ = io.Copy(connection, stdin)
			_ = connection.(*net.TCPConn).CloseWrite()
		}()

		answered := make(chan struct{})
		go func() {
			_, _ = io.Copy(stdout, connection)
			close(answered)
		}()

		select {
		case <-answered:
		case <-ctx.Done():
		}

		return 0
	}
}

// sessionConn is a session as a connection, the way a node's Docker client
// takes one: its input is written, its output is read.
type sessionConn struct {
	vm.ExecSession
}

func (c sessionConn) Read(p []byte) (int, error)       { return c.Stdout().Read(p) }
func (c sessionConn) Write(p []byte) (int, error)      { return c.Stdin().Write(p) }
func (c sessionConn) LocalAddr() net.Addr              { return &net.UnixAddr{Name: "orchestrator"} }
func (c sessionConn) RemoteAddr() net.Addr             { return &net.UnixAddr{Name: "vm"} }
func (c sessionConn) SetDeadline(time.Time) error      { return nil }
func (c sessionConn) SetReadDeadline(time.Time) error  { return nil }
func (c sessionConn) SetWriteDeadline(time.Time) error { return nil }

// TestClient_Exec_LongLived carries HTTP over sessions that last as long as
// their connections are kept: request after request, megabytes each way, on
// one session, as `docker system dial-stdio` carries a node's Docker client.
func TestClient_Exec_LongLived(t *testing.T) {
	t.Parallel()

	// it reads all of a request before it answers: an HTTP/1.1 server that
	// answers first closes the connection after it, and this is about
	// connections being kept.
	echo := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)

			return
		}

		_, _ = rw.Write(body)
	}))
	t.Cleanup(echo.Close)

	client, engine, _ := running(t, commands{"docker": dialStdio(echo.Listener.Addr().String())})

	var dialled atomic.Int32

	httpClient := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _ string, _ string) (net.Conn, error) {
			dialled.Add(1)

			session, err := client.Exec(ctx, "vm-1", vm.ExecOptions{Command: []string{"docker", "system", "dial-stdio"}})
			if err != nil {
				return nil, err
			}

			return sessionConn{ExecSession: session}, nil
		},
		MaxConnsPerHost:     1,
		MaxIdleConnsPerHost: 1,
	}}
	t.Cleanup(httpClient.CloseIdleConnections)

	for n := range 10 {
		body := bytes.Repeat([]byte{byte('a' + n)}, 1<<20)

		response, err := httpClient.Post("http://docker/echo", "application/octet-stream", bytes.NewReader(body))
		require.NoError(t, err)

		answered, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())

		assert.True(t, bytes.Equal(body, answered), "request %d is answered whole", n)
	}

	assert.Equal(t, int32(1), dialled.Load(), "one session carried every request")

	httpClient.CloseIdleConnections()
	eventually(t, "the session is ended with its connection", func() bool { return engine.Sessions("vm-1") == 0 })
}

// dockerd is as much of a Docker VM's dockerd as a node's Docker client
// asks of it here: a ping, and its containers.
func dockerd(t *testing.T) string {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Api-Version", api.DefaultVersion)

		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			_, _ = io.WriteString(rw, "OK")
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			rw.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(rw).Encode([]container.Summary{{
				ID:     "c0ffee",
				Names:  []string{"/db"},
				Image:  "postgres:17",
				State:  container.StateRunning,
				Status: "Up 2 minutes",
			}})
		default:
			http.Error(rw, "not faked", http.StatusNotImplemented)
		}
	}))
	t.Cleanup(server.Close)

	return server.Listener.Addr().String()
}

// TestClient_Exec_Docker reaches a Docker VM's dockerd the way a node does,
// through its Docker client over dial-stdio sessions, with the vmhost's
// socket in between: one request at a time on a kept session, and several at
// once on sessions of their own.
func TestClient_Exec_Docker(t *testing.T) {
	t.Parallel()

	client, engine, _ := running(t, commands{"docker": dialStdio(dockerd(t))})

	daemons := infraDocker.NewDaemons(client, settle, slog.New(slog.DiscardHandler))
	t.Cleanup(func() { daemons.Forget("vm-1") })

	daemon := daemons.Daemon("vm-1")

	require.NoError(t, daemon.Ping(t.Context()))

	var asking sync.WaitGroup

	for range 8 {
		asking.Go(func() {
			containers, err := daemon.Containers(t.Context(), docker.ContainerFilter{All: true})
			if assert.NoError(t, err) && assert.Len(t, containers, 1) {
				assert.Equal(t, "db", containers[0].Name)
			}
		})
	}

	asking.Wait()

	daemons.Forget("vm-1")
	eventually(t, "every connection is let go of with its client", func() bool { return engine.Sessions("vm-1") == 0 })

	t.Run("a vm with no docker in it says so", func(t *testing.T) {
		client, _, _ := running(t, commands{})

		err := infraDocker.NewDaemons(client, 200*time.Millisecond, slog.New(slog.DiscardHandler)).Daemon("vm-1").Ping(t.Context())
		assert.ErrorIs(t, err, vm.ErrNotDocker)
	})
}
