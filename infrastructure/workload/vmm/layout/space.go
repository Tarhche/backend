//go:build linux || darwin

package layout

import "golang.org/x/sys/unix"

// FreeSpace is how many bytes the filesystem the data directory is on still
// has for vmhost: what an unprivileged writer could still write there, so
// what root alone may write is not counted as room.
func FreeSpace(dataDir string) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(dataDir, &stat); err != nil {
		return 0, err
	}

	return uint64(stat.Bavail) * uint64(stat.Bsize), nil
}
