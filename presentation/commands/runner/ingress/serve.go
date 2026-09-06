// Package ingress serves what the containers themselves serve.
//
// It is a service of its own, and deliberately so: the manager schedules
// containers and keeps them the way they were asked to be, which is a
// different job from carrying a reader's request to one. Only one of the two
// is worth an outage, and this is the one that is not.
package ingress

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/danceable/console"
	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers/runner"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/ingress"
)

const serveName string = "serve-runner-ingress"

type ServeCommand struct {
	configs *configs.RunnerIngress

	handler   http.Handler
	forwarder *ingress.Forwarder
	logger    *slog.Logger
}

var (
	_ console.Command   = &ServeCommand{}
	_ console.Service   = &ServeCommand{}
	_ provider.Provider = &ServeCommand{}
)

func NewServeCommand() *ServeCommand {
	return &ServeCommand{configs: configs.NewRunnerIngress()}
}

// Name returns the name of the command which is used to identify it.
func (c *ServeCommand) Name() string {
	return serveName
}

// Description returns a short string (less than one line) describing the command.
func (c *ServeCommand) Description() string {
	return "serves the containers' own exposed ports."
}

// Usage returns a long string explaining the command and giving usage
// information.
func (c *ServeCommand) Usage() string {
	return fmt.Sprintf("%s [arguments]", serveName)
}

// Configure defines this command's flags, which are the fields of its
// configuration struct.
func (c *ServeCommand) Configure(flagSet *console.FlagSet) {
	if err := flagSet.Struct(c.configs); err != nil {
		panic(err)
	}
}

// Providers returns the service providers required to serve the ingress. It
// needs the database and nothing else: no queue, no docker, and nothing of the
// manager's.
func (c *ServeCommand) Providers() []provider.Provider {
	return []provider.Provider{
		providers.NewConfigsProvider(c.configs),
		providers.NewOpenTelemetryProvider("runner-ingress", "runner-ingress"),
		providers.NewProfilerProvider("runner-ingress"),
		providers.NewMongodbProvider(),
		runner.NewIngressProvider(),
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

	if err := container.Resolve(&c.forwarder); err != nil {
		return err
	}

	return container.Resolve(&c.logger, provider.WithParams("runner-ingress"))
}

// Terminate terminates the command's own resources, of which it has none.
func (c *ServeCommand) Terminate(ctx context.Context) error {
	return nil
}

func (c *ServeCommand) Run(ctx context.Context) console.ExitStatus {
	// no read timeout: what it carries is a container's own traffic, which may
	// be a long upload or a websocket rather than a request that finishes.
	server := http.Server{
		Addr:        fmt.Sprintf("0.0.0.0:%d", c.configs.Port),
		Handler:     c.handler,
		IdleTimeout: 120 * time.Second,
	}

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = server.Shutdown(shutdownCtx)
	}()

	// and beside it, the ports the containers are reached on whole.
	go c.forwarder.Serve(ctx, c.configs.PollInterval)

	c.logger.InfoContext(ctx, "the ingress is serving", "port", c.configs.Port, "domain", c.configs.Domain, "forwarding", c.configs.PortRange)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		c.logger.ErrorContext(ctx, "the ingress failed", "error", err)

		return console.ExitFailure
	}

	return console.ExitSuccess
}
