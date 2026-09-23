package runner

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
	orchestratorHeartbeat "github.com/khanzadimahdi/testproject/application/runner/orchestrator/beatHeart"
	orchestratorDeleteStack "github.com/khanzadimahdi/testproject/application/runner/orchestrator/stack/deleteStack"
	orchestratorAttachTask "github.com/khanzadimahdi/testproject/application/runner/orchestrator/task/attachTask"
	orchestratorTaskHeartbeat "github.com/khanzadimahdi/testproject/application/runner/orchestrator/task/beatHeart"
	orchestratorDeleteTask "github.com/khanzadimahdi/testproject/application/runner/orchestrator/task/deleteTask"
	orchestratorGetEndpoint "github.com/khanzadimahdi/testproject/application/runner/orchestrator/task/getEndpoint"
	orchestratorgettasks "github.com/khanzadimahdi/testproject/application/runner/orchestrator/task/getTasks"
	orchestratorkilltask "github.com/khanzadimahdi/testproject/application/runner/orchestrator/task/killTask"
	orchestratorrestarttask "github.com/khanzadimahdi/testproject/application/runner/orchestrator/task/restartTask"
	orchestratorruntask "github.com/khanzadimahdi/testproject/application/runner/orchestrator/task/runTask"
	orchestratorShipLogs "github.com/khanzadimahdi/testproject/application/runner/orchestrator/task/shipLogs"
	orchestratorstoptask "github.com/khanzadimahdi/testproject/application/runner/orchestrator/task/stopTask"
	"github.com/khanzadimahdi/testproject/domain"
	networkContract "github.com/khanzadimahdi/testproject/domain/runner/network"
	nodeContract "github.com/khanzadimahdi/testproject/domain/runner/node"
	stackEvents "github.com/khanzadimahdi/testproject/domain/runner/stack/events"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	taskEvents "github.com/khanzadimahdi/testproject/domain/runner/task/events"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	infraHealth "github.com/khanzadimahdi/testproject/infrastructure/health"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/jetstream/produceConsumer"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/profiler"
	"github.com/khanzadimahdi/testproject/infrastructure/tunnel"
	healthAPI "github.com/khanzadimahdi/testproject/presentation/http/health"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	orchestratorTaskAPI "github.com/khanzadimahdi/testproject/presentation/http/runner/orchestrator/api/task"
)

const (
	OrchestratorSubscribers = "runner:orchestrator:subscribers"

	consumerNamePrefix string = "runner-orchestrator-%s"
)

// orchestratorProvider builds the runner orchestrator's messaging singleton, HTTP handler,
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
	if err := c.Resolve(&logger, provider.WithParams("runner-orchestrator-"+nodeName)); err != nil {
		return err
	}

	consumerName := fmt.Sprintf(consumerNamePrefix, nodeName)

	pc, err := produceConsumer.NewProduceConsumer(natsConnection, consumerName, logger)
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

	return c.Bind(orchestratorConsoleCommand, provider.Singleton())
}

