// Package dispatch asks the node holding a Docker VM to run a compose command
// on one of its stacks.
//
// A stack deployed into a Docker VM that is not up yet — one made for it a
// moment ago, above all — waits: its node could not reach a dockerd that is
// not there, and would fail the deploy for it. The deploy is sent when the
// VM's node first reports it running, and the control plane's heartbeat sends
// any that report was missed for.
package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/stack/events"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// ReasonWaitingForVM is what a stack waiting for its VM to come up is waiting
// for, which is also how it is told from one whose deploy has been sent.
const ReasonWaitingForVM = "waiting_for_vm"

// Dispatcher sends stacks' compose commands.
type Dispatcher struct {
	stacks   stack.Repository
	producer domain.Producer
	logger   *slog.Logger

	now func() time.Time
}

func New(stacks stack.Repository, producer domain.Producer, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{stacks: stacks, producer: producer, logger: logger, now: time.Now}
}

// Ask sends a compose command on a stack to the node holding its VM. It
// carries the YAML itself, so the node needs nothing but the message.
func (d *Dispatcher) Ask(ctx context.Context, s *stack.Stack, v *vm.VM, action stack.Action, removeVolumes bool) error {
	payload, err := json.Marshal(events.StackRequested{
		StackUUID:     s.UUID,
		VMUUID:        v.UUID,
		NodeName:      v.NodeName,
		Action:        action,
		Project:       s.Slug,
		Compose:       s.Compose,
		RemoveVolumes: removeVolumes,
	})
	if err != nil {
		return err
	}

	// what is asked has been written down already, so a caller that has gone
	// away does not take the command back with it.
	return d.producer.Produce(context.WithoutCancel(ctx), events.StackRequestedName, payload)
}

// Waiting sends the deploys of the stacks that were waiting for a VM to come
// up. One that cannot be sent now is sent by the control plane's heartbeat.
func (d *Dispatcher) Waiting(ctx context.Context, v *vm.VM) error {
	stacks, err := d.stacks.GetAllByVM(ctx, v.UUID)
	if err != nil {
		return err
	}

	var failed error
	for i := range stacks {
		if !IsWaiting(&stacks[i]) {
			continue
		}

		if err := d.deploy(ctx, &stacks[i], v); err != nil {
			d.logger.ErrorContext(ctx, "could not deploy a stack that was waiting for its vm", "error", err, "stack", stacks[i].UUID, "vm", v.UUID)
			failed = errors.Join(failed, err)
		}
	}

	return failed
}

// IsWaiting reports whether a stack is waiting for its VM to come up before its
// deploy is sent.
func IsWaiting(s *stack.Stack) bool {
	return s.State == stack.Deploying && s.Reason == ReasonWaitingForVM
}

func (d *Dispatcher) deploy(ctx context.Context, s *stack.Stack, v *vm.VM) error {
	s.Reason = ""
	s.UpdatedAt = d.now()

	if _, err := d.stacks.Save(ctx, s); err != nil {
		return err
	}

	return d.Ask(ctx, s, v, stack.ActionUp, false)
}
