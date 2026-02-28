package runner

import (
	"context"
	"log"
	"net/http"
	"time"

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
	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers"
	noderepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/runner/nodes"
	taskrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/runner/tasks"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/scheduler/roundrobin"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	managerNodeAPI "github.com/khanzadimahdi/testproject/presentation/http/runner/manager/api/node"
	managerTaskAPI "github.com/khanzadimahdi/testproject/presentation/http/runner/manager/api/task"
)

const (
	ManagerSubscribers = "runner:manager:subscribers"
	ManagerHandler     = "runner:manager:handler"
)

var managerDependencies = []ioc.ServiceProvider{
	providers.NewMongodbProvider(),
	providers.NewNatsProvider(),
	providers.NewTranslationProvider(),
	providers.NewValidationProvider(),
	providers.NewContainerProvider(),
}

type managerProvider struct {
	dependencies []ioc.ServiceProvider
}

var _ ioc.ServiceProvider = &managerProvider{}

func NewManagerProvider() *managerProvider {
	return &managerProvider{
		dependencies: managerDependencies,
	}
}

func (p *managerProvider) Register(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	log.SetFlags(log.LstdFlags | log.Llongfile)

	for _, dependency := range p.dependencies {
		if err := dependency.Register(ctx, iocContainer); err != nil {
			return err
		}
	}

	return nil
}

func (p *managerProvider) Boot(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	for _, dependency := range p.dependencies {
		if err := dependency.Boot(ctx, iocContainer); err != nil {
			return err
		}
	}

	return iocContainer.Singleton(managerConsoleCommand, ioc.WithNameBinding(ManagerHandler))
}

func (p *managerProvider) Terminate() error {
	for _, dependency := range p.dependencies {
		defer dependency.Terminate()
	}

	return nil
}

func managerConsoleCommand(
	database *mongo.Database,
	jetStreamPublishSubscriber domain.PublishSubscriber,
	validator domain.Validator,
	translator translatorContract.Translator,
	iocContainer ioc.ServiceContainer,
) (http.Handler, error) {
	taskScheduler := roundrobin.New()

	taskRepository := taskrepository.NewRepository(database)
	nodeRepository := noderepository.NewRepository(database)

	managerRunTaskUseCase := managerRunTask.NewUseCase(taskRepository, jetStreamPublishSubscriber, validator)
	managerDeleteTaskUseCase := managerDeleteTask.NewUseCase(taskRepository, jetStreamPublishSubscriber, translator)
	managerStopTaskUseCase := managerStopTask.NewUseCase(taskRepository, jetStreamPublishSubscriber, translator)
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

	handler := middleware.NewCORSMiddleware(middleware.NewRateLimitMiddleware(mux, 600, 1*time.Minute))

	subscribers := map[string]domain.MessageHandler{
		nodeEvents.HeartbeatName:        managerHeartbeatNode.NewHeartbeatHandler(nodeRepository),
		taskEvents.HeartbeatName:        managerHeartbeatTask.NewHeartbeatHandler(taskRepository, jetStreamPublishSubscriber),
		taskEvents.TaskRunRequestedName: managerRunTask.NewTaskRunRequested(managerRunTaskUseCase),
		taskEvents.TaskCreatedName:      managerRunTask.NewTaskCreated(taskRepository, nodeRepository, taskScheduler, jetStreamPublishSubscriber),
		taskEvents.TaskRanName:          managerRunTask.NewTaskRan(taskRepository),
		taskEvents.TaskCompletedName:    managerRunTask.NewTaskCompleted(taskRepository),
		taskEvents.TaskFailedName:       managerRunTask.NewTaskFailed(taskRepository),
		taskEvents.TaskStoppedName:      managerStopTask.NewTaskStopped(taskRepository),
	}

	// manager subscribers
	if err := iocContainer.Singleton(func() map[string]domain.MessageHandler {
		return subscribers
	}, ioc.WithNameBinding(ManagerSubscribers)); err != nil {
		return nil, err
	}

	return handler, nil
}
