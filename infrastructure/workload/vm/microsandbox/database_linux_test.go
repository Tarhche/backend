//go:build linux

package microsandbox

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestHoldDatabase(t *testing.T) {
	t.Parallel()

	t.Run("a connection's lock goes when anything else in its process closes the database", func(t *testing.T) {
		t.Parallel()

		path := database(t)

		// what a SQLite connection holds while it is open in WAL mode.
		connection := opened(t, path, os.O_RDONLY)
		shared := unix.Flock_t{Type: unix.F_RDLCK, Whence: int16(io.SeekStart), Start: sqliteSharedFirst, Len: sqliteSharedSize}
		require.NoError(t, unix.FcntlFlock(connection.Fd(), unix.F_SETLK, &shared))

		probe := opened(t, path, os.O_RDWR)
		require.False(t, last(t, probe), "an open connection is not the last one")

		identityCheck(t, path)

		assert.True(t, last(t, probe), "whoever closes next takes itself for the last one, though this connection is still open")
	})

	t.Run("held, nobody is the last one, whatever else closes it, until it is let go of", func(t *testing.T) {
		t.Parallel()

		path := database(t)

		held, err := holdDatabase(path)
		require.NoError(t, err)

		probe := opened(t, path, os.O_RDWR)
		assert.False(t, last(t, probe))

		identityCheck(t, path)

		assert.False(t, last(t, probe), "closing another descriptor leaves it held")

		require.NoError(t, held.Close())
		assert.True(t, last(t, probe))
	})

	t.Run("a database that is not there is not held", func(t *testing.T) {
		t.Parallel()

		_, err := holdDatabase(filepath.Join(t.TempDir(), "msb.db"))
		assert.ErrorIs(t, err, os.ErrNotExist)
	})
}

// database is a file standing for microsandbox's database.
func database(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "msb.db")
	require.NoError(t, os.WriteFile(path, []byte("SQLite format 3\x00"), 0o600))

	return path
}

func opened(t *testing.T, path string, flag int) *os.File {
	t.Helper()

	file, err := os.OpenFile(path, flag, 0)
	require.NoError(t, err)

	t.Cleanup(func() { _ = file.Close() })

	return file
}

// identityCheck is what microsandbox 0.7.6 does to tell its database apart
// from a replaced one: it opens it, and closes it.
func identityCheck(t *testing.T, path string) {
	t.Helper()

	file, err := os.Open(path)
	require.NoError(t, err)
	require.NoError(t, file.Close())
}

// last reports whether a connection closing now would take itself for the
// last one: whether the write lock it takes on the bytes every connection
// holds a read lock on could be had. It is asked as an open file description,
// whose locks conflict with every other lock, this process's own included,
// and it is asked through a descriptor that stays open, since closing one
// would release the very locks it asks about.
func last(t *testing.T, probe *os.File) bool {
	t.Helper()

	lock := unix.Flock_t{Type: unix.F_WRLCK, Whence: int16(io.SeekStart), Start: sqliteSharedFirst, Len: sqliteSharedSize}
	require.NoError(t, unix.FcntlFlock(probe.Fd(), unix.F_OFD_GETLK, &lock))

	return lock.Type == unix.F_UNLCK
}
