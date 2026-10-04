//go:build !unix

package image

import "os"

// allocated is how large a file says it is, where nothing says more.
func allocated(info os.FileInfo) int64 {
	return info.Size()
}
