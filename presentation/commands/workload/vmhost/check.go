package vmhost

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/danceable/console"

	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/vmhost"
)

const (
	checkName = "check-workload-vmhost"

	// checkTimeout bounds the question, well within a healthcheck's own
	// timeout.
	checkTimeout = 4 * time.Second
)

// CheckCommand asks a vmhost whether it is answering, which is what its
// container's healthcheck runs: the socket answering GET /v1/info, as its
// orchestrator first asks it. It needs no shell, curl or wget in the image,
// only the binary that serves the socket.
type CheckCommand struct {
	socket string

	out io.Writer
	err io.Writer
}

var _ console.Command = &CheckCommand{}

func NewCheckCommand() *CheckCommand {
	return &CheckCommand{
		socket: configs.NewWorkloadVMHost().Socket,
		out:    os.Stdout,
		err:    os.Stderr,
	}
}

func (c *CheckCommand) Name() string {
	return checkName
}

func (c *CheckCommand) Description() string {
	return "says whether a vmhost answers on its socket."
}

func (c *CheckCommand) Usage() string {
	return fmt.Sprintf("%s [--socket=path]", checkName)
}

func (c *CheckCommand) Configure(flagSet *console.FlagSet) {
	console.Var(flagSet, &c.socket, console.Long("socket"), "Unix socket the vmhost serves its engine on.", console.Env("WORKLOAD_VMHOST_SOCKET"))
}

func (c *CheckCommand) Run(ctx context.Context) console.ExitStatus {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	info, err := vmhost.NewClient(c.socket).Info(ctx)
	if err != nil {
		fmt.Fprintf(c.err, "%s: %v\n", checkName, err)

		return console.ExitFailure
	}

	fmt.Fprintf(c.out, "%s %s answers on %s\n", info.Engine, info.Version, c.socket)

	return console.ExitSuccess
}
