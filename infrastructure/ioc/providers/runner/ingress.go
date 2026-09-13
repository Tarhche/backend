package runner

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/danceable/provider"

	checkhealth "github.com/khanzadimahdi/testproject/application/app/checkHealth"
	ingressCheckWorkerExists "github.com/khanzadimahdi/testproject/application/runner/ingress/checkWorkerExists"
	ingressContract "github.com/khanzadimahdi/testproject/domain/runner/ingress"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
	infraIngress "github.com/khanzadimahdi/testproject/infrastructure/runner/ingress"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/profiler"
	"github.com/khanzadimahdi/testproject/infrastructure/tunnel"
	healthAPI "github.com/khanzadimahdi/testproject/presentation/http/health"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	ingressAPI "github.com/khanzadimahdi/testproject/presentation/http/runner/ingress"
)

const (
	// IngressTunnel is the tunnel, which the serve command needs in order to
	// take the workers' connections.
	IngressTunnel = "runner:ingress:tunnel"

	// IngressForwarder is the ports arbitrary TCP arrives on, which the serve
	// command needs in order to listen on them.
	IngressForwarder = "runner:ingress:forwarder"

	// runnerAPIService is the name a worker offers its own http api under. The
	// ingress asks for a service rather than an address, so where the worker
	// serves it is the worker's own business.
	runnerAPIService = "api"

	ingressLoggerName string = "runner-ingress"

	// idleConnectionTimeout is how long the ingress keeps a worker's connection
	// after a request, ready for the next one. It is deliberately shorter than
	// the worker's own idle time, so the side that opened a connection is never
	// the one surprised by it closing.
	idleConnectionTimeout = 60 * time.Second
)

// ingressProvider builds the tunnel the workers connect to, the registry of
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

	var ingressConfigs *configs.RunnerIngress
	if err := c.Resolve(&ingressConfigs); err != nil {
		return err
	}

	tunnelConfig := tunnel.DefaultConfig()
	tunnelConfig.MaxStreamsPerSession = ingressConfigs.TunnelMaxStreamsPerSession
	tunnelConfig.MaxSessions = ingressConfigs.TunnelMaxSessionsPerWorker

	// who a worker is comes from the certificate TLS already verified, not from
	// what it said. Whether that worker may stay is a separate question, asked
	// of the authorizer.
	authorizer := tunnel.AllowSignedAgents()
	if allowed := ingressConfigs.AllowedWorkers(); len(allowed) > 0 {
		authorizer = tunnel.AllowAgents(allowed...)
	}

	auth := tunnel.NewCertificateAuthenticator(
		certificate.SubjectAlternativeName(ingressConfigs.TunnelIdentitySuffix),
		authorizer,
	)
	auth.MaxSessions = ingressConfigs.TunnelMaxSessionsPerWorker

	// the tunnel is the registry: a runner is reachable for exactly as long as
	// its connections are open, so there is one thing holding both facts.
	tunnelIngress, err := tunnel.NewHub(tunnelConfig, auth, logger)
	if err != nil {
		return err
	}

	if err := c.Bind(func() *tunnel.Hub { return tunnelIngress }, provider.Singleton(), provider.WithName(IngressTunnel)); err != nil {
		return err
	}

	// the ports arbitrary TCP arrives on. They carry onto the connections the
	// workers already opened, so forwarding a port adds a way in for clients
	// and no way in to a worker.
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
	tracedProfiler *profiler.TracedProfiler,
	registry ingressContract.Registry,
	iocContainer provider.Container,
) (http.Handler, error) {
	var logger *slog.Logger
	if err := iocContainer.Resolve(&logger, provider.WithParams(ingressLoggerName)); err != nil {
		return nil, err
	}

	var tunnelIngress *tunnel.Hub
	if err := iocContainer.Resolve(&tunnelIngress, provider.ResolveName(IngressTunnel)); err != nil {
		return nil, err
	}

	checkWorkerExistsUseCase := ingressCheckWorkerExists.NewUseCase(registry)

	// the ingress talks to nothing it has to reach: it holds the connections
	// the workers opened, and there is nothing to be reachable but itself.
	checkHealthUseCase := checkhealth.NewUseCase()

	// a transport that dials nothing: the address it is handed names a runner,
	// and what comes back is a stream on a connection that runner already
	// opened.
	transport := infraIngress.NewTransport(tunnelIngress, runnerAPIService, idleConnectionTimeout)

	mux := http.NewServeMux()

	mux.Handle("GET /health", middleware.NewCORSMiddleware(healthAPI.NewHealthHandler(checkHealthUseCase)))
	mux.Handle("/workers/{name}/{path...}", ingressAPI.NewProxyHandler(checkWorkerExistsUseCase, transport, logger))

	// no rate limit: what comes through here is a container's own traffic —
	// an attached terminal, a log being followed — which a cap per minute
	// would cut off rather than pace.
	handler := middleware.NewRecoveryMiddleware(
		middleware.NewRequestIDMiddleware(
			middleware.NewTelemetryMiddleware(
				"/runner/ingress",
				middleware.NewProfilingMiddleware(
					middleware.NewLogMiddleware(
						mux,
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
