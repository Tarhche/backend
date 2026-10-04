package migrations

import (
	"context"
	"slices"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// replaceTaskPermissions gives every role that could see or run the dashboard's
// tasks the permissions over VMs, snapshots and containers that took their
// place, and then takes the task permissions away. The stack permissions keep
// their names, which now mean the new stacks.
//
// Each role is written once, with the permissions it is granted already in
// and the task ones already out, so there is no moment at which a role has
// lost what it could do without having been given what replaced it. A run cut
// short leaves the roles it reached replaced and the rest as they were, and
// running it again replaces the rest.
//
// The permissions are written out rather than taken from the domain: this is
// what they were called when it was written, and it has to keep meaning that
// however they change later.
var replaceTaskPermissions = Migration{
	Name: "2026-10-04-replace-task-permissions",
	Up: func(ctx context.Context, database *mongo.Database) error {
		roles := database.Collection("roles")

		cursor, err := roles.Find(ctx, bson.D{{Key: "permissions", Value: bson.D{{Key: "$regex", Value: `^(self\.)?workload\.tasks\.`}}}})
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
			permissions, replaced := replacedTaskPermissions(role.Permissions)
			if !replaced {
				continue
			}

			update := bson.D{{Key: "$set", Value: bson.D{{Key: "permissions", Value: permissions}}}}
			if _, err := roles.UpdateByID(ctx, role.ID, update); err != nil {
				return err
			}
		}

		return nil
	},
}

// taskPermissionReplacements is what each task permission stands for now.
//
// A task was a container somebody ran, so whoever could do something to one
// can do the same to a VM, to a snapshot of one and to a container in one.
// Tasks were never edited, so updating a VM or renaming a snapshot is given to
// whoever could manage a task, which is the nearest thing to it: they could
// already change what was running.
var taskPermissionReplacements = map[string][]string{
	"workload.tasks.index":  {"workload.vms.index", "workload.snapshots.index", "workload.containers.index"},
	"workload.tasks.create": {"workload.vms.create", "workload.snapshots.create", "workload.containers.create"},
	"workload.tasks.show":   {"workload.vms.show", "workload.snapshots.show", "workload.containers.show"},
	"workload.tasks.delete": {"workload.vms.delete", "workload.snapshots.delete", "workload.containers.delete"},
	"workload.tasks.manage": {"workload.vms.manage", "workload.vms.update", "workload.snapshots.update", "workload.containers.manage"},
	"workload.tasks.logs":   {"workload.vms.logs", "workload.containers.logs"},
	"workload.tasks.attach": {"workload.vms.attach"},

	"self.workload.tasks.index":  {"self.workload.vms.index", "self.workload.snapshots.index", "self.workload.containers.index"},
	"self.workload.tasks.show":   {"self.workload.vms.show", "self.workload.snapshots.show", "self.workload.containers.show"},
	"self.workload.tasks.delete": {"self.workload.vms.delete", "self.workload.snapshots.delete", "self.workload.containers.delete"},
	"self.workload.tasks.manage": {"self.workload.vms.manage", "self.workload.vms.update", "self.workload.snapshots.update", "self.workload.containers.manage"},
	"self.workload.tasks.logs":   {"self.workload.vms.logs", "self.workload.containers.logs"},
	"self.workload.tasks.attach": {"self.workload.vms.attach"},
}

// isTaskPermission reports whether a permission is one of the dashboard's
// tasks', including any of them this migration has no replacement for.
func isTaskPermission(permission string) bool {
	return strings.HasPrefix(permission, "workload.tasks.") || strings.HasPrefix(permission, "self.workload.tasks.")
}

// replacedTaskPermissions is a role's permissions once its task permissions are
// replaced: what each of them stands for is granted, and then they go. It
// reports whether the role held any, and so whether there was anything to
// replace.
func replacedTaskPermissions(permissions []string) ([]string, bool) {
	if !slices.ContainsFunc(permissions, isTaskPermission) {
		return permissions, false
	}

	granted := slices.Clone(permissions)
	for _, permission := range permissions {
		for _, replacement := range taskPermissionReplacements[permission] {
			if !slices.Contains(granted, replacement) {
				granted = append(granted, replacement)
			}
		}
	}

	return slices.DeleteFunc(granted, isTaskPermission), true
}
