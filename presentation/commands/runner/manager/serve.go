package manager

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/danceable/console"
	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/application/runner/manager/task/reconcile"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers/runner"
)

const (
	serveName string = "serve-runner-manager"

	// heartbeatInterval is how often the manager looks at what the containers
	// are doing against what was asked of them. Often enough that a container
	// somebody stopped by hand comes back while they are still looking at it;
	// rarely enough that it is not a poll of the whole runner.
	heartbeatInterval = 10 * time.Second
)

type ServeCommand struct {
	configs  *configs.RunnerManager
	handler  http.Handler
	consumer domain.Consumer

	// ingress serves the containers' own exposed ports, on a port of its own:
	// a request there is routed to a container by the hostname it was made to,
	// which is not something the API's own routes should have to step around.
	ingress http.Handler

	consumers map[string]domain.MessageHandler

	// reconcile is the manager's own heartbeat: one pass over the containers,
	// asking the nodes for whatever would make each of them what it is meant
	// to be.
	reconcile *reconcile.UseCase

	logger *slog.Logger
}

var (
	_ console.Command   = &ServeCommand{}
	_ console.Service   = &ServeCommand{}
	_ provider.Provider = &ServeCommand{}
)

func NewServeCommand() *ServeCommand {
	return &ServeCommand{configs: configs.NewRunnerManager()}
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

// Providers returns the service providers required to serve the runner manager.
func (c *ServeCommand) Providers() []provider.Provider {
	return []provider.Provider{
		providers.NewConfigsProvider(c.configs),
		providers.NewOpenTelemetryProvider("runner-manager", "runner-manager"),
		providers.NewProfilerProvider("runner-manager"),
		providers.NewMongodbProvider(),
		providers.NewNatsProvider(),
		providers.NewTranslationProvider(),
		providers.NewValidationProvider(),
		providers.NewContainerProvider(),
		runner.NewManagerProvider(),
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

	if err := container.Resolve(&c.logger, provider.WithParams("runner-manager")); err != nil {
		return err
	}

	if err := container.Resolve(&c.ingress, provider.ResolveName(runner.ManagerIngress)); err != nil {
		return err
	}

	if err := container.Resolve(&c.reconcile); err != nil {
		return err
	}

	return container.Resolve(&c.consumers, provider.ResolveName(runner.ManagerSubscribers))
}

// Terminate terminates the command's own resources, of which it has none. The
// providers it returned are terminated by the manager.
func (c *ServeCommand) Terminate(ctx context.Context) error {
	return nil
}

// @title			Runner Manager API
// @version		1.0
// @description	Swagger/OpenAPI documentation for the runner manager service.
// @termsOfService	http://swagger.io/terms/
//
// @license.name	Apache 2.0
// @license.url	http://www.apache.org/licenses/LICENSE-2.0.html
//
// @host			0.0.0.0:80
// @basePath		/api
// @schemes		http
func (c *ServeCommand) Run(ctx context.Context) console.ExitStatus {
	server := http.Server{
		Addr:        fmt.Sprintf("0.0.0.0:%d", c.configs.Port),
		Handler:     c.handler,
		ReadTimeout: 20 * time.Second,
		IdleTimeout: 10 * time.Second,
	}

	// no read timeout: what it carries is a container's own traffic, which may
	// be a long upload or a websocket rather than a request that finishes.
	ingress := http.Server{
		Addr:        fmt.Sprintf("0.0.0.0:%d", c.configs.IngressPort),
		Handler:     c.ingress,
		IdleTimeout: 120 * time.Second,
	}

	go func() {
		<-ctx.Done()

		// Shutdown the server after getting a signal with a timeout to ensure graceful shutdown.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = server.Shutdown(shutdownCtx)
		_ = ingress.Shutdown(shutdownCtx)
	}()

	if err := c.consumeTopics(ctx); err != nil {
		c.logger.ErrorContext(ctx, "failed to consume topics", "error", err)
		return console.ExitFailure
	}

	go func() {
		if err := ingress.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			c.logger.ErrorContext(ctx, "the ingress failed", "error", err)
		}
	}()

	go c.heartbeat(ctx)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		c.logger.ErrorContext(ctx, "server failed", "error", err)
		return console.ExitFailure
	}

	return console.ExitSuccess
}

// heartbeat keeps the containers as they were asked to be, for as long as the
// manager is up.
func (c *ServeCommand) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.reconcile.Execute(ctx); err != nil {
				c.logger.ErrorContext(ctx, "the runner's heartbeat failed", "error", err)
			}

		case <-ctx.Done():
			return
		}
	}
}

func (c *ServeCommand) consumeTopics(ctx context.Context) error {
	for subject, messageHandler := range c.consumers {
		if err := c.consumer.Consume(ctx, subject, messageHandler); err != nil {
			return err
		}
	}

	return nil
}
