package console

import (
	"context"
)

type Command interface {
	Run(context.Context) int
}

type Console struct {
	commands []Command
}

func NewConsole() *Console {
	return &Console{}
}

func (c *Console) Register(command Command) {
	c.commands = append(c.commands, command)
}

func (c *Console) Run(ctx context.Context) int {
	if len(c.commands) == 0 {
		return 0
	}

	var exitCode int
	for i := range c.commands {
		exitCode = c.commands[i].Run(ctx)
		if exitCode != 0 {
			break
		}
	}

	return exitCode
}
