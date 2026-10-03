//go:build linux

package firecracker

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/layout"
)

// sleeper starts a process the hypervisor takes for a machine's firecracker,
// which only sleeps, in dir, and ends it with the test.
func sleeper(t *testing.T, dir string) *exec.Cmd {
	t.Helper()

	binary := filepath.Join(t.TempDir(), testExecName)
	copyFile(t, "/bin/sleep", binary)

	return started(t, binary, dir)
}

// started starts a command in dir, and ends it with the test.
func started(t *testing.T, binary string, dir string) *exec.Cmd {
	t.Helper()

	command := exec.Command(binary, "60")
	command.Dir = dir
	require.NoError(t, command.Start())

	exited := make(chan struct{})
	go func() {
		_ = command.Wait()
		close(exited)
	}()

	t.Cleanup(func() {
		_ = command.Process.Kill()
		<-exited
	})

	return command
}

// ownedRoot makes a machine's own directory, owned by whoever runs the tests,
// which is who the machine runs as.
func ownedRoot(t *testing.T, dataDir string, id string) string {
	t.Helper()

	root := layout.MachineRoot(dataDir, id)
	require.NoError(t, os.MkdirAll(root, 0o700))

	return root
}

func TestProcesses(t *testing.T) {
	// machines found by their users are found among every process running as
	// that user, which in these tests is whoever runs them: none of these run
	// beside another, so none finds another's firecracker.

	t.Run("a machine that runs as a user of its own is found by that user, wherever it runs", func(t *testing.T) {
		h, dataDir := hypervisor(t, true)
		ownedRoot(t, dataDir, "0123456789abcdef")

		running := sleeper(t, "/")

		found, err := h.launcher.find(t.Context())
		require.NoError(t, err)
		assert.Equal(t, map[string]process{"0123456789abcdef": {running: true, pid: running.Process.Pid}}, found)
	})

	t.Run("what else runs as a machine's user is not taken for its firecracker", func(t *testing.T) {
		h, dataDir := hypervisor(t, true)
		ownedRoot(t, dataDir, "0123456789abcdef")

		started(t, "/bin/sleep", "/")

		found, err := h.launcher.find(t.Context())
		require.NoError(t, err)
		assert.Empty(t, found)
	})

	t.Run("a machine that runs as vmhost itself is found by the directory it was started in", func(t *testing.T) {
		h, dataDir := hypervisor(t, false)
		root := ownedRoot(t, dataDir, "0123456789abcdef")

		running := sleeper(t, root)
		sleeper(t, t.TempDir())

		found, err := h.launcher.find(t.Context())
		require.NoError(t, err)
		assert.Equal(t, map[string]process{"0123456789abcdef": {running: true, pid: running.Process.Pid}}, found)
	})

	t.Run("a machine's firecracker is stopped by its process, and a stopped one is no longer found", func(t *testing.T) {
		h, dataDir := hypervisor(t, true)
		ownedRoot(t, dataDir, "0123456789abcdef")
		sleeper(t, "/")

		require.NoError(t, h.launcher.stop(t.Context(), "0123456789abcdef"))

		found, err := h.launcher.find(t.Context())
		require.NoError(t, err)
		assert.Empty(t, found)

		require.NoError(t, h.launcher.stop(t.Context(), "0123456789abcdef"), "a machine that is not running is the outcome asked for")
	})
}

func TestRealUser(t *testing.T) {
	t.Parallel()

	user, ok := realUser(os.Getpid())

	assert.True(t, ok)
	assert.Equal(t, os.Getuid(), user)

	_, ok = realUser(0)
	assert.False(t, ok, "nothing runs as process 0")
}

func TestIsZombie(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "stat"), []byte("42 (fire cracker) Z 1 2 3"), 0o644))
	assert.True(t, isZombie(dir))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "stat"), []byte("42 (firecracker) S 1 2 3"), 0o644))
	assert.False(t, isZombie(dir))

	assert.True(t, isZombie(filepath.Join(dir, "gone")), "a process that is not there is not running")
}
