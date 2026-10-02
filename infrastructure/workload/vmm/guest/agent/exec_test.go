//go:build linux

package agent

import (
	"bufio"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
)

// session is a command's connection, as vmhost holds it.
type session struct {
	t      *testing.T
	conn   net.Conn
	reader *bufio.Reader
	id     string

	stdout strings.Builder
	stderr strings.Builder

	exited   bool
	exitCode int
}

// execute runs a command beside the task, and hands back its connection.
func (a *testAgent) execute(t *testing.T, exec guest.Exec) *session {
	t.Helper()

	conn, reader, response := a.upgrade(t, "/exec", guest.UpgradeExec, exec)
	require.Equal(t, http.StatusSwitchingProtocols, response.StatusCode)

	assert.Equal(t, guest.UpgradeExec, response.Header.Get("Upgrade"))
	assert.Equal(t, guest.ProtocolVersion, response.Header.Get(guest.VersionHeader))

	id := response.Header.Get(guest.ExecIDHeader)
	require.NotEmpty(t, id)

	return &session{t: t, conn: conn, reader: reader, id: id}
}

func (s *session) send(frameType guest.FrameType, payload []byte) {
	s.t.Helper()

	require.NoError(s.t, guest.WriteFrame(s.conn, frameType, payload))
}

// next reads one frame and keeps what it carries.
func (s *session) next() {
	s.t.Helper()

	frame, err := guest.ReadFrame(s.reader)
	require.NoError(s.t, err)

	switch frame.Type {
	case guest.FrameStdout:
		s.stdout.Write(frame.Payload)
	case guest.FrameStderr:
		s.stderr.Write(frame.Payload)
	case guest.FrameExit:
		code, err := guest.ParseExit(frame.Payload)
		require.NoError(s.t, err)

		s.exited, s.exitCode = true, code
	default:
		s.t.Fatalf("the agent sent a frame of a type it never sends: %d", frame.Type)
	}
}

// until reads frames until the command's output holds text.
func (s *session) until(text string) {
	s.t.Helper()

	for !strings.Contains(s.stdout.String(), text) {
		require.False(s.t, s.exited, "the command ended before it wrote %q; it wrote %q", text, s.stdout.String())
		s.next()
	}
}

// end reads frames until the command ends, and says what it returned.
func (s *session) end() int {
	s.t.Helper()

	for !s.exited {
		s.next()
	}

	return s.exitCode
}

