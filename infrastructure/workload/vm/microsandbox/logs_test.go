package microsandbox

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestSourceOf(t *testing.T) {
	t.Parallel()

	session := func(n uint64) *uint64 { return &n }

	for _, tt := range []struct {
		name    string
		entry   logEntry
		hasMain bool
		want    string
	}{
		{
			name:  "the runtime's tracing is the runtime's",
			entry: logEntry{source: "system", data: []byte("INFO microsandbox_runtime::runner::vm: sandbox starting sandbox=vm-1")},
			want:  vm.LogSourceRuntime,
		},
		{
			name:  "tracing padded to its level's width is the runtime's too",
			entry: logEntry{source: "system", data: []byte(" WARN microsandbox_checkpoint_timing: slow")},
			want:  vm.LogSourceRuntime,
		},
		{
			name:  "the markers of a sandbox starting and stopping are the runtime's",
			entry: logEntry{source: "system", data: []byte("--- sandbox restored ---\n")},
			want:  vm.LogSourceRuntime,
		},
		{
			name:  "everything else the system says is the guest's console",
			entry: logEntry{source: "system", data: []byte("2026-10-05T01:37:37Z vminit[1 --pid1]: dockerd started (pid 223)")},
			want:  vm.LogSourceKernel,
		},
		{
			name:    "the first session of a boot is the main process's",
			entry:   logEntry{source: "stdout", session: session(1), data: []byte("1\n")},
			hasMain: true,
			want:    vm.LogSourceMain,
		},
		{
			name:    "its errors are the main process's too",
			entry:   logEntry{source: "stderr", session: session(1), data: []byte("boom\n")},
			hasMain: true,
			want:    vm.LogSourceMain,
		},
		{
			name:    "any other session is an exec's",
			entry:   logEntry{source: "stdout", session: session(2), data: []byte("x")},
			hasMain: true,
			want:    vm.LogSourceExec,
		},
		{
			name:  "an instance with no main process has only execs",
			entry: logEntry{source: "output", session: session(1), data: []byte("x")},
			want:  vm.LogSourceExec,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, sourceOf(tt.entry, tt.hasMain))
		})
	}
}

func TestLinesOf(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)
	session := uint64(2)

	lines := linesOf([]logEntry{
		{source: "stdout", session: &session, at: at, data: []byte("one\ntwo\n")},
		{source: "output", session: &session, at: at.Add(time.Second), data: []byte("tty\r\n")},
		{source: "stdout", session: &session, at: at.Add(2 * time.Second), data: []byte("no newline")},
		{source: "stdout", session: &session, at: at.Add(3 * time.Second), data: []byte("\n")},
		{source: "stdout", session: &session, at: at.Add(4 * time.Second), data: nil},
	}, false)

	assert.Equal(t, []vm.LogLine{
		{At: at, Source: vm.LogSourceExec, Line: "one"},
		{At: at, Source: vm.LogSourceExec, Line: "two"},
		{At: at.Add(time.Second), Source: vm.LogSourceExec, Line: "tty"},
		{At: at.Add(2 * time.Second), Source: vm.LogSourceExec, Line: "no newline"},
		{At: at.Add(3 * time.Second), Source: vm.LogSourceExec, Line: ""},
	}, lines)
}

func TestWindow(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)

	lines := []vm.LogLine{
		{At: at, Line: "a"},
		{At: at.Add(time.Millisecond), Line: "b"},
		{At: at.Add(time.Millisecond), Line: "c"},
		{At: at.Add(2 * time.Millisecond), Line: "d"},
	}

	line := func(lines []vm.LogLine) []string {
		out := make([]string, len(lines))
		for n, l := range lines {
			out[n] = l.Line
		}

		return out
	}

	assert.Equal(t, []string{"a", "b", "c", "d"}, line(window(lines, vm.LogOptions{})))
	assert.Equal(t, []string{"b", "c", "d"}, line(window(lines, vm.LogOptions{Since: at.Add(time.Millisecond)})), "since is inclusive")
	assert.Equal(t, []string{"c", "d"}, line(window(lines, vm.LogOptions{Tail: 2})))
	assert.Equal(t, []string{"d"}, line(window(lines, vm.LogOptions{Since: at.Add(time.Millisecond), Tail: 1})))
	assert.Equal(t, []string{"a", "b", "c", "d"}, line(window(lines, vm.LogOptions{Tail: 10})))
}

func TestMerged(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)

	merged := merged(
		[]vm.LogLine{{At: at, Line: "a"}, {At: at.Add(2 * time.Second), Line: "c"}},
		[]vm.LogLine{{At: at.Add(time.Second), Line: "b"}},
	)

	assert.Equal(t, []vm.LogLine{{At: at, Line: "a"}, {At: at.Add(time.Second), Line: "b"}, {At: at.Add(2 * time.Second), Line: "c"}}, merged)
}
