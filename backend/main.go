package main

import (
	"context"
	"os"
	"os/signal"
	"path"
	"syscall"

	"github.com/khanzadimahdi/testproject/infrastructure/console"
	"github.com/khanzadimahdi/testproject/presentation/commands"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()

	handler, wait := App(ctx)

	c := console.NewConsole(path.Base(os.Args[0]), "Application description", os.Stderr)
	c.Register(commands.NewServeCommand(handler))
	code := c.Run(ctx, os.Args)

	// waiting for parallel jobs/tasks/processes to gracefully shutdown
	wait()

	cancel()
	os.Exit(code)
}
