package workload

import (
	"time"

	orchestratorStacks "github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	infraDocker "github.com/khanzadimahdi/testproject/infrastructure/workload/docker"
)

// NodeKindDependencies are what the kinds an orchestrator runs are built
// from: the engine its VMs run on, the dockerds of its Docker VMs, and how
// long a command that may pull images first is given.
type NodeKindDependencies struct {
	Engine         vm.Engine
	Daemons        *infraDocker.Daemons
	CommandTimeout time.Duration
}

// nodeKinds are the kinds an orchestrator runs, each registered by its node
// strategy, which is all it takes for the node to carry out the kind's
// commands, answer its queries, report what it holds in every heartbeat, and
// route its streams and its ports.
//
// A stack is reached through its building blocks: the Docker VM it lives in,
// that VM's dockerd, and compose, run inside the VM. VMs, snapshots and tasks
// are carried out by their own use cases until their kinds take them over,
// each with one line here.
func nodeKinds(d NodeKindDependencies) (*kind.Registry[kind.NodeBinding], error) {
	kinds := kind.NewRegistry[kind.NodeBinding]()

	for _, binding := range []kind.NodeBinding{
		kind.BindNode[stackKind.Spec, stackKind.Status](
			stackKind.Descriptor(),
			orchestratorStacks.New(d.Engine, d.Daemons, infraDocker.NewCompose(d.Engine), d.CommandTimeout),
		),
	} {
		if err := kinds.Register(binding); err != nil {
			return nil, err
		}
	}

	return kinds, nil
}
