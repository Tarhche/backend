package workload

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/danceable/provider"
	"go.mongodb.org/mongo-driver/v2/mongo"

	checkhealth "github.com/khanzadimahdi/testproject/application/app/checkHealth"
	ingressCheckOrchestratorExists "github.com/khanzadimahdi/testproject/application/workload/ingress/checkOrchestratorExists"
	ingressContract "github.com/khanzadimahdi/testproject/domain/workload/ingress"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
	infraHealth "github.com/khanzadimahdi/testproject/infrastructure/health"
	taskrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/workload/tasks"
	vmrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/workload/vms"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/profiler"
	"github.com/khanzadimahdi/testproject/infrastructure/tunnel"
	infraIngress "github.com/khanzadimahdi/testproject/infrastructure/workload/ingress"
	healthAPI "github.com/khanzadimahdi/testproject/presentation/http/health"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	ingressAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/ingress"
)

const (
	// IngressTunnel is the tunnel, which the serve command needs in order to
	// take the orchestrators' connections.
	IngressTunnel = "workload:ingress:tunnel"

	// IngressForwarder is the ports arbitrary TCP arrives on, which the serve
	// command needs in order to listen on them.
	IngressForwarder = "workload:ingress:forwarder"

	// workloadAPIService is the name an orchestrator offers its own http api under. The
	// ingress asks for a service rather than an address, so where the orchestrator
	// serves it is the orchestrator's own business.
	workloadAPIService = "api"

	ingressLoggerName string = "workload-ingress"

	// idleConnectionTimeout is how long the ingress keeps an orchestrator's connection
	// after a request, ready for the next one. It is deliberately shorter than
	// the orchestrator's own idle time, so the side that opened a connection is never
	// the one surprised by it closing.
	idleConnectionTimeout = 3 * time.Minute
)

// ingressProvider builds the tunnel the orchestrators connect to, the registry of
// what is connected, and the HTTP handler that routes to them.
type ingressProvider struct{}

// Ensure ingressProvider implements provider interface.
var _ provider.Provider = &ingressProvider{}

func NewIngressProvider() *ingressProvider {
	return &ingressProvider{}
}

func (p *ingressProvider) Register(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *ingressProvider) Boot(ctx context.Context, c provider.Container) error {
	var logger *slog.Logger
	if err := c.Resolve(&logger, provider.WithParams(ingressLoggerName)); err != nil {
		return err
	}

	var ingressConfigs *configs.WorkloadIngress
	if err := c.Resolve(&ingressConfigs); err != nil {
		return err
	}

	tunnelConfig := tunnel.DefaultConfig()
	tunnelConfig.MaxStreamsPerSession = ingressConfigs.TunnelMaxStreamsPerSession
	tunnelConfig.MaxSessions = ingressConfigs.TunnelMaxSessionsPerOrchestrator

	// who an orchestrator is comes from the certificate TLS already verified, not from
	// what it said. Whether that orchestrator may stay is a separate question, asked
	// of the authorizer.
	authorizer := tunnel.AllowSignedAgents()
	if allowed := ingressConfigs.AllowedOrchestrators(); len(allowed) > 0 {
		authorizer = tunnel.AllowAgents(allowed...)
	}

	auth := tunnel.NewCertificateAuthenticator(
		certificate.SubjectAlternativeName(ingressConfigs.TunnelIdentitySuffix),
		authorizer,
	)
	auth.MaxSessions = ingressConfigs.TunnelMaxSessionsPerOrchestrator

	// the tunnel is the registry: a workload is reachable for exactly as long as
	// its connections are open, so there is one thing holding both facts.
	tunnelIngress, err := tunnel.NewHub(tunnelConfig, auth, logger)
	if err != nil {
		return err
	}

	if err := c.Bind(func() *tunnel.Hub { return tunnelIngress }, provider.Singleton(), provider.WithName(IngressTunnel)); err != nil {
		return err
	}

	// the ports arbitrary TCP arrives on. They carry onto the connections the
	// orchestrators already opened, so forwarding a port adds a way in for clients
	// and no way in to an orchestrator.
	forwards, err := ingressConfigs.Forwards()
	if err != nil {
		return err
	}

	forwarder, err := tunnel.NewForwarder(tunnelIngress, logger, forwards...)
	if err != nil {
		return err
	}

	if err := c.Bind(func() *tunnel.Forwarder { return forwarder }, provider.Singleton(), provider.WithName(IngressForwarder)); err != nil {
		return err
	}

	if err := c.Bind(func() ingressContract.Registry { return infraIngress.NewRegistry(tunnelIngress) }, provider.Singleton()); err != nil {
		return err
	}

	return c.Bind(ingressConsoleCommand, provider.Singleton())
}

