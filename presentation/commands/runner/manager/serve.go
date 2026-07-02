package manager

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/danceable/container/resolve"
	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/console"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers/runner"
)

const (
	serveName string = "serve-runner-manager"
)

type ServeCommand struct {
	port      int
	handler   http.Handler
	consumer  domain.Consumer
	consumers map[string]domain.MessageHandler
	logger    *slog.Logger
}

// insures it implements console.Command
var _ console.Command = &ServeCommand{}

// insures it implements console.Service
var _ console.Service = &ServeCommand{}

func NewServeCommand() *ServeCommand {
	return &ServeCommand{}
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
}

// Providers returns the service providers required to serve the runner manager.
func (c *ServeCommand) Providers() []provider.Provider {
	return runner.ManagerProviders()
}

// Boot resolves the command's dependencies from the booted container.
func (c *ServeCommand) Boot(ctx context.Context, container provider.Container) error {
	if err := container.Resolve(&c.handler); err != nil {
		return err
	}

	if err := container.Resolve(&c.consumer); err != nil {
		return err
	}

	if err := container.Resolve(&c.logger, resolve.WithParams("runner-manager")); err != nil {
		return err
	}

	return container.Resolve(&c.consumers, resolve.WithName(runner.ManagerSubscribers))
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

	if err := c.consumeTopics(ctx); err != nil {
		c.logger.ErrorContext(ctx, "failed to consume topics", "error", err)
		return console.ExitFailure
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		c.logger.ErrorContext(ctx, "server failed", "error", err)
		return console.ExitFailure
	}

	return console.ExitSuccess
}

func (c *ServeCommand) consumeTopics(ctx context.Context) error {
	for subject, messageHandler := range c.consumers {
		if err := c.consumer.Consume(ctx, subject, messageHandler); err != nil {
			return err
		}
	}

	return nil
}
