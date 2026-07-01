package runner

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/danceable/container/bind"
	"github.com/danceable/container/resolve"
	"github.com/danceable/provider"
	"github.com/nats-io/nats.go"

	workerHeartbeat "github.com/khanzadimahdi/testproject/application/runner/worker/beatHeart"
	workerTaskHeartbeat "github.com/khanzadimahdi/testproject/application/runner/worker/task/beatHeart"
	workerDeleteTask "github.com/khanzadimahdi/testproject/application/runner/worker/task/deleteTask"
	workergettasks "github.com/khanzadimahdi/testproject/application/runner/worker/task/getTasks"
	workerruntask "github.com/khanzadimahdi/testproject/application/runner/worker/task/runTask"
	workerstoptask "github.com/khanzadimahdi/testproject/application/runner/worker/task/stopTask"
	"github.com/khanzadimahdi/testproject/domain"
	containerContract "github.com/khanzadimahdi/testproject/domain/runner/container"
	nodeContract "github.com/khanzadimahdi/testproject/domain/runner/node"
	taskEvents "github.com/khanzadimahdi/testproject/domain/runner/task/events"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/jetstream/produceConsumer"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	workerTaskAPI "github.com/khanzadimahdi/testproject/presentation/http/runner/worker/api/task"
)

const (
	WorkerSubscribers = "runner:worker:subscribers"
	WorkerName        = "runner:worker:name"

	consumerNamePrefix string = "runner-worker-%s"
)

// WorkerProviders returns the full, ordered set of service providers required
// by the runner worker service. name points at the worker name configured by
// the command (flag), which is bound into the container by workerNameProvider.
func WorkerProviders(name *string) []provider.Provider {
	return []provider.Provider{
		newWorkerNameProvider(name),
		providers.NewNatsProvider(),
		providers.NewDockerProvider(),
		providers.NewTranslationProvider(),
		providers.NewValidationProvider(),
		providers.NewContainerProvider(),
		NewWorkerProvider(),
	}
}

// workerNameProvider binds the worker name, falling back to the
// RUNNER_WORKER_NAME environment variable when the flag is empty.
type workerNameProvider struct {
	name *string
}

var _ provider.Provider = &workerNameProvider{}

func newWorkerNameProvider(name *string) *workerNameProvider {
	return &workerNameProvider{name: name}
}

func (p *workerNameProvider) Register(ctx context.Context, c provider.Container) error {
	if len(*p.name) == 0 {
		*p.name = os.Getenv("RUNNER_WORKER_NAME")
	}

	name := *p.name

	return c.Bind(func() string { return name }, bind.Singleton(), bind.WithName(WorkerName))
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
	log.SetFlags(log.LstdFlags | log.Llongfile)

	return nil
}

func (p *workerProvider) Boot(ctx context.Context, c provider.Container) error {
	var nodeName string
	if err := c.Resolve(&nodeName, resolve.WithName(WorkerName)); err != nil {
		return err
	}

	var natsConnection *nats.Conn
	if err := c.Resolve(&natsConnection); err != nil {
		return err
	}

	consumerName := fmt.Sprintf(consumerNamePrefix, nodeName)

	pc, err := produceConsumer.NewProduceConsumer(natsConnection, consumerName)
	if err != nil {
		return err
	}

	c.Bind(func() domain.Producer { return pc }, bind.Singleton())
	c.Bind(func() domain.Consumer { return pc }, bind.Singleton())
	c.Bind(func() domain.ProduceConsumer { return pc }, bind.Singleton())

	p.terminate = func() {
		defer pc.Wait()
	}

	return c.Bind(workerConsoleCommand, bind.Singleton())
}

func (p *workerProvider) Terminate(ctx context.Context) error {
	if p.terminate != nil {
		p.terminate()
	}

	return nil
}

func workerConsoleCommand(
	containerManager containerContract.Manager,
	nodeManager nodeContract.Manager,
	asyncProduceConsumer domain.ProduceConsumer,
	validator domain.Validator,
	iocContainer provider.Container,
) (http.Handler, error) {
	var nodeName string
	if err := iocContainer.Resolve(&nodeName, resolve.WithName(WorkerName)); err != nil {
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

	handler := middleware.NewRecoveryMiddleware(middleware.NewCORSMiddleware(middleware.NewRateLimitMiddleware(mux, 600, 1*time.Minute)))

	subscribers := map[string]domain.MessageHandler{
		taskEvents.TaskScheduledName:         workerruntask.NewTaskScheduled(runTaskUseCase, nodeName),
		taskEvents.TaskStoppageRequestedName: workerstoptask.NewStoppageTaskHandler(stopTaskUseCase),
		taskEvents.TaskDeletedName:           workerDeleteTask.NewDeleteTaskHandler(deleteTaskUseCase),
	}

	// worker subscribers
	if err := iocContainer.Bind(func() map[string]domain.MessageHandler {
		return subscribers
	}, bind.Singleton(), bind.WithName(WorkerSubscribers)); err != nil {
		return nil, err
	}

	// worker heartbeat
	if err := iocContainer.Bind(func() *workerHeartbeat.UseCase {
		return workerHeartbeat.NewUseCase(asyncProduceConsumer, nodeManager, nodeName)
	}, bind.Singleton()); err != nil {
		return nil, err
	}

	// task heartbeat
	if err := iocContainer.Bind(func() *workerTaskHeartbeat.UseCase {
		return workerTaskHeartbeat.NewUseCase(containerManager, asyncProduceConsumer, nodeName)
	}, bind.Singleton()); err != nil {
		return nil, err
	}

	return handler, nil
}
