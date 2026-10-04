package journal

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

var epoch = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

// lines are count lines, a nanosecond apart from from, each content followed
// by its number.
func lines(from int, count int, content string) []api.LogLine {
	var lines []api.LogLine

	for i := from; i < from+count; i++ {
		lines = append(lines, api.LogLine{
			Stream:  api.StreamStdout,
			At:      epoch.Add(time.Duration(i)),
			Content: fmt.Sprintf("%s%d", content, i),
		})
	}

	return lines
}

// readAll reads a journal to the end of what has been written.
func readAll(t *testing.T, j *Journal, id string, since time.Time) []api.LogLine {
	t.Helper()

	reader, err := j.Reader(id, since)
	require.NoError(t, err)
	defer reader.Close()

	var read []api.LogLine

	for {
		line, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return read
		}

		require.NoError(t, err)

		read = append(read, line)
	}
}

func newJournal(t *testing.T, capacity int64) (*Journal, string) {
	t.Helper()

	directory := filepath.Join(t.TempDir(), "logs")

	j, err := New(directory, capacity)
	require.NoError(t, err)

	t.Cleanup(func() { _ = j.Close() })

	return j, directory
}

func TestJournal(t *testing.T) {
	t.Parallel()

	t.Run("what is appended is read back, in order", func(t *testing.T) {
		t.Parallel()

		j, _ := newJournal(t, DefaultCapacity)

		require.NoError(t, j.Append("run", lines(0, 3, "line ")))
		require.NoError(t, j.Append("run", []api.LogLine{{Stream: api.StreamStderr, At: epoch.Add(3), Content: "a <tag> & \"quotes\""}}))

		read := readAll(t, j, "run", time.Time{})

		assert.Equal(t, append(lines(0, 3, "line "), api.LogLine{Stream: api.StreamStderr, At: epoch.Add(3), Content: "a <tag> & \"quotes\""}), read)
	})

	t.Run("reading from a stamp gives that line and everything after it", func(t *testing.T) {
		t.Parallel()

		j, _ := newJournal(t, DefaultCapacity)

		require.NoError(t, j.Append("run", lines(0, 10, "line ")))

		read := readAll(t, j, "run", epoch.Add(7))

		assert.Equal(t, lines(7, 3, "line "), read)
	})

	t.Run("a run with no journal has an empty one, which fills as the run writes", func(t *testing.T) {
		t.Parallel()

		j, _ := newJournal(t, DefaultCapacity)

		reader, err := j.Reader("run", time.Time{})
		require.NoError(t, err)
		defer reader.Close()

		_, err = reader.Next()
		assert.ErrorIs(t, err, io.EOF)

		require.NoError(t, j.Append("run", lines(0, 1, "late ")))

		line, err := reader.Next()
		require.NoError(t, err)
		assert.Equal(t, "late 0", line.Content)
	})

	t.Run("a reader at the end carries on once more is written", func(t *testing.T) {
		t.Parallel()

		j, _ := newJournal(t, DefaultCapacity)

		require.NoError(t, j.Append("run", lines(0, 1, "line ")))

		reader, err := j.Reader("run", time.Time{})
		require.NoError(t, err)
		defer reader.Close()

		first, err := reader.Next()
		require.NoError(t, err)
		assert.Equal(t, "line 0", first.Content)

		_, err = reader.Next()
		assert.ErrorIs(t, err, io.EOF)

		require.NoError(t, j.Append("run", lines(1, 1, "line ")))

		second, err := reader.Next()
		require.NoError(t, err)
		assert.Equal(t, "line 1", second.Content)
	})

	t.Run("a line half written is not read until it is whole", func(t *testing.T) {
		t.Parallel()

		j, directory := newJournal(t, DefaultCapacity)

		require.NoError(t, j.Append("run", lines(0, 1, "line ")))

		reader, err := j.Reader("run", time.Time{})
		require.NoError(t, err)
		defer reader.Close()

		_, err = reader.Next()
		require.NoError(t, err)

		// a writer caught in the middle of a line.
		file, err := os.OpenFile(filepath.Join(directory, "run.1.ndjson"), os.O_WRONLY|os.O_APPEND, 0o600)
		require.NoError(t, err)
		defer file.Close()

		_, err = file.WriteString(`{"stream":"stdout","at":"2026-10-04T12:00:00.000000001Z",`)
		require.NoError(t, err)

		_, err = reader.Next()
		assert.ErrorIs(t, err, io.EOF)

		_, err = file.WriteString(`"content":"whole"}` + "\n")
		require.NoError(t, err)

		line, err := reader.Next()
		require.NoError(t, err)
		assert.Equal(t, "whole", line.Content)
	})

	t.Run("a full journal keeps its newest lines, and between half and all of its capacity", func(t *testing.T) {
		t.Parallel()

		const capacity = 8 << 10

		j, directory := newJournal(t, capacity)

		for i := range 400 {
			require.NoError(t, j.Append("run", lines(i, 1, strings.Repeat("x", 40)+" ")))
		}

		read := readAll(t, j, "run", time.Time{})

		require.NotEmpty(t, read)
		assert.Equal(t, fmt.Sprintf("%s 399", strings.Repeat("x", 40)), read[len(read)-1].Content, "the newest line is kept")

		var size int64
		segments, err := filepath.Glob(filepath.Join(directory, "run.*.ndjson"))
		require.NoError(t, err)
		require.Len(t, segments, 2)

		for _, segment := range segments {
			info, err := os.Stat(segment)
			require.NoError(t, err)

			size += info.Size()
		}

		assert.LessOrEqual(t, size, int64(capacity))
		assert.GreaterOrEqual(t, size, int64(capacity/2))

		for i := 1; i < len(read); i++ {
			assert.True(t, read[i].At.After(read[i-1].At), "lines are in order across segments")
		}
	})

	t.Run("a reader follows the journal from one segment to the next", func(t *testing.T) {
		t.Parallel()

		j, _ := newJournal(t, 2<<10)

		reader, err := j.Reader("run", time.Time{})
		require.NoError(t, err)
		defer reader.Close()

		var (
			read []api.LogLine
			wg   sync.WaitGroup
		)

		const count = 200

		written := make(chan struct{})

		wg.Go(func() {
			defer close(written)

			for i := range count {
				assert.NoError(t, j.Append("run", lines(i, 1, "line ")))
			}
		})

		for len(read) < count {
			line, err := reader.Next()
			if errors.Is(err, io.EOF) {
				select {
				case <-written:
				default:
					time.Sleep(time.Millisecond)

					continue
				}

				// once everything is written, an EOF is the end, apart from
				// what was lost to rotation while this was behind.
				line, err = reader.Next()
				if errors.Is(err, io.EOF) {
					break
				}
			}

			require.NoError(t, err)

			read = append(read, line)
		}

		wg.Wait()

		require.NotEmpty(t, read)
		assert.Equal(t, "line 199", read[len(read)-1].Content)

		for i := 1; i < len(read); i++ {
			assert.True(t, read[i].At.After(read[i-1].At), "nothing is read twice or out of order")
		}
	})

	t.Run("reading from a stamp skips the segments before it", func(t *testing.T) {
		t.Parallel()

		j, _ := newJournal(t, 4<<10)

		for i := range 100 {
			require.NoError(t, j.Append("run", lines(i, 1, "line ")))
		}

		read := readAll(t, j, "run", epoch.Add(95))

		assert.Equal(t, lines(95, 5, "line "), read)
	})

	t.Run("the last stamp is the newest line's", func(t *testing.T) {
		t.Parallel()

		j, directory := newJournal(t, DefaultCapacity)

		last, err := j.Last("run")
		require.NoError(t, err)
		assert.True(t, last.IsZero(), "a run with no journal has no last line")

		require.NoError(t, j.Append("run", lines(0, 5, "line ")))

		last, err = j.Last("run")
		require.NoError(t, err)
		assert.Equal(t, epoch.Add(4), last)

		// a journal whose writer died in the middle of a line.
		file, err := os.OpenFile(filepath.Join(directory, "run.1.ndjson"), os.O_WRONLY|os.O_APPEND, 0o600)
		require.NoError(t, err)
		_, err = file.WriteString(`{"stream":"stdout","at":"2027`)
		require.NoError(t, err)
		require.NoError(t, file.Close())

		again, err := New(directory, DefaultCapacity)
		require.NoError(t, err)

		last, err = again.Last("run")
		require.NoError(t, err)
		assert.Equal(t, epoch.Add(4), last)
	})

	t.Run("a journal survives the service, and carries on where it ended", func(t *testing.T) {
		t.Parallel()

		j, directory := newJournal(t, DefaultCapacity)

		require.NoError(t, j.Append("run", lines(0, 2, "before ")))
		require.NoError(t, j.Close())

		again, err := New(directory, DefaultCapacity)
		require.NoError(t, err)

		require.NoError(t, again.Append("run", lines(2, 1, "after ")))

		read := readAll(t, again, "run", time.Time{})

		assert.Equal(t, []string{"before 0", "before 1", "after 2"}, []string{read[0].Content, read[1].Content, read[2].Content})
	})

	t.Run("a deleted journal is gone, and deleting it again is no error", func(t *testing.T) {
		t.Parallel()

		j, directory := newJournal(t, 2<<10)

		for i := range 100 {
			require.NoError(t, j.Append("run", lines(i, 1, "line ")))
		}

		require.NoError(t, j.Append("other", lines(0, 1, "line ")))

		require.NoError(t, j.Delete("run"))
		require.NoError(t, j.Delete("run"))

		segments, err := filepath.Glob(filepath.Join(directory, "run.*"))
		require.NoError(t, err)
		assert.Empty(t, segments)

		assert.Empty(t, readAll(t, j, "run", time.Time{}))
		assert.Len(t, readAll(t, j, "other", time.Time{}), 1, "other runs' journals are not touched")

		require.NoError(t, j.Append("run", lines(0, 1, "new ")))
		assert.Len(t, readAll(t, j, "run", time.Time{}), 1)
	})

	t.Run("an ID that could climb out of the directory is refused", func(t *testing.T) {
		t.Parallel()

		j, _ := newJournal(t, DefaultCapacity)

		assert.Error(t, j.Append("../escape", lines(0, 1, "x")))

		_, err := j.Reader("../escape", time.Time{})
		assert.Error(t, err)

		_, err = j.Last("a/b")
		assert.Error(t, err)

		assert.Error(t, j.Delete(""))
	})

	t.Run("a line that cannot be read is skipped", func(t *testing.T) {
		t.Parallel()

		j, directory := newJournal(t, DefaultCapacity)

		require.NoError(t, j.Append("run", lines(0, 1, "line ")))
		require.NoError(t, j.Close())

		file, err := os.OpenFile(filepath.Join(directory, "run.1.ndjson"), os.O_WRONLY|os.O_APPEND, 0o600)
		require.NoError(t, err)
		_, err = file.WriteString("garbage\n")
		require.NoError(t, err)
		require.NoError(t, file.Close())

		require.NoError(t, j.Append("run", lines(1, 1, "line ")))

		read := readAll(t, j, "run", time.Time{})

		assert.Equal(t, lines(0, 2, "line "), read)
	})

	t.Run("a capacity that holds nothing is refused", func(t *testing.T) {
		t.Parallel()

		_, err := New(t.TempDir(), 1)

		assert.Error(t, err)
	})
}
