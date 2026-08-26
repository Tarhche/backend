package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path"

	"github.com/danceable/console"
	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/presentation/commands/blog"
	"github.com/khanzadimahdi/testproject/presentation/commands/runner/manager"
	"github.com/khanzadimahdi/testproject/presentation/commands/runner/worker"
)

//go:generate go tool swag init --generalInfo ./presentation/commands/blog/serve.go --dir ./ --output ./resources/docs/blog/openapi
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	c := console.NewConsole(
		path.Base(os.Args[0]),
		"Application description",
		os.Stdout,
		os.Stderr,
		provider.Default,
	)

	// the settings every command reads are parsed before the command name, so
	// they are defined on the console itself rather than on each command.
	globalFlags, err := console.StructFlags(&configs.GlobalConfigs)
	if err != nil {
		log.Fatal(err)
	}
	c.Flags(globalFlags)

	c.Register(blog.NewServeCommand())
	c.Register(manager.NewServeCommand())
	c.Register(worker.NewServeCommand())

	code := c.Run(ctx, os.Args)

	cancel()
	os.Exit(code)
}
