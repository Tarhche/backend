// Package heartbeatVM hears what a node says, every beat, about itself and the
// VMs it holds.
package heartbeatVM

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/stack/dispatch"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/command"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/lifecycle"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
)

const (
	// restartGrace is how long a VM restarting is waited for before a node
	// that reports it running without saying it started again is believed.
	// Not every change a restart is asked for needs the engine to restart it.
	restartGrace = 2 * time.Minute

	// defaultReason is said of a VM whose engine did not say what went wrong.
	defaultReason = "the vm failed"
)

// Heartbeat writes down what a node says about the VMs it holds and what it
// offers to more of them.
//
// What a node says a VM is doing is the engine's word for it; what that makes
// the VM is decided here, since only the control plane knows what was asked of
// it. A VM on its way somewhere takes only the report that it has arrived: a
// heartbeat taken before a stop reached the node still says the VM is running,
// and believing it would undo the stop.
//
// A node holding a VM nobody has a record of is asked to remove it, and a VM
// on its way out that its node no longer lists is gone. Nothing here fails the
// message: the next beat says it all again.
type Heartbeat struct {
	vms        vm.Repository
	nodes      node.Repository
	lifecycle  *lifecycle.Lifecycle
	commander  *command.Commander
	dispatcher *dispatch.Dispatcher
	logger     *slog.Logger
}

var _ domain.MessageHandler = &Heartbeat{}

func NewHeartbeat(
	vms vm.Repository,
	nodes node.Repository,
	lifecycle *lifecycle.Lifecycle,
	commander *command.Commander,
	dispatcher *dispatch.Dispatcher,
	logger *slog.Logger,
) *Heartbeat {
	return &Heartbeat{
		vms:        vms,
		nodes:      nodes,
		lifecycle:  lifecycle,
		commander:  commander,
		dispatcher: dispatcher,
		logger:     logger,
	}
}

func (h *Heartbeat) Handle(ctx context.Context, data []byte) error {
	var heartbeat events.VMHeartbeat
	if err := json.Unmarshal(data, &heartbeat); err != nil {
		h.logger.ErrorContext(ctx, "a vm heartbeat that cannot be read", "error", err)

		return nil
	}

	if len(heartbeat.NodeName) == 0 {
		return nil
	}

	at := heartbeat.At
	if at.IsZero() {
		at = h.lifecycle.Now()
	}

	if err := h.capacity(ctx, heartbeat.NodeName, heartbeat.Capacity.ToVM(), at); err != nil {
		h.logger.ErrorContext(ctx, "could not write down what a node offers", "error", err, "node", heartbeat.NodeName)
	}

	listed := make(map[string]bool, len(heartbeat.VMs))
	for i := range heartbeat.VMs {
		listed[heartbeat.VMs[i].UUID] = true

		if err := h.beat(ctx, heartbeat.NodeName, at, &heartbeat.VMs[i]); err != nil {
			h.logger.WarnContext(ctx, "could not write down what a node said about a vm", "error", err, "node", heartbeat.NodeName, "vm", heartbeat.VMs[i].UUID)
		}
	}

	if err := h.gone(ctx, heartbeat.NodeName, listed); err != nil {
		h.logger.ErrorContext(ctx, "could not forget the vms a node no longer holds", "error", err, "node", heartbeat.NodeName)
	}

	return nil
}

// capacity writes down what a node offers to VMs and how much of it is taken,
// which is what VMs are placed by.
func (h *Heartbeat) capacity(ctx context.Context, nodeName string, capacity vm.Info, at time.Time) error {
	n, err := h.nodes.GetOne(ctx, nodeName)
	if errors.Is(err, domain.ErrNotExists) {
		n = node.Node{Name: nodeName}
	} else if err != nil {
		return err
	}

	if len(n.Role) == 0 {
		n.Role = node.OrchestratorRole
	}

	n.Capacity = capacity

	// a node that reports its VMs is a node that is alive.
	if at.After(n.LastHeartbeatAt) {
		n.LastHeartbeatAt = at
	}

	_, err = h.nodes.Save(ctx, &n)

	return err
}

