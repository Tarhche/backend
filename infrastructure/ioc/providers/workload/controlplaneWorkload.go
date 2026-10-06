package workload

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"net/http"

	"github.com/nats-io/nats.go"

	controlPlaneCreateContainer "github.com/khanzadimahdi/testproject/application/workload/controlplane/container/createContainer"
	controlPlaneGetContainers "github.com/khanzadimahdi/testproject/application/workload/controlplane/container/getContainers"
	controlPlaneRequestDocker "github.com/khanzadimahdi/testproject/application/workload/controlplane/docker/requestDocker"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/observe"
	kindsReconcile "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/reconcile"
	controlPlaneStacks "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/stack"
	controlPlaneVMs "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/dockervm"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/placement"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/quota"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/records"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/runs"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/slugs"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/archive"
	controlPlaneCreateSnapshot "github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/createSnapshot"
	controlPlaneDeleteSnapshot "github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/deleteSnapshot"
	controlPlaneGetSnapshot "github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/getSnapshot"
	controlPlaneGetSnapshots "github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/getSnapshots"
	controlPlaneRenameSnapshot "github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/renameSnapshot"
	controlPlaneDeleteTask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/deleteTask"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	nodeContract "github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	snapshotContract "github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	snapshotEvents "github.com/khanzadimahdi/testproject/domain/workload/snapshot/events"
	taskContract "github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/core/request"
	infraTranslator "github.com/khanzadimahdi/testproject/infrastructure/translator"
	infraValidator "github.com/khanzadimahdi/testproject/infrastructure/validator"
	controlPlaneContainerAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/container"
	controlPlaneDockerAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/docker"
	controlPlaneSnapshotAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/snapshot"
)

// ControlPlaneStores are what the control plane keeps everything it runs
// in: the resources of every kind, VMs and stacks among them, the snapshots,
// the nodes it places them on, and the tasks, which share the ingress's
// hostnames with VMs and are among anybody's VMs as the code runner's runs,
// and whose logs go with a run taken away from among them. Archives is the
// bucket a snapshot's archive is taken out of when the snapshot goes, and may
// be nil: with no bucket configured, an archive is left where it is.
type ControlPlaneStores struct {
	Resources resource.Repository
	Snapshots snapshotContract.Repository
	Nodes     nodeContract.Repository
	Tasks     taskContract.Repository
	TaskLogs  taskContract.LogRepository
	Archives  snapshotContract.Store
}

