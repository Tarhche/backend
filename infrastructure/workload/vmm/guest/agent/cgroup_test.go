//go:build linux

package agent

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLimitMemory(t *testing.T) {
	t.Parallel()

	t.Run("the task may have what the machine has left once it is up, short of the agent's own", func(t *testing.T) {
		t.Parallel()

		task := t.TempDir()

		limitMemory(task, "MemTotal:  109660 kB\nMemFree:  80000 kB\nMemAvailable:  86448 kB\n")

		limit, err := os.ReadFile(filepath.Join(task, "memory.max"))
		require.NoError(t, err)
		assert.Equal(t, strconv.FormatUint(86448*1024-taskMemoryReserve, 10), string(limit))
	})

	t.Run("a machine with no more than the agent keeps leaves the task unlimited rather than unable to run", func(t *testing.T) {
		t.Parallel()

		task := t.TempDir()

		limitMemory(task, "MemTotal:  16384 kB\nMemAvailable:  4096 kB\n")

		_, err := os.Stat(filepath.Join(task, "memory.max"))
		assert.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("a machine that says nothing of its memory leaves the task as it was", func(t *testing.T) {
		t.Parallel()

		task := t.TempDir()

		limitMemory(task, "")

		_, err := os.Stat(filepath.Join(task, "memory.max"))
		assert.ErrorIs(t, err, os.ErrNotExist)
	})
}
