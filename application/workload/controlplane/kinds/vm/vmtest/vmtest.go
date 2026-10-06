// Package vmtest is the control plane's VMs, wired the way the provider wires
// them, over memory repositories and a producer that keeps what it was given:
// what the tests of the vm kind's control-plane strategy, and of what reads
// VMs beside it, are written against.
package vmtest

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/admitResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/cascade"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	controlPlaneTasks "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/task"
	controlPlaneVMs "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/dockervm"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/placement"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/quota"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/records"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/runs"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/waiters"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/slugs"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	nodesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/nodes"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
	snapshotsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/snapshots"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/scheduler/roundrobin"
)

const (
	MiB = 1 << 20
	GiB = 1 << 30

	// Node is the name of the node Workload has, unless it is given others.
	Node = "workload-orchestrator-01"
)

// Images are what VMs boot from in the tests.
var Images = controlPlaneVMs.Images{Machine: "ubuntu:24.04", Docker: "docker:29-dind"}

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

// DockerDefaults are what a Docker VM made for a container or a stack is
// given in the tests.
var DockerDefaults = dockervm.Defaults{
	Resources: vmKind.Resources{CPUs: 2, Memory: 2 * GiB, Disk: 20 * GiB},
	Ports:     []port.Port{80},
	Network:   vmKind.Network{Ingress: vm.AccessAllow, Egress: vm.AccessAllow},
}

// Workload is a control plane's VMs and what they are kept in, the code
// runner's runs among them, which are tasks, and the vm kind registered as
// the control plane registers it, beside the task kind, admitted through the
// control plane's admission.
type Workload struct {
	Memory    *resourcesMemory.Repository
	Resources *cascade.Repository
	Registry  *kind.Registry[kind.ControlPlaneBinding]
	Records   *records.Records
	Entities  *records.Entities
	Snapshots *snapshotsMemory.Repository
	Nodes     *nodesMemory.Repository
	Producer  *messagingMock.Recorder

	Placement *placement.Placement
	Quota     *quota.Quota
	Runs      *runs.Runs
	VMs       *controlPlaneVMs.VMs

	Dispatcher *dispatch.Dispatcher
	Admit      *admitResource.UseCase
	Chooser    *dockervm.Chooser
}

// Option sets up what a Workload starts with.
type Option func(*options)

type options struct {
	nodes     []node.Node
	vms       []vmKind.VM
	snapshots []snapshot.Snapshot
	tasks     []taskKind.Task
	now       func() time.Time
}

// WithNodes replaces the one node a Workload has with these.
func WithNodes(nodes ...node.Node) Option {
	return func(o *options) { o.nodes = nodes }
}

// WithVMs are VMs the control plane keeps, as their records.
func WithVMs(vms ...vmKind.VM) Option {
	return func(o *options) { o.vms = append(o.vms, vms...) }
}

func WithSnapshots(snapshots ...snapshot.Snapshot) Option {
	return func(o *options) { o.snapshots = append(o.snapshots, snapshots...) }
}

// WithTasks are tasks the workload holds, as their records: those of the
// guest's are the code runner's runs (Run).
func WithTasks(tasks ...taskKind.Task) Option {
	return func(o *options) { o.tasks = append(o.tasks, tasks...) }
}

// WithClock is the time the strategy goes by.
func WithClock(now func() time.Time) Option {
	return func(o *options) { o.now = now }
}

