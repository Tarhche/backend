package migrations

import (
	"context"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// renameRunnerToWorkload follows the runner being renamed the workload, its
// manager the control plane and its workers orchestrators, into what was
// stored under the old names.
//
// The names of collections and fields are written out rather than taken from
// the repositories: this is what they were when it was written, and it has to
// keep meaning that however they change later.
var renameRunnerToWorkload = Migration{
	Name: "2026-09-30-rename-runner-to-workload",
	Up: func(ctx context.Context, database *mongo.Database) error {
		steps := []func(context.Context, *mongo.Database) error{
			renamePermissions,
			renameLogsCollection,
			renameNodes,
			renameNodeNamesIn("tasks"),
			renameNodeNamesIn("stacks"),
		}

		for _, step := range steps {
			if err := step(ctx, database); err != nil {
				return err
			}
		}

		return nil
	},
}

var (
	permissionPrefixes = map[string]string{
		"runner.":      "workload.",
		"self.runner.": "self.workload.",
	}

	orchestratorNamePrefixes = []string{"runner-worker-", "runner-orchestrator-"}

	nodeRoles = map[string]string{
		"worker":  "orchestrator",
		"manager": "controlplane",
	}
)

// workloadPermission is what a runner permission is called now.
func workloadPermission(permission string) (string, bool) {
	for old, renamed := range permissionPrefixes {
		if rest, found := strings.CutPrefix(permission, old); found {
			return renamed + rest, true
		}
	}

	return permission, false
}

// orchestratorNodeName is what a worker, or an orchestrator of the runner, is
// called now.
func orchestratorNodeName(name string) (string, bool) {
	for _, old := range orchestratorNamePrefixes {
		if rest, found := strings.CutPrefix(name, old); found {
			return "workload-orchestrator-" + rest, true
		}
	}

	return name, false
}

// renamePermissions renames the runner's permissions on every role granting
// one. A session carries its permissions as a hint for the dashboard, so one
// issued before this sees the old names until it is refreshed; what is
// authorized is always read from the role.
func renamePermissions(ctx context.Context, database *mongo.Database) error {
	roles := database.Collection("roles")

	cursor, err := roles.Find(ctx, bson.D{{Key: "permissions", Value: bson.D{{Key: "$regex", Value: `^(self\.)?runner\.`}}}})
	if err != nil {
		return err
	}

	var granting []struct {
		ID          any      `bson:"_id"`
		Permissions []string `bson:"permissions"`
	}
	if err := cursor.All(ctx, &granting); err != nil {
		return err
	}

	for _, role := range granting {
		permissions := make([]string, len(role.Permissions))
		for i, permission := range role.Permissions {
			permissions[i], _ = workloadPermission(permission)
		}

		update := bson.D{{Key: "$set", Value: bson.D{{Key: "permissions", Value: permissions}}}}
		if _, err := roles.UpdateByID(ctx, role.ID, update); err != nil {
			return err
		}
	}

	return nil
}

// renameLogsCollection moves runner_logs to workload_logs. A control plane
// that started before this has already made workload_logs, so the old lines
// are merged into it rather than renamed onto it.
func renameLogsCollection(ctx context.Context, database *mongo.Database) error {
	const old, renamed = "runner_logs", "workload_logs"

	names, err := database.ListCollectionNames(ctx, bson.D{{Key: "name", Value: bson.D{{Key: "$in", Value: []string{old, renamed}}}}})
	if err != nil {
		return err
	}

	has := func(name string) bool {
		for _, n := range names {
			if n == name {
				return true
			}
		}

		return false
	}

	if !has(old) {
		return nil
	}

	if !has(renamed) {
		command := bson.D{
			{Key: "renameCollection", Value: database.Name() + "." + old},
			{Key: "to", Value: database.Name() + "." + renamed},
		}

		return database.Client().Database("admin").RunCommand(ctx, command).Err()
	}

	merge := mongo.Pipeline{{{Key: "$merge", Value: bson.D{
		{Key: "into", Value: renamed},
		{Key: "whenMatched", Value: "keepExisting"},
		{Key: "whenNotMatched", Value: "insert"},
	}}}}

	cursor, err := database.Collection(old).Aggregate(ctx, merge, options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		return err
	}
	if err := cursor.Close(ctx); err != nil {
		return err
	}

	return database.Collection(old).Drop(ctx)
}

// renameNodes gives the nodes their new roles, and forgets the ones recorded
// under the old names. A node is only its last heartbeat, and each
// orchestrator records itself again under its new name on its next one.
func renameNodes(ctx context.Context, database *mongo.Database) error {
	nodes := database.Collection("nodes")

	for old, renamed := range nodeRoles {
		filter := bson.D{{Key: "role", Value: old}}
		update := bson.D{{Key: "$set", Value: bson.D{{Key: "role", Value: renamed}}}}
		if _, err := nodes.UpdateMany(ctx, filter, update); err != nil {
			return err
		}
	}

	for _, old := range orchestratorNamePrefixes {
		filter := bson.D{{Key: "name", Value: bson.D{{Key: "$regex", Value: "^" + old}}}}
		if _, err := nodes.DeleteMany(ctx, filter); err != nil {
			return err
		}
	}

	return nil
}

// renameNodeNamesIn renames the node a task or a stack was held by, so what
// happened before still says where it happened.
func renameNodeNamesIn(collectionName string) func(context.Context, *mongo.Database) error {
	return func(ctx context.Context, database *mongo.Database) error {
		collection := database.Collection(collectionName)

		for _, old := range orchestratorNamePrefixes {
			filter := bson.D{{Key: "node_name", Value: bson.D{{Key: "$regex", Value: "^" + old}}}}

			var names []string
			if err := collection.Distinct(ctx, "node_name", filter).Decode(&names); err != nil {
				return err
			}

			for _, name := range names {
				renamed, _ := orchestratorNodeName(name)

				update := bson.D{{Key: "$set", Value: bson.D{{Key: "node_name", Value: renamed}}}}
				if _, err := collection.UpdateMany(ctx, bson.D{{Key: "node_name", Value: name}}, update); err != nil {
					return err
				}
			}
		}

		return nil
	}
}
