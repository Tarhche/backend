package sdk

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
)

// fakeStream is a stream whose items the test feeds in. It records what the
// process asked of it, and whether close came while recv was still waiting,
// which the SDK does not allow.
type fakeStream struct {
	items chan received

	inRecv           atomic.Int32
	closedDuringRecv atomic.Bool

	mu       sync.Mutex
	signals  []int
	resizes  [][2]uint16
	kills    int
	closes   int
	onKill   func()
	closeErr error
	callErr  error
}

type received struct {
	it  item
	err error
}

func newFakeStream() *fakeStream {
	return &fakeStream{items: make(chan received, 1024)}
}

func (f *fakeStream) feed(items ...item) {
	for _, it := range items {
		f.items <- received{it: it}
	}
}

func (f *fakeStream) fail(err error) {
	f.items <- received{err: err}
}

func (f *fakeStream) recv(ctx context.Context) (item, error) {
	f.inRecv.Add(1)
	defer f.inRecv.Add(-1)

	select {
	case r := <-f.items:
		return r.it, r.err
	case <-ctx.Done():
		return item{}, ctx.Err()
	}
}

func (f *fakeStream) signal(ctx context.Context, signal int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.signals = append(f.signals, signal)

	return f.callErr
}

func (f *fakeStream) resize(ctx context.Context, rows, cols uint16) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resizes = append(f.resizes, [2]uint16{rows, cols})

	return f.callErr
}

func (f *fakeStream) kill(ctx context.Context) error {
	f.mu.Lock()
	f.kills++
	onKill := f.onKill
	f.mu.Unlock()

	if onKill != nil {
		onKill()
	}

	return nil
}