// ControlPlaneWorkload is what the control plane runs of the workload but
// the code runner's tasks: every kind it runs, VMs and stacks, on the
// framework, and beside them what is not a kind yet, a VM's snapshots, the
// containers in Docker VMs and the Docker passthrough.
type ControlPlaneWorkload struct {
	// Registry is every kind the control plane runs.
	Registry *kind.Registry[kind.ControlPlaneBinding]

	// Route serves the resource API, every kind under its plural, and the
	// routes of what is not a kind yet; it is an error for a kind whose
	// routes are taken already.
	Route func(mux *http.ServeMux) error

	// Subscribers hear the results of every kind's commands, and what nodes
	// say of the snapshots they take.
	Subscribers map[string]domain.MessageHandler

	// Observer is what the node heartbeat consumer hands what it heard to,
	// and Reconcile the loop that keeps every resource as it was asked to be.
	Observer  *observe.Observer
	Reconcile *kindsReconcile.UseCase
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

	// a node is asked over core NATS and given as long as the operation needs:
	// one that may pull an image as long as the node may take over it, which
	// is the wait for the VM's dockerd and then the pull.
	requester := request.NewRequester(natsConnection, controlPlaneConfigs.NodeRequestTimeout, controlPlaneConfigs.PullRequestTimeout())

	// what lives in a VM, its stacks, waits on it while it is not running.
	vms := records.New(stores.Resources)

	kinds := NewControlPlaneKinds(
		registry,
		ControlPlaneKindStores{Resources: stores.Resources, Nodes: stores.Nodes},
		requester,
		producer,
		logger,
		append([]ControlPlaneKindsOption{WithParents(vms)}, options...)...,
	)

	// the control plane answers the blog with the codes it refused a request
	// for, and the blog puts them into the words of whoever asked.
	codes := infraValidator.New(infraTranslator.Codes{})

	vmPlacement := placement.New(stores.Nodes, vms, controlPlaneConfigs.VMCPUOvercommit)

	// the code runner's runs, shown among anybody's VMs: read from their tasks,
	// and stopped and taken away as their tasks are.
	codeRunner := runs.New(stores.Tasks, producer, controlPlaneDeleteTask.NewUseCase(stores.Tasks, stores.TaskLogs, producer, infraTranslator.Codes{}))

	dockerDefaults, err := dockerVMDefaults(controlPlaneConfigs)
	if err != nil {
		return nil, err
	}

	// a Docker VM made for a container or a stack is admitted as any VM is.
	chooser := dockervm.NewChooser(vms, kinds.Resources, kinds.Admit, dockerDefaults)

	if err := registerControlPlaneKinds(registry, controlPlaneVMs.Dependencies{
		Records:   vms,
		Snapshots: stores.Snapshots,
		Nodes:     stores.Nodes,
		Quota:     quota.New(vms, vmLimits(controlPlaneConfigs)),
		Placement: vmPlacement,
		Slugs: []slugs.Taken{
			slugs.By(vms.GetOneBySlug),
			slugs.By(stores.Tasks.GetOneBySlug),
		},
		Images: controlPlaneVMs.Images{Machine: controlPlaneConfigs.VMDefaultImage, Docker: controlPlaneConfigs.VMDockerImage},
		Extras: codeRunner,
	}, controlPlaneStacks.New(vms, chooser, slugs.By(func(ctx context.Context, slug string) (resource.Record, error) {
		return stores.Resources.GetOneBySlug(ctx, stackKind.Name, slug)
	}))); err != nil {
		return nil, err
	}

	// what is not a kind yet reads VMs as the vm package has them.
	entities := records.NewEntities(vms)
	remover := archive.NewRemover(stores.Archives, logger)

	route := func(mux *http.ServeMux) error {
		mux.Handle("GET /api/snapshots", controlPlaneSnapshotAPI.NewIndexHandler(controlPlaneGetSnapshots.NewUseCase(stores.Snapshots)))
		mux.Handle("POST /api/vms/{uuid}/snapshots", controlPlaneSnapshotAPI.NewCreateHandler(controlPlaneCreateSnapshot.NewUseCase(entities, codeRunner, stores.Snapshots, vmPlacement, producer, codes, controlPlaneConfigs.SnapshotUserMax)))
		mux.Handle("GET /api/snapshots/{uuid}", controlPlaneSnapshotAPI.NewShowHandler(controlPlaneGetSnapshot.NewUseCase(stores.Snapshots)))
		mux.Handle("PATCH /api/snapshots/{uuid}", controlPlaneSnapshotAPI.NewRenameHandler(controlPlaneRenameSnapshot.NewUseCase(stores.Snapshots, codes)))
		mux.Handle("DELETE /api/snapshots/{uuid}", controlPlaneSnapshotAPI.NewDeleteHandler(controlPlaneDeleteSnapshot.NewUseCase(stores.Snapshots, entities, vmPlacement, remover, codes)))

		mux.Handle("POST /api/vms/{uuid}/docker/{op}", controlPlaneDockerAPI.NewRequestHandler(controlPlaneRequestDocker.NewUseCase(entities, requester, codes)))

		mux.Handle("GET /api/containers", controlPlaneContainerAPI.NewIndexHandler(controlPlaneGetContainers.NewUseCase(entities, requester, logger)))
		mux.Handle("POST /api/containers", controlPlaneContainerAPI.NewCreateHandler(controlPlaneCreateContainer.NewUseCase(entities, chooser, requester, codes)))

		// the resource API of every kind registered, each under its own
		// plural, VMs and stacks among them, and the kinds themselves.
		return kinds.Route(mux)
	}

	subscribers := map[string]domain.MessageHandler{
		snapshotEvents.SnapshotCompletedName: controlPlaneCreateSnapshot.NewSnapshotCompleted(stores.Snapshots, remover, logger),
		snapshotEvents.SnapshotFailedName:    controlPlaneCreateSnapshot.NewSnapshotFailed(stores.Snapshots, remover, logger),
	}

	maps.Copy(subscribers, kinds.Subscribers)

	return &ControlPlaneWorkload{
		Registry:    registry,
		Route:       route,
		Subscribers: subscribers,
		Observer:    kinds.Observer,
		Reconcile:   kinds.Reconcile,
	}, nil
}

// registerControlPlaneKinds registers every kind the control plane runs,
// each through its control-plane strategy: the one place a kind is added to
// the control plane, and what its resource API, its consumers and its
// reconcile loop all run over. A VM is admitted, placed and held to its
// owner's quota as vms says; a stack is admitted into the Docker VM stacks
// chooses, or makes, which it lives in.
func registerControlPlaneKinds(registry *kind.Registry[kind.ControlPlaneBinding], vms controlPlaneVMs.Dependencies, stacks *controlPlaneStacks.Stacks) error {
	for _, binding := range []kind.ControlPlaneBinding{
		kind.BindControlPlane[vmKind.Spec, vmKind.Status](vmKind.Descriptor(), controlPlaneVMs.New(vms)),
		kind.BindControlPlane[stackKind.Spec, stackKind.Status](stackKind.Descriptor(), stacks),
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
// given, as the control plane was configured. Settings that cannot be what
// they say are refused here, when the control plane starts, rather than when
// somebody first adds a container.
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
