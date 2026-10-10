package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path"

	"github.com/danceable/console"
	"github.com/danceable/container"
	"github.com/danceable/provider"
	"github.com/danceable/provider/adapters/danceable"

	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/presentation/commands/blog"
	"github.com/khanzadimahdi/testproject/presentation/commands/certificate"
	"github.com/khanzadimahdi/testproject/presentation/commands/database"
	"github.com/khanzadimahdi/testproject/presentation/commands/workload/controlplane"
	"github.com/khanzadimahdi/testproject/presentation/commands/workload/ingress"
	"github.com/khanzadimahdi/testproject/presentation/commands/workload/orchestrator"
	"github.com/khanzadimahdi/testproject/presentation/commands/workload/vmhost"
)

// the blog's specification documents the blog. The workload services carry
// annotations of their own and are served elsewhere, so scanning them here
// only puts routes in this spec that this service does not answer — and makes
// the control plane and the orchestrator collide over the paths they share.
// Their use cases are left out for the same reason: the control plane has a
// presenter package of its own, and an annotation naming presenter.Container
// would be read as the control plane's rather than the dashboard's.
//
//go:generate go tool swag init --generalInfo ./presentation/commands/blog/serve.go --dir ./ --exclude ./presentation/http/workload,./application/workload --output ./resources/docs/blog/openapi
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	serviceProvider := provider.New(danceable.New(container.New()))

	c := console.NewWithServiceProvider(
		path.Base(os.Args[0]),
		"Application description",
		os.Stdout,
		os.Stderr,
		serviceProvider,
	)

	// the settings every command reads are parsed before the command name, so
	// they are defined on the console itself rather than on each command.
	globalFlags, err := console.StructFlags(&configs.GlobalConfigs)
	if err != nil {
		log.Fatal(err)
	}
	c.Flags(globalFlags)

	c.Register(blog.NewServeCommand(serviceProvider))
	c.Register(controlplane.NewServeCommand())
	c.Register(orchestrator.NewServeCommand())
	c.Register(ingress.NewServeCommand())

	// a node's engine, served to its orchestrator from the microsandbox
	// container, and the question its healthcheck asks it
	c.Register(vmhost.NewServeCommand())
	c.Register(vmhost.NewCheckCommand())

	// brings what is stored up to what this version reads
	c.Register(database.NewMigrateCommand())

	// the certificates the workload's tunnel authenticates with
	c.RegisterGroup(certificate.Group())

	code := c.Run(ctx, os.Args)

	cancel()
	os.Exit(code)
}
