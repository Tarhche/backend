package placement

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/driver"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

// Healthy reports whether a node is speaking. One whose last heartbeat is
// older than a few beats is down, or on its way back, and is given nothing
// until it says otherwise.
func Healthy(n node.Node, now time.Time) bool {
	return n.LastHeartbeatAt.After(now.Add(-heartbeatGrace))
}

// Offers is every class a node offers, as its last heartbeat said.
//
// A node that says nothing about classes at all is an orchestrator from before
// there were any, which runs sysbox and nothing else. It is taken for one that
// offers sysbox with everything a container can do, which is what it does:
// otherwise the control plane, deployed before its orchestrators are, would
// see nodes offering nothing, and would place nothing anywhere until every one
// of them had been redeployed.
func Offers(n node.Node) []runtime.Offer {
	if len(n.Runtimes) == 0 {
		return []runtime.Offer{legacyOffer()}
	}

	return n.Runtimes
}

// OfferOf is what a node offers of one class, if it offers the class at all.
func OfferOf(n node.Node, class runtime.Class) (runtime.Offer, bool) {
	for _, offer := range Offers(n) {
		if offer.Class.OrSysbox() == class.OrSysbox() {
			return offer, true
		}
	}

	return runtime.Offer{}, false
}

// legacyOffer is sysbox as every orchestrator ran it before there were
// classes: a container on the shared docker daemon, on any of the three
// networks, in a stack or not, with a read-only root and a terminal when asked
// for, under docker's own restart policies and docker's memory floor. Docker
// does not hold a task to its disk limit, so neither is that offered. How
// much room the node has is not something it said, so none is claimed.
func legacyOffer() runtime.Offer {
	return runtime.Offer{
		Class:   runtime.Sysbox,
		Driver:  driver.KindContainer.String(),
		Healthy: true,
		Capabilities: runtime.Capabilities{
			Isolation:       runtime.IsolationContainer,
			NetworkPolicies: []network.Policy{network.PolicyNone, network.PolicyIsolated, network.PolicyPublic},
			StackNetworks:   true,
			ReadOnlyRoot:    true,
			DiskLimit:       false,
			TTY:             true,
			RestartPolicies: []string{"no", "always", "on-failure", "unless-stopped"},
			MinMemory:       task.MinMemory,
		},
	}
}
