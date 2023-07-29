package console

import (
	"context"
	"reflect"
)

type Command interface {
	Run(context.Context) int
}

type Console struct {
	command Command
}

func NewConsole() *Console {
	return &Console{}
}

func (c *Console) Register(command Command) {
	c.command = command
}

func (c *Console) Run(ctx context.Context) int {
	if c.command == nil || reflect.ValueOf(c.command).IsNil() {
		return 0
	}

	return c.command.Run(ctx)
}
