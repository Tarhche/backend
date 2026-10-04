//go:build microsandbox

// Package microsandbox is the serve-workload-microsandbox command, which runs
// the workload-microsandbox service: microsandbox's VMs, behind the API the
// orchestrators reach them through.
//
// It is compiled only with the microsandbox tag, because the service drives
// microsandbox through its SDK, which is cgo. Only the workload-microsandbox
// image is built with it; every other build leaves this command out, and
// stays static.
package microsandbox

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/danceable/console"
	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers/workload"
)

const (
	serveName = "serve-workload-microsandbox"

	// healthAddress is where the plain healthcheck is served: on loopback,
	// for the container's own healthcheck and nothing else.
	healthAddress = "127.0.0.1:8081"

	// shutdownTimeout bounds stopping every run as the service goes away.
	// Each is given its stop grace, in parallel, and compose gives the whole
	// container sixty seconds before it kills it.
	shutdownTimeout = 45 * time.Second

	// serverShutdownTimeout bounds letting the requests still in flight
	// finish, once every run has been stopped.
	serverShutdownTimeout = 5 * time.Second
)

type ServeCommand struct {
	configs *configs.WorkloadMicrosandbox

	// sandboxes is the provider that binds runs.Sandboxes: microsandbox's
	// SDK in the image, and a fake in tests.
	sandboxes provider.Provider

	handler    http.Handler
	health     http.Handler
	supervisor *runs.Supervisor
	logger     *slog.Logger

	healthAddress string
}

var (
	_ console.Command   = &ServeCommand{}
	_ console.Service   = &ServeCommand{}
	_ provider.Provider = &ServeCommand{}
)

// NewServeCommand runs the service over the microsandbox that sandboxes binds
// as runs.Sandboxes.
func NewServeCommand(sandboxes provider.Provider) *ServeCommand {
	return &ServeCommand{
		configs:       configs.NewWorkloadMicrosandbox(),
		sandboxes:     sandboxes,
		healthAddress: healthAddress,
	}
}

// Name returns the name of the command which is used to identify it.
func (c *ServeCommand) Name() string {
	return serveName
}

// Description returns a short string (less than one line) describing the command.
func (c *ServeCommand) Description() string {
	return "serves microsandbox's VMs to the workload orchestrators."
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

// Providers returns the service providers required to serve the workload's
// microVMs. The one that binds microsandbox resolves the service's logger, so
// it comes after the OpenTelemetry provider, and before the service's own,
// which resolves microsandbox.
func (c *ServeCommand) Providers() []provider.Provider {
	return []provider.Provider{
		providers.NewConfigsProvider(c.configs),
		providers.NewOpenTelemetryProvider("workload-microsandbox", "workload-microsandbox"),
		providers.NewProfilerProvider("workload-microsandbox"),
		providers.NewContainerProvider(),
		c.sandboxes,
		workload.NewMicrosandboxProvider(),
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

	if err := task.Resolve(&c.health, provider.ResolveName(workload.MicrosandboxHealth)); err != nil {
		return err
	}

	if err := task.Resolve(&c.supervisor); err != nil {
		return err
	}

	return task.Resolve(&c.logger, provider.WithParams("workload-microsandbox"))
}

// Terminate terminates the command's own resources, of which it has none. The
// providers it returned are terminated by the manager.
func (c *ServeCommand) Terminate(ctx context.Context) error {
	return nil
}

// Run serves the API under mutual TLS and the healthcheck on loopback, makes
// the service ready by adopting what it held before, and, once it is told to
// stop, stops every run before it goes.
//
// The runs are stopped while the API still answers, so an orchestrator that
// asks in the meantime sees its runs exit rather than a service that vanished
// mid-call. Every VM goes with the service anyway: microsandbox ends the main
// processes of a client that goes away, so a graceful stop is the better end.
func (c *ServeCommand) Run(ctx context.Context) console.ExitStatus {
	tlsConfig, err := certificate.ServerTLSConfig(certificate.Credentials{
		Authority:   c.configs.Authority,
		Certificate: c.configs.Certificate,
		PrivateKey:  c.configs.Key,
	})
	if err != nil {
		c.logger.ErrorContext(ctx, "the service's certificates are unusable", "error", err)

		return console.ExitFailure
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", c.configs.Port))
	if err != nil {
		c.logger.ErrorContext(ctx, "the API could not listen", "error", err)

		return console.ExitFailure
	}

	healthListener, err := net.Listen("tcp", c.healthAddress)
	if err != nil {
		_ = listener.Close()

		c.logger.ErrorContext(ctx, "the healthcheck could not listen", "error", err)

		return console.ExitFailure
	}

	// no read or write timeout: a followed log and an exec's websocket stay
	// open for as long as the run, or the terminal, does.
	server := &http.Server{
		Handler:           c.handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	health := &http.Server{
		Handler:           c.health,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
	}

	failed := make(chan error, 2)

	go func() {
		failed <- serve(server, tls.NewListener(listener, tlsConfig))
	}()

	go func() {
		failed <- serve(health, healthListener)
	}()

	opening, stopOpening := context.WithCancel(ctx)
	defer stopOpening()

	go func() {
		if err := c.supervisor.Open(opening); err != nil && !errors.Is(err, context.Canceled) {
			c.logger.Error("the service never became ready", "error", err)
		}
	}()

	status := console.ExitSuccess

	select {
	case <-ctx.Done():
	case err := <-failed:
		if err != nil {
			c.logger.Error("a server failed", "error", err)

			status = console.ExitFailure
		}
	}

	stopOpening()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := c.supervisor.Shutdown(shutdownCtx); err != nil {
		c.logger.Error("the runs could not all be stopped", "error", err)

		status = console.ExitFailure
	}

	serverCtx, cancelServers := context.WithTimeout(context.Background(), serverShutdownTimeout)
	defer cancelServers()

	_ = server.Shutdown(serverCtx)
	_ = health.Shutdown(serverCtx)

	return status
}

// serve serves until the server is shut down, which is no failure.
func serve(server *http.Server, listener net.Listener) error {
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}
