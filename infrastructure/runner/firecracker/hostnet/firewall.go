package hostnet

import (
	"net"
	"slices"
	"strings"
)

// The chains the runner keeps its rules in. They are its own, flushed and
// written again whenever a network comes or goes, and jumped to from the top
// of the chains the kernel runs, so nothing else on the host has to be touched
// to change them.
const (
	forwardChain     = "RUNNER-FORWARD"
	inputChain       = "RUNNER-INPUT"
	postroutingChain = "RUNNER-POSTROUTING"

	// dockerUserChain is where docker lets rules of its own go ahead of its
	// own. On a host docker runs on, forwarded traffic is dropped unless
	// something there lets it through first.
	dockerUserChain = "DOCKER-USER"
)

// networkState is a network as the kernel holds it.
type networkState struct {
	owner      string
	name       string
	subnet     *net.IPNet
	masquerade bool
}

// rule is one rule of one chain.
type rule struct {
	table string
	chain string
	spec  []string
}

// deniedDestinations are what no machine reaches out to, whatever network it
// is on: everything private or local. That is the host's own networks, its
// LAN, a cloud's private networks and its metadata service, and the networks
// of the containers beside the runner's. The pool is in one of them, and what
// a network's machines say to each other is let through before these are
// reached. The pool is IPv4 alone, and IPv6 is off on every runner device, so
// there is nothing of IPv6 to deny.
var deniedDestinations = []string{
	"10.0.0.0/8",     // private
	"100.64.0.0/10",  // shared address space, which some clouds keep their own services in
	"127.0.0.0/8",    // loopback
	"169.254.0.0/16", // link-local, which a cloud's metadata service is on
	"172.16.0.0/12",  // private
	"192.168.0.0/16", // private
}

// rules are what the firewall says about the pool and the networks in it.
//
// Every network is its own subnet of the pool. Machines on one reach each
// other there, and nothing else inside the pool; a public network also reaches
// out of the pool to the internet, masqueraded as the host, and to nothing
// private or local on the way. Machines on a public network are
// kept from each other rather than let through: all a public network gives a
// machine is the way out. And nothing of the pool reaches the host itself,
// which answers only what the machines started.
//
// Traffic that is neither from the pool nor to it is left for the rest of the
// host's rules to decide.
func rules(pool *net.IPNet, networks []networkState) []rule {
	poolCIDR := pool.String()

	sorted := slices.Clone(networks)
	slices.SortFunc(sorted, func(a networkState, b networkState) int {
		return strings.Compare(a.subnet.String(), b.subnet.String())
	})

	forward := []rule{
		{table: "filter", chain: forwardChain, spec: []string{"-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"}},
	}

	var postrouting []rule

	for _, n := range sorted {
		subnet := n.subnet.String()

		if n.masquerade {
			forward = append(forward,
				rule{table: "filter", chain: forwardChain, spec: []string{"-s", subnet, "-d", subnet, "-j", "DROP"}},
			)

			for _, denied := range deniedDestinations {
				forward = append(forward,
					rule{table: "filter", chain: forwardChain, spec: []string{"-s", subnet, "-d", denied, "-j", "DROP"}},
				)
			}

			forward = append(forward,
				rule{table: "filter", chain: forwardChain, spec: []string{"-s", subnet, "!", "-d", poolCIDR, "-j", "ACCEPT"}},
			)

			postrouting = append(postrouting,
				rule{table: "nat", chain: postroutingChain, spec: []string{"-s", subnet, "!", "-d", poolCIDR, "-j", "MASQUERADE"}},
			)

			continue
		}

		forward = append(forward,
			rule{table: "filter", chain: forwardChain, spec: []string{"-s", subnet, "-d", subnet, "-j", "ACCEPT"}},
		)
	}

	forward = append(forward,
		rule{table: "filter", chain: forwardChain, spec: []string{"-s", poolCIDR, "-j", "DROP"}},
		rule{table: "filter", chain: forwardChain, spec: []string{"-d", poolCIDR, "-j", "DROP"}},
	)

	input := []rule{
		{table: "filter", chain: inputChain, spec: []string{"-i", bridgePrefix + "+", "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"}},
		{table: "filter", chain: inputChain, spec: []string{"-i", bridgePrefix + "+", "-j", "DROP"}},
	}

	return append(append(forward, input...), postrouting...)
}

// jump is where one of the runner's chains is jumped to from.
type jump struct {
	table string
	from  string
	to    string
}

// jumps are where the runner's chains are jumped to from. Forwarded traffic
// goes through docker's own chain for rules like these when there is one,
// since docker drops what its own rules do not let through.
func jumps(dockerUser bool) []jump {
	forwardFrom := "FORWARD"
	if dockerUser {
		forwardFrom = dockerUserChain
	}

	return []jump{
		{table: "filter", from: forwardFrom, to: forwardChain},
		{table: "filter", from: "INPUT", to: inputChain},
		{table: "nat", from: "POSTROUTING", to: postroutingChain},
	}
}
