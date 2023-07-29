package commands

import (
	"context"

	"github.com/khanzadimahdi/testproject.git/infrastructure/console"
)

type ServeCommand struct {
}

// insures it implements console.Command
var _ console.Command = NewServeCommand()

func NewServeCommand() *ServeCommand {
	return &ServeCommand{}
}

func (c *ServeCommand) Name() string {
	return "serve"
}

func (c *ServeCommand) Configure() {
	//
}

func (c *ServeCommand) Run(ctx context.Context) console.ExitStatus {
	return 0
}
