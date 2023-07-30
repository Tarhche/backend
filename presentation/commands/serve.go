package commands

import (
	"context"
	"flag"
	"fmt"

	"github.com/khanzadimahdi/testproject.git/infrastructure/console"
)

const (
	serveName string = "serve"
)

type ServeCommand struct {
	port int
}

// insures it implements console.Command
var _ console.Command = NewServeCommand()

func NewServeCommand() *ServeCommand {
	return &ServeCommand{}
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
	// if err := http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", c.port), articles.NewArticlesMux()); err != nil {
	// 	log.Println(err)
	// 	return 1
	// }

	return 0
}
