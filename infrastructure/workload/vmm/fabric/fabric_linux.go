//go:build linux

package fabric

import (
	"context"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// New prepares machines' networks in the namespace vmhost runs in: forwarding
// on, and the firewall as the networks already there say it should be.
func New(config Config, logger *slog.Logger) (*Fabric, error) {
	return &Fabric{config: config, logger: logger}, nil
}

func (f *Fabric) EnsureNetwork(ctx context.Context, name string, masquerade bool) (vm.Network, error) {
	return vm.Network{}, errNotImplemented
}

func (f *Fabric) RemoveNetwork(ctx context.Context, name string) error {
	return errNotImplemented
}

func (f *Fabric) Networks(ctx context.Context) ([]vm.Network, error) {
	return nil, errNotImplemented
}

func (f *Fabric) Plug(ctx context.Context, id string, uid int, attachments []vm.Attachment) ([]vm.Interface, error) {
	return nil, errNotImplemented
}

func (f *Fabric) Unplug(ctx context.Context, id string) error {
	return errNotImplemented
}

func (f *Fabric) Retain(ctx context.Context, ids []string) error {
	return errNotImplemented
}

func (f *Fabric) Repair(ctx context.Context) error {
	return errNotImplemented
}
