package docker

import (
	"context"
	"fmt"
	"io"
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
// of. What compose prints, on either stream, is kept as one, up to the last
// 16 KiB: the end of it is what says why a service did not come up.
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

// Down removes the stack's containers and networks, and its volumes when
// that is asked for too.
func (c *Compose) Down(ctx context.Context, s stack.Stack, removeVolumes bool) (string, error) {
	if removeVolumes {
		return c.run(ctx, s, "down", "--remove-orphans", "--volumes")
	}

	return c.run(ctx, s, "down", "--remove-orphans")
}

// Command is the command one compose action runs inside the VM.
func Command(project string, action ...string) []string {
	return append([]string{"docker", "compose", "-p", project, "-f", "-"}, action...)
}

func (c *Compose) run(ctx context.Context, s stack.Stack, action ...string) (string, error) {
	compose, err := Labelled(s.Spec.Compose, s.Metadata.UUID)
	if err != nil {
		return "", fmt.Errorf("its compose file cannot be read: %w", err)
	}

	session, err := c.engine.Exec(ctx, stack.VMOf(s), vm.ExecOptions{Command: Command(s.Metadata.Slug, action...)})
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

	// compose reads the file from its input, to its end. One that gave up
	// before reading all of it says why in its output and its exit code, so a
	// write it refused says nothing more.
	_, _ = io.WriteString(session.Stdin(), compose)
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
