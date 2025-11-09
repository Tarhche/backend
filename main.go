package main

import (
	"context"
	"os"
	"os/signal"
	"path"

	"github.com/khanzadimahdi/testproject/infrastructure/console"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers"
	runnerProviders "github.com/khanzadimahdi/testproject/infrastructure/ioc/providers/runner"
	"github.com/khanzadimahdi/testproject/presentation/commands/blog"
	"github.com/khanzadimahdi/testproject/presentation/commands/runner/manager"
	"github.com/khanzadimahdi/testproject/presentation/commands/runner/worker"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	c := console.NewConsole(
		path.Base(os.Args[0]),
		"Application description",
		os.Stderr,
		ioc.NewContainer(),
	)

	c.Register(blog.NewServeCommand(providers.NewBlogProvider()))
	c.Register(manager.NewServeCommand(runnerProviders.NewManagerProvider()))
	c.Register(worker.NewServeCommand(runnerProviders.NewWorkerProvider()))

	code := c.Run(ctx, os.Args)

	cancel()
	os.Exit(code)
}
