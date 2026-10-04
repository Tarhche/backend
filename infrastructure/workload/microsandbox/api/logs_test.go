package api_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

func TestLogStream(t *testing.T) {
	t.Parallel()

	// lines a nanosecond apart, as the service stamps lines read in the same
	// instant, an empty one, and one that has to be escaped to stay on its
	// line.
	lines := []api.LogLine{
		{
			Stream:  api.StreamStdout,
			At:      time.Date(2026, time.October, 4, 9, 30, 2, 0, time.UTC),
			Content: "/docker-entrypoint.sh: Configuration complete; ready for start up",
		},
		{
			Stream:  api.StreamStderr,
			At:      time.Date(2026, time.October, 4, 9, 30, 2, 1, time.UTC),
			Content: `2026/10/04 09:30:02 [notice] 1#1: using the "epoll" event method`,
		},
		{
			Stream: api.StreamStdout,
			At:     time.Date(2026, time.October, 4, 9, 30, 2, 2, time.UTC),
		},
		{
			Stream:  api.StreamStdout,
			At:      time.Date(2026, time.October, 4, 9, 30, 5, 100000000, time.UTC),
			Content: "10.89.0.21 - - [04/Oct/2026:09:30:05 +0000] \"GET / HTTP/1.1\" 200 615\t\"-\" \"curl/8.10.1\" é",
		},
	}

	pinned, err := os.ReadFile(filepath.Join("testdata", "logs.ndjson"))
	require.NoError(t, err)

	t.Run("each line is one compact value and a newline", func(t *testing.T) {
		t.Parallel()

		var stream bytes.Buffer

		encoder := json.NewEncoder(&stream)
		for _, line := range lines {
			require.NoError(t, encoder.Encode(line))
		}

		assert.Equal(t, string(pinned), stream.String())
	})

	t.Run("it reads back one line at a time", func(t *testing.T) {
		t.Parallel()

		decoder := json.NewDecoder(bytes.NewReader(pinned))

		var read []api.LogLine
		for {
			var line api.LogLine

			err := decoder.Decode(&line)
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(t, err)

			read = append(read, line)
		}

		assert.Equal(t, lines, read)
	})
}
