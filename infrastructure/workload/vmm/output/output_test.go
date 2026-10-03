package output

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/layout"
)

const id = "0123456789abcdef"

func line(seq uint64, content string) vm.LogLine {
	return vm.LogLine{Seq: seq, Stream: guest.StreamStdout, At: time.Unix(int64(seq), 0).UTC(), Content: content}
}

func contents(t *testing.T, reader vm.LogReader) []string {
	t.Helper()

	var read []string
	require.NoError(t, reader.Next(func(l vm.LogLine) error {
		read = append(read, l.Content)

		return nil
	}))

	return read
}

func TestStore(t *testing.T) {
	t.Parallel()

	t.Run("a line kept once is not kept again, and a gap says what was lost", func(t *testing.T) {
		t.Parallel()

		store := NewStore(t.TempDir(), 32<<20)

		out, err := store.Writer(id)
		require.NoError(t, err)

		require.NoError(t, out.Add(line(1, "one")))
		require.NoError(t, out.Add(line(1, "one again")))
		require.NoError(t, out.Add(line(4, "four")))
		require.NoError(t, out.Close())
		require.NoError(t, out.Close(), "closing twice is closing once")

		assert.Equal(t, []string{"one", "[vmhost] 2 lines were lost while nobody was reading them", "four"}, contents(t, store.Reader(id)))

		reopened, err := store.Writer(id)
		require.NoError(t, err)
		assert.Equal(t, uint64(4), reopened.Last(), "what was kept is known again once it is opened again")
		require.NoError(t, reopened.Close())
	})

	t.Run("a reader reads what comes as it comes, across the output being moved aside", func(t *testing.T) {
		t.Parallel()

		store := NewStore(t.TempDir(), 32<<20)

		out, err := store.Writer(id)
		require.NoError(t, err)
		defer out.Close()

		// small enough that every few lines are moved aside.
		out.(*writer).limit = 200

		reader := store.Reader(id)

		require.NoError(t, out.Add(line(1, "one")))
		assert.Equal(t, []string{"one"}, contents(t, reader))

		var expected []string
		for seq := uint64(2); seq <= 4; seq++ {
			require.NoError(t, out.Add(line(seq, fmt.Sprint("line ", seq))))
			expected = append(expected, fmt.Sprint("line ", seq))
		}

		assert.Equal(t, expected, contents(t, reader))
		assert.Empty(t, contents(t, reader), "nothing is read twice")

		_, err = os.Stat(layout.Output(store.dataDir, id) + rotatedSuffix)
		assert.NoError(t, err, "the output was moved aside")
	})

	t.Run("whoever waits is told when a line is kept", func(t *testing.T) {
		t.Parallel()

		out, err := NewStore(t.TempDir(), 32<<20).Writer(id)
		require.NoError(t, err)
		defer out.Close()

		waiting := out.Changed()
		require.NoError(t, out.Add(line(1, "now")))

		select {
		case <-waiting:
		case <-time.After(time.Second):
			t.Fatal("whoever waited was not told")
		}
	})

	t.Run("a vm that kept nothing reads nothing", func(t *testing.T) {
		t.Parallel()

		store := NewStore(t.TempDir(), 32<<20)

		assert.Empty(t, contents(t, store.Reader(id)))
		assert.Empty(t, contents(t, store.Reader("../../etc")))

		_, err := store.Writer("../../etc")
		assert.ErrorIs(t, err, vm.ErrInvalid)
	})

	t.Run("one file holds half of what a vm may keep, and never less than a long line", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, int64(16<<20), NewStore(t.TempDir(), 32<<20).fileSize)
		assert.Equal(t, int64(minFileSize), NewStore(t.TempDir(), 1024).fileSize)
	})

	t.Run("a closed writer keeps nothing more", func(t *testing.T) {
		t.Parallel()

		out, err := NewStore(t.TempDir(), 32<<20).Writer(id)
		require.NoError(t, err)
		require.NoError(t, out.Close())

		assert.ErrorIs(t, out.Add(line(1, "late")), os.ErrClosed)
	})
}
