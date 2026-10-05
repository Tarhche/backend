// Package vmtest is the control plane's VMs, wired the way the provider wires
// them, over memory repositories and a producer that keeps what it was given:
// what the VM use cases' tests are written against.
package vmtest

import (
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/command"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/lifecycle"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/placement"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/quota"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	nodesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/nodes"
	snapshotsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/snapshots"
	stacksMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/stacks"
	vmsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/vms"
)

const (
	MiB = 1 << 20
	GiB = 1 << 30

	// Node is the name of the node Workload has, unless it is given others.
	Node = "workload-orchestrator-01"
)

// Limits are the bounds and quotas the tests are held to.
var Limits = quota.Limits{
	MinMemory:       128 * MiB,
	MinDisk:         1 * GiB,
	DockerMinMemory: 512 * MiB,
	DockerMinDisk:   4 * GiB,
	MaxCPUs:         4,
	MaxMemory:       8 * GiB,
	MaxDisk:         50 * GiB,
	UserMaxVMs:      5,
	UserCPUs:        8,
	UserMemory:      16 * GiB,
	UserDisk:        200 * GiB,
	MaxLifetime:     720 * time.Hour,
}

// Workload is a control plane's VMs and what they are kept in.
type Workload struct {
	VMs       *vmsMemory.Repository
	Snapshots *snapshotsMemory.Repository
	Stacks    *stacksMemory.Repository
	Nodes     *nodesMemory.Repository
	Producer  *messagingMock.RecordingProducer

	Placement *placement.Placement
	Quota     *quota.Quota
	Commander *command.Commander
	Lifecycle *lifecycle.Lifecycle
}

// Option sets up what a Workload starts with.
type Option func(*options)

type options struct {
	nodes     []node.Node
	vms       []vm.VM
	snapshots []snapshot.Snapshot
	stacks    []stack.Stack
}

// WithNodes replaces the one node a Workload has with these.
func WithNodes(nodes ...node.Node) Option {
	return func(o *options) { o.nodes = nodes }
}

func WithVMs(vms ...vm.VM) Option {
	return func(o *options) { o.vms = append(o.vms, vms...) }
}

func WithSnapshots(snapshots ...snapshot.Snapshot) Option {
	return func(o *options) { o.snapshots = append(o.snapshots, snapshots...) }
}

func WithStacks(stacks ...stack.Stack) Option {
	return func(o *options) { o.stacks = append(o.stacks, stacks...) }
}

// New is a workload with one node, alive and roomy, unless it is told
// otherwise.
func New(opts ...Option) *Workload {
	o := options{nodes: []node.Node{Alive(Node)}}
	for _, opt := range opts {
		opt(&o)
	}

	w := &Workload{
		VMs:       vmsMemory.NewRepository(o.vms...),
		Snapshots: snapshotsMemory.NewRepository(o.snapshots...),
		Stacks:    stacksMemory.NewRepository(o.stacks...),
		Nodes:     nodesMemory.NewRepository(o.nodes...),
		Producer:  &messagingMock.RecordingProducer{},
	}

	w.Placement = placement.New(w.Nodes, w.VMs, 4)
	w.Quota = quota.New(w.VMs, Limits)
	w.Commander = command.New(w.Producer)
	w.Lifecycle = lifecycle.New(w.VMs, w.Stacks, w.Nodes, w.Placement, w.Commander)

	return w
}

// Alive is a node that spoke a moment ago and has room for anything a test
// asks for.
func Alive(name string) node.Node {
	return node.Node{
		Name:            name,
		Role:            node.OrchestratorRole,
		Capacity:        vm.Info{Engine: "microsandbox", Version: "0.7.6", CPUs: 16, Memory: 64 * GiB, Disk: 1000 * GiB},
		LastHeartbeatAt: time.Now(),
	}
}

// Gone is a node that has not spoken for a long while.
func Gone(name string) node.Node {
	n := Alive(name)
	n.LastHeartbeatAt = time.Now().Add(-time.Hour)

	return n
}

// Running is a VM on Node that its node listed a moment ago as running.
func Running(uuid string, ownerUUID string) vm.VM {
	return vm.VM{
		UUID:            uuid,
		Name:            "box",
		Slug:            "box-" + uuid,
		OwnerUUID:       ownerUUID,
		Kind:            vm.KindMachine,
		Image:           "ubuntu:24.04",
		Resources:       vm.Resources{CPUs: 1, Memory: GiB, Disk: 10 * GiB},
		Network:         vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessAllow},
		CurrentState:    vm.Running,
		ExpectedState:   vm.Running,
		NodeName:        Node,
		LastHeartbeatAt: time.Now(),
		CreatedAt:       time.Now().Add(-time.Hour),
		UpdatedAt:       time.Now().Add(-time.Hour),
	}
}

// Stopped is a VM on Node that its node listed a moment ago as stopped.
func Stopped(uuid string, ownerUUID string) vm.VM {
	v := Running(uuid, ownerUUID)
	v.CurrentState = vm.Stopped
	v.ExpectedState = vm.Stopped

	return v
}

// Docker is a running Docker VM on Node.
func Docker(uuid string, ownerUUID string) vm.VM {
	v := Running(uuid, ownerUUID)
	v.Kind = vm.KindDocker
	v.Image = "docker:29-dind"
	v.Resources = vm.Resources{CPUs: 2, Memory: 2 * GiB, Disk: 20 * GiB}

	return v
}
