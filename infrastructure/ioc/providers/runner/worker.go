package runner

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/danceable/provider"
	"github.com/nats-io/nats.go"

	checkhealth "github.com/khanzadimahdi/testproject/application/app/checkHealth"
	workerHeartbeat "github.com/khanzadimahdi/testproject/application/runner/worker/beatHeart"
	"github.com/khanzadimahdi/testproject/application/runner/worker/cluster"
	workerDeleteStack "github.com/khanzadimahdi/testproject/application/runner/worker/stack/deleteStack"
	workerAttachTask "github.com/khanzadimahdi/testproject/application/runner/worker/task/attachTask"
	workerTaskHeartbeat "github.com/khanzadimahdi/testproject/application/runner/worker/task/beatHeart"
	workerDeleteTask "github.com/khanzadimahdi/testproject/application/runner/worker/task/deleteTask"
	workergettasks "github.com/khanzadimahdi/testproject/application/runner/worker/task/getTasks"
	workerkilltask "github.com/khanzadimahdi/testproject/application/runner/worker/task/killTask"
	workerrestarttask "github.com/khanzadimahdi/testproject/application/runner/worker/task/restartTask"
	workerruntask "github.com/khanzadimahdi/testproject/application/runner/worker/task/runTask"
	workerShipLogs "github.com/khanzadimahdi/testproject/application/runner/worker/task/shipLogs"
	workerstoptask "github.com/khanzadimahdi/testproject/application/runner/worker/task/stopTask"
	"github.com/khanzadimahdi/testproject/domain"
	containerContract "github.com/khanzadimahdi/testproject/domain/runner/container"
	networkContract "github.com/khanzadimahdi/testproject/domain/runner/network"
	nodeContract "github.com/khanzadimahdi/testproject/domain/runner/node"
	stackEvents "github.com/khanzadimahdi/testproject/domain/runner/stack/events"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	taskEvents "github.com/khanzadimahdi/testproject/domain/runner/task/events"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	infraHealth "github.com/khanzadimahdi/testproject/infrastructure/health"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/core/pubsub"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/jetstream/produceConsumer"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/ingress"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/profiler"
	healthAPI "github.com/khanzadimahdi/testproject/presentation/http/health"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	workerTaskAPI "github.com/khanzadimahdi/testproject/presentation/http/runner/worker/api/task"
)

const (
	WorkerSubscribers = "runner:worker:subscribers"
	WorkerName        = "runner:worker:name"

	// WorkerIngress is what serves the containers themselves: a request is
	// routed to a container by the hostname it was made to, whichever node is
	// holding it.
	WorkerIngress = "runner:worker:ingress"

	consumerNamePrefix string = "runner-worker-%s"
)

// publicHost is where somebody outside reaches this node's forwarded ports. A
// node that does not say has only the address the runner reaches it at, which
// is better than nothing to report.
func publicHost(configs *configs.RunnerWorker) string {
	if configs.PublicHost != "" {
		return configs.PublicHost
	}

	return configs.AdvertiseHost
}

// publicIngressDomain is how somebody outside writes the name a container on
// this node answers to. It differs from the name the node matches only where
// something in front answers on another port.
func publicIngressDomain(configs *configs.RunnerWorker) string {
	if configs.PublicIngressDomain != "" {
		return configs.PublicIngressDomain
	}

	return configs.IngressDomain
}

// workerNameProvider binds the worker name, which the command loads from its
// --name flag or from the RUNNER_WORKER_NAME environment variable, under the
// name the worker providers resolve it by.
type workerNameProvider struct{}

var _ provider.Provider = &workerNameProvider{}

// NewWorkerNameProvider binds the worker name into the container so the worker
// providers can resolve it. It must be registered after the configs provider.
func NewWorkerNameProvider() *workerNameProvider {
	return &workerNameProvider{}
}

func (p *workerNameProvider) Register(ctx context.Context, c provider.Container) error {
	var workerConfigs *configs.RunnerWorker
	if err := c.Resolve(&workerConfigs); err != nil {
		return err
	}

	name := workerConfigs.Name

	return c.Bind(func() string { return name }, provider.Singleton(), provider.WithName(WorkerName))
}

