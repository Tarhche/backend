// Package vmcommand is what every command a node carries out on a VM does the
// same way: giving the VM the spec it was sent, restoring a disk from a
// snapshot, and saying what became of it.
package vmcommand

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
)

// Forgetter lets go of what this node keeps open into a VM: a client of a
// Docker VM's dockerd, whose connections are commands running inside the VM
// and so do not outlive it going down. Whatever stops, restarts, replaces or
// removes a VM has it let go of, so the next request opens fresh ones rather
// than failing on the dead.
type Forgetter interface {
	Forget(vmUUID string)
}

// Spec is the spec a node was sent for a VM, as the node gives it to its
// engine.
//
// It is named by the VM's uuid, which is what the engine calls the VM from
// then on, and it is labelled as a VM, which is what puts it in the node's
// heartbeats, and as a Docker VM when it is one, which is how the node tells
// the VMs whose dockerds it reads from the rest. Whose it is and the slug its
// ports are served under are the control plane's to say, and come with the
// labels it sent.
func Spec(vmUUID string, sent vm.Spec) vm.Spec {
	spec := sent
	spec.ID = vmUUID
	spec.Ports = slices.Clone(sent.Ports)
	spec.Command = slices.Clone(sent.Command)
	spec.Env = slices.Clone(sent.Env)

	spec.Labels = maps.Clone(sent.Labels)
	if spec.Labels == nil {
		spec.Labels = make(map[string]string, 2)
	}

	spec.Labels[vm.LabelVM] = vmUUID
	spec.Labels[vm.LabelPurpose] = vm.PurposeVM

	if spec.Kind == vm.KindDocker {
		spec.Labels[docker.LabelVM] = "true"
	}

	return spec
}

// Restore gives the instance spec names the disk a snapshot holds: the archive
// is read from where snapshots are stored and streamed into the engine as it
// arrives, so it is never held whole.
func Restore(ctx context.Context, engine vm.Engine, store snapshot.Store, spec vm.Spec, snapshotUUID string) error {
	archive, err := store.Read(ctx, snapshot.ObjectKey(snapshotUUID))
	if err != nil {
		return fmt.Errorf("the snapshot cannot be read: %w", err)
	}
	defer archive.Close()

	if _, err := engine.Restore(ctx, spec, archive); err != nil {
		return err
	}

	return nil
}

// Failed says this node could not do what it was asked of a VM, and why.
//
// It is said rather than returned: an error asks for the message to be
// delivered again, and an engine that refused once refuses again, so the VM
// would be retried behind everybody's back while nobody is told. What is
// returned is only a failure to say so, which is worth another delivery.
func Failed(ctx context.Context, producer domain.Producer, nodeName string, vmUUID string, cause error) error {
	return Publish(ctx, producer, events.VMFailedName, events.VMFailed{
		VMUUID:   vmUUID,
		NodeName: nodeName,
		Reason:   cause.Error(),
		At:       time.Now(),
	})
}

// Refused says this node would not do what it was asked of a VM, because the
// command did not say what it had to. A command that did not even name its VM
// has nobody to say it to, and one that was not refused has nothing to say.
func Refused(ctx context.Context, producer domain.Producer, nodeName string, vmUUID string, validationErrors domain.ValidationErrors) error {
	if len(validationErrors) == 0 || len(vmUUID) == 0 {
		return nil
	}

	fields := slices.Sorted(maps.Keys(validationErrors))

	reasons := make([]string, len(fields))
	for n, field := range fields {
		reasons[n] = field + ": " + validationErrors[field]
	}

	return Failed(ctx, producer, nodeName, vmUUID, fmt.Errorf("the command was refused: %s", strings.Join(reasons, "; ")))
}

// Publish sends one event. It is detached from the command's context, so what
// became of a command is said even when whoever was waiting has gone.
func Publish(ctx context.Context, producer domain.Producer, subject string, event any) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return producer.Produce(context.WithoutCancel(ctx), subject, payload)
}
