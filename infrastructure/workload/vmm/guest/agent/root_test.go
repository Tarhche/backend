//go:build linux

package agent

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
)

// scratchRoot is a tmpfs standing in for a task's root, or a skip where the
// test may mount nothing.
func scratchRoot(t *testing.T) string {
	t.Helper()

	if os.Geteuid() != 0 {
		t.Skip("mounting needs root")
	}

	root := t.TempDir()
	if err := unix.Mount("tmpfs", root, "tmpfs", 0, "mode=0755"); err != nil {
		t.Skipf("nothing can be mounted here: %v", err)
	}

	// what is mounted inside goes with it.
	t.Cleanup(func() { _ = unix.Unmount(root, unix.MNT_DETACH) })

	return root
}

func TestRootMounts(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("a read-only root gets what a task expects mounted, its files bound, and is then made read only", func(t *testing.T) {
		root := scratchRoot(t)

		// an image with an /etc, a link where resolv.conf would be, and
		// nothing else a task expects.
		require.NoError(t, os.MkdirAll(filepath.Join(root, "etc"), 0o755))
		require.NoError(t, os.Symlink("/run/systemd/resolve/stub-resolv.conf", filepath.Join(root, "etc/resolv.conf")))

		files := t.TempDir()
		config := testConfig()
		require.NoError(t, writeFiles(files, config))

		require.NoError(t, mountInsideRoot(logger, root, true, true))
		require.NoError(t, bindFiles(logger, files, root, true))
		require.NoError(t, sealRoot(root))

		assert.FileExists(t, filepath.Join(root, "proc/self/stat"), "its processes are there")
		assert.DirExists(t, filepath.Join(root, "sys/kernel"), "the kernel's view of the machine is there")

		null, err := os.Stat(filepath.Join(root, "dev/null"))
		require.NoError(t, err)
		assert.Equal(t, os.ModeDevice|os.ModeCharDevice, null.Mode().Type(), "its devices are there")

		assert.NoError(t, os.WriteFile(filepath.Join(root, "tmp/scratch"), []byte("x"), 0o644), "what is temporary can be written")
		assert.NoError(t, os.WriteFile(filepath.Join(root, "run/app.pid"), []byte("1"), 0o644))

		err = os.WriteFile(filepath.Join(root, "written"), []byte("x"), 0o644)
		assert.True(t, errors.Is(err, unix.EROFS), "nothing else can be: %v", err)

		resolv, err := os.ReadFile(filepath.Join(root, "etc/resolv.conf"))
		require.NoError(t, err)
		assert.Equal(t, "nameserver 1.1.1.1\n", string(resolv), "the link was replaced by the agent's file")

		require.NoError(t, writeInPlace(filepath.Join(files, "hosts"), hostsFile("task-xkfqz", nil, []guest.Host{{Address: "10.0.0.3", Names: []string{"db"}}})))

		hosts, err := os.ReadFile(filepath.Join(root, "etc/hosts"))
		require.NoError(t, err)
		assert.Contains(t, string(hosts), "10.0.0.3\tdb\n", "the task sees its neighbours change")
	})

	t.Run("a root the agent can make nothing in goes without what it has no place for", func(t *testing.T) {
		root := scratchRoot(t)

		require.NoError(t, os.MkdirAll(filepath.Join(root, "proc"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(root, "etc"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "etc/hosts"), nil, 0o644))

		files := t.TempDir()
		require.NoError(t, writeFiles(files, testConfig()))

		require.NoError(t, mountInsideRoot(logger, root, true, false))
		require.NoError(t, bindFiles(logger, files, root, false))

		assert.FileExists(t, filepath.Join(root, "proc/self/stat"))
		assert.NoDirExists(t, filepath.Join(root, "sys"), "nothing was made")
		assert.NoFileExists(t, filepath.Join(root, "etc/resolv.conf"))

		hosts, err := os.ReadFile(filepath.Join(root, "etc/hosts"))
		require.NoError(t, err)
		assert.Contains(t, string(hosts), "localhost", "what has a place is bound")
	})

	t.Run("a link where something is mounted is not followed out of the root", func(t *testing.T) {
		root := scratchRoot(t)
		outside := t.TempDir()

		require.NoError(t, os.Symlink(outside, filepath.Join(root, "proc")))

		require.NoError(t, mountInsideRoot(logger, root, false, true))

		entries, err := os.ReadDir(outside)
		require.NoError(t, err)
		assert.Empty(t, entries, "nothing was mounted where the link points")
	})
}
