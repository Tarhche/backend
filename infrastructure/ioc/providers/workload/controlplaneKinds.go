package workload

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/actOnResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/admitResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/deleteResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getKinds"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getResources"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/observe"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/queryResource"
	kindsReconcile "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/reconcile"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/resourceResult"
	controlPlaneStacks "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/stack"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/waiters"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/slugs"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	controlPlaneKindsAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/kinds"
)

// NewControlPlaneRegistry is a registry for every kind the control plane
// runs, with none in it yet: a kind is registered by RegisterControlPlaneKinds,
// once what its strategy is built from is.
func NewControlPlaneRegistry() (*kind.Registry[kind.ControlPlaneBinding], error) {
	return kind.NewRegistry[kind.ControlPlaneBinding](), nil
}

// RegisterControlPlaneKinds registers every kind the control plane runs,
// each through its control-plane strategy: the one place a kind is added to
// the control plane, and what its resource API, its consumers and its
// reconcile loop all run over. A stack is admitted into the Docker VMs vms
// keeps, and given a slug no stack in resources holds.
//
// VMs, snapshots and the code runner's tasks are not kinds yet. Each moves
// onto the framework in a step of its own, and is registered here when it
// does, taking its routes over from the ones it has today.
func RegisterControlPlaneKinds(registry *kind.Registry[kind.ControlPlaneBinding], vms *ControlPlaneVMs, resources resource.Repository) error {
	stackSlugs := slugs.By(func(ctx context.Context, slug string) (resource.Record, error) {
		return resources.GetOneBySlug(ctx, stackKind.Name, slug)
	})

	return registry.Register(kind.BindControlPlane[stackKind.Spec, stackKind.Status](
		stackKind.Descriptor(),
		controlPlaneStacks.New(vms.VMs, vms.Chooser, stackSlugs),
	))
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

	// Subscribers hear workloadResult.
	Subscribers map[string]domain.MessageHandler

	Observer  *observe.Observer
	Reconcile *kindsReconcile.UseCase
}

// controlPlaneKindsOptions are how the plumbing goes about its work.
type controlPlaneKindsOptions struct {
	parents   observe.Parents
	reconcile kindsReconcile.Config
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
func WithReconcileConfig(config kindsReconcile.Config) ControlPlaneKindsOption {
	return func(o *controlPlaneKindsOptions) {
		o.reconcile = config
	}
}

// NewControlPlaneKinds builds the control plane's plumbing for the kinds in
// registry, asking the nodes queries through requester and sending them
// commands with producer.
//
// It is the serve command's wiring, and what a test builds a control plane's
// kinds from, over stores kept in memory: what is tested is what is served.
func NewControlPlaneKinds(
	registry *kind.Registry[kind.ControlPlaneBinding],
	stores ControlPlaneKindStores,
	requester noderequest.Requester,
	producer domain.Producer,
	logger *slog.Logger,
	options ...ControlPlaneKindsOption,
) *ControlPlaneKinds {
	settings := controlPlaneKindsOptions{reconcile: kindsReconcile.DefaultConfig()}
	for _, option := range options {
		option(&settings)
	}

	// a request that waits for what came of its command is told by whichever
	// result handler hears it in this process.
	waiting := waiters.New()

	dispatcher := dispatch.New(stores.Resources, producer, waiting, nil)

	var observing []observe.Option
	if settings.parents != nil {
		observing = append(observing, observe.WithParents(settings.parents))
	}

	observer := observe.NewObserver(registry, stores.Resources, logger, observing...)

	useCases := controlPlaneKindsAPI.UseCases{
		Admit:  admitResource.NewUseCase(registry, stores.Resources, dispatcher, logger),
		Act:    actOnResource.NewUseCase(registry, stores.Resources, dispatcher),
		Delete: deleteResource.NewUseCase(registry, stores.Resources, dispatcher),
		Get:    getResource.NewUseCase(registry, stores.Resources),
		List:   getResources.NewUseCase(registry, stores.Resources),
		Query:  queryResource.NewUseCase(registry, stores.Resources, requester, observer, nil),
		Kinds:  getKinds.NewUseCase(registry),
	}

	return &ControlPlaneKinds{
		Route: func(mux *http.ServeMux) error {
			return controlPlaneKindsAPI.Route(mux, registry.Descriptors(), useCases)
		},
		Subscribers: map[string]domain.MessageHandler{
			kind.ResultName: resourceResult.NewResult(registry, stores.Resources, waiting, logger, nil),
		},
		Observer:  observer,
		Reconcile: kindsReconcile.NewUseCase(registry, stores.Resources, stores.Nodes, dispatcher, logger, settings.reconcile),
	}
}
