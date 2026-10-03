//go:build linux

package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
)

func TestProcess(t *testing.T) {
	forEachConfinement(t, func(t *testing.T, confine func(t *testing.T) confinement) {
		t.Run("a task runs, and says what it wrote and what it returned", func(t *testing.T) {
			a := configured(t, confine(t))

			started := a.start(t, guest.Process{Args: []string{"/bin/sh", "-c", "echo out; echo err >&2; exit 1"}})

			assert.Equal(t, guest.StateRunning, started.State)
			assert.Equal(t, uint64(1), started.Generation)
			assert.False(t, started.StartedAt.IsZero())

			ended := a.waitFor(t, 1)

			assert.Equal(t, guest.StateExited, ended.State)
			assert.Equal(t, 1, ended.ExitCode)
			assert.Equal(t, uint64(1), ended.Generation)
			assert.False(t, ended.FinishedAt.IsZero())

			written := make(map[string]string)
			for _, line := range a.logs(t, 0) {
				written[line.Content] = line.Stream
			}

			assert.Equal(t, map[string]string{"out": guest.StreamStdout, "err": guest.StreamStderr}, written)

			var status guest.Status
			require.Equal(t, http.StatusOK, a.call(t, http.MethodGet, "/process", nil, &status).StatusCode)
			assert.Equal(t, ended, status)
		})

		t.Run("a task that returns nothing returns zero", func(t *testing.T) {
			a := configured(t, confine(t))

			a.start(t, guest.Process{Args: []string{"true"}})

			assert.Equal(t, 0, a.waitFor(t, 1).ExitCode)
		})

		t.Run("a task runs with its environment, where it was told, and with what every process is given", func(t *testing.T) {
			a := configured(t, confine(t))
			dir := t.TempDir()

			a.start(t, guest.Process{
				Args:       []string{"/bin/sh", "-c", `echo "$GREETING $HOSTNAME $(pwd)"; test -n "$PATH" && test -n "$HOME"`},
				Env:        []string{"GREETING=hello"},
				WorkingDir: dir,
			})

			require.Equal(t, 0, a.waitFor(t, 1).ExitCode)

			lines := a.logs(t, 0)
			require.Len(t, lines, 1)
			assert.Equal(t, "hello task-xkfqz "+dir, lines[0].Content)
		})

		t.Run("a task killed returns 128 and the signal", func(t *testing.T) {
			a := configured(t, confine(t))

			a.start(t, guest.Process{Args: []string{"sleep", "60"}})

			assert.Equal(t, http.StatusNoContent, a.call(t, http.MethodPost, "/process/signal", guest.Signal{Signal: 9}, nil).StatusCode)

			assert.Equal(t, 137, a.waitFor(t, 1).ExitCode)
		})

		t.Run("a task stopped is asked to end, and ends", func(t *testing.T) {
			a := configured(t, confine(t))

			a.start(t, guest.Process{Args: []string{"sleep", "60"}})

			var status guest.Status
			require.Equal(t, http.StatusOK, a.call(t, http.MethodPost, "/process/stop", guest.Stop{Timeout: 10 * time.Second}, &status).StatusCode)

			assert.Equal(t, guest.StateExited, status.State)
			assert.Equal(t, 143, status.ExitCode, "it ended on TERM, well before it would have been made to")
		})

		t.Run("a task that will not end when asked is made to, with everything it started", func(t *testing.T) {
			a := configured(t, confine(t))

			a.start(t, guest.Process{Args: []string{"/bin/sh", "-c", `trap "" TERM; echo $$; sleep 60 & echo $!; wait`}})

			pids := a.pidsWritten(t, 2)

			began := time.Now()

			var status guest.Status
			require.Equal(t, http.StatusOK, a.call(t, http.MethodPost, "/process/stop", guest.Stop{Timeout: 300 * time.Millisecond}, &status).StatusCode)

			assert.Equal(t, 137, status.ExitCode)
			assert.GreaterOrEqual(t, time.Since(began), 300*time.Millisecond, "it was given its time first")

			for _, pid := range pids {
				eventually(t, func() bool { return gone(pid) }, "everything the task started is ended with it")
			}
		})

		t.Run("when the task's process ends, whatever it started ends with it", func(t *testing.T) {
			a := configured(t, confine(t))

			a.start(t, guest.Process{Args: []string{"/bin/sh", "-c", "sleep 60 & echo $!"}})

			pids := a.pidsWritten(t, 1)

			assert.Equal(t, 0, a.waitFor(t, 1).ExitCode)

			eventually(t, func() bool { return gone(pids[0]) }, "what the task left running went with it")
		})

		t.Run("a task that ended is started again, as the next run", func(t *testing.T) {
			a := configured(t, confine(t))

			a.start(t, guest.Process{Args: []string{"/bin/sh", "-c", "exit 3"}})
			require.Equal(t, 3, a.waitFor(t, 1).ExitCode)

			again := a.start(t, guest.Process{Args: []string{"sleep", "60"}})
			assert.Equal(t, uint64(2), again.Generation)

			status := a.waitFor(t, 1)
			assert.Equal(t, uint64(2), status.Generation, "waiting for a run that is over is answered at once")
			assert.Equal(t, guest.StateRunning, status.State)
		})

		t.Run("what cannot be asked of a task that is running, or not, is refused", func(t *testing.T) {
			a := newTestAgent(t, confine(t))

			assert.Equal(t, http.StatusConflict, a.call(t, http.MethodPost, "/process", guest.Process{Args: []string{"true"}}, nil).StatusCode, "a machine not told what it is runs nothing")

			require.Equal(t, http.StatusNoContent, a.call(t, http.MethodPut, "/config", testConfig(), nil).StatusCode)

			assert.Equal(t, http.StatusConflict, a.call(t, http.MethodGet, "/process/wait?generation=1", nil, nil).StatusCode, "nothing was started to wait for")
			assert.Equal(t, http.StatusBadRequest, a.call(t, http.MethodGet, "/process/wait", nil, nil).StatusCode)
			assert.Equal(t, http.StatusConflict, a.call(t, http.MethodPost, "/process/signal", guest.Signal{Signal: 15}, nil).StatusCode)

			assert.Equal(t, http.StatusUnprocessableEntity, a.call(t, http.MethodPost, "/process", guest.Process{}, nil).StatusCode)
			assert.Equal(t, http.StatusUnprocessableEntity, a.call(t, http.MethodPost, "/process", guest.Process{Args: []string{"no-such-command-anywhere"}}, nil).StatusCode)

			var status guest.Status
			require.Equal(t, http.StatusOK, a.call(t, http.MethodPost, "/process/stop", guest.Stop{}, &status).StatusCode)
			assert.Equal(t, guest.StateCreated, status.State, "a task that is not running has nothing to stop")

			a.start(t, guest.Process{Args: []string{"sleep", "60"}})

			assert.Equal(t, http.StatusConflict, a.call(t, http.MethodPost, "/process", guest.Process{Args: []string{"true"}}, nil).StatusCode, "a running task is not started twice")
			assert.Equal(t, http.StatusBadRequest, a.call(t, http.MethodPost, "/process/signal", guest.Signal{Signal: 0}, nil).StatusCode)
			assert.Equal(t, http.StatusBadRequest, a.call(t, http.MethodPost, "/process/signal", guest.Signal{Signal: 65}, nil).StatusCode)
		})
	})
}

