package controlplane

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/danceable/console"
	"github.com/danceable/provider"

	kindsReconcileResources "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/reconcileResources"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers/core"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers/workload"
)

const (
	serveName string = "serve-workload-controlplane"

	// heartbeatInterval is how often the control plane looks at what the
	// resources of every kind are doing against what was asked of them. Often
	// enough that one somebody stopped by hand comes back while they are still
	// looking at it; rarely enough that it is not a poll of the whole
	// workload.
	heartbeatInterval = 10 * time.Second

	// migrationsInterval is how often the control plane looks again at
	// whether what is stored is migrated, while it waits for it to be.
	migrationsInterval = 10 * time.Second

	// healthPath is the one route served while the control plane waits: its
	// health is its database's and its messaging's, migrated or not, so a
	// deploy finds it up and goes on to `app migrate`.
	healthPath = "/health"
)

type ServeCommand struct {
	configs   *configs.WorkloadControlPlane
	handler   http.Handler
	consumer  domain.Consumer
	consumers map[string]domain.MessageHandler

	// reconcileKinds is the control plane's own heartbeat: one pass over the
	// resources of every kind registered, VMs, stacks and tasks among them,
	// asking the nodes for whatever would make each of them what it is meant
	// to be.
	reconcileKinds *kindsReconcileResources.UseCase

	// migrations say what is still to be applied to what is stored. Until
	// none is, the control plane touches nothing of the workload: what it
	// read would not be what the workload is, and a VM a node holds that it
	// found no record of, it would ask the node to delete.
	migrations domain.Migrations

	// prepare readies the stores the workload is kept in, once what is
	// stored is migrated.
	prepare func(ctx context.Context) error

	// migrationsEvery is how often the migrations are looked at again while
	// they are waited for.
	migrationsEvery time.Duration

	// started is whether the control plane has started: until it has, its
	// API answers 503.
	started atomic.Bool

	logger *slog.Logger
}

var (
	_ console.Command   = &ServeCommand{}
	_ console.Service   = &ServeCommand{}
	_ provider.Provider = &ServeCommand{}
)

func NewServeCommand() *ServeCommand {
	return &ServeCommand{configs: configs.NewWorkloadControlPlane(), migrationsEvery: migrationsInterval}
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

	if err := task.Resolve(&c.reconcileKinds); err != nil {
		return err
	}

	if err := task.Resolve(&c.migrations); err != nil {
		return err
	}

	if err := task.Resolve(&c.prepare, provider.ResolveName(workload.ControlPlanePrepare)); err != nil {
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
		Handler:     c.unlessStarted(c.handler),
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

	// its health is served at once, and the rest once it has started, which
	// it does as soon as what is stored is migrated. One that cannot start
	// stops serving.
	failed := make(chan error, 1)

	go func() {
		if err := c.start(ctx); err != nil && ctx.Err() == nil {
			failed <- err
			_ = server.Close()
		}
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		c.logger.ErrorContext(ctx, "server failed", "error", err)
		return console.ExitFailure
	}

	select {
	case err := <-failed:
		c.logger.ErrorContext(ctx, "the control plane could not start", "error", err)
		return console.ExitFailure
	default:
		return console.ExitSuccess
	}
}

// start waits for what is stored to be migrated, and then starts the control
// plane as it always started: the stores the workload is kept in readied,
// what the nodes say heard, and every resource kept as it was asked to be,
// before its API is served. Until then, nothing of the workload is touched,
// and what the nodes say waits in its streams. It is nil when ctx ends first.
func (c *ServeCommand) start(ctx context.Context) error {
	if !c.migrated(ctx) {
		return nil
	}

	if err := c.prepare(ctx); err != nil {
		return fmt.Errorf("the workload's stores could not be readied: %w", err)
	}

	if err := c.consumeTopics(ctx); err != nil {
		return fmt.Errorf("failed to consume topics: %w", err)
	}

	go c.heartbeat(ctx)

	c.started.Store(true)

	return nil
}

// migrated waits until no migration is pending, looking again every
// migrationsEvery and saying each time what it waits for, and reports whether
// it got there before ctx ended.
func (c *ServeCommand) migrated(ctx context.Context) bool {
	ticker := time.NewTicker(c.migrationsEvery)
	defer ticker.Stop()

	waited := false

	for {
		pending, err := c.pending(ctx)

		switch {
		case ctx.Err() != nil:
			return false
		case err != nil:
			c.logger.ErrorContext(ctx, "what is migrated could not be read: waiting for `app migrate` until it can be", "error", err)
		case len(pending) > 0:
			waited = true
			c.logger.WarnContext(ctx, "waiting for `app migrate`: nothing of the workload is touched until what is stored is migrated", "pending", pending)
		default:
			if waited {
				c.logger.InfoContext(ctx, "what is stored is migrated: the control plane starts")
			}

			return true
		}

		select {
		case <-ticker.C:
		case <-ctx.Done():
			return false
		}
	}
}

// pending is the migrations not applied yet, read in no longer than the wait
// before the next look.
func (c *ServeCommand) pending(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.migrationsEvery)
	defer cancel()

	return c.migrations.Pending(ctx)
}

// unlessStarted answers every request but a health check with 503 until the
// control plane has started: what its API reads and writes is the workload,
// which it does not touch before.
func (c *ServeCommand) unlessStarted(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if !c.started.Load() && r.URL.Path != healthPath {
			http.Error(rw, "the control plane is waiting for what is stored to be migrated (app migrate)", http.StatusServiceUnavailable)

			return
		}

		handler.ServeHTTP(rw, r)
	})
}

// heartbeat keeps the resources of every kind, VMs, stacks and tasks among
// them, as they were asked to be, for as long as the control plane is up.
func (c *ServeCommand) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
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
