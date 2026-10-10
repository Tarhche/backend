package docker

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Compose is the stack kind's Applier: docker compose, run inside the Docker
// VM a stack lives in.
//
// It is compose itself rather than something that reads compose files the
// way it does, so a stack behaves exactly as compose would make it: the
// command is `docker compose -p <project> -f - <action>` exec'd into the VM,
// with the YAML on its standard input. The project is the stack's slug, and
// the YAML is the stack's own with every service labelled as the stack's
// (Labelled), so that each container it makes says exactly which stack it is
// of. A stack is taken down by its project alone, with no file (Down). What
// compose prints, on either stream, is kept as one, up to the last 16 KiB:
// the end of it is what says why a service did not come up.
type Compose struct {
	engine vm.Engine
}

var _ stack.Applier = &Compose{}

// NewCompose runs compose in the VMs engine holds.
func NewCompose(engine vm.Engine) *Compose {
	return &Compose{engine: engine}
}

// Up creates and starts what the stack's file has, in the background, and
// removes what it no longer has.
func (c *Compose) Up(ctx context.Context, s stack.Stack) (string, error) {
	return c.run(ctx, s, "up", "-d", "--remove-orphans")
}

func (c *Compose) Start(ctx context.Context, s stack.Stack) (string, error) {
	return c.run(ctx, s, "start")
}

func (c *Compose) Stop(ctx context.Context, s stack.Stack) (string, error) {
	return c.run(ctx, s, "stop")
}

func (c *Compose) Restart(ctx context.Context, s stack.Stack) (string, error) {
	return c.run(ctx, s, "restart")
}

// Down removes the stack's containers and networks, and keeps its volumes.
// Compose is given the project alone, and no file: it finds what is the
// project's by the labels on its containers and networks, so neither what
// the stack's file says nor a file compose would refuse, or could not read
// at all, keeps a stack from being taken down.
func (c *Compose) Down(ctx context.Context, s stack.Stack) (string, error) {
	return c.exec(ctx, s, nil, "down", "--remove-orphans")
}

// Command is the command one compose action runs inside the VM on a project:
// on its file, which compose reads from its input, when file says so, and on
// the project alone, which compose finds by its labels, when it does not.
func Command(project string, file bool, action ...string) []string {
	command := []string{"docker", "compose", "-p", project}
	if file {
		command = append(command, "-f", "-")
	}

	return append(command, action...)
}

// run runs one compose action on the stack's file, labelled as the stack's.
func (c *Compose) run(ctx context.Context, s stack.Stack, action ...string) (string, error) {
	compose, err := Labelled(s.Spec.Compose, s.Metadata.UUID)
	if err != nil {
		return "", fmt.Errorf("its compose file cannot be read: %w", err)
	}

	return c.exec(ctx, s, strings.NewReader(compose), action...)
}

// exec runs one compose action inside the stack's VM, on file, which compose
// reads from its input, or on the project alone when there is none, and is
// what compose printed.
func (c *Compose) exec(ctx context.Context, s stack.Stack, file io.Reader, action ...string) (string, error) {
	session, err := c.engine.Exec(ctx, stack.VMOf(s), vm.ExecOptions{Command: Command(s.Metadata.Slug, file != nil, action...)})
	if err != nil {
		return "", err
	}
	defer session.Close()

	// giving up ends compose, rather than leaving it to run on unwatched.
	stop := context.AfterFunc(ctx, func() { _ = session.Close() })
	defer stop()

	output := newTailBuffer(kind.MaxOutput)

	var read sync.WaitGroup
	read.Go(func() { _, _ = io.Copy(output, session.Stdout()) })
	read.Go(func() { _, _ = io.Copy(output, session.Stderr()) })

	// compose reads the file from its input, to its end, and finds its input
	// ended at once when it is given none. One that gave up before reading
	// all of it says why in its output and its exit code, so a write it
	// refused says nothing more.
	if file != nil {
		_, _ = io.Copy(session.Stdin(), file)
	}

	_ = session.Stdin().Close()

	read.Wait()

	exitCode, err := session.Wait(ctx)
	if err != nil {
		return output.String(), err
	}

	if exitCode != 0 {
		return output.String(), fmt.Errorf("docker compose %s exited with %d", action[0], exitCode)
	}

	return output.String(), nil
}
