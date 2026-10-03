//go:build !linux

package fabric

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// errUnsupported is every answer off Linux: bridges, taps and the firewall
// are Linux's.
var errUnsupported = fmt.Errorf("%w: machines' networks are only made on linux", errors.ErrUnsupported)

// New refuses: machines' networks are only made on Linux.
func New(config Config, logger *slog.Logger) (*Fabric, error) {
	return nil, errUnsupported
}

func (f *Fabric) EnsureNetwork(ctx context.Context, name string, masquerade bool) (vm.Network, error) {
	return vm.Network{}, errUnsupported
}

func (f *Fabric) RemoveNetwork(ctx context.Context, name string) error {
	return errUnsupported
}

func (f *Fabric) Networks(ctx context.Context) ([]vm.Network, error) {
	return nil, errUnsupported
}

func (f *Fabric) Plug(ctx context.Context, id string, uid int, attachments []vm.Attachment) ([]vm.Interface, error) {
	return nil, errUnsupported
}

func (f *Fabric) Unplug(ctx context.Context, id string) error {
	return errUnsupported
}

func (f *Fabric) Retain(ctx context.Context, ids []string) error {
	return errUnsupported
}

func (f *Fabric) Repair(ctx context.Context) error {
	return errUnsupported
}