func (p *ingressProvider) Terminate(ctx context.Context) error {
	return nil
}

func ingressConsoleCommand(
	database *mongo.Database,
	tracedProfiler *profiler.TracedProfiler,
	registry ingressContract.Registry,
	iocContainer provider.Container,
) (http.Handler, error) {
	var logger *slog.Logger
	if err := iocContainer.Resolve(&logger, provider.WithParams(ingressLoggerName)); err != nil {
		return nil, err
	}

	var ingressConfigs *configs.WorkloadIngress
	if err := iocContainer.Resolve(&ingressConfigs); err != nil {
		return nil, err
	}

	var tunnelIngress *tunnel.Hub
	if err := iocContainer.Resolve(&tunnelIngress, provider.ResolveName(IngressTunnel)); err != nil {
		return nil, err
	}

	checkOrchestratorExistsUseCase := ingressCheckOrchestratorExists.NewUseCase(registry)

	// which node is holding a VM or a task is the control plane's record of it,
	// and the only thing here that outlives a connection.
	taskRepository := taskrepository.NewRepository(database)
	vmRepository := vmrepository.NewRepository(database)

	checkHealthUseCase := checkhealth.NewUseCase(
		checkhealth.Dependency{Name: "database", Pinger: infraHealth.NewMongodbPinger(database)},
	)

	// a transport that dials nothing: the address it is handed names a workload,
	// and what comes back is a stream on a connection that workload already
	// opened.
	transport := infraIngress.NewTransport(tunnelIngress, workloadAPIService, idleConnectionTimeout)

	mux := http.NewServeMux()

	// CORS goes on the ingress's own answers and no further: what it proxies is
	// the orchestrator's or the task's to answer for, a preflight included. A
	// header set here would otherwise arrive alongside the one upstream sent,
	// and two Access-Control-Allow-Origin headers are worse than none.

	// the task healthcheck probes this
	mux.Handle("GET /health", middleware.NewCORSMiddleware(healthAPI.NewHealthHandler(checkHealthUseCase)))
	mux.Handle("/orchestrators/{name}/{path...}", ingressAPI.NewProxyHandler(checkOrchestratorExistsUseCase, transport, logger))

	// a terminal, which the browser opens here rather than anywhere else: the
	// ingress works out which node is holding the task and carries the
	// connection there. Who may open one is the node's to decide, from the
	// owner on the task and the token on this request.
	mux.Handle("GET /tasks/{uuid}/attach", ingressAPI.NewTerminalHandler(taskRepository, registry, transport, logger))

	// a terminal inside a VM, carried the same way to the node holding it.
	mux.Handle("GET /vms/{uuid}/attach", ingressAPI.NewVMTerminalHandler(vmRepository, registry, transport, logger))

	// the kinds whose resources the ingress finds, by their ingress
	// strategies: each stream of theirs, a terminal say, is carried the same
	// way to the node holding the resource, under the kind's own plural, and
	// a slug that is neither a VM's nor a task's is asked of those with
	// endpoints.
	kinds, err := ingressKinds()
	if err != nil {
		return nil, err
	}

	if err := iocContainer.Bind(func() *kind.Registry[kind.IngressBinding] { return kinds }, provider.Singleton()); err != nil {
		return nil, err
	}

	if err := ingressAPI.RouteKinds(mux, kinds, registry, transport, logger); err != nil {
		return nil, err
	}

	// a request to a hostname under the workload's domain is a VM's, a
	// task's or a resource's own traffic and goes to the node holding it;
	// everything else is one of the ingress's own routes.
	router := ingressAPI.NewRouter(
		ingressAPI.NewTaskHandler(taskRepository, vmRepository, kinds, registry, transport, ingressConfigs.Domain, logger),
		mux,
		ingressConfigs.Domain,
	)

	// no rate limit: what comes through here is a task's own traffic —
	// an attached terminal, a log being followed, whatever the task itself
	// serves — which a cap per minute would cut off rather than pace.
	handler := middleware.NewRecoveryMiddleware(
		middleware.NewRequestIDMiddleware(
			middleware.NewTelemetryMiddleware(
				"/workload/ingress",
				middleware.NewProfilingMiddleware(
					middleware.NewLogMiddleware(
						router,
						logger,
					),
					tracedProfiler,
				),
			),
		),
		logger,
	)

	return handler, nil
}
