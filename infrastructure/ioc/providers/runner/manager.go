package runner

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/danceable/provider"
	"go.mongodb.org/mongo-driver/v2/mongo"

	checkhealth "github.com/khanzadimahdi/testproject/application/app/checkHealth"
	managerGetNode "github.com/khanzadimahdi/testproject/application/runner/manager/node/getNode"
	managerGetNodes "github.com/khanzadimahdi/testproject/application/runner/manager/node/getNodes"
	managerHeartbeatNode "github.com/khanzadimahdi/testproject/application/runner/manager/node/heartbeatNode"
	managerDeleteStack "github.com/khanzadimahdi/testproject/application/runner/manager/stack/deleteStack"
	managerGetStack "github.com/khanzadimahdi/testproject/application/runner/manager/stack/getStack"
	managerGetStacks "github.com/khanzadimahdi/testproject/application/runner/manager/stack/getStacks"
	managerKillStack "github.com/khanzadimahdi/testproject/application/runner/manager/stack/killStack"
	managerRestartStack "github.com/khanzadimahdi/testproject/application/runner/manager/stack/restartStack"
	managerRunStack "github.com/khanzadimahdi/testproject/application/runner/manager/stack/runStack"
	managerStopStack "github.com/khanzadimahdi/testproject/application/runner/manager/stack/stopStack"
	managerWatchStacks "github.com/khanzadimahdi/testproject/application/runner/manager/stack/watchStacks"
	managerDeleteTask "github.com/khanzadimahdi/testproject/application/runner/manager/task/deleteTask"
	managerGetTask "github.com/khanzadimahdi/testproject/application/runner/manager/task/getTask"
	managerGetTaskLogs "github.com/khanzadimahdi/testproject/application/runner/manager/task/getTaskLogs"
	managerGetTasks "github.com/khanzadimahdi/testproject/application/runner/manager/task/getTasks"
	managerHeartbeatTask "github.com/khanzadimahdi/testproject/application/runner/manager/task/heartbeatTask"
	managerKillTask "github.com/khanzadimahdi/testproject/application/runner/manager/task/killTask"
	managerLogTask "github.com/khanzadimahdi/testproject/application/runner/manager/task/logTask"
	managerReconcile "github.com/khanzadimahdi/testproject/application/runner/manager/task/reconcile"
	managerRestartTask "github.com/khanzadimahdi/testproject/application/runner/manager/task/restartTask"
	managerRunTask "github.com/khanzadimahdi/testproject/application/runner/manager/task/runTask"
	"github.com/khanzadimahdi/testproject/application/runner/manager/task/schedule"
	managerStopTask "github.com/khanzadimahdi/testproject/application/runner/manager/task/stopTask"
	managerWatchTasks "github.com/khanzadimahdi/testproject/application/runner/manager/task/watchTasks"
	"github.com/khanzadimahdi/testproject/domain"
	nodeEvents "github.com/khanzadimahdi/testproject/domain/runner/node/events"
	stackEvents "github.com/khanzadimahdi/testproject/domain/runner/stack/events"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	taskEvents "github.com/khanzadimahdi/testproject/domain/runner/task/events"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	infraHealth "github.com/khanzadimahdi/testproject/infrastructure/health"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/jetstream/produceConsumer"
	logrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/runner/logs"
	noderepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/runner/nodes"
	stackrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/runner/stacks"
	taskrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/runner/tasks"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/ingress"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/scheduler/roundrobin"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/profiler"
	healthAPI "github.com/khanzadimahdi/testproject/presentation/http/health"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	managerNodeAPI "github.com/khanzadimahdi/testproject/presentation/http/runner/manager/api/node"
	managerStackAPI "github.com/khanzadimahdi/testproject/presentation/http/runner/manager/api/stack"
	managerTaskAPI "github.com/khanzadimahdi/testproject/presentation/http/runner/manager/api/task"
	"github.com/nats-io/nats.go"
)

const (
	ManagerSubscribers = "runner:manager:subscribers"

	// ManagerIngress is the handler that serves the containers' own exposed
	// ports, which listens on a port of its own rather than alongside the API.
	ManagerIngress = "runner:manager:ingress"
)

// managerProvider builds the runner manager's messaging singleton, HTTP handler
// and message subscribers.
type managerProvider struct {
	terminate func()
}

var _ provider.Provider = &managerProvider{}

func NewManagerProvider() *managerProvider {
	return &managerProvider{}
}

func (p *managerProvider) Register(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *managerProvider) Boot(ctx context.Context, c provider.Container) error {
	var natsConnection *nats.Conn
	if err := c.Resolve(&natsConnection); err != nil {
		return err
	}

	var logger *slog.Logger
	if err := c.Resolve(&logger, provider.WithParams("runner-manager")); err != nil {
		return err
	}

	pc, err := produceConsumer.NewProduceConsumer(natsConnection, "runner-manager", logger)
	if err != nil {
		return err
	}

	c.Bind(func() domain.Producer { return pc }, provider.Singleton())
	c.Bind(func() domain.Consumer { return pc }, provider.Singleton())
	c.Bind(func() domain.ProduceConsumer { return pc }, provider.Singleton())

	p.terminate = func() {
		defer pc.Wait()
	}

	return c.Bind(managerConsoleCommand, provider.Singleton())
}

