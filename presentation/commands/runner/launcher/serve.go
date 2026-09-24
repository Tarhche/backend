//go:build linux

// Package launcher is the command the host's launcher runs as.
package launcher

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/danceable/console"
	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers/runner"
)

const serveName string = "serve-runner-launcher"

// ServeCommand runs the launcher: the part of the runner that starts machines'
// processes and plugs them into their networks, in a container of its own that
// holds no privilege on the host. Nothing reaches it but the orchestrators,
// through a unix socket made for them alone.
type ServeCommand struct {
	configs *configs.RunnerLauncher
	handler http.Handler
	health  http.Handler
	logger  *slog.Logger
}

var (
	_ console.Command   = &ServeCommand{}
	_ console.Service   = &ServeCommand{}
	_ provider.Provider = &ServeCommand{}
)

func NewServeCommand() *ServeCommand {
	return &ServeCommand{configs: configs.NewRunnerLauncher()}
}

// Name returns the name of the command which is used to identify it.
func (c *ServeCommand) Name() string {
	return serveName
}

// Description returns a short string (less than one line) describing the command.
func (c *ServeCommand) Description() string {
	return "starts the host's microVMs, and plugs them into its networks."
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

// Providers returns the service providers required to serve the launcher.
func (c *ServeCommand) Providers() []provider.Provider {
	return []provider.Provider{
		providers.NewConfigsProvider(c.configs),
		providers.NewOpenTelemetryProvider("runner-launcher", "runner-launcher"),
		providers.NewTranslationProvider(),
		providers.NewValidationProvider(),
		providers.NewContainerProvider(),
		runner.NewLauncherProvider(),
		c,
	}
}

// Register registers the command's own dependencies, of which it has none.
func (c *ServeCommand) Register(ctx context.Context, task provider.Container) error {
	return nil
}

// Boot resolves the command's dependencies from the booted task.
func (c *ServeCommand) Boot(ctx context.Context, task provider.Container) error {
	if err := task.Resolve(&c.handler, provider.ResolveName(runner.LauncherHandler)); err != nil {
		return err
	}

	if err := task.Resolve(&c.health, provider.ResolveName(runner.LauncherHealth)); err != nil {
		return err
	}

	return task.Resolve(&c.logger, provider.WithParams("runner-launcher"))
}

// Terminate terminates the command's own resources, of which it has none. The
// providers it returned are terminated by the manager.
func (c *ServeCommand) Terminate(ctx context.Context) error {
	return nil
}

// Run takes orders on the launcher's socket until ctx is done. The machines it
// started are its container's, and end with it; their orchestrators start them
// again once a launcher is back.
func (c *ServeCommand) Run(ctx context.Context) console.ExitStatus {
	listener, err := c.listen()
	if err != nil {
		c.logger.ErrorContext(ctx, "the launcher cannot take orders", "error", err)

		return console.ExitFailure
	}

	server := &http.Server{Handler: c.handler, ReadHeaderTimeout: 10 * time.Second}

	// the healthcheck is on loopback, where the container's own healthcheck
	// can reach it and nothing else can.
	health := &http.Server{
		Addr:              net.JoinHostPort("127.0.0.1", strconv.Itoa(c.configs.HealthPort)),
		Handler:           c.health,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := health.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			c.logger.ErrorContext(ctx, "the launcher's healthcheck failed", "error", err)
		}
	}()

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = health.Shutdown(shutdownCtx)
		_ = server.Shutdown(shutdownCtx)
	}()

	c.logger.InfoContext(ctx, "the launcher is taking orders", "socket", c.configs.Socket)

	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		c.logger.ErrorContext(ctx, "the launcher failed", "error", err)

		return console.ExitFailure
	}

	return console.ExitSuccess
}

// listen makes the launcher's socket, for the orchestrators' uid and gid
// alone. Whoever can open it can start machines on this host, so that is all
// it is open to. A socket left by a launcher before this one is replaced.
func (c *ServeCommand) listen() (net.Listener, error) {
	if err := os.Remove(c.configs.Socket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	listener, err := net.Listen("unix", c.configs.Socket)
	if err != nil {
		return nil, err
	}

	if err := os.Chmod(c.configs.Socket, 0o660); err != nil {
		listener.Close()

		return nil, err
	}

	if err := os.Chown(c.configs.Socket, c.configs.OrchestratorUID, c.configs.OrchestratorGID); err != nil && os.Geteuid() == 0 {
		listener.Close()

		return nil, err
	}

	return listener, nil
}
