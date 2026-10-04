//go:build !unix

package image

import (
	"errors"
	"fmt"
)

// lock refuses: images are only made where files can be locked.
func lock(path string) (func(), error) {
	return nil, fmt.Errorf("%w: %s cannot be locked here", errors.ErrUnsupported, path)
}
