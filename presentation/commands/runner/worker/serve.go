package worker

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/console"
)

const (
	serveName    string = "serve-runner-worker"
	consumerName string = "runner-worker"
)

type ServeCommand struct {
	port        int
	handler     http.Handler
	subscriber  domain.Subscriber
	subscribers map[string]domain.MessageHandler
}

// insures it implements console.Command
var _ console.Command = &ServeCommand{}

func NewServeCommand(
	handler http.Handler,
	subscriber domain.Subscriber,
	subscribers map[string]domain.MessageHandler,
) *ServeCommand {
	return &ServeCommand{
		handler:     handler,
		subscriber:  subscriber,
		subscribers: subscribers,
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

func (c *ServeCommand) Run(ctx context.Context) console.ExitStatus {
	server := http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", c.port),
		Handler: c.handler,
	}

	go func() {
		<-ctx.Done()

		_ = server.Shutdown(context.Background())
	}()

	for subject, messageHandler := range c.subscribers {
		if err := c.subscriber.Subscribe(ctx, consumerName, subject, messageHandler); err != nil {
			log.Println(err)
			return console.ExitFailure
		}
	}

	if err := server.ListenAndServe(); err != nil {
		log.Println(err)
		return console.ExitFailure
	}

	return console.ExitSuccess
}
