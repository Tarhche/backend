package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	memory "github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

// dialled is a connection carried by a command running exec in a VM of the
// memory engine.
func dialled(t *testing.T, exec memory.ExecFunc) (*conn, *memory.Engine) {
	t.Helper()

	e := memory.New(memory.WithExec(exec))

	_, err := e.Create(t.Context(), vm.Spec{ID: "vm-1", Kind: vm.KindDocker, Image: "docker:29-dind"})
	require.NoError(t, err)

	session, err := e.Exec(t.Context(), "vm-1", vm.ExecOptions{Command: dialStdio})
	require.NoError(t, err)

	c := newConn(session, "vm-1")
	t.Cleanup(func() { _ = c.Close() })

	return c, e
}

// echo says back what it is told, which is what a daemon answering a request
// with the request looks like from here.
func echo(ctx context.Context, _ string, _ vm.ExecOptions, stdin io.Reader, stdout io.Writer, _ io.Writer) int {
	_, _ = io.Copy(stdout, stdin)

	return 0
}

// hangs runs until it is ended.
func hangs(ctx context.Context, _ string, _ vm.ExecOptions, _ io.Reader, _ io.Writer, _ io.Writer) int {
	<-ctx.Done()

	return -1
}

func TestConn(t *testing.T) {
	t.Parallel()

	t.Run("what is written is the command's input, and what it says is read", func(t *testing.T) {
		t.Parallel()

		c, _ := dialled(t, echo)

		n, err := c.Write([]byte("GET /_ping HTTP/1.1\r\n\r\n"))
		require.NoError(t, err)
		assert.Equal(t, 23, n)

		// read a little at a time: what does not fit is kept for the next
		// read.
		got := make([]byte, 0, 23)
		buffer := make([]byte, 5)

		for len(got) < 23 {
			n, err := c.Read(buffer)
			require.NoError(t, err)

			got = append(got, buffer[:n]...)
		}

		assert.Equal(t, "GET /_ping HTTP/1.1\r\n\r\n", string(got))
	})

	t.Run("a read waits no longer than its deadline, and a deadline lifted waits again", func(t *testing.T) {
		t.Parallel()

		c, _ := dialled(t, echo)

		require.NoError(t, c.SetReadDeadline(time.Now().Add(20*time.Millisecond)))

		_, err := c.Read(make([]byte, 1))
		require.ErrorIs(t, err, os.ErrDeadlineExceeded)

		var timeout net.Error
		require.True(t, errors.As(err, &timeout))
		assert.True(t, timeout.Timeout(), "net/http reads a passed deadline as a timeout")

		require.NoError(t, c.SetReadDeadline(time.Time{}))

		_, err = c.Write([]byte("x"))
		require.NoError(t, err)

		read := make([]byte, 1)
		_, err = c.Read(read)
		require.NoError(t, err)
		assert.Equal(t, "x", string(read))
	})

	t.Run("a write after its deadline is refused", func(t *testing.T) {
		t.Parallel()

		c, _ := dialled(t, echo)

		require.NoError(t, c.SetDeadline(time.Now().Add(-time.Second)))

		_, err := c.Write([]byte("x"))
		assert.ErrorIs(t, err, os.ErrDeadlineExceeded)
	})

	t.Run("closing ends the command and lets a waiting reader go", func(t *testing.T) {
		t.Parallel()

		c, e := dialled(t, hangs)

		read := make(chan error, 1)
		go func() {
			_, err := c.Read(make([]byte, 1))
			read <- err
		}()

		require.NoError(t, c.Close())

		assert.ErrorIs(t, <-read, net.ErrClosed)
		assert.Eventually(t, func() bool { return e.Sessions("vm-1") == 0 }, time.Second, time.Millisecond, "the command is ended")

		_, err := c.Write([]byte("x"))
		assert.ErrorIs(t, err, net.ErrClosed)
		assert.ErrorIs(t, c.Close(), net.ErrClosed, "closed once")
	})

	t.Run("a VM with no docker in it is said to be no Docker VM", func(t *testing.T) {
		t.Parallel()

		c, _ := dialled(t, func(_ context.Context, _ string, _ vm.ExecOptions, _ io.Reader, _ io.Writer, stderr io.Writer) int {
			fmt.Fprintln(stderr, "sh: docker: not found")

			return 127
		})

		_, err := c.Read(make([]byte, 1))
		assert.ErrorIs(t, err, vm.ErrNotDocker)
		assert.Contains(t, err.Error(), "docker: not found")
	})

	t.Run("a daemon that is not answering yet ends the connection with what docker said", func(t *testing.T) {
		t.Parallel()

		c, _ := dialled(t, func(_ context.Context, _ string, _ vm.ExecOptions, _ io.Reader, _ io.Writer, stderr io.Writer) int {
			fmt.Fprintln(stderr, "Cannot connect to the Docker daemon at unix:///var/run/docker.sock.")

			return 1
		})

		_, err := c.Read(make([]byte, 1))
		assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
		assert.NotErrorIs(t, err, vm.ErrNotDocker)
		assert.Contains(t, err.Error(), "Cannot connect to the Docker daemon")
	})

	t.Run("a command that said something and ended is the end of the connection", func(t *testing.T) {
		t.Parallel()

		c, _ := dialled(t, func(_ context.Context, _ string, _ vm.ExecOptions, _ io.Reader, stdout io.Writer, _ io.Writer) int {
			_, _ = io.WriteString(stdout, "HTTP/1.1 200 OK\r\n\r\n")

			return 0
		})

		said, err := io.ReadAll(c)
		require.NoError(t, err)
		assert.Equal(t, "HTTP/1.1 200 OK\r\n\r\n", string(said))
	})

	t.Run("its ends are named by what they are", func(t *testing.T) {
		t.Parallel()

		c, _ := dialled(t, echo)

		assert.Equal(t, "exec", c.RemoteAddr().Network())
		assert.Equal(t, "vm/vm-1", c.RemoteAddr().String())
		assert.Equal(t, "orchestrator", c.LocalAddr().String())
	})
}

func TestTailBuffer(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name   string
		limit  int
		writes []string
		want   string
	}{
		{name: "what fits is kept whole", limit: 10, writes: []string{"abc", "def"}, want: "abcdef"},
		{name: "past the limit, the front goes", limit: 5, writes: []string{"abc", "defg"}, want: "cdefg"},
		{name: "one write past the limit keeps its own end", limit: 3, writes: []string{"abcdef"}, want: "def"},
		{name: "a character cut in half is left out", limit: 4, writes: []string{"aé", "bcd"}, want: "bcd"},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tail := newTailBuffer(tt.limit)
			for _, write := range tt.writes {
				n, err := tail.Write([]byte(write))
				require.NoError(t, err)
				assert.Equal(t, len(write), n, "nothing written is refused")
			}

			assert.Equal(t, tt.want, tail.String())
		})
	}
}
