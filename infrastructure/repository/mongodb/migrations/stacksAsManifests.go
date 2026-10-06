package migrations

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// stacksAsManifests converts the stacks kept before a stack was a kind into
// the manifests the stack kind is kept as, in the same collection:
//
//	{_id, kind: "stack", metadata: {name, slug, owner_uuid, owners: [{kind:
//	 "vm", uuid}], node, created_at, updated_at}, spec: {vm: {uuid}, compose},
//	 status: {state, expected, reason, since, output}, version: 1, control: {}}
//
// A stack lives on the node its VM is on. What it was doing is said in the
// kind's words, and what an old command was in the middle of, which nothing
// is waiting on any more, is said so that the kind's own reconcile asks for
// it again: one being deployed is waiting to be, one being started is
// stopped and expected running, one being stopped is running and expected
// stopped, and one being taken down failed and is expected deleted.
//
// Its old indexes go first: the unique one on a slug that is not where a
// manifest keeps it would hold every converted stack to one slug, nothing.
// The indexes the kind is read by are made last, as the control plane makes
// them for every kind it registers.
//
// A stack already a manifest is left as it is, so this runs again as well as
// it ran the first time. The names of collections, fields and states are
// written out rather than taken from the repositories and the kind: this is
// what they were when it was written, and it has to keep meaning that however
// they change later.
var stacksAsManifests = Migration{
	Name: "2026-10-06-stacks-as-manifests",
	Up: func(ctx context.Context, database *mongo.Database) error {
		stacks := database.Collection("stacks")

		for _, index := range []string{"slug_1", "owner_uuid_1__id_-1", "vm_uuid_1"} {
			if err := stacks.Indexes().DropOne(ctx, index); err != nil && !indexNotFound(err) {
				return err
			}
		}

		cursor, err := stacks.Find(ctx, bson.D{{Key: "metadata", Value: bson.D{{Key: "$exists", Value: false}}}})
		if err != nil {
			return err
		}

		var old []oldStack
		if err := cursor.All(ctx, &old); err != nil {
			return err
		}

		vms := database.Collection("vms")

		for _, s := range old {
			var held struct {
				NodeName string `bson:"node_name"`
			}

			err := vms.FindOne(ctx, bson.D{{Key: "_id", Value: s.VMUUID}}).Decode(&held)
			if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
				return err
			}

			if _, err := stacks.ReplaceOne(ctx, bson.D{{Key: "_id", Value: s.UUID}}, stackManifest(s, held.NodeName)); err != nil {
				return err
			}
		}

		_, err = stacks.Indexes().CreateMany(ctx, []mongo.IndexModel{
			{Keys: bson.D{{Key: "metadata.slug", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true)},
			{Keys: bson.D{{Key: "metadata.owner_uuid", Value: 1}, {Key: "_id", Value: -1}}},
			{Keys: bson.D{{Key: "metadata.node", Value: 1}}},
			{Keys: bson.D{{Key: "metadata.owners.uuid", Value: 1}, {Key: "metadata.owners.kind", Value: 1}}},
		})

		return err
	},
}

// oldStack is a stack as it was kept before a stack was a kind.
type oldStack struct {
	UUID          string    `bson:"_id"`
	Name          string    `bson:"name"`
	OwnerUUID     string    `bson:"owner_uuid"`
	VMUUID        string    `bson:"vm_uuid"`
	Slug          string    `bson:"slug"`
	Compose       string    `bson:"compose"`
	ExpectedState int       `bson:"expected_state"`
	State         int       `bson:"state"`
	Reason        string    `bson:"reason"`
	Output        string    `bson:"output"`
	CreatedAt     time.Time `bson:"created_at"`
	UpdatedAt     time.Time `bson:"updated_at"`
}

// stackManifest is an old stack as the stack kind keeps it, on node.
func stackManifest(s oldStack, node string) bson.D {
	state, expected := stackStates(s.State, s.ExpectedState)

	reason := s.Reason
	if reason == "waiting_for_vm" {
		reason = "its vm is not running yet"
	}

	metadata := bson.D{
		{Key: "name", Value: s.Name},
		{Key: "slug", Value: s.Slug},
		{Key: "owner_uuid", Value: s.OwnerUUID},
		{Key: "owners", Value: bson.A{bson.D{{Key: "kind", Value: "vm"}, {Key: "uuid", Value: s.VMUUID}}}},
	}

	if len(node) > 0 {
		metadata = append(metadata, bson.E{Key: "node", Value: node})
	}

	metadata = append(metadata,
		bson.E{Key: "created_at", Value: s.CreatedAt},
		bson.E{Key: "updated_at", Value: s.UpdatedAt},
	)

	status := bson.D{
		{Key: "state", Value: state},
		{Key: "expected", Value: expected},
	}

	if len(reason) > 0 {
		status = append(status, bson.E{Key: "reason", Value: reason})
	}

	if !s.UpdatedAt.IsZero() {
		status = append(status, bson.E{Key: "since", Value: s.UpdatedAt.UTC().Format(time.RFC3339Nano)})
	}

	if len(s.Output) > 0 {
		status = append(status, bson.E{Key: "output", Value: s.Output})
	}

	return bson.D{
		{Key: "_id", Value: s.UUID},
		{Key: "kind", Value: "stack"},
		{Key: "metadata", Value: metadata},
		{Key: "spec", Value: bson.D{
			{Key: "vm", Value: bson.D{{Key: "uuid", Value: s.VMUUID}}},
			{Key: "compose", Value: s.Compose},
		}},
		{Key: "status", Value: status},
		{Key: "version", Value: int64(1)},
		{Key: "control", Value: bson.D{}},
	}
}

// stackStates are what an old stack's state and expected state are in the
// stack kind's words. A command it was in the middle of is said as what it
// left behind and what it was asked to be, so that it is asked again.
func stackStates(state int, expected int) (string, string) {
	wanted := map[int]string{2: "running", 5: "stopped"}[expected]
	if len(wanted) == 0 {
		wanted = "running"
	}

	switch state {
	case 1: // deploying
		return "waiting", "running"
	case 2: // running
		return "running", wanted
	case 3: // starting
		return "stopped", "running"
	case 4: // stopping
		return "running", "stopped"
	case 5: // stopped
		return "stopped", wanted
	case 6: // restarting
		return "running", "running"
	case 7: // removing
		return "failed", "deleted"
	default: // failed, and anything else
		return "failed", wanted
	}
}

// indexNotFound reports whether dropping an index failed for there being no
// such index, which is what dropping it was for.
func indexNotFound(err error) bool {
	var refused mongo.CommandError

	return errors.As(err, &refused) && (refused.Code == 27 || refused.Name == "IndexNotFound" || refused.Code == 26)
}
