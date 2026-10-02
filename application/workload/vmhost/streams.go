package vmhost

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// What reaches into a running VM — a command run beside its task, a
// connection to one of its task's ports — goes through its agent: nothing on
// the host reaches into a machine's network, and nothing is published on the
// host. The agent hands back a connection, and vmhost's API passes it on to
// whoever asked, byte for byte.

// running is the keeper of a VM whose task runs, or vm.ErrNotRunning.
func (e *Engine) running(ctx context.Context, id string) (vm.VM, *keeper, error) {
	v, err := e.states.Get(ctx, id)
	if err != nil {
		return vm.VM{}, nil, err
	}

	k := e.keeper(id)
	if k == nil || v.State != vm.StateRunning {
		return vm.VM{}, nil, fmt.Errorf("%w: vm %s is %s", vm.ErrNotRunning, id, v.State)
	}

	return v, k, nil
}

// Exec starts a command inside a running VM, beside its task, and hands back
// what its agent calls it and the connection its input and output travel on,
// as guest frames. The command outlives the connection: EndExec ends it.
func (e *Engine) Exec(ctx context.Context, id string, exec guest.Exec) (string, net.Conn, error) {
	_, k, err := e.running(ctx, id)
	if err != nil {
		return "", nil, err
	}

	execID, conn, err := k.client.Exec(ctx, exec)
	if err != nil {
		e.metrics.agentFailed(ctx, "exec")

		return "", nil, guestError(err)
	}

	return execID, conn, nil
}

// EndExec ends a command run inside a VM, and everything it started, once
// nobody is attached to it any more. A VM that does not run any more took its
// commands with it: there is nothing left to end.
func (e *Engine) EndExec(ctx context.Context, id string, exec string, end guest.EndExec) (guest.Ended, error) {
	_, k, err := e.running(ctx, id)
	if errors.Is(err, vm.ErrNotRunning) {
		return guest.Ended{}, nil
	}

	if err != nil {
		return guest.Ended{}, err
	}

	ended, err := k.client.EndExec(ctx, exec, end)
	if err != nil {
		e.metrics.agentFailed(ctx, "end_exec")

		return guest.Ended{}, guestError(err)
	}

	return ended, nil
}

// Dial connects to one of the ports a running VM's task is reached on, as its
// neighbours on its own network would: the agent connects to the task's own
// address, so what a task serves only to itself stays its own.
func (e *Engine) Dial(ctx context.Context, id string, port uint16) (net.Conn, error) {
	v, k, err := e.running(ctx, id)
	if err != nil {
		return nil, err
	}

	if !slices.Contains(v.Endpoints(), port) {
		return nil, fmt.Errorf("%w: port %d of vm %s cannot be reached: it is reached on %v", vm.ErrInvalid, port, id, v.Endpoints())
	}

	conn, err := k.client.Dial(ctx, port)
	if err != nil {
		e.metrics.agentFailed(ctx, "dial")

		return nil, guestError(err)
	}

	return conn, nil
}

// guestError is an agent's refusal as vmhost's: a task that is not running is
// a VM that is not.
func guestError(err error) error {
	if errors.Is(err, guest.ErrNotRunning) && !errors.Is(err, vm.ErrNotRunning) {
		return fmt.Errorf("%w: %w", vm.ErrNotRunning, err)
	}

	return err
}
