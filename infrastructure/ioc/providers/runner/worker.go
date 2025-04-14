package runner

import (
	"context"
	"log"
	"net/http"
	"time"

	workerHeartbeat "github.com/khanzadimahdi/testproject/application/runner/worker/beatHeart"
	workerTaskHeartbeat "github.com/khanzadimahdi/testproject/application/runner/worker/task/beatHeart"
	workerDeleteTask "github.com/khanzadimahdi/testproject/application/runner/worker/task/deleteTask"
	workergettasks "github.com/khanzadimahdi/testproject/application/runner/worker/task/getTasks"
	workerruntask "github.com/khanzadimahdi/testproject/application/runner/worker/task/runTask"
	workerstoptask "github.com/khanzadimahdi/testproject/application/runner/worker/task/stopTask"
	"github.com/khanzadimahdi/testproject/domain"
	containerContract "github.com/khanzadimahdi/testproject/domain/runner/container"
	taskEvents "github.com/khanzadimahdi/testproject/domain/runner/task/events"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers"
	workerTaskAPI "github.com/khanzadimahdi/testproject/presentation/http/api/runner/worker/task"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
)

const (
	WorkerSubscribers = "runner:worker:subscribers"
	WorkerHandler     = "runner:worker:handler"
	WorkerName        = "runner:worker:name"
)

var workerDependencies = []ioc.ServiceProvider{
	providers.NewNatsProvider(),
	providers.NewDockerProvider(),
	providers.NewTranslationProvider(),
	providers.NewValidationProvider(),
	providers.NewContainerProvider(),
}

type workerProvider struct {
	dependencies []ioc.ServiceProvider
}

var _ ioc.ServiceProvider = &workerProvider{}

func NewWorkerProvider() *workerProvider {
	return &workerProvider{
		dependencies: workerDependencies,
	}
}

func (p *workerProvider) Register(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	log.SetFlags(log.LstdFlags | log.Llongfile)

	for _, dependency := range p.dependencies {
		if err := dependency.Register(ctx, iocContainer); err != nil {
			return err
		}
	}

	return nil
}

func (p *workerProvider) Boot(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	for _, dependency := range p.dependencies {
		if err := dependency.Boot(ctx, iocContainer); err != nil {
			return err
		}
	}

	return iocContainer.Singleton(workerConsoleCommand, ioc.WithNameBinding(WorkerHandler))
}

func (p *workerProvider) Terminate() error {
	for _, dependency := range p.dependencies {
		defer dependency.Terminate()
	}

	return nil
}

func workerConsoleCommand(
	containerManager containerContract.Manager,
	asyncPublishSubscriber domain.PublishSubscriber,
	validator domain.Validator,
	iocContainer ioc.ServiceContainer,
) (http.Handler, error) {
	var nodeName string
	if err := iocContainer.Resolve(&nodeName, ioc.WithNameResolving(WorkerName)); err != nil {
		return nil, err
	}

	// tasks
	getTasksUseCase := workergettasks.NewUseCase(containerManager, nodeName)
	runTaskUseCase := workerruntask.NewUseCase(containerManager, validator, nodeName)
	stopTaskUseCase := workerstoptask.NewUseCase(containerManager, validator)
	deleteTaskUseCase := workerDeleteTask.NewUseCase(containerManager, validator)

	mux := http.NewServeMux()

	mux.Handle("GET /api/tasks", workerTaskAPI.NewIndexHandler(getTasksUseCase))
	mux.Handle("POST /api/tasks/run", workerTaskAPI.NewRunHandler(runTaskUseCase))
	mux.Handle("POST /api/tasks/{uuid}/stop", workerTaskAPI.NewStopHandler(stopTaskUseCase))

	handler := middleware.NewCORSMiddleware(middleware.NewRateLimitMiddleware(mux, 600, 1*time.Minute))

	subscribers := map[string]domain.MessageHandler{
		taskEvents.TaskScheduledName:         workerruntask.NewTaskScheduled(runTaskUseCase, nodeName),
		taskEvents.TaskStoppageRequestedName: workerstoptask.NewStoppageTaskHandler(stopTaskUseCase),
		taskEvents.TaskDeletedName:           workerDeleteTask.NewDeleteTaskHandler(deleteTaskUseCase),
	}

	// worker subscribers
	if err := iocContainer.Singleton(func() map[string]domain.MessageHandler {
		return subscribers
	}, ioc.WithNameBinding(WorkerSubscribers)); err != nil {
		return nil, err
	}

	// worker heartbeat
	if err := iocContainer.Singleton(func() *workerHeartbeat.UseCase {
		return workerHeartbeat.NewUseCase(asyncPublishSubscriber, nodeName)
	}); err != nil {
		return nil, err
	}

	// task heartbeat
	if err := iocContainer.Singleton(func() *workerTaskHeartbeat.UseCase {
		return workerTaskHeartbeat.NewUseCase(containerManager, asyncPublishSubscriber, nodeName)
	}); err != nil {
		return nil, err
	}

	return handler, nil
}
