package runner

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/danceable/provider"

	checkhealth "github.com/khanzadimahdi/testproject/application/app/checkHealth"
	ingressGetRunner "github.com/khanzadimahdi/testproject/application/runner/ingress/getRunner"
	"github.com/khanzadimahdi/testproject/domain"
	ingressContract "github.com/khanzadimahdi/testproject/domain/runner/ingress"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/tunnel"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/profiler"
	healthAPI "github.com/khanzadimahdi/testproject/presentation/http/health"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	ingressAPI "github.com/khanzadimahdi/testproject/presentation/http/runner/ingress"
)

const (
	// IngressTunnel is the tunnel, which the serve command needs in order to
	// take the workers' connections.
	IngressTunnel = "runner:ingress:tunnel"

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
	authorizer := tunnel.AllowSignedWorkers()
	if allowed := ingressConfigs.AllowedWorkers(); len(allowed) > 0 {
		authorizer = tunnel.AllowWorkers(allowed...)
	}

	auth := tunnel.NewCertificateAuthenticator(
		certificate.SubjectAlternativeName(ingressConfigs.TunnelIdentitySuffix),
		authorizer,
	)
	auth.MaxSessions = ingressConfigs.TunnelMaxSessionsPerWorker

	// the tunnel is the registry: a runner is reachable for exactly as long as
	// its connections are open, so there is one thing holding both facts.
	tunnelIngress, err := tunnel.NewIngress(tunnelConfig, auth, logger)
	if err != nil {
		return err
	}

	if err := c.Bind(func() *tunnel.Ingress { return tunnelIngress }, provider.Singleton(), provider.WithName(IngressTunnel)); err != nil {
		return err
	}

	if err := c.Bind(func() ingressContract.Registry { return runnerRegistry{tunnelIngress} }, provider.Singleton()); err != nil {
		return err
	}

	return c.Bind(ingressConsoleCommand, provider.Singleton())
}

// runnerRegistry is the tunnel's view of what is connected, told in the terms
// the rest of the application already has.
type runnerRegistry struct {
	ingress *tunnel.Ingress
}

var _ ingressContract.Registry = runnerRegistry{}

func (r runnerRegistry) Get(_ context.Context, id string) (ingressContract.Runner, error) {
	for _, worker := range r.ingress.Workers() {
		if worker.Worker == id {
			return asRunner(worker), nil
		}
	}

	return ingressContract.Runner{}, domain.ErrNotExists
}

func asRunner(worker tunnel.WorkerState) ingressContract.Runner {
	return ingressContract.Runner{ID: worker.Worker}
}

func (p *ingressProvider) Terminate(ctx context.Context) error {
	return nil
}

func ingressConsoleCommand(
	registry ingressContract.Registry,
	iocContainer provider.Container,
) (http.Handler, error) {
	var logger *slog.Logger
	if err := iocContainer.Resolve(&logger, provider.WithParams(ingressLoggerName)); err != nil {
		return nil, err
	}

	var tunnelIngress *tunnel.Ingress
	if err := iocContainer.Resolve(&tunnelIngress, provider.ResolveName(IngressTunnel)); err != nil {
		return nil, err
	}

	getRunnerUseCase := ingressGetRunner.NewUseCase(registry)

	// the ingress talks to nothing it has to reach: it holds the connections
	// the workers opened, and there is nothing to be reachable but itself.
	checkHealthUseCase := checkhealth.NewUseCase()

	// a transport that dials nothing. The address it is given names a runner,
	// and what comes back is a stream on one of the connections that runner
	// opened — carried to the worker's own api, which is one of the services it
	// offers. Nothing below this knows the tunnel is not a network.
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _ string, address string) (net.Conn, error) {
			worker, _, err := net.SplitHostPort(address)
			if err != nil {
				worker = address
			}

			return tunnelIngress.Dial(ctx, worker, tunnel.Target{Service: runnerAPIService})
		},
		IdleConnTimeout: idleConnectionTimeout,
	}

	mux := http.NewServeMux()

	// CORS goes on the ingress's own answers and no further: what it proxies is
	// the runner's to answer for, a preflight included. A header set here would
	// otherwise arrive alongside the one the runner sent, and two
	// Access-Control-Allow-Origin headers are worse than none.

	// the container healthcheck probes this
	mux.Handle("GET /health", middleware.NewCORSMiddleware(healthAPI.NewHealthHandler(checkHealthUseCase)))

	// everything below here belongs to a runner rather than to the ingress
	mux.Handle("/runners/{id}/{path...}", ingressAPI.NewProxyHandler(getRunnerUseCase, transport, logger))

	var tracedProfiler *profiler.TracedProfiler
	if err := iocContainer.Resolve(&tracedProfiler); err != nil {
		return nil, err
	}

	// no rate limit: what comes through here is a container's own traffic —
	// an attached terminal, a log being followed — which a cap per minute
	// would cut off rather than pace.
	handler := middleware.NewRecoveryMiddleware(
		middleware.NewRequestIDMiddleware(
			middleware.NewTelemetryMiddleware(
				"/runner/ingress",
				// inside Telemetry so profile samples link to the request span
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
