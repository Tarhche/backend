// Package command asks the node holding a VM for what the VM is to become.
//
// It is one place because it is asked for from many: when a VM is created,
// started, stopped, restarted, changed, restored or deleted, and again by the
// control plane's own heartbeat when a VM has drifted from what was asked of
// it. Every node hears every command and acts only on those addressed to it, so
// a command names its node the way a scheduled task does.
package command

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
)

// Commander asks nodes for what their VMs are to become.
type Commander struct {
	producer domain.Producer
}

func New(producer domain.Producer) *Commander {
	return &Commander{producer: producer}
}

// Spec is what a VM is asked for as. It is the same whether the VM is being
// made, changed or restored, so a node never has to piece it together.
//
// Its labels are how the node tells, from the engine alone, that the instance
// is a VM, whose it is and what its ports are served under.
func Spec(v *vm.VM) vm.Spec {
	return vm.Spec{
		ID:             v.UUID,
		Kind:           v.Kind,
		Image:          v.Image,
		Resources:      v.Resources,
		Ports:          slices.Clone(v.Ports),
		Network:        v.Network,
		PersistentDisk: v.PersistentDisk,
		Labels: map[string]string{
			vm.LabelOwner:   v.OwnerUUID,
			vm.LabelVM:      v.UUID,
			vm.LabelSlug:    v.Slug,
			vm.LabelPurpose: vm.PurposeVM,
		},
	}
}

// Schedule asks the VM's node to make it and boot it: from its image, or
// from a snapshot's archive when snapshotUUID names one.
func (c *Commander) Schedule(ctx context.Context, v *vm.VM, snapshotUUID string) error {
	return c.produce(ctx, events.VMScheduledName, events.VMScheduled{
		VMUUID:       v.UUID,
		NodeName:     v.NodeName,
		Spec:         events.NewSpec(Spec(v)),
		SnapshotUUID: snapshotUUID,
	})
}

func (c *Commander) Start(ctx context.Context, v *vm.VM) error {
	return c.produce(ctx, events.VMStartRequestedName, events.VMStartRequested{VMUUID: v.UUID, NodeName: v.NodeName})
}

func (c *Commander) Stop(ctx context.Context, v *vm.VM) error {
	return c.produce(ctx, events.VMStopRequestedName, events.VMStopRequested{VMUUID: v.UUID, NodeName: v.NodeName})
}

func (c *Commander) Restart(ctx context.Context, v *vm.VM) error {
	return c.produce(ctx, events.VMRestartRequestedName, events.VMRestartRequested{VMUUID: v.UUID, NodeName: v.NodeName})
}

// Reconfigure asks the VM's node to give it the ports, network and resources
// it now has, restarting it when the engine has to.
func (c *Commander) Reconfigure(ctx context.Context, v *vm.VM) error {
	return c.produce(ctx, events.VMReconfigureRequestedName, events.VMReconfigureRequested{
		VMUUID:   v.UUID,
		NodeName: v.NodeName,
		Spec:     events.NewSpec(Spec(v)),
	})
}

// Restore asks the VM's node to replace its disk from a snapshot. The VM keeps
// its uuid, its slug and its ports.
func (c *Commander) Restore(ctx context.Context, v *vm.VM, snapshotUUID string) error {
	return c.produce(ctx, events.VMRestoreRequestedName, events.VMRestoreRequested{
		VMUUID:       v.UUID,
		NodeName:     v.NodeName,
		SnapshotUUID: snapshotUUID,
		Spec:         events.NewSpec(Spec(v)),
	})
}

// Delete asks a node to remove a VM, disk and all. It names the VM rather than
// taking it, because a node holding an instance the control plane has no record
// of is asked to remove it too.
func (c *Commander) Delete(ctx context.Context, uuid string, nodeName string) error {
	return c.produce(ctx, events.VMDeleteRequestedName, events.VMDeleteRequested{VMUUID: uuid, NodeName: nodeName})
}

// produce publishes a command. What it is asked for has already been written
// down by the time it is published, so a caller that has gone away does not get
// to take the command back with it.
func (c *Commander) produce(ctx context.Context, subject string, command any) error {
	payload, err := json.Marshal(command)
	if err != nil {
		return err
	}

	return c.producer.Produce(context.WithoutCancel(ctx), subject, payload)
}
