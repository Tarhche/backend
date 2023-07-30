package main

import (
	"context"
	"os"
	"os/signal"
	"path"
	"syscall"

	"github.com/khanzadimahdi/testproject.git/infrastructure/console"
	"github.com/khanzadimahdi/testproject.git/presentation/commands"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()

	console := console.NewConsole(path.Base(os.Args[0]), "Application description", os.Stderr)
	console.Register(commands.NewServeCommand())
	code := console.Run(ctx, os.Args)

	cancel()
	os.Exit(code)
}
