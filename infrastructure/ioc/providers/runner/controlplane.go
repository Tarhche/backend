package runner

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/danceable/provider"
	"go.mongodb.org/mongo-driver/v2/mongo"

	checkhealth "github.com/khanzadimahdi/testproject/application/app/checkHealth"
	controlPlaneGetNode "github.com/khanzadimahdi/testproject/application/runner/controlplane/node/getNode"
	controlPlaneGetNodes "github.com/khanzadimahdi/testproject/application/runner/controlplane/node/getNodes"
	controlPlaneHeartbeatNode "github.com/khanzadimahdi/testproject/application/runner/controlplane/node/heartbeatNode"
	controlPlaneDeleteStack "github.com/khanzadimahdi/testproject/application/runner/controlplane/stack/deleteStack"
	controlPlaneGetStack "github.com/khanzadimahdi/testproject/application/runner/controlplane/stack/getStack"
	controlPlaneGetStacks "github.com/khanzadimahdi/testproject/application/runner/controlplane/stack/getStacks"
	controlPlaneKillStack "github.com/khanzadimahdi/testproject/application/runner/controlplane/stack/killStack"
	controlPlaneRestartStack "github.com/khanzadimahdi/testproject/application/runner/controlplane/stack/restartStack"
	controlPlaneRunStack "github.com/khanzadimahdi/testproject/application/runner/controlplane/stack/runStack"
	controlPlaneStopStack "github.com/khanzadimahdi/testproject/application/runner/controlplane/stack/stopStack"
	controlPlaneDeleteTask "github.com/khanzadimahdi/testproject/application/runner/controlplane/task/deleteTask"
	controlPlaneGetTask "github.com/khanzadimahdi/testproject/application/runner/controlplane/task/getTask"
	controlPlaneGetTaskLogs "github.com/khanzadimahdi/testproject/application/runner/controlplane/task/getTaskLogs"
	controlPlaneGetTasks "github.com/khanzadimahdi/testproject/application/runner/controlplane/task/getTasks"
	controlPlaneHeartbeatTask "github.com/khanzadimahdi/testproject/application/runner/controlplane/task/heartbeatTask"
	controlPlaneKillTask "github.com/khanzadimahdi/testproject/application/runner/controlplane/task/killTask"
	controlPlaneLogTask "github.com/khanzadimahdi/testproject/application/runner/controlplane/task/logTask"
	controlPlaneReconcile "github.com/khanzadimahdi/testproject/application/runner/controlplane/task/reconcile"
	controlPlaneRestartTask "github.com/khanzadimahdi/testproject/application/runner/controlplane/task/restartTask"
	controlPlaneRunTask "github.com/khanzadimahdi/testproject/application/runner/controlplane/task/runTask"
	"github.com/khanzadimahdi/testproject/application/runner/controlplane/task/schedule"
	controlPlaneStopTask "github.com/khanzadimahdi/testproject/application/runner/controlplane/task/stopTask"
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
	"github.com/khanzadimahdi/testproject/infrastructure/runner/scheduler/roundrobin"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/profiler"
	healthAPI "github.com/khanzadimahdi/testproject/presentation/http/health"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	controlPlaneNodeAPI "github.com/khanzadimahdi/testproject/presentation/http/runner/controlplane/api/node"
	controlPlaneStackAPI "github.com/khanzadimahdi/testproject/presentation/http/runner/controlplane/api/stack"
	controlPlaneTaskAPI "github.com/khanzadimahdi/testproject/presentation/http/runner/controlplane/api/task"
	"github.com/nats-io/nats.go"
)

const (
	ControlPlaneSubscribers = "runner:controlplane:subscribers"
)

// controlPlaneProvider builds the runner control plane's messaging singleton, HTTP handler
// and message subscribers.
type controlPlaneProvider struct {
	terminate func()
}

var _ provider.Provider = &controlPlaneProvider{}

func NewControlPlaneProvider() *controlPlaneProvider {
	return &controlPlaneProvider{}
}

func (p *controlPlaneProvider) Register(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *controlPlaneProvider) Boot(ctx context.Context, c provider.Container) error {
	var natsConnection *nats.Conn
	if err := c.Resolve(&natsConnection); err != nil {
		return err
	}

	var logger *slog.Logger
	if err := c.Resolve(&logger, provider.WithParams("runner-controlplane")); err != nil {
		return err
	}

	pc, err := produceConsumer.NewProduceConsumer(natsConnection, "runner-controlplane", logger)
	if err != nil {
		return err
	}

	c.Bind(func() domain.Producer { return pc }, provider.Singleton())
	c.Bind(func() domain.Consumer { return pc }, provider.Singleton())
	c.Bind(func() domain.ProduceConsumer { return pc }, provider.Singleton())

	p.terminate = func() {
		defer pc.Wait()
	}

	return c.Bind(controlPlaneConsoleCommand, provider.Singleton())
}

