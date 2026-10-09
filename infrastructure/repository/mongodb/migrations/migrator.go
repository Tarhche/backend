// Package migrations changes what is already stored into the shape this
// version of the application reads.
//
// Each migration runs once: the ones applied are recorded in a collection of
// their own, so running the migrator again applies only what is new. A
// migration is idempotent as well, so one that was cut short can simply be
// run again.
package migrations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const collectionName = "migrations"

// Migration is one change to what is stored.
type Migration struct {
	Name string
	Up   func(ctx context.Context, database *mongo.Database) error
}

type record struct {
	Name      string    `bson:"_id"`
	AppliedAt time.Time `bson:"applied_at"`
}

// Migrator applies migrations to one database.
type Migrator struct {
	database   *mongo.Database
	migrations []Migration
}

func NewMigrator(database *mongo.Database, migrations ...Migration) *Migrator {
	return &Migrator{database: database, migrations: migrations}
}

// Migrate applies, in order, every migration not yet recorded, and returns
// the names of the ones it applied. It stops at the first that fails, having
// recorded the ones before it.
func (m *Migrator) Migrate(ctx context.Context) ([]string, error) {
	collection := m.database.Collection(collectionName)

	var applied []string
	for _, migration := range m.migrations {
		err := collection.FindOne(ctx, bson.D{{Key: "_id", Value: migration.Name}}).Err()
		if err == nil {
			continue
		}

		if !errors.Is(err, mongo.ErrNoDocuments) {
			return applied, err
		}

		if err := migration.Up(ctx, m.database); err != nil {
			return applied, fmt.Errorf("%s: %w", migration.Name, err)
		}

		if _, err := collection.InsertOne(ctx, record{Name: migration.Name, AppliedAt: time.Now()}); err != nil {
			return applied, err
		}

		applied = append(applied, migration.Name)
	}

	return applied, nil
}
