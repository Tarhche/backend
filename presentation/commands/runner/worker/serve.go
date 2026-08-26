package worker

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/danceable/console"
	"github.com/danceable/provider"

	workerHeartbeat "github.com/khanzadimahdi/testproject/application/runner/worker/beatHeart"
	taskHeartbeat "github.com/khanzadimahdi/testproject/application/runner/worker/task/beatHeart"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers/runner"
)

const (
	serveName               string = "serve-runner-worker"
	workerHeartbeatInterval        = 1 * time.Second
	taskHeartbeatInterval          = 300 * time.Millisecond
)

type ServeCommand struct {
	configs         *configs.RunnerWorker
	handler         http.Handler
	consumer        domain.Consumer
	consumers       map[string]domain.MessageHandler
	taskHeartBeat   *taskHeartbeat.UseCase
	workerHeartBeat *workerHeartbeat.UseCase
	logger          *slog.Logger
}

var (
	_ console.Command   = &ServeCommand{}
	_ console.Service   = &ServeCommand{}
	_ provider.Provider = &ServeCommand{}
)

func NewServeCommand() *ServeCommand {
	return &ServeCommand{configs: configs.NewRunnerWorker()}
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

// Configure defines this command's flags, which are the fields of its
// configuration struct. A struct which cannot be bound is a programming
// mistake rather than user input, so it panics the way the console itself does
// for a flag it cannot define.
func (c *ServeCommand) Configure(flagSet *console.FlagSet) {
	if err := flagSet.Struct(c.configs); err != nil {
		panic(err)
	}
}

// Providers returns the service providers required to serve the runner worker.
// The worker name (configured by flag or environment) is bound into the
// container so the worker providers can resolve it.
func (c *ServeCommand) Providers() []provider.Provider {
	return []provider.Provider{
		providers.NewConfigsProvider(c.configs),
		runner.NewWorkerNameProvider(),
		providers.NewOpenTelemetryProvider("runner-worker", c.configs.Name),
		providers.NewProfilerProvider("runner-worker"),
		providers.NewNatsProvider(),
		providers.NewDockerProvider(),
		providers.NewTranslationProvider(),
		providers.NewValidationProvider(),
		providers.NewContainerProvider(),
		runner.NewWorkerProvider(),
		c,
	}
}

// Register registers the command's own dependencies, of which it has none.
func (c *ServeCommand) Register(ctx context.Context, container provider.Container) error {
	return nil
}

// Boot resolves the command's dependencies from the booted container.
func (c *ServeCommand) Boot(ctx context.Context, container provider.Container) error {
	if err := container.Resolve(&c.handler); err != nil {
		return err
	}

	if err := container.Resolve(&c.consumer); err != nil {
		return err
	}

	if err := container.Resolve(&c.taskHeartBeat); err != nil {
		return err
	}

	if err := container.Resolve(&c.workerHeartBeat); err != nil {
		return err
	}

	if err := container.Resolve(&c.logger, provider.WithParams("runner-worker-"+c.configs.Name)); err != nil {
		return err
	}

	return container.Resolve(&c.consumers, provider.ResolveName(runner.WorkerSubscribers))
}

// Terminate terminates the command's own resources, of which it has none. The
// providers it returned are terminated by the manager.
func (c *ServeCommand) Terminate(ctx context.Context) error {
	return nil
}

// @title			Runner Worker API
// @version		1.0
// @description	Swagger/OpenAPI documentation for the runner worker service.
// @termsOfService	http://swagger.io/terms/
//
// @license.name	Apache 2.0
// @license.url	http://www.apache.org/licenses/LICENSE-2.0.html
//
// @host			0.0.0.0:80
// @basePath		/api
// @schemes		http
func (c *ServeCommand) Run(ctx context.Context) console.ExitStatus {
	if !c.validateParams() {
		return console.ExitFailure
	}

	server := http.Server{
		Addr:        fmt.Sprintf("0.0.0.0:%d", c.configs.Port),
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

	if err := c.consumeTopics(ctx); err != nil {
		c.logger.ErrorContext(ctx, "failed to consume topics", "error", err)
		return console.ExitFailure
	}

	go c.tasksHeartbeat(ctx)
	go c.workerHeartbeat(ctx)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		c.logger.ErrorContext(ctx, "server failed", "error", err)
		return console.ExitFailure
	}

	return console.ExitSuccess
}

func (c *ServeCommand) validateParams() bool {
	if len(c.configs.Name) == 0 {
		c.logger.Error("name is required")
		return false
	}

	return true
}

func (c *ServeCommand) consumeTopics(ctx context.Context) error {
	for subject, messageHandler := range c.consumers {
		if err := c.consumer.Consume(ctx, subject, messageHandler); err != nil {
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
				c.logger.ErrorContext(ctx, "task heartbeat failed", "error", err)
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
			err := c.workerHeartBeat.Execute(ctx)
			if err != nil {
				c.logger.ErrorContext(ctx, "worker heartbeat failed", "error", err)
			}
		case <-ctx.Done():
			return
		}
	}
}
