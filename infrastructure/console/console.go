package console

import (
	"context"
)

// ExitStatus represents a Posix exit status that a command
// expects to be returned to the shell.
type ExitStatus = int

const (
	ExitSuccess    ExitStatus = 0
	ExitFailure    ExitStatus = 1
	ExitUsageError            = 2
)

// Command represents a single command.
type Command interface {
	// Name returns the command's name
	Name() string

	// Configure configures this command.
	Configure()

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
func (c *Console) Run(ctx context.Context, arguments []string) ExitStatus {
	if len(arguments) == 0 {
		return ExitUsageError
	}

	if len(c.commands) == 0 {
		return ExitSuccess
	}

	commandName := arguments[0]

	var status int
	for _, cmd := range c.commands {
		if cmd.Name() != commandName {
			continue
		}

		cmd.Configure()

		if status = cmd.Run(ctx); status != ExitSuccess {
			break
		}
	}

	return status
}
