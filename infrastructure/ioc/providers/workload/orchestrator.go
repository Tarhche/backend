package workload

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/danceable/provider"
	"github.com/nats-io/nats.go"

	checkhealth "github.com/khanzadimahdi/testproject/application/app/checkHealth"
	orchestratorAttachResource "github.com/khanzadimahdi/testproject/application/workload/orchestrator/attachResource"
	orchestratorHeartbeat "github.com/khanzadimahdi/testproject/application/workload/orchestrator/beatHeart"
	orchestratorGetResourceEndpoint "github.com/khanzadimahdi/testproject/application/workload/orchestrator/getResourceEndpoint"
	orchestratorShipLogs "github.com/khanzadimahdi/testproject/application/workload/orchestrator/task/shipLogs"
	"github.com/khanzadimahdi/testproject/domain"
	nodeContract "github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	infraHealth "github.com/khanzadimahdi/testproject/infrastructure/health"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/jetstream/produceConsumer"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/profiler"
	"github.com/khanzadimahdi/testproject/infrastructure/tunnel"
	infraNode "github.com/khanzadimahdi/testproject/infrastructure/workload/node"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/task/vmruntime"
	healthAPI "github.com/khanzadimahdi/testproject/presentation/http/health"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	orchestratorKindsAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/orchestrator/api/kinds"
)

const (
	OrchestratorSubscribers = "workload:orchestrator:subscribers"

	consumerNamePrefix string = "workload-orchestrator-%s"

	// commandConcurrency is how many of the control plane's commands of one
	// kind an orchestrator carries out at once. Restoring a VM or pulling a
	// stack's images takes minutes, and should not hold up every other VM's
	// commands meanwhile.
	commandConcurrency = 4

	// commandProgress is how often JetStream is told that a command still
	// being carried out is, which is well within its ack wait.
	commandProgress = 10 * time.Second

	// kindStateTimeout is how long each kind this node runs is given, every
	// heartbeat, to say what it holds here. A heartbeat goes every second, and
	// the control plane schedules only on a node it heard from in the last
	// three, so a kind slow to answer is left out of a beat rather than
	// holding the beat up past that.
	kindStateTimeout = time.Second
)

// orchestratorProvider builds the workload orchestrator's messaging singleton, HTTP handler,
// message subscribers and heartbeat use cases.
type orchestratorProvider struct {
	terminate func()
}

var _ provider.Provider = &orchestratorProvider{}

func NewOrchestratorProvider() *orchestratorProvider {
	return &orchestratorProvider{}
}

func (p *orchestratorProvider) Register(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *orchestratorProvider) Boot(ctx context.Context, c provider.Container) error {
	var nodeName string
	if err := c.Resolve(&nodeName, provider.ResolveName(OrchestratorName)); err != nil {
		return err
	}

	var natsConnection *nats.Conn
	if err := c.Resolve(&natsConnection); err != nil {
		return err
	}

	var logger *slog.Logger
	if err := c.Resolve(&logger, provider.WithParams("workload-orchestrator-"+nodeName)); err != nil {
		return err
	}

	consumerName := fmt.Sprintf(consumerNamePrefix, nodeName)

	pc, err := produceConsumer.NewProduceConsumer(natsConnection, consumerName, logger,
		produceConsumer.WithConcurrency(commandConcurrency),
		produceConsumer.WithProgress(commandProgress),
	)
	if err != nil {
		return err
	}

	c.Bind(func() domain.Producer { return pc }, provider.Singleton())
	c.Bind(func() domain.Consumer { return pc }, provider.Singleton())
	c.Bind(func() domain.ProduceConsumer { return pc }, provider.Singleton())

	p.terminate = func() {
		defer pc.Wait()
	}

	if err := p.bindTunnel(c, nodeName, logger); err != nil {
		return err
	}

	if err := p.bindEngine(c, logger); err != nil {
		return err
	}

	return c.Bind(orchestratorConsoleCommand, provider.Singleton())
}

