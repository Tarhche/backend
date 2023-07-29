package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/khanzadimahdi/testproject.git/infrastructure/console"
	"github.com/khanzadimahdi/testproject.git/presentation/commands"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()

	console := console.NewConsole()
	console.Register(commands.NewServeCommand())
	console.Run(ctx, os.Args[1:])
}
