package workload

import (
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
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/waiters"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	controlPlaneKindsAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/kinds"
)

// NewControlPlaneRegistry is every kind the control plane runs, each
// registered through its control-plane binding: the one place a kind is
// added to the control plane, and what its resource API, its consumers and
// its reconcile loop all run over.
//
// None is registered yet. Each kind moves onto the framework in a step of
// its own, and is registered here when it does, taking its routes over from
// the ones it has today.
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

	// Subscribers hear workloadResult.
	Subscribers map[string]domain.MessageHandler

	Observer  *observe.Observer
	Reconcile *kindsReconcile.UseCase
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
) *ControlPlaneKinds {
	// a request that waits for what came of its command is told by whichever
	// result handler hears it in this process.
	waiting := waiters.New()

	dispatcher := dispatch.New(stores.Resources, producer, waiting, nil)
	observer := observe.NewObserver(registry, stores.Resources, logger)

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
		Reconcile: kindsReconcile.NewUseCase(registry, stores.Resources, stores.Nodes, dispatcher, logger, kindsReconcile.DefaultConfig()),
	}
}
