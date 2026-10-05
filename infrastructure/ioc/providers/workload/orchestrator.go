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
	orchestratorHeartbeat "github.com/khanzadimahdi/testproject/application/workload/orchestrator/beatHeart"
	orchestratorGetEndpoint "github.com/khanzadimahdi/testproject/application/workload/orchestrator/getEndpoint"
	orchestratorAttachTask "github.com/khanzadimahdi/testproject/application/workload/orchestrator/task/attachTask"
	orchestratorTaskHeartbeat "github.com/khanzadimahdi/testproject/application/workload/orchestrator/task/beatHeart"
	orchestratorDeleteTask "github.com/khanzadimahdi/testproject/application/workload/orchestrator/task/deleteTask"
	orchestratorkilltask "github.com/khanzadimahdi/testproject/application/workload/orchestrator/task/killTask"
	orchestratorrestarttask "github.com/khanzadimahdi/testproject/application/workload/orchestrator/task/restartTask"
	orchestratorruntask "github.com/khanzadimahdi/testproject/application/workload/orchestrator/task/runTask"
	orchestratorShipLogs "github.com/khanzadimahdi/testproject/application/workload/orchestrator/task/shipLogs"
	orchestratorstoptask "github.com/khanzadimahdi/testproject/application/workload/orchestrator/task/stopTask"
	orchestratorAttachVM "github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/attachVM"
	"github.com/khanzadimahdi/testproject/domain"
	nodeContract "github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	taskEvents "github.com/khanzadimahdi/testproject/domain/workload/task/events"
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
	orchestratorPortsAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/orchestrator/api/ports"
	orchestratorTaskAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/orchestrator/api/task"
	orchestratorVMAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/orchestrator/api/vm"
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

	// tasks
	runTaskUseCase := orchestratorruntask.NewUseCase(taskManager, validator, nodeName)
	stopTaskUseCase := orchestratorstoptask.NewUseCase(taskManager, validator)
	killTaskUseCase := orchestratorkilltask.NewUseCase(taskManager, validator)
	restartTaskUseCase := orchestratorrestarttask.NewUseCase(taskManager, validator)
	deleteTaskUseCase := orchestratorDeleteTask.NewUseCase(taskManager, validator, logger)
	attachTaskUseCase := orchestratorAttachTask.NewUseCase(taskManager, validator)

	// vms
	attachVMUseCase := orchestratorAttachVM.NewUseCase(engine, validator)

	// the orchestrator talks to no database, so messaging is its only dependency
	checkHealthUseCase := checkhealth.NewUseCase(
		checkhealth.Dependency{Name: "messaging", Pinger: infraHealth.NewNatsPinger(natsConnection)},
	)

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
	// would never hear of it.
	api := http.NewServeMux()

	// the task healthcheck probes this, from inside the task, and it
	// says nothing a caller could not find out by the service being up
	api.Handle("GET /health", healthAPI.NewHealthHandler(checkHealthUseCase))

	// a terminal inside a task, which the ingress carries here. It asks for a
	// token and does not insist on one: a snippet has no owner, so there is
	// nobody it could be checked against.
	api.Handle("GET /api/tasks/{uuid}/attach", middleware.NewOptionalTokenMiddleware(
		orchestratorTaskAPI.NewAttachHandler(attachTaskUseCase, logger),
		verifier,
	))

	// a terminal in a VM, which the ingress carries here as it does a
	// task's. A VM always has an owner, so a token is insisted on, and the
	// terminal is opened for the owner alone.
	api.Handle("GET /api/vms/{uuid}/attach", middleware.NewTokenMiddleware(
		orchestratorVMAPI.NewAttachHandler(attachVMUseCase, logger),
		verifier,
	))

	rateLimited, err := middleware.NewRateLimitMiddleware(api, 600, 1*time.Minute)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()

	// the node's own answers, which are capped and carry its own headers
	mux.Handle("/", middleware.NewCORSMiddleware(rateLimited))

	// a VM or a task this node is holding, which only this node can reach:
	// the ingress works out whose it is and sends the request here. Neither
	// the cap nor the headers belong on it — what comes through is the VM's or
	// the task's own traffic, and answering for it is theirs, a preflight
	// included. Slugs are one namespace across the two, so either route
	// reaches either.
	proxy := orchestratorPortsAPI.NewProxyHandler(orchestratorGetEndpoint.NewUseCase(engine), logger)
	mux.Handle("/tasks/{slug}/{port}/{path...}", proxy)
	mux.Handle("/vms/{slug}/{port}/{path...}", proxy)

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

	subscribers := map[string]domain.MessageHandler{
		taskEvents.TaskScheduledName:         orchestratorruntask.NewTaskScheduled(runTaskUseCase, asyncProduceConsumer, nodeName, logger),
		taskEvents.TaskStoppageRequestedName: orchestratorstoptask.NewStoppageTaskHandler(stopTaskUseCase),
		taskEvents.TaskKillRequestedName:     orchestratorkilltask.NewKillTaskHandler(killTaskUseCase),
		taskEvents.TaskRestartRequestedName:  orchestratorrestarttask.NewRestartTaskHandler(restartTaskUseCase),
		taskEvents.TaskDeletedName:           orchestratorDeleteTask.NewDeleteTaskHandler(deleteTaskUseCase),
	}

	// what is asked of the VMs on this node, of their snapshots and of the
	// stacks in them, and the answers to what the control plane asks and
	// waits for.
	if err := bindOrchestratorVMs(iocContainer, OrchestratorVMDependencies{
		NATS:      natsConnection,
		Engine:    engine,
		Archives:  snapshotArchives(orchestratorConfigs.SnapshotStorage),
		Producer:  asyncProduceConsumer,
		Validator: validator,
		Configs:   orchestratorConfigs,
		NodeName:  nodeName,
		Logger:    logger,
	}, subscribers); err != nil {
		return nil, err
	}

	// orchestrator subscribers
	if err := iocContainer.Bind(func() map[string]domain.MessageHandler {
		return subscribers
	}, provider.Singleton(), provider.WithName(OrchestratorSubscribers)); err != nil {
		return nil, err
	}

	// orchestrator heartbeat
	if err := iocContainer.Bind(func() *orchestratorHeartbeat.UseCase {
		return orchestratorHeartbeat.NewUseCase(asyncProduceConsumer, nodeManager, nodeName)
	}, provider.Singleton()); err != nil {
		return nil, err
	}

	// task heartbeat
	if err := iocContainer.Bind(func() *orchestratorTaskHeartbeat.UseCase {
		return orchestratorTaskHeartbeat.NewUseCase(taskManager, asyncProduceConsumer, nodeName, logger)
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