// New is a workload with one node, alive and roomy, unless it is told
// otherwise.
func New(opts ...Option) *Workload {
	o := options{nodes: []node.Node{Alive(Node)}}
	for _, opt := range opts {
		opt(&o)
	}

	w := &Workload{
		Memory:    resourcesMemory.NewRepository(),
		Registry:  kind.NewRegistry[kind.ControlPlaneBinding](),
		Snapshots: snapshotsMemory.NewRepository(o.snapshots...),
		Nodes:     nodesMemory.NewRepository(o.nodes...),
		Producer:  &messagingMock.Recorder{},
	}

	for _, v := range o.vms {
		if _, err := w.Memory.Create(context.Background(), Record(v)); err != nil {
			panic(err)
		}
	}

	for _, t := range o.tasks {
		if _, err := w.Memory.Create(context.Background(), TaskRecord(t)); err != nil {
			panic(err)
		}
	}

	w.Resources = cascade.NewRepository(w.Registry, w.Memory)
	w.Records = records.New(w.Resources)
	w.Entities = records.NewEntities(w.Records)
	w.Dispatcher = dispatch.New(w.Resources, w.Producer, waiters.New(), o.now)

	w.Placement = placement.New(w.Nodes, w.Records, 4)
	w.Quota = quota.New(w.Records, Limits)
	w.Runs = runs.New(w.Resources, w.Registry, w.Dispatcher)

	held := []slugs.Taken{
		slugs.By(w.Records.GetOneBySlug),
		slugs.By(func(ctx context.Context, slug string) (resource.Record, error) {
			return w.Resources.GetOneBySlug(ctx, taskKind.Name, slug)
		}),
	}

	w.VMs = controlPlaneVMs.New(controlPlaneVMs.Dependencies{
		Records:   w.Records,
		Snapshots: w.Snapshots,
		Nodes:     w.Nodes,
		Quota:     w.Quota,
		Placement: w.Placement,
		Slugs:     held,
		Images:    Images,
		Extras:    w.Runs,
		Now:       o.now,
	})

	tasks := controlPlaneTasks.New(controlPlaneTasks.Dependencies{Nodes: w.Nodes, Scheduler: roundrobin.New(), Slugs: held, Now: o.now})

	for _, binding := range []kind.ControlPlaneBinding{
		kind.BindControlPlane[vmKind.Spec, vmKind.Status](vmKind.Descriptor(), w.VMs),
		kind.BindControlPlane[taskKind.Spec, taskKind.Status](taskKind.Descriptor(), tasks),
	} {
		if err := w.Registry.Register(binding); err != nil {
			panic(err)
		}
	}

	logger := slog.New(slog.DiscardHandler)

	w.Admit = admitResource.NewUseCase(w.Registry, w.Resources, w.Dispatcher, logger)
	w.Chooser = dockervm.NewChooser(w.Records, w.Resources, w.Admit, DockerDefaults)

	return w
}

// Stored is the VM kept under uuid, and whether one is.
func (w *Workload) Stored(uuid string) (vmKind.VM, bool) {
	r, kept := w.Memory.Stored(vmKind.Name, uuid)
	if !kept {
		return vmKind.VM{}, false
	}

	v, err := records.Decode(r)
	if err != nil {
		panic(err)
	}

	return v, true
}

// Change writes a VM's record as change leaves it, as what its node reports
// would: whatever else wrote the record in the meantime is read again first.
func (w *Workload) Change(uuid string, change func(v *vmKind.VM)) {
	ctx := context.Background()

	for {
		r, err := w.Memory.GetOne(ctx, vmKind.Name, uuid)
		if err != nil {
			panic(err)
		}

		v, err := records.Decode(r)
		if err != nil {
			panic(err)
		}

		change(&v)

		if r.Raw, err = kind.Encode(v); err != nil {
			panic(err)
		}

		_, err = w.Memory.Update(ctx, r)
		if errors.Is(err, resource.ErrConflict) {
			continue
		}

		if err != nil {
			panic(err)
		}

		return
	}
}

// Record is a VM as the control plane keeps it.
func Record(v vmKind.VM) resource.Record {
	raw, err := kind.Encode(v)
	if err != nil {
		panic(err)
	}

	return resource.Record{Raw: raw}
}

// TaskRecord is a task as the control plane keeps it.
func TaskRecord(t taskKind.Task) resource.Record {
	raw, err := kind.Encode(t)
	if err != nil {
		panic(err)
	}

	return resource.Record{Raw: raw}
}

