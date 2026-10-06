// Package task is what runs a task's runs, in the words of whoever runs
// them: a run (Execution) and the runtime holding it (Runtime), a run's
// status, its log and its limits. A task itself, what was asked of one and
// what it is doing, is the task kind's (domain/workload/kinds/task).
package task

import (
	"math"

	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// ResourceLimits represents the resource limits of the task.
//
// Cpu is in cores. Memory and Disk are in bytes, and stay in bytes all the
// way down: nothing between where a task is asked for and the runtime
// converts them.
type ResourceLimits struct {
	Cpu    float64
	Memory uint64
	Disk   uint64
}

// VMResources is what the VM a task runs in is given for these limits: its
// cores as whole vCPUs, rounded up and never to none, and its memory and disk
// as they are, in bytes.
func (l ResourceLimits) VMResources() vm.Resources {
	return vm.Resources{
		CPUs:   uint(max(math.Ceil(l.Cpu), 1)),
		Memory: l.Memory,
		Disk:   l.Disk,
	}
}

// GuestOwnerUUID is whose a task the code runner starts is: whoever is reading
// the page it was run from, signed in or not, which is nobody the users know.
// No user's uuid is ever the guest's, so nobody's own listing has one.
const GuestOwnerUUID = "guest"

// MinMemory is the least memory, in bytes, a task may be limited to.
//
// It is the floor a run could always be made with: a task asking for less is
// told so when it is asked for, rather than accepted and then failed on
// whichever node it was given to.
const MinMemory = 6 << 20

// Scheduler picks the node a task goes to, among those that can take one.
type Scheduler interface {
	Pick(candidates []node.Node) node.Node
}
