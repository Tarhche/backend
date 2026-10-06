package kinds

import (
	"fmt"
	"log/slog"
	"net/http"

	attachresource "github.com/khanzadimahdi/testproject/application/workload/orchestrator/attachResource"
	getresourceendpoint "github.com/khanzadimahdi/testproject/application/workload/orchestrator/getResourceEndpoint"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	portsAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/orchestrator/api/ports"
)

// Routes are the routes of every kind a node runs, which the ingress carries
// streams and ports to.
type Routes struct {
	kinds     *kind.Registry[kind.NodeBinding]
	attach    *attachresource.UseCase
	endpoints *getresourceendpoint.UseCase

	// verifier is what the tokens a stream is opened with are verified
	// against.
	verifier *jwt.JWT

	logger *slog.Logger
}

func NewRoutes(
	kinds *kind.Registry[kind.NodeBinding],
	attach *attachresource.UseCase,
	endpoints *getresourceendpoint.UseCase,
	verifier *jwt.JWT,
	logger *slog.Logger,
) *Routes {
	return &Routes{kinds: kinds, attach: attach, endpoints: endpoints, verifier: verifier, logger: logger}
}

// Register routes every kind registered, under the kind's own plural:
//
//   - on api, each stream action of a kind whose node strategy serves
//     streams, GET /api/{plural}/{uuid}/{action}, which insists on a token
//     and opens the stream for the resource's owner alone, as a VM's terminal
//     is; a public one takes a token when there is one and opens the stream
//     for whomever its kind says, as a code-runner snippet's terminal is
//     opened for anybody;
//   - on ports, the ports of a kind with endpoints whose node strategy
//     serves them, /{plural}/{slug}/{port}/{path...}, which, like a VM's, are
//     the resource's own traffic and answer for themselves. A kind that
//     declares no endpoints has none served, whatever its strategy could
//     say: the node's API is reachable through the ingress, path and all,
//     and its ports are served only where the kind says they are.
//
// A kind is routed once it is registered and not before, so these routes
// never meet the VMs' and the tasks' own until a kind takes them over; and
// one whose route is taken already is an error, rather than the panic a mux
// answers it with.
func (r *Routes) Register(api *http.ServeMux, ports *http.ServeMux) error {
	for _, binding := range r.kinds.All() {
		d := binding.Descriptor()

		if binding.Attaches() {
			for _, action := range d.Actions {
				if action.Mode != kind.ModeStream {
					continue
				}

				var attach http.Handler = middleware.NewTokenMiddleware(NewAttachHandler(r.attach, d.Name, action.Name, r.logger), r.verifier)
				if action.Public {
					attach = middleware.NewOptionalTokenMiddleware(NewAttachHandler(r.attach, d.Name, action.Name, r.logger), r.verifier)
				}

				if err := handle(api, "GET /api/"+d.Plural+"/{uuid}/"+action.Name, attach); err != nil {
					return err
				}
			}
		}

		if d.Endpoints && binding.Exposes() {
			if err := handle(ports, "/"+d.Plural+"/{slug}/{port}/{path...}", portsAPI.NewResourceProxyHandler(r.endpoints, d.Name, r.logger)); err != nil {
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
