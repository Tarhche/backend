package guest

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	guestProtocol "github.com/khanzadimahdi/testproject/domain/workload/guest"
)

// hybridListener stands in for firecracker's side of a machine's vsock: it
// takes connections on a unix socket, answers the CONNECT a host sends first,
// and hands what follows to whatever listens on the port inside.
type hybridListener struct {
	net.Listener
	port uint32
}

func (l hybridListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}

		// read byte by byte: whatever follows the line belongs to the guest.
		var line []byte
		one := make([]byte, 1)
		for !strings.HasSuffix(string(line), "\n") {
			if _, err := conn.Read(one); err != nil {
				break
			}

			line = append(line, one[0])
		}

		if strings.TrimSpace(string(line)) != fmt.Sprintf("CONNECT %d", l.port) {
			conn.Close()

			continue
		}

		if _, err := io.WriteString(conn, "OK 1073741824\n"); err != nil {
			conn.Close()

			continue
		}

		return conn, nil
	}
}

// socketPath is where a fake machine's vsock is exposed: somewhere short, since
// a unix socket's path is at most 104 bytes on darwin, and a test's own
// temporary directory there is most of that already.
func socketPath(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("/tmp", "guest")
	require.NoError(t, err)

	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	return filepath.Join(dir, "v.sock")
}

// versioned answers every request in the protocol's version, as the agent
// does.
func versioned(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set(guestProtocol.VersionHeader, guestProtocol.ProtocolVersion)
		handler.ServeHTTP(rw, r)
	})
}

// listen stands up an agent answering with handler behind a hybrid vsock at
// socket.
func listen(t *testing.T, socket string, handler http.Handler) {
	t.Helper()

	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)

	server := &http.Server{Handler: handler}
	go func() { _ = server.Serve(hybridListener{Listener: listener, port: guestProtocol.Port}) }()

	t.Cleanup(func() { _ = server.Close() })
}

// serve stands up an agent answering with handler, in the protocol's version,
// and returns a client for it.
func serve(t *testing.T, handler http.Handler) *Client {
	t.Helper()

	socket := socketPath(t)
	listen(t, socket, versioned(handler))

	client := NewClient(socket)
	t.Cleanup(func() { _ = client.Close() })

	return client
}

func hijack(t *testing.T, rw http.ResponseWriter, protocol string, header string) (net.Conn, *bufio.ReadWriter) {
	conn, buffered, err := rw.(http.Hijacker).Hijack()
	require.NoError(t, err)

	_, err = buffered.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: " + protocol + "\r\n" + header + "\r\n")
	require.NoError(t, err)
	require.NoError(t, buffered.Flush())

	return conn, buffered
}

// request is what a fake agent was asked.
type request struct {
	route string
	path  string
	query string
	body  string
}

// recorder is a fake agent that answers every route the protocol names with
// answers[route], and remembers what it was asked.
type recorder struct {
	lock     sync.Mutex
	asked    []request
	answers  map[string]any
	statuses map[string]int
}

func (r *recorder) handler() http.Handler {
	mux := http.NewServeMux()

	for _, route := range []string{
		guestProtocol.RouteHealth, guestProtocol.RouteConfig, guestProtocol.RouteHosts,
		guestProtocol.RouteStart, guestProtocol.RouteStatus, guestProtocol.RouteWait,
		guestProtocol.RouteSignal, guestProtocol.RouteStop, guestProtocol.RouteLogs,
		guestProtocol.RouteStats, guestProtocol.RouteEndExec, guestProtocol.RoutePowerOff,
	} {
		mux.HandleFunc(route, func(rw http.ResponseWriter, req *http.Request) {
			body, _ := io.ReadAll(req.Body)

			r.lock.Lock()
			r.asked = append(r.asked, request{route: route, path: req.URL.Path, query: req.URL.RawQuery, body: string(body)})
			answer, status := r.answers[route], r.statuses[route]
			r.lock.Unlock()

			if status != 0 {
				http.Error(rw, "refused", status)

				return
			}

			if answer != nil {
				_ = json.NewEncoder(rw).Encode(answer)
			}
		})
	}

	return mux
}

func (r *recorder) last() request {
	r.lock.Lock()
	defer r.lock.Unlock()

	if len(r.asked) == 0 {
		return request{}
	}

	return r.asked[len(r.asked)-1]
}

