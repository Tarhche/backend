package network

// Policy describes how much of the network a task is allowed to reach.
type Policy string

const (
	// PolicyNone gives the task no network interface at all. Nothing can
	// be reached from it and nothing can reach it, so no port can be exposed.
	PolicyNone Policy = "none"

	// PolicyIsolated puts the task on a network that does not route out.
	// The workload publishes its ports, but nothing on that network can reach
	// the internet.
	PolicyIsolated Policy = "isolated"

	// PolicyPublic is PolicyIsolated plus the default bridge, which routes out
	// to the internet.
	PolicyPublic Policy = "public"
)

// DefaultPolicy is what a spec that names no policy runs under. Isolated is the
// safe default: the task serves its ports, but cannot call home.
const DefaultPolicy = PolicyIsolated

const (
	// IsolatedNetworkName is the network a standalone isolated task joins.
	IsolatedNetworkName = "workload-isolated"

	// PublicNetworkName is docker's default bridge, which routes out.
	PublicNetworkName = "bridge"

	// NoNetworkName is docker's own "no network at all" mode.
	NoNetworkName = "none"
)

// Attachment is a network a task joins.
type Attachment struct {
	Name string

	// Gateway marks the network the task's default route goes through. A
	// task on more than one network has to be told which, because only
	// one of them routes out, and the wrong default is a task that cannot
	// call out at all.
	Gateway bool
}

// Attachments resolves the networks a task joins: the shared isolated
// network, and for a public task the default bridge as well, which is the
// only thing that routes out.
func Attachments(policy Policy) []Attachment {
	if policy == PolicyNone {
		return []Attachment{{Name: NoNetworkName}}
	}

	isolated := Attachment{Name: IsolatedNetworkName}

	if policy == PolicyPublic {
		return []Attachment{isolated, {Name: PublicNetworkName, Gateway: true}}
	}

	return []Attachment{isolated}
}

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
