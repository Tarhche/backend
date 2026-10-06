package workload

import (
	"time"

	orchestratorBlocks "github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/blocks"
	orchestratorContainers "github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/container"
	orchestratorImages "github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/image"
	orchestratorNetworks "github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/network"
	orchestratorStacks "github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/stack"
	orchestratorVMs "github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/vm"
	orchestratorVolumes "github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	imageKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/image"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	snapshotContract "github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	infraDocker "github.com/khanzadimahdi/testproject/infrastructure/workload/docker"
)

// NodeKindDependencies are what the kinds an orchestrator runs are built
// from: the engine its VMs run on, the dockerds of its Docker VMs, the bucket
// a VM made or restored from a snapshot reads it from, where what the node
// holds, and how long its building blocks' requests to a dockerd took, is
// published for the dashboards, and how long a command that may pull images
// first is given.
type NodeKindDependencies struct {
	Engine         vm.Engine
	Daemons        *infraDocker.Daemons
	Archives       snapshotContract.Store
	Gauges         orchestratorVMs.Gauges
	Timings        orchestratorBlocks.Timings
	NodeName       string
	CommandTimeout time.Duration
}

// nodeKinds are the kinds an orchestrator runs, each registered by its node
// strategy, which is all it takes for the node to carry out the kind's
// commands, answer its queries, report what it holds in every heartbeat, and
// route its streams and its ports.
//
// A VM is run on the node's engine. A stack is reached through its building
// blocks: the Docker VM it lives in, that VM's dockerd, and compose, run
// inside the VM. The building blocks themselves, a Docker VM's containers,
// images, networks and volumes, are carried out in that dockerd, and read
// from every running Docker VM in one look each beat, which the four share.
// Snapshots and tasks are carried out by their own use cases until their
// kinds take them over, each with one line here.
func nodeKinds(d NodeKindDependencies) (*kind.Registry[kind.NodeBinding], error) {
	kinds := kind.NewRegistry[kind.NodeBinding]()

	dockerVMs := orchestratorBlocks.NewReader(d.Engine, orchestratorBlocks.Timed(d.Daemons, d.Timings), orchestratorBlocks.Fresh)

	for _, binding := range []kind.NodeBinding{
		kind.BindNode[vmKind.Spec, vmKind.Status](
			vmKind.Descriptor(),
			orchestratorVMs.New(d.Engine, d.Archives, d.Daemons, d.Gauges, d.NodeName),
		),
		kind.BindNode[stackKind.Spec, stackKind.Status](
			stackKind.Descriptor(),
			orchestratorStacks.New(d.Engine, d.Daemons, infraDocker.NewCompose(d.Engine), d.CommandTimeout),
		),
		kind.BindNode[containerKind.Spec, containerKind.Status](containerKind.Descriptor(), orchestratorContainers.New(dockerVMs, d.CommandTimeout)),
		kind.BindNode[imageKind.Spec, imageKind.Status](imageKind.Descriptor(), orchestratorImages.New(dockerVMs, d.CommandTimeout)),
		kind.BindNode[networkKind.Spec, networkKind.Status](networkKind.Descriptor(), orchestratorNetworks.New(dockerVMs, d.CommandTimeout)),
		kind.BindNode[volumeKind.Spec, volumeKind.Status](volumeKind.Descriptor(), orchestratorVolumes.New(dockerVMs, d.CommandTimeout)),
	} {
		if err := kinds.Register(binding); err != nil {
			return nil, err
		}
	}

	return kinds, nil
}
