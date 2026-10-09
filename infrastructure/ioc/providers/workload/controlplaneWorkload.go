package workload

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks"
	controlPlaneContainers "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/container"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/heartbeatResources"
	controlPlaneImages "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/image"
	controlPlaneNetworks "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/network"
	kindsReconcileResources "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/reconcileResources"
	controlPlaneSnapshots "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/snapshot"
	controlPlaneStacks "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/stack"
	controlPlaneTasks "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/task"
	controlPlaneVMs "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/dockervm"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/placement"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/quota"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/records"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/runs"
	controlPlaneVolumes "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/volume"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/slugs"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	imageKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/image"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	snapshotKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/snapshot"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	nodeContract "github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	taskContract "github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/core/request"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/scheduler/roundrobin"
)

// ControlPlaneStores are what the control plane keeps everything it runs
// in: the resources of every kind, VMs, snapshots, stacks and tasks among
// them, the nodes it places them on, and the logs services' tasks ship, which
// go with their tasks. Archives is the bucket a snapshot's archive is taken
// out of when the snapshot goes, and may be nil: with no bucket configured, an
// archive is left where it is.
type ControlPlaneStores struct {
	Resources resource.Repository
	Nodes     nodeContract.Repository
	TaskLogs  taskContract.LogRepository
	Archives  snapshotKind.Store
}

// ControlPlaneWorkload is what the control plane runs of the workload: every
// kind it runs, VMs, their snapshots, stacks, the code runner's tasks and the
// building blocks of Docker VMs, on the framework.
type ControlPlaneWorkload struct {
	// Registry is every kind the control plane runs.
	Registry *kind.Registry[kind.ControlPlaneBinding]

	// Route serves the resource API, every kind under its plural; it is an
	// error for a kind whose routes are taken already.
	Route func(mux *http.ServeMux) error

	// Subscribers hear the results of every kind's commands, and what the
	// nodes hold of every kind whose state is theirs to say, each kind in a
	// heartbeat on its own subject.
	Subscribers map[string]domain.MessageHandler

	// Reconcile is the loop that keeps every resource as it was asked to be.
	Reconcile *kindsReconcileResources.UseCase
}

