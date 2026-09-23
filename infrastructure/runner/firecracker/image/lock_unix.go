//go:build unix

package image

import (
	"os"

	"golang.org/x/sys/unix"
)

// lock holds a lock file for as long as the returned function is not called,
// waiting for anybody else holding it first. Whoever held it and went away
// without letting go lets go of it by going away.
func lock(path string) (func(), error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}

	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		file.Close()

		return nil, err
	}

	return func() {
		_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
		file.Close()
	}, nil
}
