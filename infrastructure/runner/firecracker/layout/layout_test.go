package layout

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepare(t *testing.T) {
	t.Run("the state directory is laid out, and the kernel installed where machines boot it", func(t *testing.T) {
		state := filepath.Join(t.TempDir(), "runner")

		kernel := filepath.Join(t.TempDir(), "vmlinux")
		require.NoError(t, os.WriteFile(kernel, []byte("a kernel"), 0o600))

		require.NoError(t, Prepare(state, kernel, os.Getuid(), os.Getgid()))

		for _, dir := range []string{BootDir, ImagesDir, NodesDir} {
			info, err := os.Stat(filepath.Join(state, dir))
			require.NoError(t, err)
			assert.True(t, info.IsDir())
			assert.Equal(t, os.FileMode(0o750), info.Mode().Perm(), "what the orchestrators write is theirs alone")
		}

		info, err := os.Stat(state)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o711), info.Mode().Perm(), "gone through by everybody, listed by nobody")

		installed, err := os.ReadFile(Kernel(state))
		require.NoError(t, err)
		assert.Equal(t, "a kernel", string(installed))

		info, err = os.Stat(Kernel(state))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o644), info.Mode().Perm(), "every machine reads the kernel")

		require.NoError(t, Prepare(state, kernel, os.Getuid(), os.Getgid()), "a state directory laid out already is taken as it is")
	})

	t.Run("a state directory that is not a directory is refused", func(t *testing.T) {
		state := filepath.Join(t.TempDir(), "runner")
		require.NoError(t, os.WriteFile(state, nil, 0o644))

		assert.Error(t, Prepare(state, "", os.Getuid(), os.Getgid()))
	})

	t.Run("an orchestrator's machines are its own", func(t *testing.T) {
		assert.Equal(t, "/var/lib/runner/nodes/runner-orchestrator-01/machines", Machines("/var/lib/runner", "runner-orchestrator-01"))
	})
}