// NewControlPlaneWorkload builds the control plane's workload over stores,
// asking the nodes through natsConnection and sending them commands with
// producer.
//
// It is the serve command's wiring, and what the workload's end-to-end test
// builds a control plane from, over stores kept in memory: what is tested is
// what is served.
func NewControlPlaneWorkload(
	controlPlaneConfigs *configs.WorkloadControlPlane,
	stores ControlPlaneStores,
	natsConnection *nats.Conn,
	producer domain.Producer,
	logger *slog.Logger,
	options ...ControlPlaneKindsOption,
) (*ControlPlaneWorkload, error) {
	registry, err := NewControlPlaneRegistry()
	if err != nil {
		return nil, err
	}

	// a node is asked over core NATS, and given as long as it is configured
	// to be: what may take longer, an image pulled say, is a command.
	requester := request.NewRequester(natsConnection, controlPlaneConfigs.NodeRequestTimeout)

	// what lives in a VM, its stacks, waits on it while it is not running.
	vms := records.New(stores.Resources)

	// a command that may take its node longer than the reconcile loop's
	// patience is not asked again while the node is still at it: a pull is
	// given what the nodes give one, and a disk streamed to or from the
	// bucket what a snapshot is.
	timeouts := map[kind.Timeout]time.Duration{
		kind.TimeoutPull:     controlPlaneConfigs.PullTimeout(),
		kind.TimeoutTransfer: snapshotKind.TransferTimeout,
	}

	kinds := NewControlPlaneKinds(
		registry,
		ControlPlaneKindStores{Resources: stores.Resources, Nodes: stores.Nodes},
		requester,
		producer,
		logger,
		append([]ControlPlaneKindsOption{WithParents(vms), WithTimeouts(timeouts)}, options...)...,
	)

	vmPlacement := placement.New(stores.Nodes, vms, controlPlaneConfigs.VMCPUOvercommit)

	// the code runner's runs, shown among anybody's VMs: read from their tasks,
	// and stopped and taken away as their tasks are, as any task's commands
	// are asked.
	codeRunner := runs.New(kinds.Resources, registry, kinds.Dispatcher)

	// a slug is one resource's, whatever its kind, as the resources of every
	// kind are kept together: a VM's and a task's are what the ingress serves
	// their ports under, and a stack's is its compose project.
	slugsHeld := []slugs.Taken{
		slugs.By(vms.GetOneBySlug),
		slugs.By(func(ctx context.Context, slug string) (resource.Record, error) {
			return stores.Resources.GetOneBySlug(ctx, taskKind.Name, slug)
		}),
		slugs.By(func(ctx context.Context, slug string) (resource.Record, error) {
			return stores.Resources.GetOneBySlug(ctx, stackKind.Name, slug)
		}),
	}

	dockerDefaults, err := dockerVMDefaults(controlPlaneConfigs)
	if err != nil {
		return nil, err
	}

	// a Docker VM made for a container or a stack is admitted as any VM is.
	chooser := dockervm.NewChooser(vms, kinds.Resources, kinds.Admit, dockerDefaults)

	// a snapshot is taken of one of its owner's VMs, by the node holding it,
	// and is the kind's own to say a VM can be restored or made from: one of
	// the same kind, as both their images say.
	snapshots := controlPlaneSnapshots.New(controlPlaneSnapshots.Dependencies{
		VMs:         vms,
		Nodes:       vmPlacement,
		Resources:   kinds.Resources,
		Archives:    stores.Archives,
		UserMax:     controlPlaneConfigs.SnapshotUserMax,
		DockerImage: controlPlaneConfigs.VMDockerImage,
		Logger:      logger,
	})

	// what the building blocks of Docker VMs share: what the nodes report of
	// them that nobody keeps a record of, which each control plane keeps from
	// the heartbeats it hears, shown for twice as long as a record goes
	// unheard before it is taken to be gone, and the Docker VMs they go into.
	buildingBlocks := blocks.Dependencies{
		Resources: kinds.Resources,
		VMs:       vms,
		Chooser:   chooser,
		Requester: requester,
		Sightings: blocks.NewSightings(2 * settingsOf(options).reconcile.ResourceSilentAfter),
		Logger:    logger,
	}

	if err := registerControlPlaneKinds(registry, controlPlaneVMs.Dependencies{
		Records:   vms,
		Snapshots: snapshots,
		Nodes:     stores.Nodes,
		Quota:     quota.New(vms, vmLimits(controlPlaneConfigs)),
		Placement: vmPlacement,
		Slugs:     slugsHeld,
		Images:    controlPlaneVMs.Images{Machine: controlPlaneConfigs.VMDefaultImage, Docker: controlPlaneConfigs.VMDockerImage},
		Extras:    codeRunner,
	}, snapshots, controlPlaneStacks.New(vms, chooser, slugsHeld...), controlPlaneTasks.New(controlPlaneTasks.Dependencies{
		Nodes:     stores.Nodes,
		Scheduler: roundrobin.New(),
		Slugs:     slugsHeld,
		Logs:      stores.TaskLogs,
	}), buildingBlocks); err != nil {
		return nil, err
	}

	// the resource API of every kind registered, each under its own plural,
	// and the kinds themselves.
	route := func(mux *http.ServeMux) error {
		return kinds.Route(mux)
	}

	// what the nodes hold of every kind whose state is theirs to say, heard an
	// instance at a time, on the kind's own subject; a kind whose state is the
	// control plane's has none.
	subscribers := maps.Clone(kinds.Subscribers)
	heartbeats := heartbeatResources.NewHeartbeatHandler(kinds.Observer, logger)

	for _, d := range registry.Descriptors() {
		if d.StateBy == kind.OnNode {
			subscribers[kind.HeartbeatName(d.Name)] = heartbeats
		}
	}

	return &ControlPlaneWorkload{
		Registry:    registry,
		Route:       route,
		Subscribers: subscribers,
		Reconcile:   kinds.Reconcile,
	}, nil
}

