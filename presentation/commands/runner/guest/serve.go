//go:build linux

// Package guest is the command a microVM's init runs as.
package guest

import (
	"context"
	"log/slog"
	"os"

	"github.com/danceable/console"

	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/guest/agent"
)

const serveName = "serve-runner-guest"

// ServeCommand runs a machine's agent. The kernel starts it as init, with this
// command's name as its argument.
type ServeCommand struct {
	logger *slog.Logger
}

var _ console.Command = &ServeCommand{}

func NewServeCommand() *ServeCommand {
	return &ServeCommand{
		// what the agent says goes to the machine's console, which the host
		// keeps next to the machine.
		logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}
}

// Name returns the name of the command which is used to identify it.
func (c *ServeCommand) Name() string {
	return serveName
}

// Description returns a short string (less than one line) describing the command.
func (c *ServeCommand) Description() string {
	return "serves as a microVM's init."
}

// Usage returns a long string explaining the command and giving usage
// information.
func (c *ServeCommand) Usage() string {
	return serveName
}

// Configure defines this command's flags, of which it has none: a machine is
// told what it is over its vsock, once it is up.
func (c *ServeCommand) Configure(flagSet *console.FlagSet) {}

// Run runs the agent until the machine is turned off. An agent that cannot
// run leaves nothing to run the machine for, so it turns the machine off
// rather than leaving it up with nobody in it.
func (c *ServeCommand) Run(ctx context.Context) console.ExitStatus {
	if err := agent.New(c.logger).Run(ctx); err != nil {
		c.logger.Error("the agent could not run", "error", err)
		agent.Halt()

		return console.ExitFailure
	}

	return console.ExitSuccess
}
