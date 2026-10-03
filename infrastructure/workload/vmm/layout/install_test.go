package layout

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstall(t *testing.T) {
	t.Parallel()

	t.Run("a file is put in by what it holds, once, and a new one beside it", func(t *testing.T) {
		t.Parallel()

		sources := t.TempDir()
		dir := filepath.Join(t.TempDir(), BinDir)

		source := filepath.Join(sources, "firecracker")
		require.NoError(t, os.WriteFile(source, []byte("v1"), 0o600))

		first, err := Install(dir, source, "firecracker", 0o755)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(filepath.Base(first), "firecracker-"))

		installed, err := os.ReadFile(first)
		require.NoError(t, err)
		assert.Equal(t, "v1", string(installed))

		info, err := os.Stat(first)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())

		again, err := Install(dir, source, "firecracker", 0o755)
		require.NoError(t, err)
		assert.Equal(t, first, again, "the same file is put in once")

		require.NoError(t, os.WriteFile(source, []byte("v2"), 0o600))

		upgraded, err := Install(dir, source, "firecracker", 0o755)
		require.NoError(t, err)
		assert.NotEqual(t, first, upgraded, "an upgrade is put beside what was there")

		_, err = os.Stat(first)
		assert.NoError(t, err, "what a running machine was started from stays")
	})

	t.Run("a file that is not there is said to be missing", func(t *testing.T) {
		t.Parallel()

		_, err := Install(t.TempDir(), filepath.Join(t.TempDir(), "missing"), "vmlinux", 0o644)
		assert.ErrorIs(t, err, os.ErrNotExist)
	})
}

func TestFreeSpace(t *testing.T) {
	t.Parallel()

	free, err := FreeSpace(t.TempDir())
	require.NoError(t, err)
	assert.NotZero(t, free)
}
