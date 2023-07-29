package console

import (
	"context"
)

// ExitStatus represents a Posix exit status that a command
// expects to be returned to the shell.
type ExitStatus = int

const (
	ExitSuccess ExitStatus = 0
	ExitFailure ExitStatus = 1
)

// Command represents a single command.
type Command interface {
	// Run attems to run the command
	Run(context.Context) ExitStatus
}

// Console represents a set of commands.
type Console struct {
	commands []Command
}

// NewConsole returns a new Console.
func NewConsole() *Console {
	return &Console{}
}

// Register registers a command.
func (c *Console) Register(command Command) {
	c.commands = append(c.commands, command)
}

// Run attempts to invoke registered commands.
func (c *Console) Run(ctx context.Context) ExitStatus {
	if len(c.commands) == 0 {
		return ExitSuccess
	}

	var status int
	for i := range c.commands {
		if status = c.commands[i].Run(ctx); status != ExitSuccess {
			break
		}
	}

	return status
}
