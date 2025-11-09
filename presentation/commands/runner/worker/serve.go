package worker

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	workerHeartbeat "github.com/khanzadimahdi/testproject/application/runner/worker/beatHeart"
	taskHeartbeat "github.com/khanzadimahdi/testproject/application/runner/worker/task/beatHeart"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/console"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers/runner"
)

const (
	serveName               string = "serve-runner-worker"
	consumerNamePrefix      string = "runner-worker-%s"
	workerHeartbeatInterval        = 1 * time.Second
	taskHeartbeatInterval          = 300 * time.Millisecond
)

type ServeCommand struct {
	port            int
	name            string
	handler         http.Handler
	subscriber      domain.Subscriber
	subscribers     map[string]domain.MessageHandler
	serviceProvider ioc.ServiceProvider
	taskHeartBeat   *taskHeartbeat.UseCase
	workerHeartBeat *workerHeartbeat.UseCase
}

// insures it implements console.Command
var _ console.Command = &ServeCommand{}

// insures it implements ioc.ServiceProvider
var _ ioc.ServiceProvider = &ServeCommand{}

func NewServeCommand(serviceProvider ioc.ServiceProvider) *ServeCommand {
	return &ServeCommand{
		serviceProvider: serviceProvider,
	}
}

// Name returns the name of the command which is used to identify it.
func (c *ServeCommand) Name() string {
	return serveName
}

// Description returns a short string (less than one line) describing the command.
func (c *ServeCommand) Description() string {
	return "serves a http server."
}

// Usage returns a long string explaining the command and giving usage
// information.
func (c *ServeCommand) Usage() string {
	return fmt.Sprintf("%s [arguments]", serveName)
}

func (c *ServeCommand) Configure(flagSet *flag.FlagSet) {
	flagSet.IntVar(&c.port, "port", 80, "specifies which port server should listen to.")
	flagSet.StringVar(&c.name, "name", "", "specifies the unique name of the worker.")
}

func (c *ServeCommand) Register(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	return c.serviceProvider.Register(ctx, iocContainer)
}

func (c *ServeCommand) Boot(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	if len(c.name) == 0 {
		c.name = os.Getenv("RUNNER_WORKER_NAME")
	}

	if err := iocContainer.Singleton(
		func() string { return c.name },
		ioc.WithNameBinding(runner.WorkerName),
	); err != nil {
		return err
	}

	if err := c.serviceProvider.Boot(ctx, iocContainer); err != nil {
		return err
	}

	if err := iocContainer.Resolve(&c.handler, ioc.WithNameResolving(runner.WorkerHandler)); err != nil {
		return err
	}

	if err := iocContainer.Resolve(&c.subscriber); err != nil {
		return err
	}

	if err := iocContainer.Resolve(&c.taskHeartBeat); err != nil {
		return err
	}

	if err := iocContainer.Resolve(&c.workerHeartBeat); err != nil {
		return err
	}

	return iocContainer.Resolve(&c.subscribers, ioc.WithNameResolving(runner.WorkerSubscribers))
}

func (c *ServeCommand) Terminate() error {
	return c.serviceProvider.Terminate()
}

func (c *ServeCommand) Run(ctx context.Context) console.ExitStatus {
	c.validateParams()

	server := http.Server{
		Addr:        fmt.Sprintf("0.0.0.0:%d", c.port),
		Handler:     c.handler,
		ReadTimeout: 20 * time.Second,
		IdleTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()

		// Shutdown the server after getting a signal with a timeout to ensure graceful shutdown.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = server.Shutdown(shutdownCtx)
	}()

	if err := c.subscribeToTopics(ctx); err != nil {
		log.Println(err)
		return console.ExitFailure
	}

	go c.tasksHeartbeat(ctx)
	go c.workerHeartbeat(ctx)

	if err := server.ListenAndServe(); err != nil {
		log.Println(err)
		return console.ExitFailure
	}

	return console.ExitSuccess
}

func (c *ServeCommand) validateParams() {
	if len(c.name) == 0 {
		log.Fatalf("name is required")
	}
}

func (c *ServeCommand) subscribeToTopics(ctx context.Context) error {
	consumerName := fmt.Sprintf(consumerNamePrefix, c.name)

	for subject, messageHandler := range c.subscribers {
		if err := c.subscriber.Subscribe(ctx, consumerName, subject, messageHandler); err != nil {
			return err
		}
	}

	return nil
}

func (c *ServeCommand) tasksHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(taskHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			err := c.taskHeartBeat.Execute(ctx)
			if err != nil {
				log.Println(err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (c *ServeCommand) workerHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(workerHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			err := c.workerHeartBeat.Execute()
			if err != nil {
				log.Println(err)
			}
		case <-ctx.Done():
			return
		}
	}
}
