package workload

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/gofrs/uuid/v5"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/execute"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/ownership"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/reconcile"
	controlPlaneContainers "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/container"
	controlPlaneNetworks "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/network"
	kindsReconcileResources "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/reconcileResources"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/requirements"
	controlPlaneVMs "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/dockervm"
	controlPlaneVolumes "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
)

// graphRoots are the kinds whose resources are roots of graphs of their own,
// each one nothing owns, which the control plane keeps made: a container
// somebody asked for, which no stack owns. Stacks are compose's still.
func graphRoots() graph.Roots {
	return graph.Roots{containerKind.Name}
}

// controlPlaneGraphKinds are the kinds a root's graph is made of, as a graph
// needs each (graph.Kind): a Docker VM, made through the chooser as one made
// for what lives in it is, and the building blocks that live in one, its
// containers, networks and volumes, over their control-plane strategies; each
// made, asked and removed through records.
func controlPlaneGraphKinds(chooser *dockervm.Chooser, strategies blockStrategies, records *requirements.Records) graph.Kinds {
	return graph.Kinds{
		vmKind.Name:        controlPlaneVMs.NewGraph(chooser, records),
		containerKind.Name: controlPlaneContainers.NewGraph(strategies.containers, records),
		networkKind.Name:   controlPlaneNetworks.NewGraph(strategies.networks, records),
		volumeKind.Name:    controlPlaneVolumes.NewGraph(strategies.volumes, records),
	}
}

// controlPlaneGraphRecords are what a root's graph makes, asks and removes
// records through: the control plane's own admission, commands and delete,
// over its registry and the repository every part of it keeps resources in,
// each command waited on for as long as the reconcile loop waits on it, its
// node's timeout and the loop's patience.
func controlPlaneGraphRecords(registry *kind.Registry[kind.ControlPlaneBinding], kinds *ControlPlaneKinds, reconcileConfig kindsReconcileResources.Config) *requirements.Records {
	return requirements.New(requirements.Dependencies{
		Registry:   registry,
		Resources:  kinds.Resources,
		Dispatcher: kinds.Dispatcher,
		Admission:  kinds.Admit,
		Deletion:   kinds.Delete,
		Timeouts:   reconcileConfig.Timeouts,
		Patience:   reconcileConfig.Patience,
	})
}

// newControlPlaneGraphs is what keeps every root's graph made (reconcile),
// and the holder of the roots' leases this control plane is, a uuid made now,
// once, which it logs.
//
// A root's graph is planned as policy types its edges, and carried out a
// level at a time, as many nodes of a level at once as the control plane is
// configured to, keeping how each node went in the stores' executions; what
// owns each node's record, and what each uses, is the records' to say, as
// kinds keeps them. Its roots are reconciled by as many workers as the
// control plane is configured with, under the stores' leases, held for as
// long as it is configured to, and backed off as the reconcile loop backs a
// resource off. Settings that cannot be gone about are refused here, when the
// control plane starts, and so are stores that keep no executions or leases.
func newControlPlaneGraphs(
	controlPlaneConfigs *configs.WorkloadControlPlane,
	registry *kind.Registry[kind.ControlPlaneBinding],
	kinds *ControlPlaneKinds,
	graphKinds graph.Kinds,
	stores ControlPlaneStores,
	policy graph.DependencyPolicy,
	reconcileConfig kindsReconcileResources.Config,
	logger *slog.Logger,
) (*reconcile.Reconciler, string, error) {
	switch {
	case stores.Executions == nil || stores.Leases == nil:
		return nil, "", errors.New("the graphs of roots keep how their plans went and their leases somewhere, and are given nowhere to keep one of them")
	case controlPlaneConfigs.GraphResync <= 0:
		return nil, "", fmt.Errorf("every root is reconciled every so often (WORKLOAD_GRAPH_RESYNC, --graph-resync), and %s is not so often", controlPlaneConfigs.GraphResync)
	}

	executor, err := execute.New(graphKinds, ownership.New(registry, kinds.Resources, nil), stores.Executions, nil, execute.Concurrency(controlPlaneConfigs.GraphConcurrency))
	if err != nil {
		return nil, "", fmt.Errorf("the graphs of roots cannot be carried out as configured (WORKLOAD_GRAPH_CONCURRENCY, --graph-concurrency): %w", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, "", err
	}

	holder := id.String()

	config := reconcile.DefaultConfig()
	config.Workers = controlPlaneConfigs.GraphWorkers
	config.Lease = controlPlaneConfigs.GraphLease
	config.Backoff = graph.Backoff{First: reconcileConfig.Backoff, Most: reconcileConfig.MaxBackoff}

	graphs, err := reconcile.New(reconcile.Dependencies{
		Registry:   registry,
		Resources:  kinds.Resources,
		Kinds:      graphKinds,
		Roots:      graphRoots(),
		Planner:    graph.NewPlanner(graphKinds, policy),
		Executor:   executor,
		Executions: stores.Executions,
		Leases:     stores.Leases,
		Holder:     holder,
		Logger:     logger,
	}, config)
	if err != nil {
		return nil, "", fmt.Errorf("the graphs of roots cannot be reconciled as configured (WORKLOAD_GRAPH_WORKERS, WORKLOAD_GRAPH_LEASE): %w", err)
	}

	logger.Info("this control plane holds the leases of the roots it reconciles as its own", "holder", holder, "roots", graphRoots())

	return graphs, holder, nil
}
