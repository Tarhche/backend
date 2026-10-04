//go:build unix

package image

import (
	"os"
	"syscall"
)

// allocated is how much of the disk a file takes, rather than how large it
// says it is: a sparse file takes only what has been written to it.
func allocated(info os.FileInfo) int64 {
	return info.Sys().(*syscall.Stat_t).Blocks * 512
}
