package docker

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Compose runs docker compose inside one Docker VM.
//
// It is compose itself rather than something that reads compose files the
// way it does, so a stack behaves exactly as compose would make it: the
// command is `docker compose -p <project> -f - <action>` exec'd into the VM,
// with the YAML on its standard input. The project is the stack's slug, which
// is how its containers are found again by compose's own labels. What compose
// prints, on either stream, is kept as one, up to the last 16 KiB: the end of
// it is what says why a service did not come up.
type Compose struct {
	engine vm.Engine
	vmUUID string
}

var _ docker.Compose = &Compose{}

func NewCompose(engine vm.Engine, vmUUID string) *Compose {
	return &Compose{engine: engine, vmUUID: vmUUID}
}

// Up creates and starts what the project needs, in the background, and
// removes what it no longer has.
func (c *Compose) Up(ctx context.Context, project string, compose string) (string, error) {
	return c.run(ctx, project, compose, "up", "-d", "--remove-orphans")
}

func (c *Compose) Start(ctx context.Context, project string, compose string) (string, error) {
	return c.run(ctx, project, compose, "start")
}

func (c *Compose) Stop(ctx context.Context, project string, compose string) (string, error) {
	return c.run(ctx, project, compose, "stop")
}

func (c *Compose) Restart(ctx context.Context, project string, compose string) (string, error) {
	return c.run(ctx, project, compose, "restart")
}

// Down removes the project's containers and networks, and its volumes when
// that is asked for too.
func (c *Compose) Down(ctx context.Context, project string, compose string, removeVolumes bool) (string, error) {
	if removeVolumes {
		return c.run(ctx, project, compose, "down", "--remove-orphans", "--volumes")
	}

	return c.run(ctx, project, compose, "down", "--remove-orphans")
}

// Command is the command one compose action runs inside the VM.
func Command(project string, action ...string) []string {
	return append([]string{"docker", "compose", "-p", project, "-f", "-"}, action...)
}

func (c *Compose) run(ctx context.Context, project string, compose string, action ...string) (string, error) {
	session, err := c.engine.Exec(ctx, c.vmUUID, vm.ExecOptions{Command: Command(project, action...)})
	if err != nil {
		return "", err
	}
	defer session.Close()

	// giving up ends compose, rather than leaving it to run on unwatched.
	stop := context.AfterFunc(ctx, func() { _ = session.Close() })
	defer stop()

	output := newTailBuffer(stack.MaxOutput)

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
