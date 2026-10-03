package migrations

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// backfillRuntime writes down the class every task and stack stored before
// there were classes ran with, which is sysbox: there was nothing else.
//
// The code reads a missing class as sysbox anyway, so neither the order of a
// deploy and this migration nor how long it takes matters; what this buys is
// that what is stored says what it means, for anything that reads the
// collections without going through the repositories.
//
// The names are written out rather than taken from the repositories or the
// domain: this is what they were when it was written, and it has to keep
// meaning that however they change later.
var backfillRuntime = Migration{
	Name: "2026-10-02-backfill-runtime",
	Up: func(ctx context.Context, database *mongo.Database) error {
		for _, collectionName := range []string{"tasks", "stacks"} {
			if err := backfillRuntimeIn(ctx, database.Collection(collectionName)); err != nil {
				return err
			}
		}

		return nil
	},
}

// backfillRuntimeIn gives every document of a collection that names no class
// the one it ran with. Only those: running it again finds nothing to do.
func backfillRuntimeIn(ctx context.Context, collection *mongo.Collection) error {
	_, err := collection.UpdateMany(ctx, withoutRuntime(), bson.D{{Key: "$set", Value: bson.D{{Key: "runtime", Value: "sysbox"}}}})

	return err
}

// withoutRuntime matches a document that names no class: one that has no such
// field, or one that has it empty. A null matches a missing field as well.
func withoutRuntime() bson.D {
	return bson.D{{Key: "runtime", Value: bson.D{{Key: "$in", Value: bson.A{nil, ""}}}}}
}
