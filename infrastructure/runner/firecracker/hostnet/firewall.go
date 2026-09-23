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

// rules are what the firewall says about the pool and the networks in it.
//
// Every network is its own subnet of the pool. Machines on one reach each
// other there, and nothing else inside the pool; a public network also reaches
// out of the pool, masqueraded as the host. Machines on a public network are
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
