package firecracker

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/guest"
)

func line(seq uint64, content string) guest.LogLine {
	return guest.LogLine{Seq: seq, Stream: guest.StreamStdout, At: time.Unix(int64(seq), 0).UTC(), Content: content}
}

func contents(t *testing.T, f *follower) []string {
	t.Helper()

	var read []string
	require.NoError(t, f.next(func(l guest.LogLine) error {
		read = append(read, l.Content)

		return nil
	}))

	return read
}

func TestOutput(t *testing.T) {
	t.Run("a line kept once is not kept again, and a gap says what was lost", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), outputName)

		out, err := openOutput(path, outputLimit)
		require.NoError(t, err)

		require.NoError(t, out.add(line(1, "one")))
		require.NoError(t, out.add(line(1, "one again")))
		require.NoError(t, out.add(line(4, "four")))
		require.NoError(t, out.close())

		f := &follower{path: path}
		assert.Equal(t, []string{"one", "[runner] 2 lines were lost while nobody was reading them", "four"}, contents(t, f))

		reopened, err := openOutput(path, outputLimit)
		require.NoError(t, err)
		assert.Equal(t, uint64(4), reopened.lastSeq(), "what was kept is known again once it is opened again")
		require.NoError(t, reopened.close())
	})

	t.Run("a follower reads what comes as it comes, across the output being moved aside", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), outputName)

		// small enough that every few lines are moved aside.
		out, err := openOutput(path, 200)
		require.NoError(t, err)
		defer out.close()

		f := &follower{path: path}

		require.NoError(t, out.add(line(1, "one")))
		assert.Equal(t, []string{"one"}, contents(t, f))

		var expected []string
		for seq := uint64(2); seq <= 4; seq++ {
			require.NoError(t, out.add(line(seq, fmt.Sprint("line ", seq))))
			expected = append(expected, fmt.Sprint("line ", seq))
		}

		assert.Equal(t, expected, contents(t, f))
		assert.Empty(t, contents(t, f), "nothing is read twice")
	})

	t.Run("whoever waits is told when a line is kept", func(t *testing.T) {
		out, err := openOutput(filepath.Join(t.TempDir(), outputName), outputLimit)
		require.NoError(t, err)
		defer out.close()

		waiting := out.wait()
		require.NoError(t, out.add(line(1, "now")))

		select {
		case <-waiting:
		case <-time.After(time.Second):
			t.Fatal("whoever waited was not told")
		}
	})
}
