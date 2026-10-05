// Package ask puts a question to the node holding a VM and says what came of
// it, in the one shape the control plane's API reports a node's answers in.
package ask

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

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
