package network

// Policy describes how much of the network a container is allowed to reach.
type Policy string

const (
	// PolicyNone gives the container no network interface at all. Nothing can
	// be reached from it and nothing can reach it, so no port can be exposed
	// and a service under it cannot talk to the rest of its stack either.
	PolicyNone Policy = "none"

	// PolicyIsolated puts the container on a network that does not route out.
	// It reaches the other containers there — the rest of its stack, for a
	// service — and the runner publishes its ports, but nothing on that network
	// can reach the internet.
	PolicyIsolated Policy = "isolated"

	// PolicyPublic is PolicyIsolated plus the default bridge, which routes out
	// to the internet.
	PolicyPublic Policy = "public"
)

// DefaultPolicy is what a spec that names no policy runs under. Isolated is the
// safe default: the container serves its ports and reaches its own stack, but
// cannot call home.
const DefaultPolicy = PolicyIsolated

const (
	// IsolatedNetworkName is the network a standalone isolated container joins.
	IsolatedNetworkName = "runner-isolated"

	// stackNetworkPrefix namespaces the private network each stack gets.
	stackNetworkPrefix = "runner-stack-"

	// PublicNetworkName is docker's default bridge, which routes out.
	PublicNetworkName = "bridge"

	// NoNetworkName is docker's own "no network at all" mode.
	NoNetworkName = "none"
)

// StackNetworkName is the private network the services of one stack share. It
// is theirs alone, so two stacks can both hold a service called "db" without
// either reaching the other's.
func StackNetworkName(stackSlug string) string {
	return stackNetworkPrefix + stackSlug
}

// Attachment is a network a container joins, and the names its neighbours on
// that network can reach it by.
type Attachment struct {
	Name    string
	Aliases []string

	// Gateway marks the network the container's default route goes through. A
	// container on more than one network has to be told which, because only
	// one of them routes out — reaching its own stack and reaching the internet
	// are different networks, and the wrong default is a container that cannot
	// call out at all.
	Gateway bool
}

// Attachments resolves the networks a container joins.
//
// A service in a stack joins that stack's private network under its service
// name, which is what lets one service reach another at "http://api:8080" the
// way a compose file expects. A standalone container joins the shared isolated
// network instead. Either way, a public container also joins the default
// bridge, and that second attachment is the only thing that routes out.
func Attachments(policy Policy, stackSlug string, serviceName string) []Attachment {
	if policy == PolicyNone {
		return []Attachment{{Name: NoNetworkName}}
	}

	private := Attachment{Name: IsolatedNetworkName}
	if len(stackSlug) > 0 {
		private = Attachment{Name: StackNetworkName(stackSlug)}

		if len(serviceName) > 0 {
			private.Aliases = []string{serviceName}
		}
	}

	if policy == PolicyPublic {
		return []Attachment{private, {Name: PublicNetworkName, Gateway: true}}
	}

	return []Attachment{private}
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

// AllowsPorts reports whether a container under this policy can expose ports.
func (p Policy) AllowsPorts() bool {
	return p == PolicyIsolated || p == PolicyPublic
}

// ReachesInternet reports whether a container under this policy routes out.
func (p Policy) ReachesInternet() bool {
	return p == PolicyPublic
}

func (p Policy) String() string {
	return string(p)
}
