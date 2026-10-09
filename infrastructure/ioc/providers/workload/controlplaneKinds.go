package workload

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/actOnResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/admitResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/cascade"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/deleteResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getKinds"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getResources"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/observe"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/queryResource"
	kindsReconcileResources "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/reconcileResources"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/recordResult"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/waiters"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	controlPlaneKindsAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/kinds"
)

// NewControlPlaneRegistry is a registry for every kind the control plane
// runs, with none in it yet: a kind is registered when what its strategy is
// built from is (NewControlPlaneWorkload).
func NewControlPlaneRegistry() (*kind.Registry[kind.ControlPlaneBinding], error) {
	return kind.NewRegistry[kind.ControlPlaneBinding](), nil
}

// ControlPlaneKindStores are what the control plane keeps the resources of
// every kind in, and the nodes it weighs them against.
type ControlPlaneKindStores struct {
	Resources resource.Repository
	Nodes     node.Repository
}

// ControlPlaneKinds is the control plane's generic plumbing for the kinds it
// runs: the resource API, the result consumer, the observer the node
// heartbeat consumer hands what it heard to, and the reconcile loop that
// keeps every resource as it was asked to be.
type ControlPlaneKinds struct {
	// Route serves the resource API, a kind's routes under its plural, and
	// is an error for a kind whose routes are taken already.
	Route func(mux *http.ServeMux) error

	// Subscribers hear workloadResourceActedOn.
	Subscribers map[string]domain.MessageHandler

	Observer  *observe.Observer
	Reconcile *kindsReconcileResources.UseCase

	// Admit takes in a resource of any kind, as the resource API does: a
	// Docker VM made for a container or a stack is admitted through it.
	Admit *admitResource.UseCase

	// Resources are what every part of the plumbing keeps resources in: the
	// stores' resources, deleting what lives in a resource with it, as the
	// kinds that live in it say, wherever it is deleted from.
	Resources *cascade.Repository

	// Dispatcher asks resources of any kind for their kinds' commands: the
	// code runner's runs, shown among anybody's VMs, are asked their tasks'
	// through it.
	Dispatcher *dispatch.Dispatcher
}

// controlPlaneKindsOptions are how the plumbing goes about its work.
type controlPlaneKindsOptions struct {
	parents   observe.Parents
	reconcile kindsReconcileResources.Config
	timeouts  map[kind.Timeout]time.Duration
}

// ControlPlaneKindsOption changes how the plumbing goes about its work.
type ControlPlaneKindsOption func(*controlPlaneKindsOptions)

// WithParents has what lives in a parent that its node did not look into
// observed waiting on it, as parents say the parent is: a stack in a VM that
// is stopped.
func WithParents(parents observe.Parents) ControlPlaneKindsOption {
	return func(o *controlPlaneKindsOptions) {
		o.parents = parents
	}
}

// WithReconcileConfig is how patient the reconcile loop is, in place of the
// control plane's own: a test's is far less.
func WithReconcileConfig(config kindsReconcileResources.Config) ControlPlaneKindsOption {
	return func(o *controlPlaneKindsOptions) {
		o.reconcile = config
	}
}

// WithTimeouts is how long a node may take over a command given each
// timeout, as the nodes are configured to give it: the reconcile loop does
// not send one again before then. A reconcile config that sizes them itself
// keeps its own.
func WithTimeouts(timeouts map[kind.Timeout]time.Duration) ControlPlaneKindsOption {
	return func(o *controlPlaneKindsOptions) {
		o.timeouts = timeouts
	}
}

// NewControlPlaneKinds builds the control plane's plumbing for the kinds in
// registry, asking the nodes queries through requester and sending them
// commands with producer.
//
// What lives in a resource follows it by its kind's rules: it is deleted with
// it, wherever the resource is deleted from, and reset to what it holds once
// it is restored from a snapshot. What a node holds that nobody keeps a
// record of, a VM deleted while its node could not be told, is deleted on
// that node.
//
// It is what the serve command and a test build a control plane's kinds
// from, the test over stores kept in memory: what is tested is what is
// served.
func NewControlPlaneKinds(
	registry *kind.Registry[kind.ControlPlaneBinding],
	stores ControlPlaneKindStores,
	requester noderequest.Requester,
	producer domain.Producer,
	logger *slog.Logger,
	options ...ControlPlaneKindsOption,
) *ControlPlaneKinds {
	settings := controlPlaneKindsOptions{reconcile: kindsReconcileResources.DefaultConfig()}
	for _, option := range options {
		option(&settings)
	}

	if settings.reconcile.Timeouts == nil {
		settings.reconcile.Timeouts = settings.timeouts
	}

	resources := cascade.NewRepository(registry, stores.Resources)

	// a request that waits for what came of its command is told by whichever
	// result handler hears it in this process.
	waiting := waiters.New()

	dispatcher := dispatch.New(resources, producer, waiting, nil)

	observing := []observe.Option{observe.WithOrphans(dispatcher)}
	if settings.parents != nil {
		observing = append(observing, observe.WithParents(settings.parents))
	}

	observer := observe.NewObserver(registry, resources, logger, observing...)

	useCases := controlPlaneKindsAPI.UseCases{
		Admit:  admitResource.NewUseCase(registry, resources, dispatcher, logger),
		Act:    actOnResource.NewUseCase(registry, resources, dispatcher, logger),
		Delete: deleteResource.NewUseCase(registry, resources, dispatcher),
		Get:    getResource.NewUseCase(registry, resources),
		List:   getResources.NewUseCase(registry, resources),
		Query:  queryResource.NewUseCase(registry, resources, requester, observer, nil),
		Kinds:  getKinds.NewUseCase(registry),
	}

	return &ControlPlaneKinds{
		Route: func(mux *http.ServeMux) error {
			return controlPlaneKindsAPI.Route(mux, registry.Descriptors(), useCases)
		},
		Subscribers: map[string]domain.MessageHandler{
			kind.ResourceActedOnName: recordResult.NewResourceActedOnHandler(registry, resources, waiting, logger, nil, recordResult.WithRestorer(resources.Cascade())),
		},
		Observer:   observer,
		Reconcile:  kindsReconcileResources.NewUseCase(registry, resources, stores.Nodes, dispatcher, logger, settings.reconcile),
		Admit:      useCases.Admit,
		Resources:  resources,
		Dispatcher: dispatcher,
	}
}