// bindEngine binds what this node runs VMs on, and what it runs the code
// runner's tasks and reports itself through, which is the same engine: a task
// is an ephemeral VM of its own.
func (p *orchestratorProvider) bindEngine(c provider.Container, logger *slog.Logger) error {
	var orchestratorConfigs *configs.WorkloadOrchestrator
	if err := c.Resolve(&orchestratorConfigs); err != nil {
		return err
	}

	engine, err := vmhostEngine(orchestratorConfigs.VMHostSocket)
	if err != nil {
		return err
	}

	runtime := vmruntime.New(engine, logger)
	nodeManager := infraNode.NewManager(engine)

	if err := c.Bind(func() vm.Engine { return engine }, provider.Singleton()); err != nil {
		return err
	}

	if err := c.Bind(func() task.Runtime { return runtime }, provider.Singleton()); err != nil {
		return err
	}

	return c.Bind(func() nodeContract.Manager { return nodeManager }, provider.Singleton())
}

// bindTunnel builds this orchestrator's connections to the ingresses.
//
// Nothing dials an orchestrator, so these are the only way a request reaches one. What
// arrives on them is carried to whichever of the orchestrator's own services was
// asked for — its http api today, a task's port once there is one — so the
// ingress never learns where any of them are.
func (p *orchestratorProvider) bindTunnel(c provider.Container, nodeName string, logger *slog.Logger) error {
	var orchestratorConfigs *configs.WorkloadOrchestrator
	if err := c.Resolve(&orchestratorConfigs); err != nil {
		return err
	}

	tlsConfig, err := tunnel.ClientTLS(certificate.Credentials{
		Authority:   orchestratorConfigs.TunnelAuthority,
		Certificate: orchestratorConfigs.TunnelCertificate,
		PrivateKey:  orchestratorConfigs.TunnelKey,
		ServerName:  orchestratorConfigs.TunnelServerName,
	})
	if err != nil {
		return err
	}

	tunnelConfig := tunnel.DefaultConfig()
	tunnelConfig.MinSessions = orchestratorConfigs.TunnelMinConnections
	tunnelConfig.MaxSessions = orchestratorConfigs.TunnelMaxConnections
	tunnelConfig.MaxStreamsPerSession = orchestratorConfigs.TunnelMaxStreamsPerSession
	tunnelConfig.IdleSessionTimeout = orchestratorConfigs.TunnelMaxIdleTime

	// what a stream may be connected to: the api this orchestrator already serves,
	// offered under a name so that the ingress asks for the service rather than
	// for a port, and whatever addresses this orchestrator was told to allow. Nothing
	// else, however the ingress asks.
	allowed, err := orchestratorConfigs.AllowedTargets()
	if err != nil {
		return err
	}

	targets := tunnel.NewServiceTargets(map[string]string{
		workloadAPIService: net.JoinHostPort("127.0.0.1", strconv.Itoa(orchestratorConfigs.Port)),
	}, allowed...)

	orchestrator, err := tunnel.NewAgent(
		nodeName,
		orchestratorConfigs.IngressAddresses(),
		tunnelConfig,
		tunnel.TLSDialer(tlsConfig, tunnelConfig.DialTimeout),
		targets,
		logger,
	)
	if err != nil {
		return err
	}

	if err := c.Bind(func() tunnel.Targets { return targets }, provider.Singleton()); err != nil {
		return err
	}

	return c.Bind(func() *tunnel.Agent { return orchestrator }, provider.Singleton())
}

func (p *orchestratorProvider) Terminate(ctx context.Context) error {
	if p.terminate != nil {
		p.terminate()
	}

	return nil
}

