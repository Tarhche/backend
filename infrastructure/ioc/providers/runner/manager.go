package runner

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/danceable/container/bind"
	"github.com/danceable/container/resolve"
	"github.com/danceable/provider"
	"go.mongodb.org/mongo-driver/v2/mongo"

	managerGetNode "github.com/khanzadimahdi/testproject/application/runner/manager/node/getNode"
	managerGetNodes "github.com/khanzadimahdi/testproject/application/runner/manager/node/getNodes"
	managerHeartbeatNode "github.com/khanzadimahdi/testproject/application/runner/manager/node/heartbeatNode"
	managerDeleteTask "github.com/khanzadimahdi/testproject/application/runner/manager/task/deleteTask"
	managerGetTask "github.com/khanzadimahdi/testproject/application/runner/manager/task/getTask"
	managerGetTasks "github.com/khanzadimahdi/testproject/application/runner/manager/task/getTasks"
	managerHeartbeatTask "github.com/khanzadimahdi/testproject/application/runner/manager/task/heartbeatTask"
	managerRunTask "github.com/khanzadimahdi/testproject/application/runner/manager/task/runTask"
	managerStopTask "github.com/khanzadimahdi/testproject/application/runner/manager/task/stopTask"
	"github.com/khanzadimahdi/testproject/domain"
	nodeEvents "github.com/khanzadimahdi/testproject/domain/runner/node/events"
	taskEvents "github.com/khanzadimahdi/testproject/domain/runner/task/events"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/jetstream/produceConsumer"
	noderepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/runner/nodes"
	taskrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/runner/tasks"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/scheduler/roundrobin"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/profiler"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	managerNodeAPI "github.com/khanzadimahdi/testproject/presentation/http/runner/manager/api/node"
	managerTaskAPI "github.com/khanzadimahdi/testproject/presentation/http/runner/manager/api/task"
	"github.com/nats-io/nats.go"
)

const (
	ManagerSubscribers = "runner:manager:subscribers"
)

// ManagerProviders returns the full, ordered set of service providers required
// by the runner manager service.
func ManagerProviders() []provider.Provider {
	return []provider.Provider{
		providers.NewOpenTelemetryProvider("runner-manager", "runner-manager"),
		providers.NewProfilerProvider("runner-manager"),
		providers.NewMongodbProvider(),
		providers.NewNatsProvider(),
		providers.NewTranslationProvider(),
		providers.NewValidationProvider(),
		providers.NewContainerProvider(),
		NewManagerProvider(),
	}
}

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
	if err := c.Resolve(&logger, resolve.WithParams("runner-manager")); err != nil {
		return err
	}

	pc, err := produceConsumer.NewProduceConsumer(natsConnection, "runner-manager", logger)
	if err != nil {
		return err
	}

	c.Bind(func() domain.Producer { return pc }, bind.Singleton())
	c.Bind(func() domain.Consumer { return pc }, bind.Singleton())
	c.Bind(func() domain.ProduceConsumer { return pc }, bind.Singleton())

	p.terminate = func() {
		defer pc.Wait()
	}

	return c.Bind(managerConsoleCommand, bind.Singleton())
}

func (p *managerProvider) Terminate(ctx context.Context) error {
	if p.terminate != nil {
		p.terminate()
	}

	return nil
}

func managerConsoleCommand(
	database *mongo.Database,
	jetStreamProduceConsumer domain.ProduceConsumer,
	validator domain.Validator,
	translator translatorContract.Translator,
	iocContainer provider.Container,
) (http.Handler, error) {
	var logger *slog.Logger
	if err := iocContainer.Resolve(&logger, resolve.WithParams("runner-manager")); err != nil {
		return nil, err
	}

	taskScheduler := roundrobin.New()

	taskRepository := taskrepository.NewRepository(database)
	nodeRepository := noderepository.NewRepository(database)

	managerRunTaskUseCase := managerRunTask.NewUseCase(taskRepository, jetStreamProduceConsumer, validator)
	managerDeleteTaskUseCase := managerDeleteTask.NewUseCase(taskRepository, jetStreamProduceConsumer, translator)
	managerStopTaskUseCase := managerStopTask.NewUseCase(taskRepository, jetStreamProduceConsumer, translator)
	managerGetTaskUseCase := managerGetTask.NewUseCase(taskRepository)
	managerGetTasksUseCase := managerGetTasks.NewUseCase(taskRepository)

	managerGetNodeUseCase := managerGetNode.NewUseCase(nodeRepository)
	managerGetNodesUseCase := managerGetNodes.NewUseCase(nodeRepository)

	mux := http.NewServeMux()

	mux.Handle("GET /api/tasks", managerTaskAPI.NewIndexHandler(managerGetTasksUseCase))
	mux.Handle("GET /api/tasks/{uuid}", managerTaskAPI.NewShowHandler(managerGetTaskUseCase))
	mux.Handle("DELETE /api/tasks/{uuid}", managerTaskAPI.NewDeleteHandler(managerDeleteTaskUseCase))
	mux.Handle("POST /api/tasks/run", managerTaskAPI.NewRunHandler(managerRunTaskUseCase))
	mux.Handle("POST /api/tasks/{uuid}/stop", managerTaskAPI.NewStopHandler(managerStopTaskUseCase))

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
		taskEvents.HeartbeatName:        managerHeartbeatTask.NewHeartbeatHandler(taskRepository, jetStreamProduceConsumer),
		taskEvents.TaskRunRequestedName: managerRunTask.NewTaskRunRequested(managerRunTaskUseCase, logger),
		taskEvents.TaskCreatedName:      managerRunTask.NewTaskCreated(taskRepository, nodeRepository, taskScheduler, jetStreamProduceConsumer),
		taskEvents.TaskRanName:          managerRunTask.NewTaskRan(taskRepository),
		taskEvents.TaskCompletedName:    managerRunTask.NewTaskCompleted(taskRepository),
		taskEvents.TaskFailedName:       managerRunTask.NewTaskFailed(taskRepository),
		taskEvents.TaskStoppedName:      managerStopTask.NewTaskStopped(taskRepository),
	}

	// manager subscribers
	if err := iocContainer.Bind(func() map[string]domain.MessageHandler {
		return subscribers
	}, bind.Singleton(), bind.WithName(ManagerSubscribers)); err != nil {
		return nil, err
	}

	return handler, nil
}