func (p *workerNameProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *workerNameProvider) Terminate(ctx context.Context) error {
	return nil
}

// workerProvider builds the runner worker's messaging singleton, HTTP handler,
// message subscribers and heartbeat use cases.
type workerProvider struct {
	terminate func()
}

var _ provider.Provider = &workerProvider{}

func NewWorkerProvider() *workerProvider {
	return &workerProvider{}
}

func (p *workerProvider) Register(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *workerProvider) Boot(ctx context.Context, c provider.Container) error {
	var nodeName string
	if err := c.Resolve(&nodeName, provider.ResolveName(WorkerName)); err != nil {
		return err
	}

	var natsConnection *nats.Conn
	if err := c.Resolve(&natsConnection); err != nil {
		return err
	}

	var logger *slog.Logger
	if err := c.Resolve(&logger, provider.WithParams("runner-worker-"+nodeName)); err != nil {
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

	return c.Bind(workerConsoleCommand, provider.Singleton())
}

func (p *workerProvider) Terminate(ctx context.Context) error {
	if p.terminate != nil {
		p.terminate()
	}

	return nil
}

func workerConsoleCommand(
	natsConnection *nats.Conn,
	containerManager containerContract.Manager,
	networkManager networkContract.Manager,
	nodeManager nodeContract.Manager,
	asyncProduceConsumer domain.ProduceConsumer,
	validator domain.Validator,
	iocContainer provider.Container,
) (http.Handler, error) {
	var nodeName string
	if err := iocContainer.Resolve(&nodeName, provider.ResolveName(WorkerName)); err != nil {
		return nil, err
	}

	var logger *slog.Logger
	if err := iocContainer.Resolve(&logger, provider.WithParams("runner-worker-"+nodeName)); err != nil {
		return nil, err
	}

	var workerConfigs *configs.RunnerWorker
	if err := iocContainer.Resolve(&workerConfigs); err != nil {
		return nil, err
	}

	// the network standalone isolated containers share is made when the first
	// container joins it rather than here. A node whose docker daemon is away
	// for a moment — it restarts, or it comes up after the node does — would
	// otherwise fail to start at all, and stay down until somebody noticed.

	// tasks
	getTasksUseCase := workergettasks.NewUseCase(containerManager, nodeName)
	// what this node accepts whole connections on. A range it cannot read is a
	// mistake in how the node was started, and it says so rather than quietly
	// forwarding nothing.
	publicPorts, err := task.ParsePortRange(workerConfigs.PublicPortRange)
	if err != nil {
		return nil, err
	}

	runTaskUseCase := workerruntask.NewUseCase(containerManager, networkManager, validator, nodeName, publicPorts)
	stopTaskUseCase := workerstoptask.NewUseCase(containerManager, validator)
	killTaskUseCase := workerkilltask.NewUseCase(containerManager, validator)
	restartTaskUseCase := workerrestarttask.NewUseCase(containerManager, validator)
	deleteTaskUseCase := workerDeleteTask.NewUseCase(containerManager, validator, logger)
	attachTaskUseCase := workerAttachTask.NewUseCase(containerManager, validator)

	// the worker talks to no database, so messaging is its only dependency
	checkHealthUseCase := checkhealth.NewUseCase(
		checkhealth.Dependency{Name: "messaging", Pinger: infraHealth.NewNatsPinger(natsConnection)},
	)

	mux := http.NewServeMux()

	// the container healthcheck probes this
	mux.Handle("GET /health", healthAPI.NewHealthHandler(checkHealthUseCase))

	mux.Handle("GET /api/tasks", workerTaskAPI.NewIndexHandler(getTasksUseCase))
	mux.Handle("POST /api/tasks/run", workerTaskAPI.NewRunHandler(runTaskUseCase))
	mux.Handle("POST /api/tasks/{uuid}/stop", workerTaskAPI.NewStopHandler(stopTaskUseCase))
	mux.Handle("POST /api/tasks/{uuid}/kill", workerTaskAPI.NewKillHandler(killTaskUseCase))
	mux.Handle("POST /api/tasks/{uuid}/restart", workerTaskAPI.NewRestartHandler(restartTaskUseCase))

	// a terminal inside a container. Only the manager reaches this, and it is
	// what decides who may open one.
	mux.Handle("GET /api/tasks/{uuid}/attach", workerTaskAPI.NewAttachHandler(attachTaskUseCase, logger))

	rateLimited, err := middleware.NewRateLimitMiddleware(mux, 600, 1*time.Minute)
	if err != nil {
		return nil, err
	}

	var tracedProfiler *profiler.TracedProfiler
	if err := iocContainer.Resolve(&tracedProfiler); err != nil {
		return nil, err
	}

	handler := middleware.NewRecoveryMiddleware(
		middleware.NewRequestIDMiddleware(
			middleware.NewTelemetryMiddleware(
				"/runner/worker/"+nodeName,
				// inside Telemetry so profile samples link to the request span
				middleware.NewProfilingMiddleware(
					middleware.NewLogMiddleware(
						middleware.NewCORSMiddleware(
							rateLimited,
						),
						logger,
					),
					tracedProfiler,
				),
			),
		),
		logger,
	)

	subscribers := map[string]domain.MessageHandler{
		taskEvents.TaskScheduledName:         workerruntask.NewTaskScheduled(runTaskUseCase, asyncProduceConsumer, nodeName, logger),
		taskEvents.TaskStoppageRequestedName: workerstoptask.NewStoppageTaskHandler(stopTaskUseCase),
		taskEvents.TaskKillRequestedName:     workerkilltask.NewKillTaskHandler(killTaskUseCase),
		taskEvents.TaskRestartRequestedName:  workerrestarttask.NewRestartTaskHandler(restartTaskUseCase),
		taskEvents.TaskDeletedName:           workerDeleteTask.NewDeleteTaskHandler(deleteTaskUseCase),
		stackEvents.StackDeletedName:         workerDeleteStack.NewStackDeletedHandler(networkManager, nodeName, logger),
	}

	// What every node is holding, which every node hears: a beat carries a
	// container's name and where its ports came up, so this node can answer
	// for a container another node is holding by passing the request on.
	view := cluster.NewView(nodeName)

	if err := iocContainer.Bind(func() *cluster.View { return view }, provider.Singleton()); err != nil {
		return nil, err
	}

	// the node only listens to what the others say, so that is what it is
	// given: the same connection, under the half of the contract it uses.
	subscriber := pubsub.NewPublishSubscriber(natsConnection, logger)

	if err := iocContainer.Bind(func() domain.Subscriber { return subscriber }, provider.Singleton()); err != nil {
		return nil, err
	}

	// what somebody is shown while a container is still coming up is a view
	// like any other, drawn by whoever draws the rest of them.
	var renderer domain.Renderer
	if err := iocContainer.Resolve(&renderer); err != nil {
		return nil, err
	}

	if err := iocContainer.Bind(func() http.Handler {
		return ingress.NewHandler(view, workerConfigs.IngressDomain, renderer)
	}, provider.Singleton(), provider.WithName(WorkerIngress)); err != nil {
		return nil, err
	}

	// worker subscribers
	if err := iocContainer.Bind(func() map[string]domain.MessageHandler {
		return subscribers
	}, provider.Singleton(), provider.WithName(WorkerSubscribers)); err != nil {
		return nil, err
	}

	// worker heartbeat
	if err := iocContainer.Bind(func() *workerHeartbeat.UseCase {
		return workerHeartbeat.NewUseCase(asyncProduceConsumer, nodeManager, nodeName, workerConfigs.APIAddress)
	}, provider.Singleton()); err != nil {
		return nil, err
	}

	// task heartbeat
	if err := iocContainer.Bind(func() *workerTaskHeartbeat.UseCase {
		return workerTaskHeartbeat.NewUseCase(containerManager, asyncProduceConsumer, nodeName, workerConfigs.AdvertiseHost, publicHost(workerConfigs), publicIngressDomain(workerConfigs), logger)
	}, provider.Singleton()); err != nil {
		return nil, err
	}

	// log shipping, which is what makes a long-running container's output
	// outlive the container.
	if err := iocContainer.Bind(func() *workerShipLogs.UseCase {
		return workerShipLogs.NewUseCase(containerManager, asyncProduceConsumer, nodeName, logger)
	}, provider.Singleton()); err != nil {
		return nil, err
	}

	return handler, nil
}
