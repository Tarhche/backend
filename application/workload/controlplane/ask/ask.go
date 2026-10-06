// Package ask puts a question to the node holding a VM and says what came of
// it, in the one shape the control plane's API reports a node's answers in.
package ask

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// listedFor is how long ago a VM's node must have last spoken for it for the
// node to be holding it: a node says what it holds every second, so this is
// many heartbeats.
const listedFor = time.Minute

// Node asks nodeName, and is its reply, or why there is none. A node that
// refused says why itself; one that did not answer in time is a timeout, and
// one nobody answered for is a node that is not there to ask.
func Node(ctx context.Context, requester noderequest.Requester, nodeName string, request noderequest.Request) (noderequest.Reply, *noderequest.Error) {
	reply, err := requester.Request(ctx, nodeName, request)

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return noderequest.Reply{}, &noderequest.Error{Code: noderequest.CodeTimeout, Message: "the node holding the vm did not answer in time"}
	case err != nil:
		return noderequest.Reply{}, &noderequest.Error{Code: noderequest.CodeInternal, Message: "the node holding the vm is not answering"}
	}

	var refused *noderequest.Error
	if errors.As(reply.Err(), &refused) {
		return noderequest.Reply{}, refused
	}

	return reply, nil
}

// Payload is what an operation is asked with, as it travels.
func Payload(payload any) json.RawMessage {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil
	}

	return encoded
}

// NotRunning is the answer about a VM that cannot be asked anything, because
// nothing of it is running anywhere.
func NotRunning() *noderequest.Error {
	return noderequest.ErrorOf(vm.ErrNotRunning)
}

// NotDocker is the answer about a VM that has no dockerd to ask.
func NotDocker() *noderequest.Error {
	return noderequest.ErrorOf(vm.ErrNotDocker)
}

// NotHeld is the answer about a VM its node has not listed lately, or nil when
// it has. A node answers only for the instances it holds, and one scheduled a
// moment ago is not made yet: asked about it, the node would say there is no
// such VM, which whoever asked would take for the VM not being there at all.
// So it is not asked, and the VM is not running yet, which is so.
func NotHeld(v *vm.VM, now time.Time) *noderequest.Error {
	if !v.LastHeartbeatAt.IsZero() && now.Sub(v.LastHeartbeatAt) <= listedFor {
		return nil
	}

	return NotRunning()
}

// DockerRefusal is why a VM's dockerd cannot be asked anything, or nil when it
// can: a Docker VM, placed on a node, running or on its way up. One still
// booting is waited for by its node, which gives dockerd time to come up.
func DockerRefusal(v *vm.VM) *noderequest.Error {
	if v.Kind != vm.KindDocker {
		return NotDocker()
	}

	if len(v.NodeName) == 0 {
		return NotRunning()
	}

	switch v.CurrentState {
	case vm.Scheduled, vm.Starting, vm.Restarting, vm.Running:
		return nil
	}

	return NotRunning()
}