// registerControlPlaneKinds registers every kind the control plane runs,
// each through its control-plane strategy: the one place a kind is added to
// the control plane, and what its resource API, its consumers and its
// reconcile loop all run over. A VM is admitted, placed and held to its
// owner's quota as vms says; a snapshot is taken of one of its owner's VMs
// and held to their quota of snapshots, as snapshots says; a stack is
// admitted into the Docker VM stacks chooses, or makes, which it lives in; a
// task, the code runner's runs among them, is admitted and placed as tasks
// says; and the building blocks of a Docker VM, its containers, images,
// networks and volumes, live in it, over what building blocks share.
func registerControlPlaneKinds(registry *kind.Registry[kind.ControlPlaneBinding], vms controlPlaneVMs.Dependencies, snapshots *controlPlaneSnapshots.Snapshots, stacks *controlPlaneStacks.Stacks, tasks *controlPlaneTasks.Tasks, buildingBlocks blocks.Dependencies) error {
	for _, binding := range []kind.ControlPlaneBinding{
		kind.BindControlPlane[vmKind.Spec, vmKind.Status](vmKind.Descriptor(), controlPlaneVMs.New(vms)),
		kind.BindControlPlane[snapshotKind.Spec, snapshotKind.Status](snapshotKind.Descriptor(), snapshots),
		kind.BindControlPlane[stackKind.Spec, stackKind.Status](stackKind.Descriptor(), stacks),
		kind.BindControlPlane[taskKind.Spec, taskKind.Status](taskKind.Descriptor(), tasks),
		kind.BindControlPlane[containerKind.Spec, containerKind.Status](containerKind.Descriptor(), controlPlaneContainers.New(buildingBlocks)),
		kind.BindControlPlane[imageKind.Spec, imageKind.Status](imageKind.Descriptor(), controlPlaneImages.New(buildingBlocks)),
		kind.BindControlPlane[networkKind.Spec, networkKind.Status](networkKind.Descriptor(), controlPlaneNetworks.New(buildingBlocks)),
		kind.BindControlPlane[volumeKind.Spec, volumeKind.Status](volumeKind.Descriptor(), controlPlaneVolumes.New(buildingBlocks)),
	} {
		if err := registry.Register(binding); err != nil {
			return err
		}
	}

	return nil
}

// vmLimits are what one VM may be given, and one person's VMs between them,
// as the control plane was configured.
func vmLimits(controlPlaneConfigs *configs.WorkloadControlPlane) quota.Limits {
	return quota.Limits{
		MinMemory:       controlPlaneConfigs.VMMinMemory,
		MinDisk:         controlPlaneConfigs.VMMinDisk,
		DockerMinMemory: controlPlaneConfigs.VMDockerMinMemory,
		DockerMinDisk:   controlPlaneConfigs.VMDockerMinDisk,
		MaxCPUs:         controlPlaneConfigs.VMMaxCPUs,
		MaxMemory:       controlPlaneConfigs.VMMaxMemory,
		MaxDisk:         controlPlaneConfigs.VMMaxDisk,
		UserMaxVMs:      controlPlaneConfigs.VMUserMaxVMs,
		UserCPUs:        controlPlaneConfigs.VMUserCPUs,
		UserMemory:      controlPlaneConfigs.VMUserMemory,
		UserDisk:        controlPlaneConfigs.VMUserDisk,
		MaxLifetime:     controlPlaneConfigs.VMMaxLifetime,
	}
}

// dockerVMDefaults is what a Docker VM made for a container or a stack is
// given, as the control plane was configured: the Docker image, which is what
// makes it one, among it. Settings that cannot be what they say are refused
// here, when the control plane starts, rather than when somebody first adds a
// container.
func dockerVMDefaults(controlPlaneConfigs *configs.WorkloadControlPlane) (dockervm.Defaults, error) {
	ports, err := controlPlaneConfigs.DockerDefaultPorts()
	if err != nil {
		return dockervm.Defaults{}, err
	}

	network, valid := dockervm.NetworkOf(controlPlaneConfigs.VMDockerDefaultIngress, controlPlaneConfigs.VMDockerDefaultEgress)
	if !valid {
		return dockervm.Defaults{}, fmt.Errorf("a docker vm's default network is allow or deny each way, got ingress %q and egress %q", network.Ingress, network.Egress)
	}

	return dockervm.Defaults{
		Image: controlPlaneConfigs.VMDockerImage,
		Resources: vmKind.Resources{
			CPUs:   controlPlaneConfigs.VMDockerDefaultCPUs,
			Memory: controlPlaneConfigs.VMDockerDefaultMemory,
			Disk:   controlPlaneConfigs.VMDockerDefaultDisk,
		},
		Ports:          ports,
		Network:        network,
		PersistentDisk: controlPlaneConfigs.VMDockerDefaultPersistentDisk,
		Lifetime:       controlPlaneConfigs.VMDockerDefaultLifetime,
	}, nil
}