func TestLogs(t *testing.T) {
	t.Run("lines are read after the one asked about", func(t *testing.T) {
		a := configured(t, newProcessGroups())

		a.start(t, guest.Process{Args: []string{"/bin/sh", "-c", "echo one; echo two; echo three"}})
		a.waitFor(t, 1)

		lines := a.logs(t, 2)
		require.Len(t, lines, 1)
		assert.Equal(t, "three", lines[0].Content)
		assert.Equal(t, uint64(3), lines[0].Seq)

		assert.Empty(t, a.logs(t, 3))
		assert.Equal(t, http.StatusBadRequest, a.call(t, http.MethodGet, "/logs?after=last", nil, nil).StatusCode)
	})

	t.Run("followed, lines are sent as they come", func(t *testing.T) {
		a := configured(t, newProcessGroups())

		a.start(t, guest.Process{Args: []string{"/bin/sh", "-c", "for i in 1 2 3; do echo line $i; sleep 0.2; done; sleep 60"}})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.server.URL+"/logs?after=1&follow=1", nil)
		require.NoError(t, err)

		response, err := a.server.Client().Do(request)
		require.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, "application/x-ndjson", response.Header.Get("Content-Type"))
		assert.Equal(t, guest.ProtocolVersion, response.Header.Get(guest.VersionHeader))

		lines := make(chan guest.LogLine)
		go func() {
			defer close(lines)

			scanner := bufio.NewScanner(response.Body)
			for scanner.Scan() {
				var line guest.LogLine
				if json.Unmarshal(scanner.Bytes(), &line) == nil {
					lines <- line
				}
			}
		}()

		for _, expected := range []string{"line 2", "line 3"} {
			select {
			case line := <-lines:
				assert.Equal(t, expected, line.Content)
			case <-time.After(5 * time.Second):
				t.Fatalf("%q was not sent as it came", expected)
			}
		}

		select {
		case line, open := <-lines:
			if open {
				t.Fatalf("a line nobody wrote was sent: %q", line.Content)
			}

			t.Fatal("a followed stream ended while the task still ran")
		case <-time.After(300 * time.Millisecond):
		}
	})
}

func TestStatsAnswer(t *testing.T) {
	forEachConfinement(t, func(t *testing.T, confine func(t *testing.T) confinement) {
		c := confine(t)
		a := configured(t, c)

		a.start(t, guest.Process{Args: []string{"sleep", "60"}})

		var stats guest.Stats
		require.Equal(t, http.StatusOK, a.call(t, http.MethodGet, "/stats", nil, &stats).StatusCode)

		assert.NotZero(t, stats.MemoryLimit)
		assert.LessOrEqual(t, stats.MemoryUsage, stats.MemoryLimit)

		if cgroups, ok := c.(cgroups); ok && readCounter(cgroups.task()+"/pids.current") > 0 {
			assert.NotZero(t, stats.PIDs, "the task's processes are counted")
		}
	})
}

// pidsWritten reads the first count lines the task wrote as process numbers.
func (a *testAgent) pidsWritten(t *testing.T, count int) []int {
	t.Helper()

	var pids []int

	eventually(t, func() bool {
		pids = pids[:0]
		for _, line := range a.logs(t, 0) {
			if pid, err := strconv.Atoi(strings.TrimSpace(line.Content)); err == nil {
				pids = append(pids, pid)
			}
		}

		return len(pids) >= count
	}, "the task said which processes it is")

	return pids
}
