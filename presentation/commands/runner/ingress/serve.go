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
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers/runner"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/tunnel"
)

const (
	serveName string = "serve-runner-ingress"
)

type ServeCommand struct {
	configs *configs.RunnerIngress
	handler http.Handler

	// tunnel is where the workers connect. Requests go back down those
	// connections, so this is the only way into a worker and the only thing
	// that says a worker is there at all.
	tunnel *tunnel.Ingress

	logger *slog.Logger
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

// Providers returns the service providers required to serve the runner ingress.
// It reaches nothing: the workers come to it, so it needs neither a database
// nor messaging.
func (c *ServeCommand) Providers() []provider.Provider {
	return []provider.Provider{
		providers.NewConfigsProvider(c.configs),
		providers.NewOpenTelemetryProvider("runner-ingress", "runner-ingress"),
		providers.NewProfilerProvider("runner-ingress"),
		providers.NewContainerProvider(),
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

	if err := container.Resolve(&c.logger, provider.WithParams("runner-ingress")); err != nil {
		return err
	}

	return container.Resolve(&c.tunnel, provider.ResolveName(runner.IngressTunnel))
}

// Terminate terminates the command's own resources, of which it has none. The
// providers it returned are terminated by the manager.
func (c *ServeCommand) Terminate(ctx context.Context) error {
	return nil
}

// @title			Runner Ingress API
// @version		1.0
// @description	Swagger/OpenAPI documentation for the runner ingress service.
// @termsOfService	http://swagger.io/terms/
//
// @license.name	Apache 2.0
// @license.url	http://www.apache.org/licenses/LICENSE-2.0.html
//
// @host			0.0.0.0:80
// @basePath		/
// @schemes		http
func (c *ServeCommand) Run(ctx context.Context) console.ExitStatus {
	tunnelConfig, err := tunnel.ServerTLS(certificate.TLSFiles{
		Authority:   c.configs.TunnelAuthority,
		Certificate: c.configs.TunnelCertificate,
		PrivateKey:  c.configs.TunnelKey,
	})
	if err != nil {
		c.logger.ErrorContext(ctx, "the tunnel's certificates are unusable", "error", err)
		return console.ExitFailure
	}

	// the workers have to be able to arrive before the first request does, or
	// the ingress answers for runners it has simply not met yet.
	listener, err := tunnel.Listen(fmt.Sprintf("0.0.0.0:%d", c.configs.TunnelPort), tunnelConfig)
	if err != nil {
		c.logger.ErrorContext(ctx, "the tunnel could not listen", "error", err)
		return console.ExitFailure
	}

	go func() {
		if err := c.tunnel.Serve(ctx, listener); err != nil {
			c.logger.ErrorContext(ctx, "the tunnel failed", "error", err)
		}
	}()

	// no read timeout: what this carries is a runner's own traffic, which may
	// be an attached terminal or a log being followed rather than a request
	// that finishes.
	server := http.Server{
		Addr:        fmt.Sprintf("0.0.0.0:%d", c.configs.Port),
		Handler:     c.handler,
		IdleTimeout: 120 * time.Second,
	}

	go func() {
		<-ctx.Done()

		// Shutdown the server after getting a signal with a timeout to ensure graceful shutdown.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = server.Shutdown(shutdownCtx)
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		c.logger.ErrorContext(ctx, "server failed", "error", err)
		return console.ExitFailure
	}

	return console.ExitSuccess
}
