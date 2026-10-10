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
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
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
// runs: the resource API, the result consumer, the observer every kind's
// heartbeat is handed to, and the reconcile loop that keeps every resource as
// it was asked to be.
type ControlPlaneKinds struct {
	// Route serves the resource API, a kind's routes under its plural, and
	// is an error for a kind whose routes are taken already.
	Route func(mux *http.ServeMux) error

	// Subscribers hear workloadResourceActedOn.
	Subscribers map[string]domain.MessageHandler

	Observer  *observe.Observer
	Reconcile *kindsReconcileResources.UseCase

	// Admit takes in a resource of any kind, as the resource API does: a
	// Docker VM made for a container or a stack is admitted through it. Delete
	// deletes one, as the resource API does: what a root's graph made is
	// removed through it.
	Admit  *admitResource.UseCase
	Delete *deleteResource.UseCase

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

	// roots are the kinds whose roots their graphs make and remove, and
	// graphs what is told of them and of what changes under them.
	roots  graph.Roots
	graphs graph.Graphs
}

// ControlPlaneKindsOption changes how the plumbing goes about its work.
type ControlPlaneKindsOption func(*controlPlaneKindsOptions)

// WithParents has what lives in a parent that is not running observed
// waiting on it, as parents say the parent is, rather than gone, once its
// node has long said nothing of it: a stack in a VM that is stopped.
func WithParents(parents observe.Parents) ControlPlaneKindsOption {
	return func(o *controlPlaneKindsOptions) {
		o.parents = parents
	}
}

// WithGraphs leaves to the graphs of roots of the kinds roots names what
// they make and remove, and tells graphs of it: a root is queued to graphs
// once it is admitted, and once it is asked to go, which marks it rather than
// asking its node and is never asked by the reconcile loop; a VM's delete
// keeps what a live root's graph makes again, as gone with it; and graphs is
// told of what went missing, was deleted, was kept or taken away with what it
// lived in, and failed to be made, so that the roots whose graphs it is in
// are carried out at once.
func WithGraphs(roots graph.Roots, graphs graph.Graphs) ControlPlaneKindsOption {
	return func(o *controlPlaneKindsOptions) {
		o.roots = roots
		o.graphs = graphs
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

// settingsOf is how the plumbing goes about its work, as options say.
func settingsOf(options []ControlPlaneKindsOption) controlPlaneKindsOptions {
	settings := controlPlaneKindsOptions{reconcile: kindsReconcileResources.DefaultConfig()}
	for _, option := range options {
		option(&settings)
	}

	if settings.reconcile.Timeouts == nil {
		settings.reconcile.Timeouts = settings.timeouts
	}

	return settings
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
	settings := settingsOf(options)

	// what a VM's delete takes with it, but what a live root's graph makes
	// again, which the graphs are told of once the VM is gone.
	var cascading []cascade.Option
	if settings.graphs != nil {
		cascading = append(cascading, cascade.WithGraphs(settings.roots, settings.graphs))
	}

	resources := cascade.NewRepository(registry, stores.Resources, cascading...)

	// a request that waits for what came of its command is told by whichever
	// result handler hears it in this process.
	waiting := waiters.New()

	dispatcher := dispatch.New(resources, producer, waiting, nil)

	// what a node keeps saying of a resource is written down at least twice in
	// the time the reconcile loop gives a resource to be heard of, so that one
	// its node keeps saying is never taken to be gone.
	observing := []observe.Option{observe.WithOrphans(dispatcher), observe.WithRefresh(settings.reconcile.ResourceSilentAfter / 2)}
	if settings.parents != nil {
		observing = append(observing, observe.WithParents(settings.parents))
	}

	// what the graphs of roots are told of, and leave to nobody else: a root
	// admitted or asked to go, and what went missing, was deleted or failed
	// to be made.
	var (
		admitting   []admitResource.Option
		acting      []actOnResource.Option
		deleting    []deleteResource.Option
		recording   []recordResult.Option
		reconciling []kindsReconcileResources.Option
	)

	if settings.graphs != nil {
		admitting = append(admitting, admitResource.WithGraphs(settings.roots, settings.graphs))
		acting = append(acting, actOnResource.WithGraphs(settings.roots, settings.graphs))
		deleting = append(deleting, deleteResource.WithGraphs(settings.roots, settings.graphs))
		observing = append(observing, observe.WithNotifier(settings.graphs))
		recording = append(recording, recordResult.WithNotifier(settings.graphs))
		reconciling = append(reconciling, kindsReconcileResources.WithRoots(settings.roots))
	}

	observer := observe.NewObserver(registry, resources, logger, observing...)

	useCases := controlPlaneKindsAPI.UseCases{
		Admit:  admitResource.NewUseCase(registry, resources, dispatcher, logger, admitting...),
		Act:    actOnResource.NewUseCase(registry, resources, dispatcher, logger, acting...),
		Delete: deleteResource.NewUseCase(registry, resources, dispatcher, deleting...),
		Get:    getResource.NewUseCase(registry, resources),
		List:   getResources.NewUseCase(registry, resources),
		Query:  queryResource.NewUseCase(registry, resources, requester, observer, nil),
		Kinds:  getKinds.NewUseCase(registry),
	}

	recording = append(recording, recordResult.WithRestorer(resources.Cascade()))

	return &ControlPlaneKinds{
		Route: func(mux *http.ServeMux) error {
			return controlPlaneKindsAPI.Route(mux, registry.Descriptors(), useCases)
		},
		Subscribers: map[string]domain.MessageHandler{
			kind.ResourceActedOnName: recordResult.NewResourceActedOnHandler(registry, resources, waiting, logger, nil, recording...),
		},
		Observer:   observer,
		Reconcile:  kindsReconcileResources.NewUseCase(registry, resources, stores.Nodes, dispatcher, observer, logger, settings.reconcile, reconciling...),
		Admit:      useCases.Admit,
		Delete:     useCases.Delete,
		Resources:  resources,
		Dispatcher: dispatcher,
	}
}
