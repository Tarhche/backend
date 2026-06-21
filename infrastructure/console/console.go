package console

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/danceable/provider"
)

// ExitStatus represents a Posix exit status that a command expects to be returned to the shell.
type ExitStatus = int

const (
	// ExitSuccess is the exit status for a successful command.
	ExitSuccess ExitStatus = 0

	// ExitFailure is the exit status for a failed command.
	ExitFailure ExitStatus = 1

	// ExitUsageError is the exit status for a usage error.
	ExitUsageError ExitStatus = 2
)

const (
	// terminationDelay is the grace period before service providers are terminated.
	terminationDelay = 1 * time.Second

	// terminationDeadline is the maximum duration allowed for providers to terminate.
	terminationDeadline = 10 * time.Second
)

// Command represents a single command.
type Command interface {
	// Name returns the command's name.
	Name() string

	// Description returns a short string (less than one line) describing the command.
	Description() string

	// Usage returns a long string explaining the command and giving usage information.
	Usage() string

	// Configure configures this command.
	Configure(*flag.FlagSet)

	// Run attempts to run the command.
	Run(context.Context) ExitStatus
}

// Service is an optional interface that a Command can implement to provide
// service providers whose lifecycle (register, boot and terminate) is managed
// by the danceable service provider manager. Boot is called once all providers
// have been booted, allowing the command to resolve its dependencies before Run.
type Service interface {
	// Providers returns the service providers required by the command.
	Providers() []provider.Provider

	// Boot resolves the command's dependencies from the booted container.
	Boot(ctx context.Context, container provider.Container) error
}

// Console represents a set of commands.
type Console struct {
	name        string // normally path.Base(os.Args[0])
	description string
	commands    map[string]Command

	errWriter io.Writer // specifies where should write errors (default: os.Stderr).
	manager   *provider.Manager
}

// NewConsole returns a new Console.
func NewConsole(name, description string, errWriter io.Writer, manager *provider.Manager) *Console {
	return &Console{
		name:        name,
		description: description,
		commands:    make(map[string]Command),
		errWriter:   errWriter,
		manager:     manager,
	}
}

// Register registers a command.
func (c *Console) Register(command Command) {
	c.commands[command.Name()] = command
}

// Run attempts to invoke registered commands.
func (c *Console) Run(ctx context.Context, arguments []string) ExitStatus {
	if len(arguments) < 2 {
		c.explain()
		return ExitUsageError
	}

	cmdName := arguments[1]

	cmd, ok := c.commands[cmdName]
	if !ok {
		if cmdName == "-h" || cmdName == "-help" {
			c.explain()
			return ExitSuccess
		}

		fmt.Fprintf(c.errWriter, "%q is not a command, See %q.\n", cmdName, c.name+" -help")
		return ExitUsageError
	}

	flagSet := flag.NewFlagSet(cmdName, flag.ContinueOnError)
	flagSet.SetOutput(c.errWriter)
	flagSet.Usage = func() { explain(c.errWriter, cmd) }

	inputArgs := arguments[2:]
	if cmd.Configure(flagSet); flagSet.Parse(inputArgs) != nil {
		return ExitUsageError
	}

	if service, providesServices := cmd.(Service); providesServices {
		return c.runService(ctx, cmd, service)
	}

	return cmd.Run(ctx)
}

// runService registers the command's service providers on the manager, boots
// them, runs the command and finally terminates the providers gracefully.
func (c *Console) runService(ctx context.Context, cmd Command, service Service) ExitStatus {
	for _, p := range service.Providers() {
		c.manager.Register(p)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	exitStatus := make(chan ExitStatus, 1)

	err := c.manager.Run(
		runCtx,
		provider.WithTerminationDelay(terminationDelay),
		provider.WithTerminationDeadline(terminationDeadline),
		provider.WithCallback(func(callbackCtx context.Context, container provider.Container) {
			if err := service.Boot(callbackCtx, container); err != nil {
				fmt.Fprintln(c.errWriter, err)
				exitStatus <- ExitFailure
				cancel()
				return
			}

			exitStatus <- cmd.Run(callbackCtx)
			cancel()
		}),
	)
	if err != nil {
		fmt.Fprintln(c.errWriter, err)
		return ExitFailure
	}

	return <-exitStatus
}

// explain writes a brief description of console commands.
func (c *Console) explain() {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\nUsage:\n", c.description)
	fmt.Fprintf(&b, "\n  %s %s\n", c.name, "[flags] <command> [command arguments]")

	fmt.Fprint(&b, "\nThe commands are:\n\n")
	for _, cmd := range c.commands {
		fmt.Fprintf(&b, "  %-10s  %s\n", cmd.Name(), cmd.Description())
	}
	fmt.Fprintf(&b, "\nUse \"%s <command> -h\" for more information about a command.\n", c.name)

	fmt.Fprint(c.errWriter, b.String())
}

// explain prints a brief description of a single command.
func explain(w io.Writer, cmd Command) {
	var b strings.Builder

	fmt.Fprintf(&b, "%s\n\nUsage:\n", cmd.Description())
	fmt.Fprintf(&b, "\n  %s\n\n", cmd.Usage())

	f := flag.NewFlagSet(cmd.Name(), flag.PanicOnError)
	f.SetOutput(&b)
	cmd.Configure(f)
	f.PrintDefaults()

	fmt.Fprint(w, b.String())
}
