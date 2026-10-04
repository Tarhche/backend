package client

import (
	"errors"
	"strings"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

// What microsandbox cannot run. Each is refused before anything is asked of the
// service, and the orchestrator reports a task it could not run with the
// error's text as its reason, so each is written for the person who reads it
// there, word for word.
var (
	// ErrStack is a service of a stack. A stack's services reach each other
	// by name on a network of their own, and microsandbox has no network
	// between sandboxes.
	ErrStack = errors.New("microsandbox cannot run a stack: its microVMs share no network")

	// ErrReadOnlyRoot is a task asked to run on an immutable root
	// filesystem, which microsandbox cannot give one.
	ErrReadOnlyRoot = errors.New("microsandbox cannot give a task a read-only root")

	// ErrNoNetwork is a task asked to run with no network at all.
	// Microsandbox cannot make a sandbox without a network interface, only
	// one whose traffic is denied, which is not what no network promises.
	ErrNoNetwork = errors.New("microsandbox cannot run a task with no network interface")
)

// refuse says why microsandbox cannot run an execution, when it cannot.
func refuse(execution *task.Execution) error {
	if joinsStack(execution.Networks) {
		return ErrStack
	}

	if execution.ReadOnly {
		return ErrReadOnlyRoot
	}

	if hasNoNetwork(execution.Networks) {
		return ErrNoNetwork
	}

	return nil
}

// joinsStack is an execution on a stack's private network, which is what makes
// it a service of that stack as far as running it goes.
func joinsStack(attachments []network.Attachment) bool {
	stackNetworks := network.StackNetworkName("")

	for _, attachment := range attachments {
		if strings.HasPrefix(attachment.Name, stackNetworks) {
			return true
		}
	}

	return false
}

// hasNoNetwork is an execution docker would give no network: one on docker's
// none, or on nothing at all.
func hasNoNetwork(attachments []network.Attachment) bool {
	if len(attachments) == 0 {
		return true
	}

	for _, attachment := range attachments {
		if attachment.Name == network.NoNetworkName {
			return true
		}
	}

	return false
}
