package network

import "github.com/khanzadimahdi/testproject/domain/workload/vm"

// Policy describes how much of the network a task is allowed to reach.
type Policy string

const (
	// PolicyNone gives the task no network at all. Nothing can be reached
	// from it and nothing can reach it, so no port can be exposed.
	PolicyNone Policy = "none"

	// PolicyIsolated serves the task's ports through the ingress and lets it
	// reach nothing: it cannot call out.
	PolicyIsolated Policy = "isolated"

	// PolicyPublic is PolicyIsolated that also reaches the public internet.
	PolicyPublic Policy = "public"
)

// DefaultPolicy is what a spec that names no policy runs under. Isolated is the
// safe default: the task serves its ports, but cannot call home.
const DefaultPolicy = PolicyIsolated

// IsValid reports whether p is one of the known policies.
func (p Policy) IsValid() bool {
	switch p {
	case PolicyNone, PolicyIsolated, PolicyPublic:
		return true
	default:
		return false
	}
}

// AllowsPorts reports whether a task under this policy can expose ports.
func (p Policy) AllowsPorts() bool {
	return p == PolicyIsolated || p == PolicyPublic
}

// ReachesInternet reports whether a task under this policy routes out.
func (p Policy) ReachesInternet() bool {
	return p == PolicyPublic
}

func (p Policy) String() string {
	return string(p)
}

// VMNetwork is the network a task under this policy is given once it runs in a
// VM.
//
// No network is nothing either way. An isolated task still serves its ports,
// so the ingress reaches it, but it calls nothing; a public one calls out as
// well. A policy that names nothing is the default one, and one nobody knows
// is given nothing at all, which is the safe answer to a question nobody
// asked.
func (p Policy) VMNetwork() vm.Network {
	policy := p
	if len(policy) == 0 {
		policy = DefaultPolicy
	}

	switch policy {
	case PolicyIsolated:
		return vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny}
	case PolicyPublic:
		return vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessAllow}
	default:
		return vm.Network{Ingress: vm.AccessDeny, Egress: vm.AccessDeny}
	}
}
