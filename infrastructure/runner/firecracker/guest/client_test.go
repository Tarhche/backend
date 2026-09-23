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
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// serve stands up an agent answering with handler behind a hybrid vsock, and
// returns a client for it.
func serve(t *testing.T, handler http.Handler) *Client {
	t.Helper()

	socket := filepath.Join(t.TempDir(), "v.sock")

	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)

	server := &http.Server{Handler: handler}
	go func() { _ = server.Serve(hybridListener{Listener: listener, port: Port}) }()

	t.Cleanup(func() { _ = server.Close() })

	client := NewClient(socket)
	t.Cleanup(client.Close)

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

func TestClient(t *testing.T) {
	t.Run("what is asked travels as JSON, and so does what comes back", func(t *testing.T) {
		var told Config

		mux := http.NewServeMux()
		mux.HandleFunc("GET /health", func(rw http.ResponseWriter, r *http.Request) {})
		mux.HandleFunc("PUT /config", func(rw http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&told)
		})
		mux.HandleFunc("POST /process", func(rw http.ResponseWriter, r *http.Request) {
			var process Process
			_ = json.NewDecoder(r.Body).Decode(&process)
			_ = json.NewEncoder(rw).Encode(Status{State: StateRunning, Generation: 1})
		})

		client := serve(t, mux)
		ctx := t.Context()

		require.NoError(t, client.Ready(ctx))
		require.NoError(t, client.Configure(ctx, Config{Hostname: "nginx-xkfqz", Root: Root{Image: "/dev/vda"}}))
		assert.Equal(t, "nginx-xkfqz", told.Hostname)
		assert.True(t, told.Root.ReadOnly())

		status, err := client.Start(ctx, Process{Args: []string{"nginx"}})
		require.NoError(t, err)
		assert.Equal(t, Status{State: StateRunning, Generation: 1}, status)
	})

	t.Run("a refusal of what only a running task can do reads as not running", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("POST /process/signal", func(rw http.ResponseWriter, r *http.Request) {
			http.Error(rw, "the task is not running", http.StatusConflict)
		})

		err := serve(t, mux).Signal(t.Context(), 9)

		assert.ErrorIs(t, err, ErrNotRunning)
		assert.ErrorContains(t, err, "the task is not running")
	})

	t.Run("an agent that is not up yet is waited for, for as long as the caller allows", func(t *testing.T) {
		client := NewClient(filepath.Join(t.TempDir(), "nothing-here.sock"))
		defer client.Close()

		ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
		defer cancel()

		assert.ErrorIs(t, client.Ready(ctx), context.DeadlineExceeded)
	})

	t.Run("logs are handed over line by line, in order", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /logs", func(rw http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "2", r.URL.Query().Get("after"))

			for seq := uint64(3); seq <= 5; seq++ {
				_ = json.NewEncoder(rw).Encode(LogLine{Seq: seq, Stream: StreamStdout, Content: fmt.Sprint("line ", seq)})
			}
		})

		var lines []LogLine
		err := serve(t, mux).Logs(t.Context(), 2, false, func(line LogLine) error {
			lines = append(lines, line)

			return nil
		})

		require.NoError(t, err)
		require.Len(t, lines, 3)
		assert.Equal(t, uint64(3), lines[0].Seq)
		assert.Equal(t, "line 5", lines[2].Content)
	})

	t.Run("a line refused ends the reading", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /logs", func(rw http.ResponseWriter, r *http.Request) {
			for seq := uint64(1); seq <= 3; seq++ {
				_ = json.NewEncoder(rw).Encode(LogLine{Seq: seq})
			}
		})

		refused := errors.New("enough")

		read := 0
		err := serve(t, mux).Logs(t.Context(), 0, false, func(line LogLine) error {
			read++

			return refused
		})

		assert.ErrorIs(t, err, refused)
		assert.Equal(t, 1, read)
	})

	t.Run("a command's output and end come back as frames, and its input goes as them", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("POST /exec", func(rw http.ResponseWriter, r *http.Request) {
			var exec Exec
			_ = json.NewDecoder(r.Body).Decode(&exec)

			conn, buffered := hijack(t, rw, UpgradeExec, ExecIDHeader+": 0123456789abcdef\r\n")
			defer conn.Close()

			// an echo: what comes in goes out, and the command ends when its
			// input does.
			for {
				frame, err := ReadFrame(buffered)
				if err != nil {
					return
				}

				switch frame.Type {
				case FrameStdin:
					_ = WriteFrame(conn, FrameStdout, frame.Payload)
				case FrameResize:
					rows, cols, _ := ParseResize(frame.Payload)
					_ = WriteFrame(conn, FrameStderr, fmt.Appendf(nil, "%dx%d", rows, cols))
				case FrameCloseStdin:
					_ = WriteFrame(conn, FrameExit, ExitPayload(3))

					return
				}
			}
		})

		stream, err := serve(t, mux).Exec(t.Context(), Exec{Process: Process{Args: []string{"sh"}}, TTY: true})
		require.NoError(t, err)
		defer stream.Close()

		assert.Equal(t, "0123456789abcdef", stream.ID)

		_, err = stream.Write([]byte("echo hi"))
		require.NoError(t, err)
		require.NoError(t, stream.Resize(24, 80))

		stream.writing.Lock()
		require.NoError(t, WriteFrame(stream.conn, FrameCloseStdin, nil))
		stream.writing.Unlock()

		output, err := io.ReadAll(stream)
		require.NoError(t, err)
		assert.Equal(t, "echo hi24x80", string(output))

		code, exited := stream.ExitCode()
		assert.True(t, exited)
		assert.Equal(t, 3, code)
	})

	t.Run("a port is dialled through the agent, as raw bytes", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("POST /dial", func(rw http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("port") != "8080" {
				http.Error(rw, "not that port", http.StatusNotFound)

				return
			}

			conn, buffered := hijack(t, rw, UpgradeDial, "")
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
