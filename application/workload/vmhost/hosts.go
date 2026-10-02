package vmhost

import (
	"context"
	"slices"
	"strings"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// A stack's services reach each other by service name, as they do on a
// docker network. There is no DNS server for that: every machine's /etc/hosts
// names its neighbours, and vmhost writes it again in every machine on a
// network whenever one of its members comes or goes.

// hostnameOf is what a VM calls itself: what its spec says, or failing that
// the beginning of its own ID, as a container's is.
func hostnameOf(v vm.VM) string {
	if len(v.Spec.Hostname) > 0 {
		return v.Spec.Hostname
	}

	return v.ID[:min(12, len(v.ID))]
}

// hostsOf is the names a VM's neighbours answer to.
func (e *Engine) hostsOf(ctx context.Context, v vm.VM) []guest.Host {
	all, err := e.states.All(ctx)
	if err != nil {
		e.logger.WarnContext(ctx, "who a vm's neighbours are cannot be told", "vm", v.ID, "error", err)

		return nil
	}

	return hostsFor(all, v)
}

// hostsFor is the names v's neighbours answer to, on every network it shares
// with them under a name of their own: the services of its stack, each under
// its service name and its hostname. A neighbour on its way back up keeps its
// address, so it keeps its name.
func hostsFor(all []vm.VM, v vm.VM) []guest.Host {
	var hosts []guest.Host

	for _, other := range all {
		if other.ID == v.ID || !other.State.Up() {
			continue
		}

		for _, theirs := range other.Interfaces {
			if len(theirs.Aliases) == 0 || !sharesNetwork(v.Interfaces, theirs.Network) {
				continue
			}

			address, _, _ := strings.Cut(theirs.Address, "/")
			hosts = append(hosts, guest.Host{Address: address, Names: append(slices.Clone(theirs.Aliases), hostnameOf(other))})
		}
	}

	return hosts
}

func sharesNetwork(interfaces []vm.Interface, network string) bool {
	return slices.ContainsFunc(interfaces, func(i vm.Interface) bool { return i.Network == network })
}

// refreshHosts tells every running machine on the networks given who its
// neighbours now are, which is what changes when one of them — except, which
// knows already — comes or goes. A machine that cannot be told keeps the
// names it had: it is told again the next time anything on its network
// changes.
func (e *Engine) refreshHosts(ctx context.Context, except string, interfaces []vm.Interface) {
	if len(interfaces) == 0 {
		return
	}

	all, err := e.states.All(ctx)
	if err != nil {
		e.logger.WarnContext(ctx, "the vms whose neighbours changed cannot be told", "error", err)

		return
	}

	for _, other := range all {
		if other.ID == except || other.State != vm.StateRunning {
			continue
		}

		shares := slices.ContainsFunc(interfaces, func(i vm.Interface) bool { return sharesNetwork(other.Interfaces, i.Network) })
		if !shares {
			continue
		}

		k := e.keeper(other.ID)
		if k == nil {
			continue
		}

		if err := k.client.SetHosts(ctx, hostsFor(all, other)); err != nil {
			e.metrics.agentFailed(ctx, "hosts")
			e.logger.WarnContext(ctx, "a vm could not be told who its neighbours are", "vm", other.ID, "error", err)
		}
	}
}