func (f *fakeStream) close() error {
	if f.inRecv.Load() > 0 {
		f.closedDuringRecv.Store(true)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.closes++

	return f.closeErr
}

func (f *fakeStream) counts() (kills, closes int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.kills, f.closes
}

func started() item {
	return item{event: runs.Event{Kind: runs.EventStarted}}
}

func stdout(data string) item {
	return item{event: runs.Event{Kind: runs.EventStdout, Data: []byte(data)}}
}

func exited(code int) item {
	return item{event: runs.Event{Kind: runs.EventExited, ExitCode: code}}
}

func done() item {
	return item{done: true}
}

// all reads a process's events until they are closed.
func all(t *testing.T, p *process) []runs.Event {
	t.Helper()

	var events []runs.Event
	timeout := time.After(10 * time.Second)
	for {
		select {
		case event, ok := <-p.Events():
			if !ok {
				return events
			}
			events = append(events, event)
		case <-timeout:
			t.Fatalf("the events were not closed; got %v", events)
		}
	}
}

func kinds(events []runs.Event) []runs.EventKind {
	var out []runs.EventKind
	for _, event := range events {
		out = append(out, event.Kind)
	}

	return out
}

func TestProcessEvents(t *testing.T) {
	t.Parallel()

	t.Run("the exit comes last, after output that followed it", func(t *testing.T) {
		t.Parallel()

		s := newFakeStream()
		s.feed(started(), stdout("a"), exited(3), stdout("late"), done())

		events := all(t, newProcess(s, nil, runs.Command{}))
		assert.Equal(t, []runs.EventKind{runs.EventStarted, runs.EventStdout, runs.EventStdout, runs.EventExited}, kinds(events))
		assert.Equal(t, "late", string(events[2].Data))
		assert.Equal(t, 3, events[3].ExitCode)
	})

	t.Run("a signal's -1 is handed on as it is", func(t *testing.T) {
		t.Parallel()

		s := newFakeStream()
		s.feed(started(), exited(-1), done())

		events := all(t, newProcess(s, nil, runs.Command{}))
		assert.Equal(t, runs.Event{Kind: runs.EventExited, ExitCode: -1}, events[len(events)-1])
	})

	t.Run("a command that never started fails", func(t *testing.T) {
		t.Parallel()

		s := newFakeStream()
		failed := runs.Event{Kind: runs.EventFailed, Errno: "ENOENT", Message: `spawn "x": No such file or directory`}
		s.feed(item{event: failed}, done())

		assert.Equal(t, []runs.Event{failed}, all(t, newProcess(s, nil, runs.Command{})))
	})

	t.Run("a stream that ends without an exit is lost", func(t *testing.T) {
		t.Parallel()

		s := newFakeStream()
		s.feed(started(), stdout("a"), done())

		events := all(t, newProcess(s, nil, runs.Command{}))
		assert.Equal(t, []runs.EventKind{runs.EventStarted, runs.EventStdout, runs.EventLost}, kinds(events))
		assert.NotEmpty(t, events[2].Message)
	})

	t.Run("a stream that breaks is lost, with why", func(t *testing.T) {
		t.Parallel()

		s := newFakeStream()
		s.feed(started())
		s.fail(errors.New("agent connection closed"))

		events := all(t, newProcess(s, nil, runs.Command{}))
		assert.Equal(t, runs.Event{Kind: runs.EventLost, Message: "agent connection closed"}, events[len(events)-1])
	})

	t.Run("an exit whose stream never ends is still reported", func(t *testing.T) {
		t.Parallel()

		s := newFakeStream()
		s.feed(started(), exited(0))

		p := startProcess(s, nil, runs.Command{}, 50*time.Millisecond, time.Second)
		assert.Equal(t, []runs.EventKind{runs.EventStarted, runs.EventExited}, kinds(all(t, p)))
	})

	t.Run("an exit after an exit is not reported", func(t *testing.T) {
		t.Parallel()

		s := newFakeStream()
		s.feed(exited(1), exited(2), done())

		assert.Equal(t, []runs.Event{{Kind: runs.EventExited, ExitCode: 1}}, all(t, newProcess(s, nil, runs.Command{})))
	})

	t.Run("what the port has no event for is left out", func(t *testing.T) {
		t.Parallel()

		s := newFakeStream()
		s.feed(started(), item{skip: true}, exited(0), done())

		assert.Equal(t, []runs.EventKind{runs.EventStarted, runs.EventExited}, kinds(all(t, newProcess(s, nil, runs.Command{}))))
	})
}

func TestProcessTerminalSize(t *testing.T) {
	t.Parallel()

	t.Run("a terminal is sized before the start is handed on", func(t *testing.T) {
		t.Parallel()

		s := newFakeStream()
		s.feed(started())
		p := newProcess(s, nil, runs.Command{TTY: true, Rows: 40, Cols: 100})

		event := <-p.Events()
		assert.Equal(t, runs.EventStarted, event.Kind)

		s.mu.Lock()
		assert.Equal(t, [][2]uint16{{40, 100}}, s.resizes)
		s.mu.Unlock()

		s.feed(exited(0), done())
		all(t, p)
	})

	for name, command := range map[string]runs.Command{
		"without a terminal":    {Rows: 40, Cols: 100},
		"without a size":        {TTY: true},
		"with only half a size": {TTY: true, Rows: 40},
	} {
		t.Run("nothing is sized "+name, func(t *testing.T) {
			t.Parallel()

			s := newFakeStream()
			s.feed(started(), exited(0), done())
			all(t, newProcess(s, nil, command))

			s.mu.Lock()
			defer s.mu.Unlock()
			assert.Empty(t, s.resizes)
		})
	}
}

func TestProcessCalls(t *testing.T) {
	t.Parallel()

	t.Run("signals and resizes reach the stream", func(t *testing.T) {
		t.Parallel()

		s := newFakeStream()
		p := newProcess(s, nil, runs.Command{})

		require.NoError(t, p.Signal(context.Background(), syscall.SIGTERM))
		require.NoError(t, p.Signal(context.Background(), syscall.Signal(9)))
		require.NoError(t, p.Resize(context.Background(), 50, 132))

		s.mu.Lock()
		assert.Equal(t, []int{15, 9}, s.signals)
		assert.Equal(t, [][2]uint16{{50, 132}}, s.resizes)
		s.mu.Unlock()

		s.feed(exited(0), done())
		all(t, p)
		require.NoError(t, p.Close())
	})

	t.Run("their errors say what was asked", func(t *testing.T) {
		t.Parallel()

		s := newFakeStream()
		s.callErr = errors.New("invalid handle")
		p := newProcess(s, nil, runs.Command{})

		assert.ErrorContains(t, p.Signal(context.Background(), syscall.SIGINT), "signal 2: invalid handle")
		assert.ErrorContains(t, p.Resize(context.Background(), 1, 2), "resize to 1x2: invalid handle")

		s.feed(exited(0), done())
		all(t, p)
	})

	t.Run("stdin is nil unless there is one", func(t *testing.T) {
		t.Parallel()

		s := newFakeStream()
		p := newProcess(s, nil, runs.Command{})
		assert.Nil(t, p.Stdin())

		var w io.WriteCloser = nopWriteCloser{}
		withStdin := startProcess(newFakeStream(), w, runs.Command{Stdin: true}, time.Second, 10*time.Millisecond)
		assert.Equal(t, w, withStdin.Stdin())
		require.NoError(t, withStdin.Close())

		s.feed(exited(0), done())
		all(t, p)
	})
}

type nopWriteCloser struct{}

func (nopWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopWriteCloser) Close() error                { return nil }

func TestProcessClose(t *testing.T) {
	t.Parallel()

	t.Run("a running command is killed, and its exit still comes last", func(t *testing.T) {
		t.Parallel()

		s := newFakeStream()
		s.onKill = func() { s.feed(exited(-1), done()) }
		s.feed(started())
		p := newProcess(s, nil, runs.Command{})
		require.Equal(t, runs.EventStarted, (<-p.Events()).Kind)

		require.NoError(t, p.Close())

		assert.Equal(t, []runs.Event{{Kind: runs.EventExited, ExitCode: -1}}, all(t, p))
		kills, closes := s.counts()
		assert.Equal(t, 1, kills)
		assert.Equal(t, 1, closes)
		assert.False(t, s.closedDuringRecv.Load(), "close came while recv was waiting")
	})

	t.Run("a command that has ended is not killed", func(t *testing.T) {
		t.Parallel()

		s := newFakeStream()
		s.feed(started(), exited(0), done())
		p := newProcess(s, nil, runs.Command{})
		all(t, p)

		require.NoError(t, p.Close())

		kills, closes := s.counts()
		assert.Equal(t, 0, kills)
		assert.Equal(t, 1, closes)
	})

	t.Run("a kill that is never reported does not hold Close up", func(t *testing.T) {
		t.Parallel()

		s := newFakeStream()
		s.feed(started())
		p := startProcess(s, nil, runs.Command{}, time.Second, 50*time.Millisecond)

		closed := make(chan error)
		go func() { closed <- p.Close() }()

		select {
		case err := <-closed:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("Close did not return")
		}

		events := all(t, p)
		assert.Equal(t, runs.EventStarted, events[0].Kind)
		assert.False(t, s.closedDuringRecv.Load(), "close came while recv was waiting")
	})

	t.Run("nobody reading the events does not hold Close up", func(t *testing.T) {
		t.Parallel()

		s := newFakeStream()
		s.onKill = func() { s.feed(exited(-1), done()) }
		s.feed(started())
		for range eventBuffer * 2 {
			s.feed(stdout("x"))
		}
		p := startProcess(s, nil, runs.Command{}, time.Second, 50*time.Millisecond)

		closed := make(chan error)
		go func() { closed <- p.Close() }()

		select {
		case err := <-closed:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("Close did not return")
		}

		all(t, p)
		assert.False(t, s.closedDuringRecv.Load(), "close came while recv was waiting")
	})

	t.Run("it closes once and says why it could not", func(t *testing.T) {
		t.Parallel()

		s := newFakeStream()
		s.closeErr = errors.New("invalid handle")
		s.feed(exited(0), done())
		p := newProcess(s, nil, runs.Command{})
		all(t, p)

		first, second := p.Close(), p.Close()
		assert.ErrorContains(t, first, "invalid handle")
		assert.Equal(t, first, second)

		_, closes := s.counts()
		assert.Equal(t, 1, closes)
	})

	t.Run("closing at the same time from several places is one close", func(t *testing.T) {
		t.Parallel()

		s := newFakeStream()
		s.onKill = func() { s.feed(exited(-1), done()) }
		s.feed(started())
		p := newProcess(s, nil, runs.Command{})

		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				assert.NoError(t, p.Close())
			}()
		}
		wg.Wait()

		kills, closes := s.counts()
		assert.Equal(t, 1, kills)
		assert.Equal(t, 1, closes)
	})
}

func TestProcessLostMessageNamesTheCause(t *testing.T) {
	t.Parallel()

	s := newFakeStream()
	s.feed(done())

	events := all(t, newProcess(s, nil, runs.Command{}))
	require.Len(t, events, 1)
	assert.True(t, strings.Contains(events[0].Message, "without an exit"), events[0].Message)
}
