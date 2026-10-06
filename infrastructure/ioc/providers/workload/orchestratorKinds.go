package workload

import (
	"time"

	orchestratorBlocks "github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/blocks"
	orchestratorContainers "github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/container"
	orchestratorImages "github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/image"
	orchestratorNetworks "github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/network"
	orchestratorSnapshots "github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/snapshot"
	orchestratorStacks "github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/stack"
	orchestratorTasks "github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/task"
	orchestratorVMs "github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/vm"
	orchestratorVolumes "github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	imageKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/image"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	snapshotKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/snapshot"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	infraDocker "github.com/khanzadimahdi/testproject/infrastructure/workload/docker"
)

// NodeKindDependencies are what the kinds an orchestrator runs are built
// from: the engine its VMs run on, the dockerds of its Docker VMs, the bucket
// snapshots are stored in and VMs made or restored from them read from, the
// locks every command to a VM takes, which taking a snapshot of it takes too,
// where what the node holds, the snapshots it takes and how long its building
// blocks' requests to a dockerd took are published for the dashboards, how
// long a command that may pull images first is given, and the runtime the code
// runner's tasks run on, each run a VM of its own on the same engine.
type NodeKindDependencies struct {
	Engine         vm.Engine
	Tasks          task.Runtime
	Daemons        *infraDocker.Daemons
	Archives       snapshotKind.Store
	Locks          orchestratorSnapshots.Locks
	Gauges         orchestratorVMs.Gauges
	Snapshots      orchestratorSnapshots.Recorder
	Timings        orchestratorBlocks.Timings
	NodeName       string
	CommandTimeout time.Duration
}

// nodeKinds are the kinds an orchestrator runs, each registered by its node
// strategy, which is all it takes for the node to carry out the kind's
// commands, answer its queries, report what it holds in every heartbeat, and
// route its streams and its ports.
//
// A VM is run on the node's engine, and a snapshot is taken of one there,
// under the VM's lock. A stack is reached through its building blocks: the
// Docker VM it lives in, that VM's dockerd, and compose, run inside the VM.
// The building blocks themselves, a Docker VM's containers, images, networks
// and volumes, are carried out in that dockerd, and read from every running
// Docker VM in one look each beat, which the four share. A task is run on the
// task runtime, each run an ephemeral VM of its own.
func nodeKinds(d NodeKindDependencies) (*kind.Registry[kind.NodeBinding], error) {
	kinds := kind.NewRegistry[kind.NodeBinding]()

	dockerVMs := orchestratorBlocks.NewReader(d.Engine, orchestratorBlocks.Timed(d.Daemons, d.Timings))

	for _, binding := range []kind.NodeBinding{
		kind.BindNode[vmKind.Spec, vmKind.Status](
			vmKind.Descriptor(),
			orchestratorVMs.New(d.Engine, d.Archives, d.Daemons, d.Gauges, d.NodeName),
		),
		kind.BindNode[snapshotKind.Spec, snapshotKind.Status](
			snapshotKind.Descriptor(),
			orchestratorSnapshots.New(d.Engine, d.Archives, d.Locks, d.Snapshots),
		),
		kind.BindNode[stackKind.Spec, stackKind.Status](
			stackKind.Descriptor(),
			orchestratorStacks.New(d.Engine, d.Daemons, infraDocker.NewCompose(d.Engine), d.CommandTimeout),
		),
		kind.BindNode[taskKind.Spec, taskKind.Status](
			taskKind.Descriptor(),
			orchestratorTasks.New(d.Tasks, d.NodeName),
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
