// Package kinds is the control plane's resource API: the same routes for
// every kind it runs, under the kind's plural, which the blog reaches every
// kind's resources through.
//
//	GET    /api/{plural}?owner=&parent=&page=        a page of manifests
//	POST   /api/{plural}?owner=&parent=&wait=        admit one
//	GET    /api/{plural}/{uuid}?owner=               one manifest
//	DELETE /api/{plural}/{uuid}?owner=&wait=         delete one
//	POST   /api/{plural}/{uuid}/actions/{action}?owner=&wait=   a command
//	GET    /api/{plural}/{uuid}/{query}?owner=&…     a query, its payload as parameters
//	GET    /api/kinds                                every kind, as it describes itself
//
// They answer the way the rest of the control plane's API does: JSON, a
// refusal as 400 with the codes it was refused for, what is not there as
// 404, and what a node refused, or a resource that cannot be asked, with
// the status its code stands for.
//
// A kind's routes are registered for the kinds registered, each under its
// own plural, never as a pattern that would take any plural: the routes of a
// kind that is not on the framework yet, /api/vms say, stay exactly as they
// are, and a kind registered under a plural whose routes are taken already is
// refused when the control plane starts, rather than shadowing them.
package kinds

import (
	"fmt"
	"net/http"

	actOnResource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/actOnResource"
	admitResource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/admitResource"
	deleteResource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/deleteResource"
	getKinds "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getKinds"
	getResource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getResource"
	getResources "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getResources"
	queryResource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/queryResource"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// UseCases are what the resource API is served by.
type UseCases struct {
	Admit  *admitResource.UseCase
	Act    *actOnResource.UseCase
	Delete *deleteResource.UseCase
	Get    *getResource.UseCase
	List   *getResources.UseCase
	Query  *queryResource.UseCase
	Kinds  *getKinds.UseCase
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
		mux.Handle("POST "+base, NewCreateHandler(useCases.Admit, d.Name))
		mux.Handle("GET "+base+"/{uuid}", NewShowHandler(useCases.Get, d.Name))
		mux.Handle("DELETE "+base+"/{uuid}", NewDeleteHandler(useCases.Delete, d.Name))
		mux.Handle("POST "+base+"/{uuid}/actions/{action}", NewActionHandler(useCases.Act, d.Name))
		mux.Handle("GET "+base+"/{uuid}/{query}", NewQueryHandler(useCases.Query, d))
	}

	mux.Handle("GET /api/kinds", NewKindsHandler(useCases.Kinds))

	return nil
}
