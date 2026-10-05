package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/danceable/console"
	"github.com/danceable/provider"

	orchestratorHeartbeat "github.com/khanzadimahdi/testproject/application/workload/orchestrator/beatHeart"
	taskHeartbeat "github.com/khanzadimahdi/testproject/application/workload/orchestrator/task/beatHeart"
	shipLogs "github.com/khanzadimahdi/testproject/application/workload/orchestrator/task/shipLogs"
	vmHeartbeat "github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/beatHeart"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers/workload"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/core/request"
	"github.com/khanzadimahdi/testproject/infrastructure/tunnel"
)

const (
	serveName                     string = "serve-workload-orchestrator"
	orchestratorHeartbeatInterval        = 1 * time.Second
	taskHeartbeatInterval                = 300 * time.Millisecond

	// logShippingInterval is how often the followers are brought in line with
	// what is running. A task that has just started is followed within
	// this long, and one that has gone is let go.
	logShippingInterval = 1 * time.Second

	// vmHeartbeatInterval is how often this node says what its VMs are doing,
	// with a sample of what each running one uses.
	vmHeartbeatInterval = 2 * time.Second
)

// requestServer answers the control plane's requests to this node.
type requestServer interface {
	Serve(ctx context.Context, subject string) error
}

type ServeCommand struct {
	configs               *configs.WorkloadOrchestrator
	handler               http.Handler
	consumer              domain.Consumer
	consumers             map[string]domain.MessageHandler
	taskHeartBeat         *taskHeartbeat.UseCase
	orchestratorHeartBeat *orchestratorHeartbeat.UseCase
	logShipper            *shipLogs.UseCase
	vmHeartBeat           *vmHeartbeat.UseCase

	// requests answers what the control plane asks this node and waits for:
	// a VM's log, and whatever is asked of a Docker VM's dockerd.
	requests requestServer

	// tunnel holds this orchestrator's connections to the ingresses. They are how a
	// request reaches it: nothing dials an orchestrator, so its own port answers only
	// the healthcheck, whoever is on the machine, and the tunnel itself.
	tunnel *tunnel.Agent

	logger *slog.Logger
}

var (
	_ console.Command   = &ServeCommand{}
	_ console.Service   = &ServeCommand{}
	_ provider.Provider = &ServeCommand{}
)

func NewServeCommand() *ServeCommand {
	return &ServeCommand{configs: configs.NewWorkloadOrchestrator()}
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

// Providers returns the service providers required to serve the workload orchestrator.
// The orchestrator name (configured by flag or environment) is bound into the
// task so the orchestrator providers can resolve it.
func (c *ServeCommand) Providers() []provider.Provider {
	return []provider.Provider{
		providers.NewConfigsProvider(c.configs),
		workload.NewOrchestratorNameProvider(),
		providers.NewOpenTelemetryProvider("workload-orchestrator", c.configs.Name),
		providers.NewProfilerProvider("workload-orchestrator"),
		providers.NewNatsProvider(),
		providers.NewTranslationProvider(),
		providers.NewValidationProvider(),
		providers.NewContainerProvider(),
		workload.NewOrchestratorProvider(),
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

	if err := task.Resolve(&c.taskHeartBeat); err != nil {
		return err
	}

	if err := task.Resolve(&c.orchestratorHeartBeat); err != nil {
		return err
	}

	if err := task.Resolve(&c.logShipper); err != nil {
		return err
	}

	if err := task.Resolve(&c.vmHeartBeat); err != nil {
		return err
	}

	var responder *request.Responder
	if err := task.Resolve(&responder); err != nil {
		return err
	}

	c.requests = responder

	if err := task.Resolve(&c.tunnel); err != nil {
		return err
	}

	if err := task.Resolve(&c.logger, provider.WithParams("workload-orchestrator-"+c.configs.Name)); err != nil {
		return err
	}

	return task.Resolve(&c.consumers, provider.ResolveName(workload.OrchestratorSubscribers))
}

// Terminate terminates the command's own resources, of which it has none. The
// providers it returned are terminated by the manager.
func (c *ServeCommand) Terminate(ctx context.Context) error {
	return nil
}

// @title			Workload Orchestrator API
// @version		1.0
// @description	Swagger/OpenAPI documentation for the workload orchestrator service.
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

	if err := c.requests.Serve(ctx, noderequest.Subject(c.configs.Name)); err != nil {
		c.logger.ErrorContext(ctx, "failed to answer node requests", "error", err)
		return console.ExitFailure
	}

	go c.tasksHeartbeat(ctx)
	go c.vmsHeartbeat(ctx)
	go c.orchestratorHeartbeat(ctx)
	go c.shipLogs(ctx)
	go c.serveTunnel(ctx)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		c.logger.ErrorContext(ctx, "server failed", "error", err)
		return console.ExitFailure
	}

	return console.ExitSuccess
}

// serveTunnel keeps this orchestrator's connections to the ingresses open.
//
// What arrives on them is carried to whichever of this orchestrator's services was
// asked for, which for its own api is the port it is already listening on. So
// there is one server rather than two, and a stream reaching it is
// indistinguishable from a request made on the machine itself.
func (c *ServeCommand) serveTunnel(ctx context.Context) {
	c.tunnel.Run(ctx)
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

// shipLogs keeps a follower on every long-running task this node holds, so
// what they write reaches the control plane as it is written.
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

// vmsHeartbeat says, every beat, what this node's VMs are doing and what the
// node offers them.
func (c *ServeCommand) vmsHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(vmHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.vmHeartBeat.Execute(ctx); err != nil {
				c.logger.ErrorContext(ctx, "vm heartbeat failed", "error", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (c *ServeCommand) orchestratorHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(orchestratorHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			err := c.orchestratorHeartBeat.Execute(ctx)
			if err != nil {
				c.logger.ErrorContext(ctx, "orchestrator heartbeat failed", "error", err)
			}
		case <-ctx.Done():
			return
		}
	}
}
