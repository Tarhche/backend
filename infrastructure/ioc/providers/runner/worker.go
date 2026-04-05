package runner

import (
	"fmt"
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
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/jetstream/produceConsumer"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	workerTaskAPI "github.com/khanzadimahdi/testproject/presentation/http/runner/worker/api/task"
	"github.com/nats-io/nats.go"
)

const (
	WorkerSubscribers = "runner:worker:subscribers"
	WorkerHandler     = "runner:worker:handler"
	WorkerName        = "runner:worker:name"

	consumerNamePrefix string = "runner-worker-%s"
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
	terminate    func()
}

var _ ioc.ServiceProvider = &workerProvider{}

func NewWorkerProvider() *workerProvider {
	return &workerProvider{
		dependencies: workerDependencies,
	}
}

func (p *workerProvider) Register(app *ioc.Application) error {
	log.SetFlags(log.LstdFlags | log.Llongfile)

	for _, dependency := range p.dependencies {
		if err := dependency.Register(app); err != nil {
			return err
		}
	}

	return nil
}

func (p *workerProvider) Boot(app *ioc.Application) error {
	for _, dependency := range p.dependencies {
		if err := dependency.Boot(app); err != nil {
			return err
		}
	}

	var nodeName string
	if err := app.Container.Resolve(&nodeName, ioc.WithNameResolving(WorkerName)); err != nil {
		return err
	}

	var natsConnection *nats.Conn
	if err := app.Container.Resolve(&natsConnection); err != nil {
		return err
	}

	consumerName := fmt.Sprintf(consumerNamePrefix, nodeName)

	pc, err := produceConsumer.NewProduceConsumer(natsConnection, consumerName)
	if err != nil {
		return err
	}

	app.Container.Singleton(func() domain.Producer { return pc })
	app.Container.Singleton(func() domain.Consumer { return pc })
	app.Container.Singleton(func() domain.ProduceConsumer { return pc })

	p.terminate = func() {
		defer pc.Wait()
	}

	return app.Container.Singleton(workerConsoleCommand, ioc.WithNameBinding(WorkerHandler))
}

func (p *workerProvider) Terminate() error {
	for _, dependency := range p.dependencies {
		defer dependency.Terminate()
	}

	if p.terminate != nil {
		p.terminate()
	}

	return nil
}

func workerConsoleCommand(
	containerManager containerContract.Manager,
	asyncProduceConsumer domain.ProduceConsumer,
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
		return workerHeartbeat.NewUseCase(asyncProduceConsumer, nodeName)
	}); err != nil {
		return nil, err
	}

	// task heartbeat
	if err := iocContainer.Singleton(func() *workerTaskHeartbeat.UseCase {
		return workerTaskHeartbeat.NewUseCase(containerManager, asyncProduceConsumer, nodeName)
	}); err != nil {
		return nil, err
	}

	return handler, nil
}
