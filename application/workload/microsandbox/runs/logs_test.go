package runs_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// collect reads a run's log, without following it.
func (h *harness) collect(id string, since time.Time) []api.LogLine {
	h.t.Helper()

	var lines []api.LogLine

	err := h.supervisor.Logs(context.Background(), id, since, false, func(line api.LogLine) error {
		lines = append(lines, line)

		return nil
	})
	require.NoError(h.t, err)

	return lines
}

// contents are lines without their stamps.
func contents(lines []api.LogLine) []string {
	var contents []string

	for _, line := range lines {
		contents = append(contents, line.Stream+": "+line.Content)
	}

	return contents
}

func TestLogs(t *testing.T) {
	t.Parallel()

	t.Run("output is split into lines, whatever pieces it arrives in", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("job"))
		main := h.main(run.ID, 1)

		main.Write("hel")
		main.Write("lo\nwor")
		main.WriteErr("oops\n")
		main.Write("ld\n\nlast without a newline")
		main.Exit(0)

		h.waitFor(run.ID, api.StateExited)

		assert.Equal(t, []string{
			"stdout: hello",
			"stderr: oops",
			"stdout: world",
			"stdout: ",
			"stdout: last without a newline",
		}, contents(h.collect(run.ID, time.Time{})))
	})

	t.Run("stamps are unique and strictly increasing", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("job"))
		main := h.main(run.ID, 1)

		main.Write(strings.Repeat("line\n", 500))
		main.Exit(0)

		h.waitFor(run.ID, api.StateExited)

		lines := h.collect(run.ID, time.Time{})
		require.Len(t, lines, 500)

		for i := 1; i < len(lines); i++ {
			assert.True(t, lines[i].At.After(lines[i-1].At), "line %d does not come after the one before it", i)
		}
	})

	t.Run("reading from a line's stamp gives that line and everything after it", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("job"))
		main := h.main(run.ID, 1)

		main.Write("one\ntwo\nthree\n")
		main.Exit(0)

		h.waitFor(run.ID, api.StateExited)

		lines := h.collect(run.ID, time.Time{})
		require.Len(t, lines, 3)

		assert.Equal(t, []string{"stdout: two", "stdout: three"}, contents(h.collect(run.ID, lines[1].At)))
	})

	t.Run("a line longer than the most a line holds is cut", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("job"))
		main := h.main(run.ID, 1)

		main.Write(strings.Repeat("x", api.MaxLogContent+5))
		main.Write("\n")
		main.Exit(0)

		h.waitFor(run.ID, api.StateExited)

		lines := h.collect(run.ID, time.Time{})
		require.Len(t, lines, 2)
		assert.Len(t, lines[0].Content, api.MaxLogContent)
		assert.Equal(t, "xxxxx", lines[1].Content)
	})

	t.Run("the journal is kept across the run's restarts", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web", withPolicy("on-failure")))

		h.main(run.ID, 1).Write("first run\n")
		h.main(run.ID, 1).Exit(1)
		h.main(run.ID, 2).Write("second run\n")

		require.Eventually(t, func() bool { return len(h.collect(run.ID, time.Time{})) == 2 }, eventually, tick)

		assert.Equal(t, []string{"stdout: first run", "stdout: second run"}, contents(h.collect(run.ID, time.Time{})))
	})

	t.Run("following hands lines over as they are written, and ends with the run", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web"))
		main := h.main(run.ID, 1)
		main.Write("before\n")

		var (
			mu    sync.Mutex
			lines []string
		)

		done := make(chan error, 1)
		go func() {
			done <- h.supervisor.Logs(context.Background(), run.ID, time.Time{}, true, func(line api.LogLine) error {
				mu.Lock()
				defer mu.Unlock()

				lines = append(lines, line.Content)

				return nil
			})
		}()

		received := func(n int) func() bool {
			return func() bool {
				mu.Lock()
				defer mu.Unlock()

				return len(lines) == n
			}
		}

		require.Eventually(t, received(1), eventually, tick)

		main.Write("during\n")
		require.Eventually(t, received(2), eventually, tick)

		select {
		case <-done:
			t.Fatal("following ended while the run was running")
		case <-time.After(20 * time.Millisecond):
		}

		main.Write("at the end, without a newline")
		main.Exit(0)

		require.NoError(t, <-done)

		assert.Equal(t, []string{"before", "during", "at the end, without a newline"}, lines)
	})

	t.Run("following a run that is not up reads what there is and ends", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.create(spec("web"))

		err := h.supervisor.Logs(context.Background(), run.ID, time.Time{}, true, func(api.LogLine) error { return nil })

		assert.NoError(t, err)
	})

	t.Run("following ends when the caller goes away", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web"))

		ctx, cancel := context.WithCancel(context.Background())

		done := make(chan error, 1)
		go func() {
			done <- h.supervisor.Logs(ctx, run.ID, time.Time{}, true, func(api.LogLine) error { return nil })
		}()

		cancel()

		assert.NoError(t, <-done)
	})

	t.Run("following ends when the run is deleted", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web"))
		h.main(run.ID, 1).Write("following\n")

		following := make(chan struct{})
		done := make(chan error, 1)

		go func() {
			done <- h.supervisor.Logs(context.Background(), run.ID, time.Time{}, true, func(api.LogLine) error {
				close(following)

				return nil
			})
		}()

		<-following

		require.NoError(t, h.supervisor.Delete(context.Background(), run.ID))

		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(eventually):
			t.Fatal("following outlived the run")
		}
	})

	t.Run("a line the caller refuses ends the stream with its reason", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		run := h.started(spec("web"))
		h.main(run.ID, 1).Write("one\ntwo\n")

		require.Eventually(t, func() bool { return len(h.collect(run.ID, time.Time{})) == 2 }, eventually, tick)

		refused := errors.New("enough")
		calls := 0

		err := h.supervisor.Logs(context.Background(), run.ID, time.Time{}, true, func(api.LogLine) error {
			calls++

			return refused
		})

		assert.ErrorIs(t, err, refused)
		assert.Equal(t, 1, calls)
	})

	t.Run("a run that is not there is not found", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		err := h.supervisor.Logs(context.Background(), "nope", time.Time{}, false, func(api.LogLine) error { return nil })

		assert.Equal(t, api.CodeNotFound, runs.Code(err))
	})
}
