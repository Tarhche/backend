package workload

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/danceable/provider"
	"github.com/nats-io/nats.go"

	checkhealth "github.com/khanzadimahdi/testproject/application/app/checkHealth"
	ingressCheckOrchestratorExists "github.com/khanzadimahdi/testproject/application/workload/ingress/checkOrchestratorExists"
	"github.com/khanzadimahdi/testproject/domain"
	ingressContract "github.com/khanzadimahdi/testproject/domain/workload/ingress"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
	infraHealth "github.com/khanzadimahdi/testproject/infrastructure/health"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/core/pubsub"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/profiler"
	"github.com/khanzadimahdi/testproject/infrastructure/tunnel"
	infraIngress "github.com/khanzadimahdi/testproject/infrastructure/workload/ingress"
	ingressMemory "github.com/khanzadimahdi/testproject/infrastructure/workload/ingress/memory"
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

	// IngressSubscribers are what the ingress hears, which the serve command
	// subscribes to before it routes anything: what says where the tasks and
	// the VMs are, their nodes' heartbeats, and the commands sent to their
	// nodes and what came of them.
	IngressSubscribers = "workload:ingress:subscribers"

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
// what is connected, what the ingress hears of where the tasks and the VMs
// are, and the HTTP handler that routes to them.
type ingressProvider struct {
	terminate func()
}

// Ensure ingressProvider implements provider interface.
var _ provider.Provider = &ingressProvider{}

func NewIngressProvider() *ingressProvider {
	return &ingressProvider{}
}

func (p *ingressProvider) Register(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *ingressProvider) Boot(ctx context.Context, c provider.Container) error {
	var natsConnection *nats.Conn
	if err := c.Resolve(&natsConnection); err != nil {
		return err
	}

	var logger *slog.Logger
	if err := c.Resolve(&logger, provider.WithParams(ingressLoggerName)); err != nil {
		return err
	}

	var ingressConfigs *configs.WorkloadIngress
	if err := c.Resolve(&ingressConfigs); err != nil {
		return err
	}

	// what the ingress hears, it hears over core NATS rather than through a
	// JetStream consumer: every ingress hears every heartbeat, command and
	// result as it is said, beside the consumers that keep them for the nodes
	// and the control plane, and nothing is kept for one that is not
	// listening, which a beat later would hear said again anyway.
	ps := pubsub.NewPublishSubscriber(natsConnection, logger)

	if err := c.Bind(func() domain.Subscriber { return ps }, provider.Singleton()); err != nil {
		return err
	}

	if err := c.Bind(func() domain.PublishSubscriber { return ps }, provider.Singleton()); err != nil {
		return err
	}

	p.terminate = func() {
		defer ps.Wait()
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
	if p.terminate != nil {
		p.terminate()
	}

	return nil
}

func ingressConsoleCommand(
	natsConnection *nats.Conn,
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

	// the ingress talks to no database, so messaging, which says where
	// everything it routes to is, is its only dependency
	checkHealthUseCase := checkhealth.NewUseCase(
		checkhealth.Dependency{Name: "messaging", Pinger: infraHealth.NewNatsPinger(natsConnection)},
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

	// which node holds a task or a VM is what that node last said of it,
	// less what a command on its way to it takes away, kept here for as long
	// as it goes on saying it: one unheard for as long as the control plane
	// waits before it takes one to be gone from its node is forgotten.
	locations := ingressMemory.NewLocations(ingressConfigs.ResourceSilentAfter)

	// the kinds whose resources the ingress finds, by their ingress
	// strategies: each stream of theirs, a terminal, which the browser opens
	// here rather than anywhere else, is carried to the node holding the
	// resource, under the kind's own plural, a task's at /tasks/{uuid}/attach
	// and a VM's at /vms/{uuid}/attach; and a slug is asked of those with
	// endpoints, a task's first. Who may open a terminal is the node's to
	// decide, from the owner on the resource and the token on the request.
	// What the ingress reads of them is what their nodes said and what was
	// sent to their nodes, which the subscribers hear.
	workload, err := NewIngressWorkload(locations, logger)
	if err != nil {
		return nil, err
	}

	if err := iocContainer.Bind(func() *kind.Registry[kind.IngressBinding] { return workload.Kinds }, provider.Singleton()); err != nil {
		return nil, err
	}

	if err := ingressAPI.RouteKinds(mux, workload.Kinds, registry, transport, logger); err != nil {
		return nil, err
	}

	// a request to a hostname under the workload's domain is a resource's own
	// traffic, a task's or a VM's, and goes to the node holding it;
	// everything else is one of the ingress's own routes.
	router := ingressAPI.NewRouter(
		ingressAPI.NewTaskHandler(workload.Kinds, registry, transport, ingressConfigs.Domain, logger),
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

	// ingress subscribers
	if err := iocContainer.Bind(func() map[string]domain.MessageHandler {
		return workload.Subscribers
	}, provider.Singleton(), provider.WithName(IngressSubscribers)); err != nil {
		return nil, err
	}

	return handler, nil
}
