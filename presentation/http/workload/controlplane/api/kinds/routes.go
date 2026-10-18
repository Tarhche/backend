// Package kinds is the control plane's resource API: the same routes for
// every kind it runs, under the kind's plural, which the blog reaches every
// kind's resources through.
//
//	GET    /api/{plural}?owner=&parent=&page=        a page of manifests
//	POST   /api/{plural}?owner=&parent=&wait=&dry_run=        admit one
//	GET    /api/{plural}/{uuid}?owner=&parent=       one manifest
//	DELETE /api/{plural}/{uuid}?owner=&parent=&wait=&cascade=&dry_run= delete one
//	POST   /api/{plural}/{uuid}/actions/{action}?owner=&parent=&wait=&cascade=   a command
//	GET    /api/{plural}/{uuid}/plan?owner=&parent=  how a root's plan went
//	GET    /api/{plural}/{uuid}/dependents?owner=&parent=  which roots need it
//	GET    /api/{plural}/{uuid}/{query}?owner=&parent=&…     a query, its payload as parameters
//	GET    /api/kinds                                every kind, as it describes itself
//
// A parent holds a request about one resource to what lives in it, and
// inside it a kind may name the resource by more than its uuid: a container
// by its Docker id or its name, in its Docker VM.
//
// A delete, or a command that deletes, of what the graphs of roots need is
// answered with those roots, and whether one of them makes it again, as 202
// whatever came of it; cascade deletes them with it. The same roots are read
// with a GET of what needs it (getDependents), which changes nothing and asks
// nothing of any node, whatever else the control plane knows: a delete that
// was only to be asked about is never made.
//
// dry_run asks what an admission or a delete would do, and does nothing
// (planResource): 200 with what would be admitted, and, of a root, what
// carrying out its graph would do; or what removing a root would do, or which
// roots need what is not one, and, with cascade, what removing each would do.
// A route that serves no dry run refuses one, rather than doing what it asks.
//
// They answer the way the rest of the control plane's API does: JSON, a
// refusal as 400 with the codes it was refused for, what is not there as
// 404, and what a node refused, or a resource that cannot be asked, with
// the status its code stands for.
//
// A kind's routes are registered for the kinds registered, each under its
// own plural, never as a pattern that would take any plural: the routes the
// control plane serves beside them, its nodes' say, stay exactly as they
// are, and a kind registered under a plural whose routes are taken already is
// refused when the control plane starts, rather than shadowing them.
package kinds

import (
	"fmt"
	"net/http"

	actOnResource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/actOnResource"
	admitResource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/admitResource"
	deleteResource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/deleteResource"
	getDependents "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getDependents"
	getKinds "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getKinds"
	getPlan "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getPlan"
	getResource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getResource"
	getResources "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getResources"
	planResource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/planResource"
	queryResource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/queryResource"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// UseCases are what the resource API is served by. Plan, the dry runs,
// GetPlan, how a root's plan went, and Dependents, which roots need a
// resource, may be nothing: a route then refuses a dry run, and no kind's
// plan, nor what needs one of its resources, is served.
type UseCases struct {
	Admit      *admitResource.UseCase
	Act        *actOnResource.UseCase
	Delete     *deleteResource.UseCase
	Get        *getResource.UseCase
	List       *getResources.UseCase
	Query      *queryResource.UseCase
	Kinds      *getKinds.UseCase
	Plan       *planResource.UseCase
	GetPlan    *getPlan.UseCase
	Dependents *getDependents.UseCase
}

// Route serves the resource API on mux for every kind described, under its
// plural, and the kinds themselves. A route taken already is an error, and
// nothing that was being registered after it is.
func Route(mux *http.ServeMux, descriptors []kind.Descriptor, useCases UseCases) (err error) {
	defer func() {
		// the mux refuses a pattern another one takes by panicking, which is
		// a kind registered over routes that are not its.
		if refused := recover(); refused != nil {
			err = fmt.Errorf("the resource api cannot be served: %v", refused)
		}
	}()

	for _, d := range descriptors {
		base := "/api/" + d.Plural

		mux.Handle("GET "+base, NewIndexHandler(useCases.List, d.Name))
		mux.Handle("POST "+base, NewCreateHandler(useCases.Admit, useCases.Plan, d.Name))
		mux.Handle("GET "+base+"/{uuid}", NewShowHandler(useCases.Get, d.Name))
		mux.Handle("DELETE "+base+"/{uuid}", NewDeleteHandler(useCases.Delete, useCases.Plan, d.Name))
		mux.Handle("POST "+base+"/{uuid}/actions/{action}", NewActionHandler(useCases.Act, d.Name))
		mux.Handle("GET "+base+"/{uuid}/{query}", NewQueryHandler(useCases.Query, d))

		// a plan is no query of a kind's, and takes the place of none: no kind
		// may name an action after it (kind.Check); and nor is what needs a
		// resource.
		if useCases.GetPlan != nil {
			mux.Handle("GET "+base+"/{uuid}/"+kind.PlanAction, NewPlanHandler(useCases.GetPlan, d.Name))
		}

		if useCases.Dependents != nil {
			mux.Handle("GET "+base+"/{uuid}/"+kind.DependentsAction, NewDependentsHandler(useCases.Dependents, d.Name))
		}
	}

	mux.Handle("GET /api/kinds", NewKindsHandler(useCases.Kinds))

	return nil
}
