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
	"github.com/khanzadimahdi/testproject/application/runner/worker/cluster"
	taskHeartbeat "github.com/khanzadimahdi/testproject/application/runner/worker/task/beatHeart"
	shipLogs "github.com/khanzadimahdi/testproject/application/runner/worker/task/shipLogs"
	"github.com/khanzadimahdi/testproject/domain"
	taskEvents "github.com/khanzadimahdi/testproject/domain/runner/task/events"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers/runner"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/ingress"
)

const (
	serveName               string = "serve-runner-worker"
	workerHeartbeatInterval        = 1 * time.Second
	taskHeartbeatInterval          = 300 * time.Millisecond

	// logShippingInterval is how often the followers are brought in line with
	// what is running. A container that has just started is followed within
	// this long, and one that has gone is let go.
	logShippingInterval = 1 * time.Second

	// forwardInterval is how often the node looks at what it is holding, which
	// is what tells it which ports to be listening on; forgetInterval how
	// often it drops what no node has spoken for.
	forwardInterval = time.Second
	forgetInterval  = 5 * time.Second
)

type ServeCommand struct {
	configs *configs.RunnerWorker
	handler http.Handler

	// ingress serves what this node's containers serve, and passes on what
	// belongs to another node: a container answers on a name of its own, and
	// every node hears where every container is.
	ingress         http.Handler
	forwarder       *ingress.Forwarder
	view            *cluster.View
	subscriber      domain.Subscriber
	consumer        domain.Consumer
	consumers       map[string]domain.MessageHandler
	taskHeartBeat   *taskHeartbeat.UseCase
	workerHeartBeat *workerHeartbeat.UseCase
	logShipper      *shipLogs.UseCase
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

	if err := container.Resolve(&c.logShipper); err != nil {
		return err
	}

	if err := container.Resolve(&c.logger, provider.WithParams("runner-worker-"+c.configs.Name)); err != nil {
		return err
	}

	if err := container.Resolve(&c.ingress, provider.ResolveName(runner.WorkerIngress)); err != nil {
		return err
	}

	if err := container.Resolve(&c.forwarder); err != nil {
		return err
	}

	if err := container.Resolve(&c.view); err != nil {
		return err
	}

	if err := container.Resolve(&c.subscriber); err != nil {
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

	// what every node is holding, heard from every node. It is a plain
	// subscription rather than a queue: each node keeps its own view, so each
	// node needs its own copy of what is said.
	if err := c.subscriber.Subscribe(ctx, taskEvents.HeartbeatName, c.view); err != nil {
		c.logger.ErrorContext(ctx, "failed to listen for what the other nodes are holding", "error", err)

		return console.ExitFailure
	}

	// no read timeout: what it carries is a container's own traffic, which may
	// be a long upload or a websocket rather than a request that finishes.
	containers := http.Server{
		Addr:        fmt.Sprintf("0.0.0.0:%d", c.configs.IngressPort),
		Handler:     c.ingress,
		IdleTimeout: 120 * time.Second,
	}

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = containers.Shutdown(shutdownCtx)
	}()

	go func() {
		if err := containers.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			c.logger.ErrorContext(ctx, "serving the containers failed", "error", err)
		}
	}()

	go c.forwarder.Serve(ctx, forwardInterval)
	go c.forgetGoneContainers(ctx)

	go c.tasksHeartbeat(ctx)
	go c.workerHeartbeat(ctx)
	go c.shipLogs(ctx)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		c.logger.ErrorContext(ctx, "server failed", "error", err)
		return console.ExitFailure
	}

	return console.ExitSuccess
}

// forgetGoneContainers drops what no node has spoken for lately, so a node
// that goes away takes its containers out of this one's view with it.
func (c *ServeCommand) forgetGoneContainers(ctx context.Context) {
	ticker := time.NewTicker(forgetInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.view.Forget()
		case <-ctx.Done():
			return
		}
	}
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

// shipLogs keeps a follower on every long-running container this node holds, so
// what they write reaches the manager as it is written.
func (c *ServeCommand) shipLogs(ctx context.Context) {
	defer c.logShipper.Close()

	ticker := time.NewTicker(logShippingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.logShipper.Execute(ctx); err != nil {
				c.logger.ErrorContext(ctx, "log shipping failed", "error", err)
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