// beat writes down what a node said about one VM.
func (h *Heartbeat) beat(ctx context.Context, nodeName string, at time.Time, b *events.VMBeat) error {
	v, err := h.vms.GetOne(ctx, b.UUID)
	if errors.Is(err, domain.ErrNotExists) {
		// held by a node and known to nobody: deleted while its node could not
		// be told, or made after its delete was carried out. Either way,
		// nobody is going to want it.
		return h.commander.Delete(ctx, b.UUID, nodeName)
	} else if err != nil {
		return err
	}

	// a VM lives on one node; another one speaking for it is not the one
	// that holds it.
	if v.NodeName != nodeName {
		return nil
	}

	// older than what was last heard of it.
	if at.Before(v.LastHeartbeatAt) {
		return nil
	}

	previous := v.CurrentState

	v.LastHeartbeatAt = at
	v.Stats = b.Stats.ToVM()

	if observed, known := observe(b.State); known && accepts(&v, observed, b, at) {
		h.arrive(&v, observed, b)
	}

	if _, err := h.vms.Save(ctx, &v); err != nil {
		// asked for something while this was read: the next beat says it again.
		return err
	}

	if previous != vm.Running && v.CurrentState == vm.Running && v.Kind == vm.KindDocker {
		return h.dispatcher.Waiting(ctx, &v)
	}

	return nil
}

// arrive writes down that a VM is what its node says it is.
func (h *Heartbeat) arrive(v *vm.VM, observed vm.State, b *events.VMBeat) {
	v.CurrentState = observed

	switch observed {
	case vm.Running:
		v.Reason = ""

		if !b.StartedAt.IsZero() {
			v.StartedAt = b.StartedAt
		}

		// it was made, from whatever it was to be made from.
		v.RestoreFrom = ""

		// a VM that is running was not given up on after all: it came back,
		// so keeping it up is what is wanted of it again.
		if v.ExpectedState == vm.Failed {
			v.ExpectedState = vm.Running
		}

	case vm.Failed:
		v.Reason = b.Reason
		if len(v.Reason) == 0 {
			v.Reason = defaultReason
		}

	case vm.Stopped:
		v.Reason = ""
	}
}

// gone forgets the VMs on their way out that a node no longer lists: the node
// says it no longer holds them, which is what their deletion was waiting on.
func (h *Heartbeat) gone(ctx context.Context, nodeName string, listed map[string]bool) error {
	held, err := h.vms.GetAllByNode(ctx, nodeName)
	if err != nil {
		return err
	}

	var failed error
	for i := range held {
		if held[i].CurrentState != vm.Deleting || listed[held[i].UUID] {
			continue
		}

		failed = errors.Join(failed, h.lifecycle.Forget(ctx, held[i].UUID))
	}

	return failed
}

// observe is what the engine's word for an instance makes a VM. An instance
// that is only created says nothing either way.
func observe(state vm.InstanceState) (vm.State, bool) {
	switch state {
	case vm.InstanceRunning:
		return vm.Running, true
	case vm.InstanceStopped, vm.InstanceExited:
		return vm.Stopped, true
	case vm.InstanceFailed:
		return vm.Failed, true
	default:
		return 0, false
	}
}

// accepts reports whether what a node says a VM is doing is where the VM is,
// given where it was on its way to.
func accepts(v *vm.VM, observed vm.State, b *events.VMBeat, at time.Time) bool {
	switch v.CurrentState {
	case vm.Deleting, vm.Restoring:
		// over once the node says so in so many words.
		return false

	case vm.Created, vm.Scheduled, vm.Starting:
		return observed == vm.Running || observed == vm.Failed

	case vm.Stopping:
		return observed == vm.Stopped || observed == vm.Failed

	case vm.Restarting:
		if observed == vm.Failed {
			return true
		}

		// running since it was asked to restart, or for long enough that it
		// was not going to.
		return observed == vm.Running &&
			(b.StartedAt.IsZero() || b.StartedAt.After(v.UpdatedAt) || at.Sub(v.UpdatedAt) > restartGrace)

	default:
		return true
	}
}
