package console

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
)

// ExitStatus represents a Posix exit status that a command expects to be returned to the shell.
type ExitStatus = int

const (
	ExitSuccess    ExitStatus = 0
	ExitFailure    ExitStatus = 1
	ExitUsageError ExitStatus = 2
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

// Console represents a set of commands.
type Console struct {
	name        string // normally path.Base(os.Args[0])
	description string
	commands    map[string]Command

	errWriter io.Writer // specifies where should write errors (default: os.Stderr).
	container ioc.ServiceContainer
}

// NewConsole returns a new Console.
func NewConsole(name, description string, errWriter io.Writer, container ioc.ServiceContainer) *Console {
	return &Console{
		name:        name,
		description: description,
		commands:    make(map[string]Command),
		errWriter:   errWriter,
		container:   container,
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

	serviceProvider, providesServices := cmd.(ioc.ServiceProvider)
	if providesServices {
		if err := serviceProvider.Register(ctx, c.container); err != nil {
			return ExitFailure
		}
		defer serviceProvider.Terminate()
	}

	cmd.Configure(flagSet)
	if flagSet.Parse(arguments[2:]) != nil {
		return ExitUsageError
	}

	if providesServices {
		if err := serviceProvider.Boot(ctx, c.container); err != nil {
			return ExitFailure
		}
	}

	return cmd.Run(ctx)
}

// Explain writes a brief description of console commands.
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
