package runtime

import (
	"slices"
	"strings"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
)

// How a class keeps a task apart from the host and from other tasks.
const (
	// IsolationContainer shares the host's kernel, behind namespaces and
	// cgroups.
	IsolationContainer = "container"

	// IsolationMicroVM gives the task a kernel of its own, behind a VMM.
	IsolationMicroVM = "microvm"
)

// Capabilities is what a class can do, so that a task asking for more is
// turned away before it is placed rather than after it fails to start.
//
// A class declares what it enforces rather than what it accepts: a limit a
// class takes and then ignores is one a user believes holds and does not.
// That is why the sysbox class says it has no disk limit, although every task
// names one.
type Capabilities struct {
	// Isolation is how the class keeps tasks apart: IsolationContainer or
	// IsolationMicroVM.
	Isolation string `json:"isolation"`

	// NetworkPolicies are the policies the class can honour.
	NetworkPolicies []network.Policy `json:"network_policies"`

	// StackNetworks says the class gives a stack's services a network of
	// their own, on which they reach each other by service name.
	StackNetworks bool `json:"stack_networks"`

	// ReadOnlyRoot says a task can be given a root it cannot write to.
	ReadOnlyRoot bool `json:"read_only_root"`

	// DiskLimit says a task is held to the disk it names, rather than only
	// asked for it.
	DiskLimit bool `json:"disk_limit"`

	// TTY says a terminal can be opened in a running task.
	TTY bool `json:"tty"`

	// RestartPolicies are the compose restart policies the class applies
	// in place: "no", "always", "on-failure", "unless-stopped".
	RestartPolicies []string `json:"restart_policies"`

	// MinMemory and MaxMemory bound the memory a task may ask for, in
	// bytes; zero is no bound. MaxCPU bounds its CPUs; zero is no bound.
	MinMemory uint64  `json:"min_memory"`
	MaxMemory uint64  `json:"max_memory"`
	MaxCPU    float64 `json:"max_cpu"`

	// Architectures are the CPU architectures, as Go names them ("amd64",
	// "arm64"), the class runs images for.
	Architectures []string `json:"architectures"`
}

// SupportsNetworkPolicy reports whether the class can honour a policy.
func (c Capabilities) SupportsNetworkPolicy(policy network.Policy) bool {
	return slices.Contains(c.NetworkPolicies, policy)
}

// SupportsRestartPolicy reports whether the class applies a restart policy
// in place. A policy may carry a count after a colon, as "on-failure:3"
// does; what is asked for is the policy before it. Naming none is "no", which
// every class honours by doing nothing.
func (c Capabilities) SupportsRestartPolicy(policy string) bool {
	name, _, _ := strings.Cut(policy, ":")
	if len(name) == 0 || name == "no" {
		return true
	}

	return slices.Contains(c.RestartPolicies, name)
}
