package database

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/danceable/console"
	"github.com/danceable/provider"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/migrations"
)

const migrateName = "migrate"

// MigrateCommand brings what is stored up to what this version reads. It is
// run before a version is deployed, and running it again applies nothing that
// was applied already.
type MigrateCommand struct {
	migrator *migrations.Migrator

	out io.Writer
	err io.Writer
}

var (
	_ console.Command   = &MigrateCommand{}
	_ console.Service   = &MigrateCommand{}
	_ provider.Provider = &MigrateCommand{}
)

func NewMigrateCommand() *MigrateCommand {
	return &MigrateCommand{out: os.Stdout, err: os.Stderr}
}

func (c *MigrateCommand) Name() string {
	return migrateName
}

func (c *MigrateCommand) Description() string {
	return "applies the migrations the database has not had yet."
}

func (c *MigrateCommand) Usage() string {
	return migrateName
}

func (c *MigrateCommand) Configure(flagSet *console.FlagSet) {}

func (c *MigrateCommand) Providers() []provider.Provider {
	return []provider.Provider{
		providers.NewConfigsProvider(),
		providers.NewMongodbProvider(),
		c,
	}
}

func (c *MigrateCommand) Register(ctx context.Context, container provider.Container) error {
	return nil
}

func (c *MigrateCommand) Boot(ctx context.Context, container provider.Container) error {
	var database *mongo.Database
	if err := container.Resolve(&database); err != nil {
		return err
	}

	c.migrator = migrations.NewMigrator(database, migrations.All()...)

	return nil
}

func (c *MigrateCommand) Terminate(ctx context.Context) error {
	return nil
}

func (c *MigrateCommand) Run(ctx context.Context) console.ExitStatus {
	applied, err := c.migrator.Migrate(ctx)
	for _, name := range applied {
		fmt.Fprintf(c.out, "applied %s\n", name)
	}

	if err != nil {
		fmt.Fprintf(c.err, "migrating failed: %v\n", err)
		return console.ExitFailure
	}

	if len(applied) == 0 {
		fmt.Fprintln(c.out, "nothing to migrate")
	}

	return console.ExitSuccess
}
