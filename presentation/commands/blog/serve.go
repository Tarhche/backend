package blog

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/console"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers"
)

const (
	serveName    string = "serve-blog"
	consumerName string = "blog"
)

type ServeCommand struct {
	port            int
	handler         http.Handler
	subscriber      domain.Subscriber
	requester       domain.Requester
	subscribers     map[string]domain.MessageHandler
	requestReplyers map[string]domain.Replyer
	serviceProvider ioc.ServiceProvider
}

// insures it implements console.Command
var _ console.Command = &ServeCommand{}

// insures it implements ioc.ServiceProvider
var _ ioc.ServiceProvider = &ServeCommand{}

func NewServeCommand(serviceProvider ioc.ServiceProvider) *ServeCommand {
	return &ServeCommand{
		serviceProvider: serviceProvider,
	}
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

func (c *ServeCommand) Register(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	return c.serviceProvider.Register(ctx, iocContainer)
}

func (c *ServeCommand) Boot(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	if err := c.serviceProvider.Boot(ctx, iocContainer); err != nil {
		return err
	}

	if err := iocContainer.Resolve(&c.handler, ioc.WithNameResolving(providers.BlogHandler)); err != nil {
		return err
	}

	if err := iocContainer.Resolve(&c.subscriber); err != nil {
		return err
	}

	if err := iocContainer.Resolve(&c.requester, ioc.WithNameResolving(providers.BlogRequestReplyer)); err != nil {
		return err
	}

	if err := iocContainer.Resolve(&c.subscribers, ioc.WithNameResolving(providers.BlogSubscribers)); err != nil {
		return err
	}

	if err := iocContainer.Resolve(&c.requestReplyers, ioc.WithNameResolving(providers.BlogRequestReplyers)); err != nil {
		return err
	}

	return nil
}

func (c *ServeCommand) Terminate() error {
	return c.serviceProvider.Terminate()
}

// @title			Backend API
// @version		1.0
// @description	Swagger/OpenAPI documentation for the backend service.
// @termsOfService	http://swagger.io/terms/
//
// @license.name	Apache 2.0
// @license.url	http://www.apache.org/licenses/LICENSE-2.0.html
//
// @basePath		/api
// @schemes		http https
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

	if err := c.subscribeToTopics(ctx); err != nil {
		log.Println(err)
		return console.ExitFailure
	}

	if err := c.registerRequestReplyers(ctx); err != nil {
		log.Println(err)
		return console.ExitFailure
	}

	if err := server.ListenAndServe(); err != nil {
		log.Println(err)
		return console.ExitFailure
	}

	return console.ExitSuccess
}

func (c *ServeCommand) subscribeToTopics(ctx context.Context) error {
	for subject, messageHandler := range c.subscribers {
		if err := c.subscriber.Subscribe(ctx, consumerName, subject, messageHandler); err != nil {
			return err
		}
	}

	return nil
}

func (c *ServeCommand) registerRequestReplyers(ctx context.Context) error {
	for subject, replyer := range c.requestReplyers {
		if err := c.requester.RegisterReplyer(ctx, subject, replyer); err != nil {
			return err
		}
	}

	return nil
}
