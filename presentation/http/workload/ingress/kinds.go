package ingress

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/khanzadimahdi/testproject/domain/workload/ingress"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// RouteKinds routes every stream action of every kind the ingress finds the
// resources of, GET /{plural}/{uuid}/{action}, to the node holding the
// resource, the way a VM's terminal is carried: the kind's ingress strategy
// says which node that is, and the node decides who may open one.
//
// A kind is routed once it is registered and not before, so these routes
// never meet the VMs' and the tasks' own until a kind takes them over; and
// one whose route is taken already is an error, rather than the panic a mux
// answers it with.
func RouteKinds(
	mux *http.ServeMux,
	kinds *kind.Registry[kind.IngressBinding],
	registry ingress.Registry,
	transport http.RoundTripper,
	logger *slog.Logger,
) error {
	for _, binding := range kinds.All() {
		d := binding.Descriptor()

		for _, action := range d.Actions {
			if action.Mode != kind.ModeStream {
				continue
			}

			if err := handle(mux, "GET /"+d.Plural+"/{uuid}/"+action.Name, NewKindTerminalHandler(binding, action.Name, registry, transport, logger)); err != nil {
				return err
			}
		}
	}

	return nil
}

// handle routes pattern to handler, and is why it cannot be when the mux
// refuses it: a pattern taken already.
func handle(mux *http.ServeMux, pattern string, handler http.Handler) (err error) {
	defer func() {
		if refused := recover(); refused != nil {
			err = fmt.Errorf("%q cannot be routed: %v", pattern, refused)
		}
	}()

	mux.Handle(pattern, handler)

	return nil
}
