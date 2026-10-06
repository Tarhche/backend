package controlplane

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/danceable/console"
	"github.com/danceable/provider"

	kindsReconcile "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/reconcile"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/task/reconcile"
	vmReconcile "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/reconcile"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers/core"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers/workload"
)

const (
	serveName string = "serve-workload-controlplane"

	// heartbeatInterval is how often the control plane looks at what the tasks
	// are doing against what was asked of them. Often enough that a task
	// somebody stopped by hand comes back while they are still looking at it;
	// rarely enough that it is not a poll of the whole workload.
	heartbeatInterval = 10 * time.Second
)

type ServeCommand struct {
	configs   *configs.WorkloadControlPlane
	handler   http.Handler
	consumer  domain.Consumer
	consumers map[string]domain.MessageHandler

	// reconcile is the control plane's own heartbeat: one pass over the tasks,
	// asking the nodes for whatever would make each of them what it is meant
	// to be. reconcileVMs is the same for the VMs, and the stacks waiting on
	// them, and reconcileKinds for the resources of every kind registered.
	reconcile      *reconcile.UseCase
	reconcileVMs   *vmReconcile.UseCase
	reconcileKinds *kindsReconcile.UseCase

	logger *slog.Logger
}

var (
	_ console.Command   = &ServeCommand{}
	_ console.Service   = &ServeCommand{}
	_ provider.Provider = &ServeCommand{}
)

func NewServeCommand() *ServeCommand {
	return &ServeCommand{configs: configs.NewWorkloadControlPlane()}
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

// Providers returns the service providers required to serve the workload control plane.
func (c *ServeCommand) Providers() []provider.Provider {
	return []provider.Provider{
		core.NewConfigsProvider(c.configs),
		core.NewOpenTelemetryProvider("workload-controlplane", "workload-controlplane"),
		core.NewProfilerProvider("workload-controlplane"),
		providers.NewMongodbProvider(),
		providers.NewNatsProvider(),
		providers.NewTranslationProvider(),
		providers.NewValidationProvider(),
		core.NewContainerProvider(),
		workload.NewManagerProvider(),
		c,
	}
}

// Register registers the command's own dependencies, of which it has none.
func (c *ServeCommand) Register(ctx context.Context, task provider.Container) error {
	return nil
}

// Boot resolves the command's dependencies from the booted task.
func (c *ServeCommand) Boot(ctx context.Context, task provider.Container) error {
	if err := task.Resolve(&c.handler); err != nil {
		return err
	}

	if err := task.Resolve(&c.consumer); err != nil {
		return err
	}

	if err := task.Resolve(&c.logger, provider.WithParams("workload-controlplane")); err != nil {
		return err
	}

	if err := task.Resolve(&c.reconcile); err != nil {
		return err
	}

	if err := task.Resolve(&c.reconcileVMs); err != nil {
		return err
	}

	if err := task.Resolve(&c.reconcileKinds); err != nil {
		return err
	}

	return task.Resolve(&c.consumers, provider.ResolveName(workload.ControlPlaneSubscribers))
}

// Terminate terminates the command's own resources, of which it has none. The
// providers it returned are terminated by the manager.
func (c *ServeCommand) Terminate(ctx context.Context) error {
	return nil
}

// @title			Workload Control Plane API
// @version		1.0
// @description	Swagger/OpenAPI documentation for the workload control plane service.
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

	go c.heartbeat(ctx)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		c.logger.ErrorContext(ctx, "server failed", "error", err)
		return console.ExitFailure
	}

	return console.ExitSuccess
}

// heartbeat keeps the tasks, the VMs and the resources of every kind as they
// were asked to be, for as long as the control plane is up. One failing is no
// reason to skip the others.
func (c *ServeCommand) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if c.reconcile != nil {
				if err := c.reconcile.Execute(ctx); err != nil {
					c.logger.ErrorContext(ctx, "the workload's heartbeat failed", "error", err)
				}
			}

			if c.reconcileVMs != nil {
				if err := c.reconcileVMs.Execute(ctx); err != nil {
					c.logger.ErrorContext(ctx, "the vms' heartbeat failed", "error", err)
				}
			}

			if c.reconcileKinds != nil {
				if err := c.reconcileKinds.Execute(ctx); err != nil {
					c.logger.ErrorContext(ctx, "the kinds' heartbeat failed", "error", err)
				}
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