func (p *managerProvider) Terminate(ctx context.Context) error {
	if p.terminate != nil {
		p.terminate()
	}

	return nil
}

func managerConsoleCommand(
	database *mongo.Database,
	natsConnection *nats.Conn,
	jetStreamProduceConsumer domain.ProduceConsumer,
	validator domain.Validator,
	translator translatorContract.Translator,
	iocContainer provider.Container,
) (http.Handler, error) {
	ctx := context.Background()

	var logger *slog.Logger
	if err := iocContainer.Resolve(&logger, provider.WithParams("runner-manager")); err != nil {
		return nil, err
	}

	// the stack events reach the workers, and a stack is deleted through this
	// service, so the subject has to exist before anything produces onto it.
	_ = stackEvents.StackDeletedName

	var managerConfigs *configs.RunnerManager
	if err := iocContainer.Resolve(&managerConfigs); err != nil {
		return nil, err
	}

	defaultLimits := task.ResourceLimits{
		Cpu:    managerConfigs.DefaultCpu,
		Memory: managerConfigs.DefaultMemory,
		Disk:   managerConfigs.DefaultDisk,
	}

	taskScheduler := roundrobin.New()

	taskRepository := taskrepository.NewRepository(database)
	nodeRepository := noderepository.NewRepository(database)
	stackRepository := stackrepository.NewRepository(database)
	logRepository := logrepository.NewRepository(database)

	// a container's log is only ever read one container at a time, in the
	// order it was written, so that is the index it needs.
	if err := logRepository.EnsureIndexes(ctx); err != nil {
		return nil, err
	}

	// the one place a container is handed to a node, whether it is being asked
	// for the first time, again, or after a failure.
	taskSchedule := schedule.New(stackRepository, jetStreamProduceConsumer)

	managerRunTaskUseCase := managerRunTask.NewUseCase(taskRepository, jetStreamProduceConsumer, validator)
	managerDeleteTaskUseCase := managerDeleteTask.NewUseCase(taskRepository, logRepository, jetStreamProduceConsumer, translator)
	managerStopTaskUseCase := managerStopTask.NewUseCase(taskRepository, jetStreamProduceConsumer, translator)
	managerKillTaskUseCase := managerKillTask.NewUseCase(taskRepository, jetStreamProduceConsumer, translator)
	managerRestartTaskUseCase := managerRestartTask.NewUseCase(taskRepository, taskSchedule, jetStreamProduceConsumer, translator)
	managerGetTaskUseCase := managerGetTask.NewUseCase(taskRepository)
	managerGetTasksUseCase := managerGetTasks.NewUseCase(taskRepository)
	managerWatchTasksUseCase := managerWatchTasks.NewUseCase(taskRepository)
	managerWatchStacksUseCase := managerWatchStacks.NewUseCase(stackRepository, taskRepository)

	// the manager's own heartbeat, which the serve command runs on a ticker.
	if err := iocContainer.Bind(func() *managerReconcile.UseCase {
		return managerReconcile.NewUseCase(taskRepository, taskSchedule, jetStreamProduceConsumer, logger)
	}, provider.Singleton()); err != nil {
		return nil, err
	}

	managerGetTaskLogsUseCase := managerGetTaskLogs.NewUseCase(logRepository, validator)

	managerRunStackUseCase := managerRunStack.NewUseCase(stackRepository, nodeRepository, managerRunTaskUseCase, taskScheduler, defaultLimits, validator, logger)
	managerGetStackUseCase := managerGetStack.NewUseCase(stackRepository, taskRepository)
	managerGetStacksUseCase := managerGetStacks.NewUseCase(stackRepository, taskRepository)
	managerStopStackUseCase := managerStopStack.NewUseCase(stackRepository, taskRepository, managerStopTaskUseCase, logger)
	managerKillStackUseCase := managerKillStack.NewUseCase(stackRepository, taskRepository, managerKillTaskUseCase, logger)
	managerRestartStackUseCase := managerRestartStack.NewUseCase(stackRepository, taskRepository, managerRestartTaskUseCase, logger)
	managerDeleteStackUseCase := managerDeleteStack.NewUseCase(stackRepository, taskRepository, managerDeleteTaskUseCase, jetStreamProduceConsumer, logger)

	managerGetNodeUseCase := managerGetNode.NewUseCase(nodeRepository)
	managerGetNodesUseCase := managerGetNodes.NewUseCase(nodeRepository)

	checkHealthUseCase := checkhealth.NewUseCase(
		checkhealth.Dependency{Name: "database", Pinger: infraHealth.NewMongodbPinger(database)},
		checkhealth.Dependency{Name: "messaging", Pinger: infraHealth.NewNatsPinger(natsConnection)},
	)

	mux := http.NewServeMux()

	// the container healthcheck probes this
	mux.Handle("GET /health", healthAPI.NewHealthHandler(checkHealthUseCase))

	mux.Handle("GET /api/tasks", managerTaskAPI.NewIndexHandler(managerGetTasksUseCase))
	mux.Handle("GET /api/tasks/watch", managerTaskAPI.NewWatchHandler(managerWatchTasksUseCase, logger))
	mux.Handle("GET /api/tasks/{uuid}", managerTaskAPI.NewShowHandler(managerGetTaskUseCase))
	mux.Handle("DELETE /api/tasks/{uuid}", managerTaskAPI.NewDeleteHandler(managerDeleteTaskUseCase))
	mux.Handle("POST /api/tasks/run", managerTaskAPI.NewRunHandler(managerRunTaskUseCase))
	mux.Handle("POST /api/tasks/{uuid}/stop", managerTaskAPI.NewStopHandler(managerStopTaskUseCase))
	mux.Handle("POST /api/tasks/{uuid}/kill", managerTaskAPI.NewKillHandler(managerKillTaskUseCase))
	mux.Handle("POST /api/tasks/{uuid}/restart", managerTaskAPI.NewRestartHandler(managerRestartTaskUseCase))
	mux.Handle("GET /api/tasks/{uuid}/logs", managerTaskAPI.NewLogsHandler(managerGetTaskLogsUseCase))
	mux.Handle("GET /api/tasks/{uuid}/logs/stream", managerTaskAPI.NewLogsStreamHandler(managerGetTaskLogsUseCase, logger))
	mux.Handle("GET /api/tasks/{uuid}/attach", managerTaskAPI.NewAttachHandler(managerGetTaskUseCase, managerGetNodeUseCase, logger))

	// a long-running container is run from a compose service rather than from
	// the flat shape a one-shot task takes.
	mux.Handle("POST /api/containers/run", managerTaskAPI.NewRunContainerHandler(managerRunTaskUseCase, managerGetTaskUseCase, defaultLimits))

	mux.Handle("GET /api/stacks", managerStackAPI.NewIndexHandler(managerGetStacksUseCase))
	mux.Handle("GET /api/stacks/watch", managerStackAPI.NewWatchHandler(managerWatchStacksUseCase, logger))
	mux.Handle("GET /api/stacks/{uuid}", managerStackAPI.NewShowHandler(managerGetStackUseCase))
	mux.Handle("POST /api/stacks/run", managerStackAPI.NewRunHandler(managerRunStackUseCase, managerGetStackUseCase))
	mux.Handle("POST /api/stacks/{uuid}/stop", managerStackAPI.NewStopHandler(managerStopStackUseCase))
	mux.Handle("POST /api/stacks/{uuid}/kill", managerStackAPI.NewKillHandler(managerKillStackUseCase))
	mux.Handle("POST /api/stacks/{uuid}/restart", managerStackAPI.NewRestartHandler(managerRestartStackUseCase))
	mux.Handle("DELETE /api/stacks/{uuid}", managerStackAPI.NewDeleteHandler(managerDeleteStackUseCase))

	mux.Handle("GET /api/nodes", managerNodeAPI.NewIndexHandler(managerGetNodesUseCase))
	mux.Handle("GET /api/nodes/{name}", managerNodeAPI.NewShowHandler(managerGetNodeUseCase))

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
				"/runner/manager",
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
		nodeEvents.HeartbeatName:        managerHeartbeatNode.NewHeartbeatHandler(nodeRepository),
		taskEvents.HeartbeatName:        managerHeartbeatTask.NewHeartbeatHandler(taskRepository, jetStreamProduceConsumer, managerDeleteTaskUseCase, managerKillTaskUseCase),
		taskEvents.TaskRunRequestedName: managerRunTask.NewTaskRunRequested(managerRunTaskUseCase, logger),
		taskEvents.TaskCreatedName:      managerRunTask.NewTaskCreated(taskRepository, nodeRepository, stackRepository, taskScheduler, taskSchedule, logger),
		taskEvents.TaskRanName:          managerRunTask.NewTaskRan(taskRepository),
		taskEvents.TaskRestartedName:    managerRunTask.NewTaskRestarted(taskRepository),
		taskEvents.TaskCompletedName:    managerRunTask.NewTaskCompleted(taskRepository),
		taskEvents.TaskFailedName:       managerRunTask.NewTaskFailed(taskRepository, logRepository, taskSchedule, managerDeleteTaskUseCase, logger),
		taskEvents.TaskStoppedName:      managerStopTask.NewTaskStopped(taskRepository),
		taskEvents.TaskLoggedName:       managerLogTask.NewTaskLogged(taskRepository, logRepository, managerConfigs.MaxLogBytes, logger),
	}

	// the ingress serves the containers' own ports, on a port of its own: a
	// request there is routed to a container by the hostname it was made to.
	if err := iocContainer.Bind(func() http.Handler {
		return ingress.NewHandler(taskRepository, managerConfigs.IngressDomain)
	}, provider.Singleton(), provider.WithName(ManagerIngress)); err != nil {
		return nil, err
	}

	// manager subscribers
	if err := iocContainer.Bind(func() map[string]domain.MessageHandler {
		return subscribers
	}, provider.Singleton(), provider.WithName(ManagerSubscribers)); err != nil {
		return nil, err
	}

	return handler, nil
}