func TestExec(t *testing.T) {
	forEachConfinement(t, func(t *testing.T, confine func(t *testing.T) confinement) {
		t.Run("a command takes input and gives output, each stream its own, and says what it returned", func(t *testing.T) {
			a := configured(t, confine(t))
			a.start(t, guest.Process{Args: []string{"sleep", "60"}})

			s := a.execute(t, guest.Exec{Process: guest.Process{Args: []string{"/bin/sh", "-c", "cat; echo done >&2; exit 3"}}})

			s.send(guest.FrameStdin, []byte("hello\n"))
			s.send(guest.FrameCloseStdin, nil)

			assert.Equal(t, 3, s.end())
			assert.Equal(t, "hello\n", s.stdout.String())
			assert.Equal(t, "done\n", s.stderr.String())
		})

		t.Run("a command with a terminal is told its size, and told again when it changes", func(t *testing.T) {
			a := configured(t, confine(t))
			a.start(t, guest.Process{Args: []string{"sleep", "60"}})

			s := a.execute(t, guest.Exec{Process: guest.Process{Args: []string{"/bin/sh"}}, TTY: true, Rows: 24, Cols: 80})

			s.send(guest.FrameStdin, []byte("stty size; echo term=$TERM\n"))
			s.until("24 80")
			s.until("term=xterm")

			s.send(guest.FrameResize, guest.ResizePayload(30, 100))
			s.send(guest.FrameStdin, []byte("stty size\n"))
			s.until("30 100")

			s.send(guest.FrameStdin, []byte("exit 7\n"))
			assert.Equal(t, 7, s.end())
			assert.Empty(t, s.stderr.String(), "a terminal is one stream")
		})

		t.Run("a command runs as the task does, with the task's environment under its own", func(t *testing.T) {
			a := configured(t, confine(t))
			dir := t.TempDir()

			a.start(t, guest.Process{Args: []string{"sleep", "60"}, Env: []string{"GREETING=hello", "NAME=task"}, WorkingDir: dir})

			s := a.execute(t, guest.Exec{Process: guest.Process{Args: []string{"/bin/sh", "-c", `echo "$GREETING $NAME $HOSTNAME $(pwd)"`}, Env: []string{"NAME=command"}}})

			assert.Equal(t, 0, s.end())
			assert.Equal(t, "hello command task-xkfqz "+dir+"\n", s.stdout.String())
		})

		t.Run("a command left behind is given its grace, asked to stop, and made to", func(t *testing.T) {
			a := configured(t, confine(t))
			a.start(t, guest.Process{Args: []string{"sleep", "60"}})

			s := a.execute(t, guest.Exec{Process: guest.Process{Args: []string{"/bin/sh", "-c", `trap "" TERM; echo $$; sleep 60 & echo $!; wait`}}})

			s.until("\n")
			for strings.Count(s.stdout.String(), "\n") < 2 {
				s.next()
			}

			pids := numbersIn(t, s.stdout.String())
			require.Len(t, pids, 2)

			// whoever was attached goes away; the command stays until it is
			// ended.
			require.NoError(t, s.conn.Close())

			time.Sleep(100 * time.Millisecond)
			for _, pid := range pids {
				assert.False(t, gone(pid), "a command nobody is attached to runs on until it is ended")
			}

			var ended guest.Ended
			require.Equal(t, http.StatusOK, a.call(t, http.MethodPost, "/exec/"+s.id+"/end", guest.EndExec{Grace: 50 * time.Millisecond, KillGrace: 200 * time.Millisecond}, &ended).StatusCode)

			assert.True(t, ended.Signalled)

			for _, pid := range pids {
				eventually(t, func() bool { return gone(pid) }, "the command and everything it started are ended")
			}

			populated, err := a.confine.populated(execGroup(s.id))
			require.NoError(t, err)
			assert.False(t, populated)

			require.Equal(t, http.StatusOK, a.call(t, http.MethodPost, "/exec/"+s.id+"/end", guest.EndExec{}, &ended).StatusCode)
			assert.False(t, ended.Signalled, "a command already ended has nothing left to end")
		})

		t.Run("a command that ended on its own leaves nothing to end", func(t *testing.T) {
			a := configured(t, confine(t))
			a.start(t, guest.Process{Args: []string{"sleep", "60"}})

			s := a.execute(t, guest.Exec{Process: guest.Process{Args: []string{"true"}}})
			assert.Equal(t, 0, s.end())

			var ended guest.Ended
			require.Equal(t, http.StatusOK, a.call(t, http.MethodPost, "/exec/"+s.id+"/end", guest.EndExec{Grace: time.Second, KillGrace: time.Second}, &ended).StatusCode)
			assert.False(t, ended.Signalled)
		})

		t.Run("a command is ended with the task", func(t *testing.T) {
			a := configured(t, confine(t))
			a.start(t, guest.Process{Args: []string{"sleep", "60"}})

			s := a.execute(t, guest.Exec{Process: guest.Process{Args: []string{"sleep", "60"}}})

			require.Equal(t, http.StatusNoContent, a.call(t, http.MethodPost, "/process/signal", guest.Signal{Signal: 9}, nil).StatusCode)
			require.Equal(t, 137, a.waitFor(t, 1).ExitCode)

			assert.Equal(t, 137, s.end(), "a command does not outlive the task it was run beside")
		})
	})

	t.Run("nothing is run beside a task that is not running", func(t *testing.T) {
		a := configured(t, newProcessGroups())

		_, _, response := a.upgrade(t, "/exec", guest.UpgradeExec, guest.Exec{Process: guest.Process{Args: []string{"true"}}})

		assert.Equal(t, http.StatusConflict, response.StatusCode)
		assert.Equal(t, guest.ProtocolVersion, response.Header.Get(guest.VersionHeader))
	})

	t.Run("a command that cannot be run is refused before the connection is taken", func(t *testing.T) {
		a := configured(t, newProcessGroups())
		a.start(t, guest.Process{Args: []string{"sleep", "60"}})

		_, _, response := a.upgrade(t, "/exec", guest.UpgradeExec, guest.Exec{Process: guest.Process{Args: []string{"no-such-command-anywhere"}}})
		assert.Equal(t, http.StatusUnprocessableEntity, response.StatusCode)

		_, _, response = a.upgrade(t, "/exec", guest.UpgradeExec, guest.Exec{})
		assert.Equal(t, http.StatusUnprocessableEntity, response.StatusCode)
	})

	t.Run("ending a command nobody ever ran ends nothing", func(t *testing.T) {
		a := configured(t, newProcessGroups())

		var ended guest.Ended
		require.Equal(t, http.StatusOK, a.call(t, http.MethodPost, "/exec/0123456789abcdef/end", guest.EndExec{}, &ended).StatusCode)
		assert.False(t, ended.Signalled)
	})
}

// numbersIn is every whole number on a line of its own in text.
func numbersIn(t *testing.T, text string) []int {
	t.Helper()

	var numbers []int
	for _, line := range strings.Fields(text) {
		if number, err := strconv.Atoi(line); err == nil {
			numbers = append(numbers, number)
		}
	}

	return numbers
}
