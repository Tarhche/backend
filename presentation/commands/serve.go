package commands

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/khanzadimahdi/testproject/infrastructure/console"
)

const (
	serveName string = "serve"
)

type ServeCommand struct {
	port    int
	handler http.Handler
}

// insures it implements console.Command
var _ console.Command = NewServeCommand(nil)

func NewServeCommand(handler http.Handler) *ServeCommand {
	return &ServeCommand{
		handler: handler,
	}
}

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

	if err := server.ListenAndServe(); err != nil {
		log.Println(err)
		return console.ExitFailure
	}

	return console.ExitSuccess
}