// StoredTask is the task kept under uuid, and whether one is, with what the
// control plane keeps of what it is in the middle of asking of it.
func (w *Workload) StoredTask(uuid string) (taskKind.Task, resource.Record, bool) {
	r, kept := w.Memory.Stored(taskKind.Name, uuid)
	if !kept {
		return taskKind.Task{}, resource.Record{}, false
	}

	t, err := kind.Decode[taskKind.Spec, taskKind.Status](r.Raw)
	if err != nil {
		panic(err)
	}

	return t, r, true
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

// Running is a machine on Node that its node said a moment ago runs, with
// the config its node gave it when it made it.
func Running(uuid string, ownerUUID string) vmKind.VM {
	made := time.Now().Add(-time.Hour)

	v := vmKind.VM{
		Kind: vmKind.Name,
		Metadata: kind.Metadata{
			UUID:      uuid,
			Name:      "box",
			Slug:      "box-" + uuid,
			OwnerUUID: ownerUUID,
			Labels:    map[string]string{vmKind.LabelFlavor: string(vmKind.FlavorMachine)},
			Node:      Node,
			CreatedAt: made,
			UpdatedAt: made,
		},
		Spec: vmKind.Spec{
			Flavor:    vmKind.FlavorMachine,
			Image:     "ubuntu:24.04",
			Resources: vmKind.Resources{CPUs: 1, Memory: GiB, Disk: 10 * GiB},
			Ports:     []port.Port{},
			Network:   vmKind.Network{Ingress: vm.AccessAllow, Egress: vm.AccessAllow},
		},
		Status: vmKind.Status{
			Status: kind.Status{State: vmKind.Running, Expected: vmKind.Running, Since: made, ObservedAt: time.Now()},
		},
	}

	config := v.Spec.Config()
	v.Status.Applied = &config

	return v
}

// Stopped is a machine on Node that its node said a moment ago is stopped.
func Stopped(uuid string, ownerUUID string) vmKind.VM {
	v := Running(uuid, ownerUUID)
	v.Status.State = vmKind.Stopped
	v.Status.Expected = vmKind.Stopped

	return v
}

// Docker is a running Docker VM on Node.
func Docker(uuid string, ownerUUID string) vmKind.VM {
	v := Running(uuid, ownerUUID)
	v.Metadata.Labels[vmKind.LabelFlavor] = string(vmKind.FlavorDocker)
	v.Spec.Flavor = vmKind.FlavorDocker
	v.Spec.Image = "docker:29-dind"
	v.Spec.Resources = vmKind.Resources{CPUs: 2, Memory: 2 * GiB, Disk: 20 * GiB}

	config := v.Spec.Config()
	v.Status.Applied = &config

	return v
}

// In is v as it is when change says.
func In(v vmKind.VM, change func(v *vmKind.VM)) vmKind.VM {
	change(&v)

	return v
}

// Run is a snippet the code runner is running on Node: a job of the guest's,
// in a VM of its own, which came up a moment ago.
func Run(uuid string) taskKind.Task {
	created := time.Now().Add(-10 * time.Second)
	started := created.Add(time.Second)
	none := 0

	return taskKind.Task{
		Kind: taskKind.Name,
		Metadata: kind.Metadata{
			UUID:      uuid,
			Name:      "request-" + uuid,
			Slug:      "request-" + uuid + "-abcde",
			OwnerUUID: task.GuestOwnerUUID,
			Node:      Node,
			CreatedAt: created,
			UpdatedAt: started,
		},
		Spec: taskKind.Spec{
			Kind:          task.KindJob,
			Image:         "ghcr.io/tarhche/code-runner:nodejs-22.14-latest",
			Command:       []string{"--timeout", "30", "console.log(1)"},
			NetworkPolicy: network.PolicyIsolated,
			TTL:           time.Minute,
			Limits:        taskKind.Limits{CPU: 2, Memory: 200 * MiB, Disk: 100 * MiB},
			MaxRetries:    &none,
		},
		Status: taskKind.Status{
			Status: kind.Status{State: taskKind.Running, Expected: taskKind.Running, Since: started, ObservedAt: time.Now()},
			Run: &taskKind.Run{
				ID:        "execution-" + uuid,
				Name:      "request-" + uuid,
				Slug:      "request-" + uuid + "-abcde",
				Kind:      task.KindJob,
				StartedAt: started,
				Deadline:  started.Add(time.Minute),
			},
		},
	}
}
