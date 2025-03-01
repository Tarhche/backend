package main

import (
	"context"
	"os"
	"os/signal"
	"path"
	"syscall"

	"github.com/khanzadimahdi/testproject/infrastructure/console"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()

	container, wait := NewContainer(ctx)

	c := console.NewConsole(path.Base(os.Args[0]), "Application description", os.Stderr)

	c.Register(Blog(container))
	c.Register(RunnerMannager(container))
	c.Register(RunnerWorker(container))

	code := c.Run(ctx, os.Args)

	// waiting for parallel jobs/tasks/processes to gracefully shutdown
	wait()

	cancel()
	os.Exit(code)
}
