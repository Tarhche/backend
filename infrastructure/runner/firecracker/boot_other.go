//go:build !linux

package firecracker

import (
	"context"
	"errors"

	"github.com/khanzadimahdi/testproject/domain/runner/machine"
)

// boot boots nothing: a microVM needs linux's KVM under it.
func (r *Runtime) boot(ctx context.Context, launched machine.Machine, rec record) error {
	return errors.New("microVMs only boot on linux")
}
