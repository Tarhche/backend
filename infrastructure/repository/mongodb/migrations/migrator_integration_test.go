//go:build integration

package migrations

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// databases tells apart the databases one run makes.
var databases atomic.Int64

// freshDatabase is a database of the test's own on the server MONGO_TEST_URI
// names, dropped once the test is done, or a skipped test when none is
// named.
func freshDatabase(t *testing.T) *mongo.Database {
	t.Helper()

	uri := os.Getenv("MONGO_TEST_URI")
	if len(uri) == 0 {
		t.Skip("MONGO_TEST_URI names no database to test against")
	}

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	require.NoError(t, err)

	database := client.Database(fmt.Sprintf("migrations_test_%s_%d", time.Now().Format("150405000000"), databases.Add(1)))
	t.Cleanup(func() {
		_ = database.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	return database
}

// migrateAfter applies, in order, every migration after the one named, as
// `app migrate` goes on to: what a migration made is read where the ones
// after it leave it.
func migrateAfter(t *testing.T, database *mongo.Database, name string) {
	t.Helper()

	all := All()

	applied := slices.IndexFunc(all, func(m Migration) bool { return m.Name == name })
	require.GreaterOrEqual(t, applied, 0, "%s is a migration", name)

	for _, later := range all[applied+1:] {
		require.NoError(t, later.Up(context.Background(), database), later.Name)
	}
}

// TestMigrator_Pending reads which migrations a database has not had, on the
// server MONGO_TEST_URI names, as a service that waits for them does.
func TestMigrator_Pending(t *testing.T) {
	ctx := context.Background()
	database := freshDatabase(t)

	failure := errors.New("it fell over")

	first := Migration{Name: "2026-01-01-first", Up: func(context.Context, *mongo.Database) error { return nil }}
	failing := Migration{Name: "2026-01-02-failing", Up: func(context.Context, *mongo.Database) error { return failure }}
	last := Migration{Name: "2026-01-03-last", Up: func(context.Context, *mongo.Database) error { return nil }}

	migrator := NewMigrator(database, first, failing, last)

	pending, err := migrator.Pending(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{first.Name, failing.Name, last.Name}, pending, "a database nobody migrated has had none of them")

	_, err = migrator.Migrate(ctx)
	require.ErrorIs(t, err, failure)

	pending, err = migrator.Pending(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{failing.Name, last.Name}, pending, "one that failed is still to be applied, and so is everything after it")

	failing.Up = func(context.Context, *mongo.Database) error { return nil }

	migrator = NewMigrator(database, first, failing, last)

	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)

	pending, err = migrator.Pending(ctx)
	require.NoError(t, err)
	assert.Empty(t, pending, "once every one is applied, none is pending")

	pending, err = NewMigrator(database, first, failing, last, Migration{Name: "2026-01-04-new"}).Pending(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-01-04-new"}, pending, "and a version that knows of another waits for it")
}
