package commands

import (
	"context"

	"github.com/khanzadimahdi/testproject.git/infrastructure/console"
)

type ServeCommand struct {
}

func NewServeCommand() *ServeCommand {
	return &ServeCommand{}
}

func (c *ServeCommand) Name() string {
	return "serve"
}

func (c *ServeCommand) Run(ctx context.Context) console.ExitStatus {
	return 0
}
