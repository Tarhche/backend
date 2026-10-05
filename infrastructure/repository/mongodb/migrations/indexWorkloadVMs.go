package migrations

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// indexWorkloadVMs gives the collections that keep VMs, their snapshots and
// their stacks the indexes they are read by.
//
// A VM's slug is the left-most label of the hostname its ports are served
// under and a stack's is its compose project, so each is unique, and the
// database is what holds two of them to that when they are asked for at the
// same moment. The rest are how each collection is read: a person's own,
// newest first; the VMs one node holds, which every heartbeat reads; a
// person's VMs of one kind, which is how a Docker VM is chosen; and what was
// taken of, or deployed into, one VM.
//
// Creating an index that is already there, with the same keys and options, is
// not an error, so this runs again as well as it ran the first time. The names
// of collections and fields are written out rather than taken from the
// repositories: this is what they were when it was written, and it has to keep
// meaning that however they change later.
var indexWorkloadVMs = Migration{
	Name: "2026-10-04-index-workload-vms",
	Up: func(ctx context.Context, database *mongo.Database) error {
		indexes := map[string][]mongo.IndexModel{
			"vms": {
				{Keys: bson.D{{Key: "slug", Value: 1}}, Options: options.Index().SetUnique(true)},
				{Keys: bson.D{{Key: "owner_uuid", Value: 1}, {Key: "_id", Value: -1}}},
				{Keys: bson.D{{Key: "owner_uuid", Value: 1}, {Key: "kind", Value: 1}}},
				{Keys: bson.D{{Key: "node_name", Value: 1}}},
			},
			"snapshots": {
				{Keys: bson.D{{Key: "owner_uuid", Value: 1}, {Key: "_id", Value: -1}}},
				{Keys: bson.D{{Key: "vm_uuid", Value: 1}}},
			},
			"stacks": {
				{Keys: bson.D{{Key: "slug", Value: 1}}, Options: options.Index().SetUnique(true)},
				{Keys: bson.D{{Key: "owner_uuid", Value: 1}, {Key: "_id", Value: -1}}},
				{Keys: bson.D{{Key: "vm_uuid", Value: 1}}},
			},
		}

		// in a fixed order, so a run that fails says the same thing every time.
		for _, collection := range []string{"vms", "snapshots", "stacks"} {
			if _, err := database.Collection(collection).Indexes().CreateMany(ctx, indexes[collection]); err != nil {
				return err
			}
		}

		return nil
	},
}