func orchestratorConsoleCommand(
	natsConnection *nats.Conn,
	engine vm.Engine,
	taskManager task.Runtime,
	nodeManager nodeContract.Manager,
	asyncProduceConsumer domain.ProduceConsumer,
	validator domain.Validator,
	iocContainer provider.Container,
) (http.Handler, error) {
	var nodeName string
	if err := iocContainer.Resolve(&nodeName, provider.ResolveName(OrchestratorName)); err != nil {
		return nil, err
	}

	var logger *slog.Logger
	if err := iocContainer.Resolve(&logger, provider.WithParams("workload-orchestrator-"+nodeName)); err != nil {
		return nil, err
	}

	var orchestratorConfigs *configs.WorkloadOrchestrator
	if err := iocContainer.Resolve(&orchestratorConfigs); err != nil {
		return nil, err
	}

	// the orchestrator talks to no database, so messaging is its only dependency
	checkHealthUseCase := checkhealth.NewUseCase(
		checkhealth.Dependency{Name: "messaging", Pinger: infraHealth.NewNatsPinger(natsConnection)},
	)

	subscribers := map[string]domain.MessageHandler{}

	// what is asked of every kind this node runs, VMs, their snapshots, the
	// stacks in them and the code runner's tasks, and the answers to what the
	// control plane asks and waits for.
	workload, err := bindOrchestratorWorkload(iocContainer, OrchestratorDependencies{
		NATS:     natsConnection,
		Engine:   engine,
		Tasks:    taskManager,
		Archives: snapshotArchives(orchestratorConfigs.SnapshotStorage),
		Producer: asyncProduceConsumer,
		Configs:  orchestratorConfigs,
		NodeName: nodeName,
		Logger:   logger,
	}, subscribers)
	if err != nil {
		return nil, err
	}

	// what the tokens the blog signs are verified against. An orchestrator never mints
	// one, so it holds the public half and could not sign a token if it tried.
	publicKey, err := ecdsa.ParsePublicKey([]byte(orchestratorConfigs.PublicKey))
	if err != nil {
		return nil, err
	}

	verifier := jwt.NewJWT(nil, publicKey)

	// what a task or a VM is told to do -- run, stop, restart, snapshot, be
	// deleted -- reaches this node as the control plane's messages, below, and
	// in no other way, and what the node is holding goes back the same way, in
	// its heartbeats. So there is no route for either. Everything under /api is
	// reachable from outside, through the ingress, and a token proves only that
	// the estate signed it for somebody: a route that ran or stopped something
	// would do it for anybody signed in, to anybody's, and the control plane
	// would never hear of it. A VM's terminal is the vm kind's stream, routed
	// with every kind's below.
	api := http.NewServeMux()

	// the task healthcheck probes this, from inside the task, and it
	// says nothing a caller could not find out by the service being up
	api.Handle("GET /health", healthAPI.NewHealthHandler(checkHealthUseCase))

	rateLimited, err := middleware.NewRateLimitMiddleware(api, 600, 1*time.Minute)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()

	// the node's own answers, which are capped and carry its own headers
	mux.Handle("/", middleware.NewCORSMiddleware(rateLimited))

	// what the kinds this node runs serve, each under its own plural: the
	// streams of those that serve them, among the node's own answers, a VM's
	// terminal, opened for its owner alone, and a task's, opened for anybody
	// for a snippet of the guest's; and the ports of those that serve them, a
	// VM's and a task's, which only this node can reach: the ingress works out
	// which node holds one and sends the request here. Neither the cap nor
	// the headers belong on a port: what comes through is the resource's own
	// traffic, and answering for it is its own, a preflight included. A kind
	// takes no route until it is registered.
	kindRoutes := orchestratorKindsAPI.NewRoutes(
		workload.Kinds,
		orchestratorAttachResource.NewUseCase(workload.Kinds, validator),
		orchestratorGetResourceEndpoint.NewUseCase(workload.Kinds),
		verifier,
		logger,
	)
	if err := kindRoutes.Register(api, mux); err != nil {
		return nil, err
	}

	var tracedProfiler *profiler.TracedProfiler
	if err := iocContainer.Resolve(&tracedProfiler); err != nil {
		return nil, err
	}

	handler := middleware.NewRecoveryMiddleware(
		middleware.NewRequestIDMiddleware(
			middleware.NewTelemetryMiddleware(
				"/workload/orchestrator/"+nodeName,
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

	// orchestrator subscribers
	if err := iocContainer.Bind(func() map[string]domain.MessageHandler {
		return subscribers
	}, provider.Singleton(), provider.WithName(OrchestratorSubscribers)); err != nil {
		return nil, err
	}

	// orchestrator heartbeat
	if err := iocContainer.Bind(func() *orchestratorHeartbeat.UseCase {
		return orchestratorHeartbeat.NewUseCase(asyncProduceConsumer, nodeManager, workload.Kinds, kindStateTimeout, nodeName, logger)
	}, provider.Singleton()); err != nil {
		return nil, err
	}

	// log shipping, which is what makes a long-running task's output
	// outlive the task.
	if err := iocContainer.Bind(func() *orchestratorShipLogs.UseCase {
		return orchestratorShipLogs.NewUseCase(taskManager, asyncProduceConsumer, nodeName, logger)
	}, provider.Singleton()); err != nil {
		return nil, err
	}

	return handler, nil
}