func (p *controlPlaneProvider) Terminate(ctx context.Context) error {
	if p.terminate != nil {
		p.terminate()
	}

	return nil
}

func controlPlaneConsoleCommand(
	database *mongo.Database,
	natsConnection *nats.Conn,
	jetStreamProduceConsumer domain.ProduceConsumer,
	validator domain.Validator,
	translator translatorContract.Translator,
	iocContainer provider.Container,
) (http.Handler, error) {
	ctx := context.Background()

	var logger *slog.Logger
	if err := iocContainer.Resolve(&logger, provider.WithParams("runner-controlplane")); err != nil {
		return nil, err
	}

	// the stack events reach the orchestrators, and a stack is deleted through this
	// service, so the subject has to exist before anything produces onto it.
	_ = stackEvents.StackDeletedName

	var controlPlaneConfigs *configs.RunnerControlPlane
	if err := iocContainer.Resolve(&controlPlaneConfigs); err != nil {
		return nil, err
	}

	defaultLimits := task.ResourceLimits{
		Cpu:    controlPlaneConfigs.DefaultCpu,
		Memory: controlPlaneConfigs.DefaultMemory,
		Disk:   controlPlaneConfigs.DefaultDisk,
	}

	taskScheduler := roundrobin.New()

	taskRepository := taskrepository.NewRepository(database)
	nodeRepository := noderepository.NewRepository(database)
	stackRepository := stackrepository.NewRepository(database)
	logRepository := logrepository.NewRepository(database)

	// a task's log is only ever read one task at a time, in the
	// order it was written, so that is the index it needs.
	if err := logRepository.EnsureIndexes(ctx); err != nil {
		return nil, err
	}

	// the one place a task is handed to a node, whether it is being asked
	// for the first time, again, or after a failure.
	taskSchedule := schedule.New(stackRepository, jetStreamProduceConsumer)

	controlPlaneRunTaskUseCase := controlPlaneRunTask.NewUseCase(taskRepository, jetStreamProduceConsumer, validator)
	controlPlaneDeleteTaskUseCase := controlPlaneDeleteTask.NewUseCase(taskRepository, logRepository, jetStreamProduceConsumer, translator)
	controlPlaneStopTaskUseCase := controlPlaneStopTask.NewUseCase(taskRepository, jetStreamProduceConsumer, translator)
	controlPlaneKillTaskUseCase := controlPlaneKillTask.NewUseCase(taskRepository, jetStreamProduceConsumer, translator)
	controlPlaneRestartTaskUseCase := controlPlaneRestartTask.NewUseCase(taskRepository, taskSchedule, jetStreamProduceConsumer, translator)
	controlPlaneGetTaskUseCase := controlPlaneGetTask.NewUseCase(taskRepository)
	controlPlaneGetTasksUseCase := controlPlaneGetTasks.NewUseCase(taskRepository)

	// the control plane's own heartbeat, which the serve command runs on a ticker.
	if err := iocContainer.Bind(func() *controlPlaneReconcile.UseCase {
		return controlPlaneReconcile.NewUseCase(taskRepository, taskSchedule, jetStreamProduceConsumer, logger)
	}, provider.Singleton()); err != nil {
		return nil, err
	}

	controlPlaneGetTaskLogsUseCase := controlPlaneGetTaskLogs.NewUseCase(logRepository, validator)

	controlPlaneRunStackUseCase := controlPlaneRunStack.NewUseCase(stackRepository, nodeRepository, controlPlaneRunTaskUseCase, taskScheduler, defaultLimits, validator, logger)
	controlPlaneGetStackUseCase := controlPlaneGetStack.NewUseCase(stackRepository, taskRepository)
	controlPlaneGetStacksUseCase := controlPlaneGetStacks.NewUseCase(stackRepository, taskRepository)
	controlPlaneStopStackUseCase := controlPlaneStopStack.NewUseCase(stackRepository, taskRepository, controlPlaneStopTaskUseCase, logger)
	controlPlaneKillStackUseCase := controlPlaneKillStack.NewUseCase(stackRepository, taskRepository, controlPlaneKillTaskUseCase, logger)
	controlPlaneRestartStackUseCase := controlPlaneRestartStack.NewUseCase(stackRepository, taskRepository, controlPlaneRestartTaskUseCase, logger)
	controlPlaneDeleteStackUseCase := controlPlaneDeleteStack.NewUseCase(stackRepository, taskRepository, controlPlaneDeleteTaskUseCase, jetStreamProduceConsumer, logger)

	controlPlaneGetNodeUseCase := controlPlaneGetNode.NewUseCase(nodeRepository)
	controlPlaneGetNodesUseCase := controlPlaneGetNodes.NewUseCase(nodeRepository)

	checkHealthUseCase := checkhealth.NewUseCase(
		checkhealth.Dependency{Name: "database", Pinger: infraHealth.NewMongodbPinger(database)},
		checkhealth.Dependency{Name: "messaging", Pinger: infraHealth.NewNatsPinger(natsConnection)},
	)

	mux := http.NewServeMux()

	// the task healthcheck probes this
	mux.Handle("GET /health", healthAPI.NewHealthHandler(checkHealthUseCase))

	mux.Handle("GET /api/tasks", controlPlaneTaskAPI.NewIndexHandler(controlPlaneGetTasksUseCase))
	mux.Handle("GET /api/tasks/{uuid}", controlPlaneTaskAPI.NewShowHandler(controlPlaneGetTaskUseCase))
	mux.Handle("DELETE /api/tasks/{uuid}", controlPlaneTaskAPI.NewDeleteHandler(controlPlaneDeleteTaskUseCase))
	mux.Handle("POST /api/tasks/run", controlPlaneTaskAPI.NewRunTaskHandler(controlPlaneRunTaskUseCase, controlPlaneGetTaskUseCase, defaultLimits))
	mux.Handle("POST /api/tasks/{uuid}/stop", controlPlaneTaskAPI.NewStopHandler(controlPlaneStopTaskUseCase))
	mux.Handle("POST /api/tasks/{uuid}/kill", controlPlaneTaskAPI.NewKillHandler(controlPlaneKillTaskUseCase))
	mux.Handle("POST /api/tasks/{uuid}/restart", controlPlaneTaskAPI.NewRestartHandler(controlPlaneRestartTaskUseCase))
	mux.Handle("GET /api/tasks/{uuid}/logs", controlPlaneTaskAPI.NewLogsHandler(controlPlaneGetTaskLogsUseCase))

	mux.Handle("GET /api/stacks", controlPlaneStackAPI.NewIndexHandler(controlPlaneGetStacksUseCase))
	mux.Handle("GET /api/stacks/{uuid}", controlPlaneStackAPI.NewShowHandler(controlPlaneGetStackUseCase))
	mux.Handle("POST /api/stacks/run", controlPlaneStackAPI.NewRunHandler(controlPlaneRunStackUseCase, controlPlaneGetStackUseCase))
	mux.Handle("POST /api/stacks/{uuid}/stop", controlPlaneStackAPI.NewStopHandler(controlPlaneStopStackUseCase))
	mux.Handle("POST /api/stacks/{uuid}/kill", controlPlaneStackAPI.NewKillHandler(controlPlaneKillStackUseCase))
	mux.Handle("POST /api/stacks/{uuid}/restart", controlPlaneStackAPI.NewRestartHandler(controlPlaneRestartStackUseCase))
	mux.Handle("DELETE /api/stacks/{uuid}", controlPlaneStackAPI.NewDeleteHandler(controlPlaneDeleteStackUseCase))

	mux.Handle("GET /api/nodes", controlPlaneNodeAPI.NewIndexHandler(controlPlaneGetNodesUseCase))
	mux.Handle("GET /api/nodes/{name}", controlPlaneNodeAPI.NewShowHandler(controlPlaneGetNodeUseCase))

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
				"/runner/controlplane",
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
		nodeEvents.HeartbeatName:        controlPlaneHeartbeatNode.NewHeartbeatHandler(nodeRepository),
		taskEvents.HeartbeatName:        controlPlaneHeartbeatTask.NewHeartbeatHandler(taskRepository, jetStreamProduceConsumer, controlPlaneDeleteTaskUseCase, controlPlaneKillTaskUseCase),
		taskEvents.TaskRunRequestedName: controlPlaneRunTask.NewTaskRunRequested(controlPlaneRunTaskUseCase, logger),
		taskEvents.TaskCreatedName:      controlPlaneRunTask.NewTaskCreated(taskRepository, nodeRepository, stackRepository, taskScheduler, taskSchedule, logger),
		taskEvents.TaskRanName:          controlPlaneRunTask.NewTaskRan(taskRepository),
		taskEvents.TaskRestartedName:    controlPlaneRunTask.NewTaskRestarted(taskRepository),
		taskEvents.TaskCompletedName:    controlPlaneRunTask.NewTaskCompleted(taskRepository),
		taskEvents.TaskFailedName:       controlPlaneRunTask.NewTaskFailed(taskRepository, logRepository, taskSchedule, controlPlaneDeleteTaskUseCase, logger),
		taskEvents.TaskStoppedName:      controlPlaneStopTask.NewTaskStopped(taskRepository),
		taskEvents.TaskLoggedName:       controlPlaneLogTask.NewTaskLogged(taskRepository, logRepository, controlPlaneConfigs.MaxLogBytes, logger),
	}

	// control plane subscribers
	if err := iocContainer.Bind(func() map[string]domain.MessageHandler {
		return subscribers
	}, provider.Singleton(), provider.WithName(ControlPlaneSubscribers)); err != nil {
		return nil, err
	}

	return handler, nil
}
