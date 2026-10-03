// Package vmhost is the command vmhost runs as: serve-workload-vmhost, the
// privileged daemon on a KVM host that runs the workload's microVMs for the
// orchestrator on the same host, as dockerd runs containers.
package vmhost

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/danceable/console"
	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost/reconcile"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers/workload"
)

const (
	serveName = "serve-workload-vmhost"

	// reconcileInterval is how often vmhost holds what it knows against what
	// the host holds: a machine that went away, a firewall something
	// flushed, images past their budget.
	reconcileInterval = 10 * time.Second

	// shutdownTimeout bounds answering the requests in flight once vmhost is
	// told to stop. Streams are not waited for: they end with vmhost, and
	// whoever followed them asks again of the next one.
	shutdownTimeout = 5 * time.Second
)

// ServeCommand runs vmhost: it takes back the VMs an earlier vmhost left
// running, and takes requests for them on a unix socket that only the
// orchestrators' group can open.
type ServeCommand struct {
	configs   *configs.WorkloadVMHost
	handler   http.Handler
	reconcile *reconcile.UseCase
	logger    *slog.Logger
}

var (
	_ console.Command   = &ServeCommand{}
	_ console.Service   = &ServeCommand{}
	_ provider.Provider = &ServeCommand{}
)

func NewServeCommand() *ServeCommand {
	return &ServeCommand{configs: configs.NewWorkloadVMHost()}
}

// Name returns the name of the command which is used to identify it.
func (c *ServeCommand) Name() string {
	return serveName
}

// Description returns a short string (less than one line) describing the command.
func (c *ServeCommand) Description() string {
	return "runs the host's microVMs, and takes requests for them on a unix socket."
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

// Providers returns the service providers required to serve vmhost.
func (c *ServeCommand) Providers() []provider.Provider {
	return []provider.Provider{
		providers.NewConfigsProvider(c.configs),
		providers.NewOpenTelemetryProvider("workload-vmhost", "workload-vmhost"),
		providers.NewTranslationProvider(),
		providers.NewValidationProvider(),
		providers.NewContainerProvider(),
		workload.NewVMHostProvider(),
		c,
	}
}

// Register registers the command's own dependencies, of which it has none.
func (c *ServeCommand) Register(ctx context.Context, task provider.Container) error {
	return nil
}

// Boot resolves the command's dependencies from the booted task.
func (c *ServeCommand) Boot(ctx context.Context, task provider.Container) error {
	if err := task.Resolve(&c.handler, provider.ResolveName(workload.VMHostHandler)); err != nil {
		return err
	}

	if err := task.Resolve(&c.reconcile); err != nil {
		return err
	}

	return task.Resolve(&c.logger, provider.WithParams(workload.VMHostLogger))
}

// Terminate terminates the command's own resources, of which it has none. The
// providers it returned are terminated by the manager: vmhost's lets go of the
// VMs, which keep running.
func (c *ServeCommand) Terminate(ctx context.Context) error {
	return nil
}

// Run takes back the VMs an earlier vmhost left running, and then takes
// requests on vmhost's socket until ctx is done, holding what it knows against
// what the host holds every few seconds meanwhile. The VMs are the host's, and
// keep running when vmhost stops.
func (c *ServeCommand) Run(ctx context.Context) console.ExitStatus {
	if c.configs.MaxMemory == 0 {
		c.logger.ErrorContext(ctx, "WORKLOAD_VMHOST_MAX_MEMORY has to be set: it is the memory, in bytes, every VM together may be given")

		return console.ExitFailure
	}

	socket, err := c.configs.SocketPath()
	if err != nil {
		c.logger.ErrorContext(ctx, "vmhost cannot take requests", "error", err)

		return console.ExitFailure
	}

	// before any request: a request must never find a VM that vmhost has not
	// taken back yet. One that cannot be done now is done again shortly, and
	// vmhost says it is unhealthy meanwhile.
	if err := c.reconcile.Execute(ctx); err != nil {
		c.logger.WarnContext(ctx, "vmhost cannot run vms yet", "error", err)
	}

	listener, err := listen(socket, c.configs.SocketGroup)
	if err != nil {
		c.logger.ErrorContext(ctx, "vmhost cannot take requests", "socket", socket, "error", err)

		return console.ExitFailure
	}

	// no write timeout: a VM's output is followed, and a terminal is kept
	// open, for as long as they last.
	server := &http.Server{
		Handler:           c.handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	reconciled := make(chan struct{})
	go func() {
		defer close(reconciled)
		c.reconcileUntilDone(ctx)
	}()

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		_ = server.Shutdown(shutdownCtx)
	}()

	c.logger.InfoContext(ctx, "vmhost is taking requests", "socket", socket, "process_mode", c.configs.ProcessMode)

	err = server.Serve(listener)
	<-reconciled

	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		c.logger.ErrorContext(ctx, "vmhost failed", "error", err)

		return console.ExitFailure
	}

	return console.ExitSuccess
}

// reconcileUntilDone holds what vmhost knows against what the host holds,
// every reconcileInterval, until ctx is done.
func (c *ServeCommand) reconcileUntilDone(ctx context.Context) {
	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.reconcile.Execute(ctx); err != nil && ctx.Err() == nil {
				c.logger.WarnContext(ctx, "what vmhost knows and what the host holds could not all be put in line", "error", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

// listen makes vmhost's socket: mode 0660, for root and the orchestrators'
// group alone, since whoever can open it can run VMs on this host. A socket a
// vmhost before this one left is replaced; one another vmhost still answers on
// is not taken from it.
func listen(socket string, group int) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(socket), 0o755); err != nil {
		return nil, err
	}

	if conn, err := net.DialTimeout("unix", socket, time.Second); err == nil {
		conn.Close()

		return nil, fmt.Errorf("another vmhost is taking requests on %s", socket)
	}

	if info, err := os.Lstat(socket); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("%s is there, and is not a socket", socket)
		}

		if err := os.Remove(socket); err != nil {
			return nil, err
		}
	}

	listener, err := net.Listen("unix", socket)
	if err != nil {
		return nil, err
	}

	if err := os.Chmod(socket, 0o660); err != nil {
		listener.Close()

		return nil, err
	}

	// only root can give a file to a group it is not in; anyone else runs
	// vmhost for development, where the socket stays theirs.
	if err := os.Chown(socket, -1, group); err != nil && os.Geteuid() == 0 {
		listener.Close()

		return nil, err
	}

	return listener, nil
}