// bindTunnel builds this orchestrator's connections to the ingresses.
//
// Nothing dials an orchestrator, so these are the only way a request reaches one. What
// arrives on them is carried to whichever of the orchestrator's own services was
// asked for — its http api today, a task's port once there is one — so the
// ingress never learns where any of them are.
func (p *orchestratorProvider) bindTunnel(c provider.Container, nodeName string, logger *slog.Logger) error {
	var orchestratorConfigs *configs.RunnerOrchestrator
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
		runnerAPIService: net.JoinHostPort("127.0.0.1", strconv.Itoa(orchestratorConfigs.Port)),
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
	taskManager task.Runtime,
	networkManager networkContract.Manager,
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
	if err := iocContainer.Resolve(&logger, provider.WithParams("runner-orchestrator-"+nodeName)); err != nil {
		return nil, err
	}

	var orchestratorConfigs *configs.RunnerOrchestrator
	if err := iocContainer.Resolve(&orchestratorConfigs); err != nil {
		return nil, err
	}

	// the network standalone isolated tasks share is made when the first
	// task joins it rather than here. A node whose docker daemon is away
	// for a moment — it restarts, or it comes up after the node does — would
	// otherwise fail to start at all, and stay down until somebody noticed.

	// tasks
	getTasksUseCase := orchestratorgettasks.NewUseCase(taskManager, nodeName)
	runTaskUseCase := orchestratorruntask.NewUseCase(taskManager, networkManager, validator, nodeName)
	stopTaskUseCase := orchestratorstoptask.NewUseCase(taskManager, validator)
	killTaskUseCase := orchestratorkilltask.NewUseCase(taskManager, validator)
	restartTaskUseCase := orchestratorrestarttask.NewUseCase(taskManager, validator)
	deleteTaskUseCase := orchestratorDeleteTask.NewUseCase(taskManager, validator, logger)
	attachTaskUseCase := orchestratorAttachTask.NewUseCase(taskManager, validator)

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

	// everything under /api is reachable from outside, through the ingress, so
	// everything under /api says who it is -- everything, that is, but the
	// terminal, which is opened on snippets that belong to nobody as readily as
	// on tasks that belong to somebody. Whose a task is decides that,
	// and only the node holding it knows.
	tasks := http.NewServeMux()

	tasks.Handle("GET /api/tasks", orchestratorTaskAPI.NewIndexHandler(getTasksUseCase))
	tasks.Handle("POST /api/tasks/run", orchestratorTaskAPI.NewRunHandler(runTaskUseCase))
	tasks.Handle("POST /api/tasks/{uuid}/stop", orchestratorTaskAPI.NewStopHandler(stopTaskUseCase))
	tasks.Handle("POST /api/tasks/{uuid}/kill", orchestratorTaskAPI.NewKillHandler(killTaskUseCase))
	tasks.Handle("POST /api/tasks/{uuid}/restart", orchestratorTaskAPI.NewRestartHandler(restartTaskUseCase))

	// a terminal inside a task. Only the control plane reaches this, and it is
	// what decides who may open one.

	api := http.NewServeMux()

	// the task healthcheck probes this, from inside the task, and it
	// says nothing a caller could not find out by the service being up
	api.Handle("GET /health", healthAPI.NewHealthHandler(checkHealthUseCase))

	api.Handle("/api/", middleware.NewTokenMiddleware(tasks, verifier))

	// a terminal, which asks for a token and does not insist on one: a snippet
	// has no owner, so there is nobody it could be checked against.
	api.Handle("GET /api/tasks/{uuid}/attach", middleware.NewOptionalTokenMiddleware(
		orchestratorTaskAPI.NewAttachHandler(attachTaskUseCase, logger),
		verifier,
	))

	rateLimited, err := middleware.NewRateLimitMiddleware(api, 600, 1*time.Minute)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()

	// the node's own answers, which are capped and carry its own headers
	mux.Handle("/", middleware.NewCORSMiddleware(rateLimited))

	// a task this node is holding, which only this node can reach: the
	// ingress works out whose it is and sends the request here. Neither the cap
	// nor the headers belong on it — what comes through is the task's own
	// traffic, and answering for it is the task's, a preflight included.
	getEndpointUseCase := orchestratorGetEndpoint.NewUseCase(taskManager)
	mux.Handle("/tasks/{slug}/{port}/{path...}", orchestratorTaskAPI.NewProxyHandler(getEndpointUseCase, taskManager, logger))

	var tracedProfiler *profiler.TracedProfiler
	if err := iocContainer.Resolve(&tracedProfiler); err != nil {
		return nil, err
	}

	handler := middleware.NewRecoveryMiddleware(
		middleware.NewRequestIDMiddleware(
			middleware.NewTelemetryMiddleware(
				"/runner/orchestrator/"+nodeName,
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
		stackEvents.StackDeletedName:         orchestratorDeleteStack.NewStackDeletedHandler(networkManager, nodeName, logger),
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
