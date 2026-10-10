//go:build linux

package microsandbox

import (
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// The bytes of a SQLite database a connection holds a read lock on for as
// long as it is open in WAL mode, and which a connection closing takes a
// write lock on to tell whether it is the last one: SHARED_FIRST and
// SHARED_SIZE in SQLite's os_unix.c.
const (
	sqliteSharedFirst = 0x40000000 + 2
	sqliteSharedSize  = 510
)

// holdDatabase holds microsandbox's database open as a connection does, until
// what it returns is closed: with a read lock on the bytes every connection
// holds one on, which nothing but closing that descriptor releases.
//
// microsandbox 0.7.6 tells its database apart from a replaced one by opening
// and closing it, and closing any descriptor of a file releases every POSIX
// lock the process holds on it, its SQLite connections' too
// (superradcompany/microsandbox#1709, fixed by #1766 after 0.7.7). A VM's
// msb process that closes its connection then, when the VM stops or every 30
// minutes, takes itself for the last one and removes the WAL and its index,
// while the vmhost's connections go on through the index removed: every query
// is answered "file is not a database" until the vmhost restarts. An open file
// description's lock is the descriptor's own, not the process's, so whatever
// else closes leaves it held, and no connection takes itself for the last one
// while the vmhost runs.
func holdDatabase(path string) (io.Closer, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	lock := unix.Flock_t{Type: unix.F_RDLCK, Whence: int16(io.SeekStart), Start: sqliteSharedFirst, Len: sqliteSharedSize}
	if err := unix.FcntlFlock(file.Fd(), unix.F_OFD_SETLK, &lock); err != nil {
		_ = file.Close()

		return nil, err
	}

	return file, nil
}
