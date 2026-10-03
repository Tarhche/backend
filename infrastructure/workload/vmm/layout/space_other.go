//go:build !linux && !darwin

package layout

import (
	"errors"
	"fmt"
)

// FreeSpace cannot be told here: vmhost only runs on linux.
func FreeSpace(dataDir string) (uint64, error) {
	return 0, fmt.Errorf("%w: the free space of %s cannot be told here", errors.ErrUnsupported, dataDir)
}
