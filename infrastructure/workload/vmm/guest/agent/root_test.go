//go:build linux

package agent

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
)

// privateMounts gives the test's goroutine a mount namespace of its own, in
// which nothing is shared with the host, or skips where it may have none.
//
// A host's mounts are usually shared, as systemd makes them, and so are copies
// of them: the /dev the agent binds into a root, recursively, would be a peer
// of the host's own /dev/pts and /dev/shm, and taking the root apart again
// would take those apart on the host too. So the goroutine keeps a thread of
// its own, which is never let go: it ends with the test, and its namespace,
// and whatever is still mounted there, with it.
func privateMounts(t *testing.T) {
	t.Helper()

	if os.Geteuid() != 0 {
		t.Skip("mounting needs root")
	}

	runtime.LockOSThread()

	if err := unix.Unshare(unix.CLONE_NEWNS); err != nil {
		t.Skipf("no mount namespace can be made here: %v", err)
	}

	require.NoError(t, unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""))
}

// scratch is a tmpfs of its own, taken away again before anything removes
// the directory it is on: the /dev bound into a root is the host's devtmpfs
// itself, and removing a directory with it still mounted inside would remove
// the host's devices.
func scratch(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, unix.Mount("tmpfs", dir, "tmpfs", 0, "mode=0755"))

	// cleanups run last first, so this runs before the directory is removed.
	t.Cleanup(func() { _ = unix.Unmount(dir, unix.MNT_DETACH) })

	return dir
}

// scratchRoot is a tmpfs standing in for a task's root, in a mount namespace
// of the test's own.
func scratchRoot(t *testing.T) string {
	t.Helper()

	privateMounts(t)

	return scratch(t)
}

// mountsUnder is every mount point under dir that the test process's own
// mount namespace has, which is the host's for a goroutine that never gave
// its thread a namespace of its own.
func mountsUnder(t *testing.T, dir string) []string {
	t.Helper()

	content, err := os.ReadFile("/proc/self/mountinfo")
	require.NoError(t, err)

	var points []string
	for _, line := range strings.Split(string(content), "\n") {
		if fields := strings.Fields(line); len(fields) > 4 && strings.HasPrefix(fields[4], dir) {
			points = append(points, fields[4])
		}
	}

	slices.Sort(points)

	return points
}

func TestRootMounts(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// whatever the cases mount and take apart, the host's own mounts stay as
	// they were.
	before := mountsUnder(t, "/dev")
	t.Cleanup(func() {
		assert.Equal(t, before, mountsUnder(t, "/dev"), "the host's mounts were left as they were")
	})

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
		outside := scratch(t)

		require.NoError(t, os.Symlink(outside, filepath.Join(root, "proc")))

		require.NoError(t, mountInsideRoot(logger, root, false, true))

		entries, err := os.ReadDir(outside)
		require.NoError(t, err)
		assert.Empty(t, entries, "nothing was mounted where the link points")
	})
}
