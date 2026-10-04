package migrations

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// removeTaskStacks takes away the stacks that were sets of tasks, together with
// the tasks that were their services and the lines those tasks wrote.
//
// A stack is now a compose project deployed into a Docker VM, which nothing
// stored before this knows anything about, so there is nothing to carry over.
// The containers those services ran in are not this migration's to stop: they
// live on the shared docker daemon, which goes with the runtime that used it,
// and a node reporting one the control plane no longer knows is not listened
// to.
//
// The names of collections and fields are written out rather than taken from
// the repositories: this is what they were when it was written, and it has to
// keep meaning that however they change later.
var removeTaskStacks = Migration{
	Name: "2026-10-04-remove-task-stacks",
	Up: func(ctx context.Context, database *mongo.Database) error {
		if err := removeStackServices(ctx, database); err != nil {
			return err
		}

		// dropping a collection that is not there is not an error, so this
		// runs again as well as it ran the first time.
		return database.Collection("stacks").Drop(ctx)
	},
}

// stackServicesBatch is how many services are taken away at a time, which
// keeps each delete's list of uuids small whatever there is to remove.
const stackServicesBatch = 500

// removeStackServices deletes every task that was a service of a stack, and
// the lines it wrote. Its lines go first, so a run that is cut short leaves a
// task with no log rather than a log with no task.
func removeStackServices(ctx context.Context, database *mongo.Database) error {
	tasks := database.Collection("tasks")
	logs := database.Collection("workload_logs")

	services := bson.D{{Key: "stack_uuid", Value: bson.D{{Key: "$exists", Value: true}, {Key: "$ne", Value: ""}}}}

	for {
		cursor, err := tasks.Find(
			ctx,
			services,
			options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}}).SetLimit(stackServicesBatch),
		)
		if err != nil {
			return err
		}

		var found []struct {
			UUID string `bson:"_id"`
		}
		if err := cursor.All(ctx, &found); err != nil {
			return err
		}

		if len(found) == 0 {
			return nil
		}

		uuids := make([]string, len(found))
		for i := range found {
			uuids[i] = found[i].UUID
		}

		inBatch := bson.D{{Key: "$in", Value: uuids}}

		if _, err := logs.DeleteMany(ctx, bson.D{{Key: "task_uuid", Value: inBatch}}); err != nil {
			return err
		}

		if _, err := tasks.DeleteMany(ctx, bson.D{{Key: "_id", Value: inBatch}}); err != nil {
			return err
		}
	}
}