func TestClient(t *testing.T) {
	t.Parallel()

	t.Run("what is asked travels as JSON, and so does what comes back", func(t *testing.T) {
		t.Parallel()

		var told guestProtocol.Config

		mux := http.NewServeMux()
		mux.HandleFunc(guestProtocol.RouteHealth, func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusNoContent)
		})
		mux.HandleFunc(guestProtocol.RouteConfig, func(rw http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&told)
		})
		mux.HandleFunc(guestProtocol.RouteStart, func(rw http.ResponseWriter, r *http.Request) {
			var process guestProtocol.Process
			_ = json.NewDecoder(r.Body).Decode(&process)
			_ = json.NewEncoder(rw).Encode(guestProtocol.Status{State: guestProtocol.StateRunning, Generation: 1})
		})

		client := serve(t, mux)
		ctx := t.Context()

		require.NoError(t, client.Ready(ctx))
		require.NoError(t, client.Configure(ctx, guestProtocol.Config{Hostname: "nginx-xkfqz", Root: guestProtocol.Root{Image: guestProtocol.ImageDevice}}))
		assert.Equal(t, "nginx-xkfqz", told.Hostname)
		assert.True(t, told.Root.ReadOnly())

		status, err := client.Start(ctx, guestProtocol.Process{Args: []string{"nginx"}})
		require.NoError(t, err)
		assert.Equal(t, guestProtocol.Status{State: guestProtocol.StateRunning, Generation: 1}, status)
	})

	t.Run("every question goes to the route the protocol names it by", func(t *testing.T) {
		t.Parallel()

		exited := guestProtocol.Status{State: guestProtocol.StateExited, Generation: 3, ExitCode: 137}
		agent := &recorder{answers: map[string]any{
			guestProtocol.RouteStatus:  exited,
			guestProtocol.RouteWait:    exited,
			guestProtocol.RouteStop:    exited,
			guestProtocol.RouteStats:   guestProtocol.Stats{PIDs: 4, MemoryUsage: 1 << 20},
			guestProtocol.RouteEndExec: guestProtocol.Ended{Signalled: true},
		}}

		client := serve(t, agent.handler())
		ctx := t.Context()

		status, err := client.Status(ctx)
		require.NoError(t, err)
		assert.Equal(t, exited, status)
		assert.Equal(t, request{route: guestProtocol.RouteStatus, path: "/process"}, agent.last())

		status, err = client.Wait(ctx, 3)
		require.NoError(t, err)
		assert.Equal(t, exited, status)
		assert.Equal(t, request{route: guestProtocol.RouteWait, path: "/process/wait", query: "generation=3"}, agent.last())

		require.NoError(t, client.Signal(ctx, 15))
		assert.Equal(t, request{route: guestProtocol.RouteSignal, path: "/process/signal", body: `{"signal":15}`}, agent.last())

		status, err = client.Stop(ctx, 10*time.Second)
		require.NoError(t, err)
		assert.Equal(t, exited, status)
		assert.Equal(t, request{route: guestProtocol.RouteStop, path: "/process/stop", body: `{"timeout":10000000000}`}, agent.last())

		stats, err := client.Stats(ctx)
		require.NoError(t, err)
		assert.Equal(t, guestProtocol.Stats{PIDs: 4, MemoryUsage: 1 << 20}, stats)
		assert.Equal(t, request{route: guestProtocol.RouteStats, path: "/stats"}, agent.last())

		require.NoError(t, client.SetHosts(ctx, []guestProtocol.Host{{Address: "10.250.1.3", Names: []string{"db"}}}))
		assert.Equal(t, request{route: guestProtocol.RouteHosts, path: "/hosts", body: `[{"address":"10.250.1.3","names":["db"]}]`}, agent.last())

		ended, err := client.EndExec(ctx, "0123456789abcdef", guestProtocol.EndExec{Grace: time.Second, KillGrace: 2 * time.Second})
		require.NoError(t, err)
		assert.True(t, ended.Signalled)
		assert.Equal(t, request{route: guestProtocol.RouteEndExec, path: "/exec/0123456789abcdef/end", body: `{"grace":1000000000,"kill_grace":2000000000}`}, agent.last())

		require.NoError(t, client.PowerOff(ctx))
		assert.Equal(t, request{route: guestProtocol.RoutePowerOff, path: "/poweroff"}, agent.last())
	})

	t.Run("an agent that speaks another version of the protocol is refused at once", func(t *testing.T) {
		t.Parallel()

		for version, says := range map[string]string{"2": `it speaks "2"`, "": "it does not say which it speaks"} {
			socket := socketPath(t)
			listen(t, socket, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				if len(version) > 0 {
					rw.Header().Set(guestProtocol.VersionHeader, version)
				}

				rw.WriteHeader(http.StatusNoContent)
			}))

			client := NewClient(socket)
			t.Cleanup(func() { _ = client.Close() })

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			started := time.Now()

			err := client.Ready(ctx)
			cancel()

			assert.ErrorIs(t, err, guestProtocol.ErrUnknownVersion, version)
			assert.ErrorContains(t, err, says)
			assert.Less(t, time.Since(started), 5*time.Second, "refusing an agent it cannot drive is not waiting for it")
		}
	})

	t.Run("a refusal of what only a running task can do reads as not running", func(t *testing.T) {
		t.Parallel()

		agent := &recorder{statuses: map[string]int{guestProtocol.RouteSignal: http.StatusConflict}}

		err := serve(t, agent.handler()).Signal(t.Context(), 9)

		assert.ErrorIs(t, err, guestProtocol.ErrNotRunning)
		assert.ErrorContains(t, err, "409 Conflict: refused")
	})

	t.Run("any other refusal says what the agent said", func(t *testing.T) {
		t.Parallel()

		agent := &recorder{statuses: map[string]int{guestProtocol.RouteConfig: http.StatusBadRequest}}

		err := serve(t, agent.handler()).Configure(t.Context(), guestProtocol.Config{})

		assert.NotErrorIs(t, err, guestProtocol.ErrNotRunning)
		assert.ErrorContains(t, err, "400 Bad Request: refused")
	})

	t.Run("an agent that is not up yet is waited for, for as long as the caller allows", func(t *testing.T) {
		t.Parallel()

		client := NewClient(socketPath(t))
		defer client.Close()

		ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
		defer cancel()

		assert.ErrorIs(t, client.Ready(ctx), context.DeadlineExceeded)
	})

	t.Run("an agent that comes up while it is waited for is found", func(t *testing.T) {
		t.Parallel()

		socket := socketPath(t)

		client := NewClient(socket)
		defer client.Close()

		server := &http.Server{Handler: versioned(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {}))}
		t.Cleanup(func() { _ = server.Close() })

		listening := make(chan error, 1)
		time.AfterFunc(300*time.Millisecond, func() {
			listener, err := net.Listen("unix", socket)
			listening <- err

			if err == nil {
				_ = server.Serve(hybridListener{Listener: listener, port: guestProtocol.Port})
			}
		})

		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()

		assert.NoError(t, client.Ready(ctx))
		assert.NoError(t, <-listening)
	})

	t.Run("an answer larger than an answer may be is not read whole", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		mux.HandleFunc(guestProtocol.RouteStats, func(rw http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(rw, `{"pids": 1, "padding": "`+strings.Repeat("x", 2*maxAnswer)+`"}`)
		})

		_, err := serve(t, mux).Stats(t.Context())

		assert.ErrorContains(t, err, "not what was asked for")
	})

	t.Run("logs are handed over line by line, in order", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		mux.HandleFunc(guestProtocol.RouteLogs, func(rw http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "2", r.URL.Query().Get(guestProtocol.QueryAfter))
			assert.Equal(t, "1", r.URL.Query().Get(guestProtocol.QueryFollow))

			for seq := uint64(3); seq <= 5; seq++ {
				_ = json.NewEncoder(rw).Encode(guestProtocol.LogLine{Seq: seq, Stream: guestProtocol.StreamStdout, Content: fmt.Sprint("line ", seq)})
			}
		})

		var lines []guestProtocol.LogLine
		err := serve(t, mux).Logs(t.Context(), 2, true, func(line guestProtocol.LogLine) error {
			lines = append(lines, line)

			return nil
		})

		require.NoError(t, err)
		require.Len(t, lines, 3)
		assert.Equal(t, uint64(3), lines[0].Seq)
		assert.Equal(t, "line 5", lines[2].Content)
	})

	t.Run("a line refused ends the reading", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		mux.HandleFunc(guestProtocol.RouteLogs, func(rw http.ResponseWriter, r *http.Request) {
			assert.Empty(t, r.URL.Query().Get(guestProtocol.QueryFollow), "only what is there already is asked for")

			for seq := uint64(1); seq <= 3; seq++ {
				_ = json.NewEncoder(rw).Encode(guestProtocol.LogLine{Seq: seq})
			}
		})

		refused := errors.New("enough")

		read := 0
		err := serve(t, mux).Logs(t.Context(), 0, false, func(line guestProtocol.LogLine) error {
			read++

			return refused
		})

		assert.ErrorIs(t, err, refused)
		assert.Equal(t, 1, read)
	})

	t.Run("a command's output and end come back as frames, and its input goes as them", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		mux.HandleFunc(guestProtocol.RouteExec, func(rw http.ResponseWriter, r *http.Request) {
			var exec guestProtocol.Exec
			_ = json.NewDecoder(r.Body).Decode(&exec)

			if !assert.Equal(t, guestProtocol.UpgradeExec, r.Header.Get("Upgrade")) || !assert.True(t, exec.TTY) {
				http.Error(rw, "not that", http.StatusBadRequest)

				return
			}

			conn, buffered := hijack(t, rw, guestProtocol.UpgradeExec, guestProtocol.ExecIDHeader+": 0123456789abcdef\r\n")
			defer conn.Close()

			// an echo: what comes in goes out, and the command ends when its
			// input does.
			for {
				frame, err := guestProtocol.ReadFrame(buffered)
				if err != nil {
					return
				}

				switch frame.Type {
				case guestProtocol.FrameStdin:
					_ = guestProtocol.WriteFrame(conn, guestProtocol.FrameStdout, frame.Payload)
				case guestProtocol.FrameResize:
					rows, cols, _ := guestProtocol.ParseResize(frame.Payload)
					_ = guestProtocol.WriteFrame(conn, guestProtocol.FrameStderr, fmt.Appendf(nil, "%dx%d", rows, cols))
				case guestProtocol.FrameCloseStdin:
					_ = guestProtocol.WriteFrame(conn, guestProtocol.FrameExit, guestProtocol.ExitPayload(3))

					return
				}
			}
		})

		id, conn, err := serve(t, mux).Exec(t.Context(), guestProtocol.Exec{Process: guestProtocol.Process{Args: []string{"sh"}}, TTY: true})
		require.NoError(t, err)
		defer conn.Close()

		assert.Equal(t, "0123456789abcdef", id)

		require.NoError(t, guestProtocol.WriteFrame(conn, guestProtocol.FrameStdin, []byte("echo hi")))
		require.NoError(t, guestProtocol.WriteFrame(conn, guestProtocol.FrameResize, guestProtocol.ResizePayload(24, 80)))
		require.NoError(t, guestProtocol.WriteFrame(conn, guestProtocol.FrameCloseStdin, nil))

		var frames []guestProtocol.Frame
		for {
			frame, err := guestProtocol.ReadFrame(conn)
			require.NoError(t, err)

			frames = append(frames, frame)
			if frame.Type == guestProtocol.FrameExit {
				break
			}
		}

		assert.Equal(t, []guestProtocol.Frame{
			{Type: guestProtocol.FrameStdout, Payload: []byte("echo hi")},
			{Type: guestProtocol.FrameStderr, Payload: []byte("24x80")},
			{Type: guestProtocol.FrameExit, Payload: guestProtocol.ExitPayload(3)},
		}, frames)
	})

	t.Run("a command the agent gives no name to is let go of, since it could never be ended", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		mux.HandleFunc(guestProtocol.RouteExec, func(rw http.ResponseWriter, r *http.Request) {
			conn, _ := hijack(t, rw, guestProtocol.UpgradeExec, "")
			defer conn.Close()

			_, _ = io.Copy(io.Discard, conn)
		})

		_, conn, err := serve(t, mux).Exec(t.Context(), guestProtocol.Exec{Process: guestProtocol.Process{Args: []string{"sh"}}})

		assert.ErrorContains(t, err, "without saying what it calls it")
		assert.Nil(t, conn)
	})

	t.Run("a port is dialled through the agent, as raw bytes", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		mux.HandleFunc(guestProtocol.RouteDial, func(rw http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get(guestProtocol.QueryPort) != "8080" {
				http.Error(rw, "not that port", http.StatusNotFound)

				return
			}

			conn, buffered := hijack(t, rw, guestProtocol.UpgradeDial, "")
			defer conn.Close()

			line, _ := buffered.ReadString('\n')
			_, _ = io.WriteString(conn, strings.ToUpper(line))
		})

		client := serve(t, mux)

		conn, err := client.Dial(t.Context(), 8080)
		require.NoError(t, err)
		defer conn.Close()

		_, err = io.WriteString(conn, "hello\n")
		require.NoError(t, err)

		answer, err := bufio.NewReader(conn).ReadString('\n')
		require.NoError(t, err)
		assert.Equal(t, "HELLO\n", answer)

		_, err = client.Dial(t.Context(), 9090)
		assert.ErrorContains(t, err, "not that port")
	})
}

func TestConnector(t *testing.T) {
	t.Parallel()

	client, ok := NewConnector().Connect("/var/lib/workload-vmhost/j/0123456789abcdef/root/run/v.sock").(*Client)

	require.True(t, ok)
	assert.Equal(t, "/var/lib/workload-vmhost/j/0123456789abcdef/root/run/v.sock", client.socketPath)
}
