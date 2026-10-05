// Package vmhost is the vmhost's commands: serve-workload-vmhost, which serves
// a node's engine to its orchestrator, and check-workload-vmhost, which asks
// whether it is answering.
//
// The vmhost runs in the microsandbox container, built with the microsandbox
// tag (Dockerfile target production-workload-vmhost); a build without it has
// no engine, and serve-workload-vmhost says so and exits.
package vmhost

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/danceable/console"
	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers/core"
	vmhostProviders "github.com/khanzadimahdi/testproject/infrastructure/ioc/providers/workload/vmhost"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/microsandbox"
	vmhostAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/vmhost"
)

const (
	serveName = "serve-workload-vmhost"

	// socketGroup is who may connect to the socket besides the vmhost: the
	// group the orchestrator runs in, which is the vmhost's own.
	socketGroup = 10001

	// socketMode lets the vmhost and its group connect, and nobody else.
	socketMode fs.FileMode = 0o660

	// readHeaderTimeout bounds reading a request's headers. Nothing else is
	// bounded: an archive takes as long as a disk takes to stream, and an
	// exec session lasts as long as it is open.
	readHeaderTimeout = 10 * time.Second

	// idleTimeout is how long a connection is kept with nothing on it. It is
	// longer than the client's own, so the client is always the side that
	// lets one go.
	idleTimeout = 90 * time.Second

	// staleWait bounds finding out whether a socket left behind is still
	// served.
	staleWait = time.Second
)

// ServeCommand serves a node's engine to its orchestrator on a unix socket,
// until it is told to stop: SIGTERM, which is what docker stop sends, or an
// interrupt. It then stops taking requests and stops every VM gracefully, so
// none loses what its guest had not yet written, within ShutdownTimeout.
type ServeCommand struct {
	configs *configs.WorkloadVMHost

	engine microsandbox.Engine
	server *vmhostAPI.Server
	logger *slog.Logger

	// group is the group the socket is given to.
	group int

	// signals end the command, besides its context ending.
	signals []os.Signal
}

var (
	_ console.Command   = &ServeCommand{}
	_ console.Service   = &ServeCommand{}
	_ provider.Provider = &ServeCommand{}
)

func NewServeCommand() *ServeCommand {
	return &ServeCommand{
		configs: configs.NewWorkloadVMHost(),
		group:   socketGroup,
		signals: []os.Signal{syscall.SIGTERM},
	}
}

// Name returns the name of the command which is used to identify it.
func (c *ServeCommand) Name() string {
	return serveName
}

// Description returns a short string (less than one line) describing the command.
func (c *ServeCommand) Description() string {
	return "serves a node's engine to its orchestrator, on a unix socket."
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

// Providers are what a vmhost is made of, and nothing that wires another
// service: what this command imports decides when its image is rebuilt.
func (c *ServeCommand) Providers() []provider.Provider {
	return []provider.Provider{
		core.NewConfigsProvider(c.configs),
		core.NewOpenTelemetryProvider("workload-vmhost", "workload-vmhost"),
		core.NewProfilerProvider("workload-vmhost"),
		vmhostProviders.NewProvider(),
		c,
	}
}

// Register registers the command's own dependencies, of which it has none.
func (c *ServeCommand) Register(ctx context.Context, container provider.Container) error {
	return nil
}

// Boot resolves the engine, the API that serves it, and the logger.
func (c *ServeCommand) Boot(ctx context.Context, container provider.Container) error {
	if err := container.Resolve(&c.engine); err != nil {
		return err
	}

	if err := container.Resolve(&c.server); err != nil {
		return err
	}

	return container.Resolve(&c.logger, provider.ResolveName(vmhostProviders.Logger))
}

// Terminate terminates the command's own resources, of which it has none.
func (c *ServeCommand) Terminate(ctx context.Context) error {
	return nil
}

func (c *ServeCommand) Run(ctx context.Context) console.ExitStatus {
	// listed, never left empty: no signals at all would be every signal,
	// the runtime's own among them.
	if len(c.signals) > 0 {
		var stop context.CancelFunc
		ctx, stop = signal.NotifyContext(ctx, c.signals...)
		defer stop()
	}

	listener, err := listen(c.configs.Socket, c.group)
	if err != nil {
		c.logger.ErrorContext(ctx, "the engine cannot be served", "socket", c.configs.Socket, "error", err)

		// the engine may hold VMs it took over at start: they are stopped
		// rather than killed with the container.
		c.shutdown(context.WithoutCancel(ctx))

		return console.ExitFailure
	}

	server := &http.Server{
		Handler:           c.server,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
	}

	served := make(chan error, 1)
	go func() {
		served <- server.Serve(listener)
	}()

	c.logger.InfoContext(ctx, "serving the engine", "socket", c.configs.Socket)

	status := console.ExitSuccess

	select {
	case <-ctx.Done():
	case err := <-served:
		c.logger.ErrorContext(ctx, "the engine stopped being served", "error", err)
		status = console.ExitFailure
	}

	c.stop(context.WithoutCancel(ctx), server, listener)

	return status
}

// stop stops taking requests and then stops every VM, within
// ShutdownTimeout.
//
// The socket goes first, so the orchestrator finds its vmhost gone rather
// than half there. The requests being answered are let finish meanwhile —
// stopping the VMs ends those that wait on one — and the exec sessions, which
// the http.Server does not know of, are ended with the VMs they run in.
func (c *ServeCommand) stop(ctx context.Context, server *http.Server, listener net.Listener) {
	ctx, cancel := context.WithTimeout(ctx, vmhostProviders.ShutdownTimeout)
	defer cancel()

	c.logger.InfoContext(ctx, "stopping: no more requests, then every vm")

	_ = listener.Close()

	answered := make(chan error, 1)
	go func() {
		answered <- server.Shutdown(ctx)
	}()

	c.shutdown(ctx)
	_ = c.server.Close()

	if err := <-answered; err != nil {
		c.logger.WarnContext(ctx, "requests were still being answered once every vm had stopped", "error", err)
		_ = server.Close()
	}

	c.logger.InfoContext(ctx, "stopped")
}

// shutdown stops every VM the engine holds.
func (c *ServeCommand) shutdown(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, vmhostProviders.ShutdownTimeout)
	defer cancel()

	started := time.Now()

	if err := c.engine.Shutdown(ctx); err != nil {
		c.logger.ErrorContext(ctx, "not every vm stopped gracefully", "error", err, "took", time.Since(started))

		return
	}

	c.logger.InfoContext(ctx, "every vm stopped", "took", time.Since(started))
}

// listen serves the socket at path, given to group with socketMode. A socket
// a vmhost left behind is taken over; one another vmhost still serves is not,
// and neither is anything at path that is not a socket.
func listen(path string, group int) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o770); err != nil {
		return nil, err
	}

	if err := removeStale(path); err != nil {
		return nil, err
	}

	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}

	if err := os.Chmod(path, socketMode); err != nil {
		_ = listener.Close()

		return nil, err
	}

	if err := os.Chown(path, -1, group); err != nil {
		_ = listener.Close()

		return nil, fmt.Errorf("the socket cannot be given to group %d, which its orchestrator connects as: %w", group, err)
	}

	return listener, nil
}

// removeStale removes the socket a vmhost that is gone left at path.
func removeStale(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	if info.Mode().Type() != fs.ModeSocket {
		return fmt.Errorf("%s is there and is not a socket", path)
	}

	if conn, err := net.DialTimeout("unix", path, staleWait); err == nil {
		_ = conn.Close()

		return fmt.Errorf("another vmhost is serving %s", path)
	}

	return os.Remove(path)
}
