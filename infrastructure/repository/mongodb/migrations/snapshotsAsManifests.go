package migrations

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// snapshotsAsManifests converts the snapshots kept before a snapshot was a
// kind into the manifests the snapshot kind is kept as, in the same
// collection:
//
//	{_id, kind: "snapshot", metadata: {name, owner_uuid, owners: [{kind:
//	 "vm", uuid}], created_at, updated_at}, spec: {vm: {uuid, name}},
//	 status: {state, expected, reason, since, flavor, image, disk, engine,
//	 size, completed_at}, version: 1, control: {}}
//
// A snapshot keeps its uuid, which its archive is kept under, belongs to the
// VM it was taken of, and keeps what it took of it: the VM's name, flavor,
// image and disk. What it was doing is said in the kind's words. One being
// taken when the workload was upgraded never hears how that went, since
// nothing listens for what its node says of it any more, and failed saying
// so; one deleted while it was being taken failed the same way, and is
// expected deleted, so that the control plane takes it away, archive and all.
//
// Its old indexes go first, and the indexes the kind is read by are made
// last, as the control plane makes them for every kind it registers.
//
// A snapshot already a manifest is left as it is, so this runs again as well
// as it ran the first time. The names of collections, fields and states are
// written out rather than taken from the repositories and the kind: this is
// what they were when it was written, and it has to keep meaning that however
// they change later.
var snapshotsAsManifests = Migration{
	Name: "2026-10-06-snapshots-as-manifests",
	Up: func(ctx context.Context, database *mongo.Database) error {
		snapshots := database.Collection("snapshots")

		for _, index := range []string{"owner_uuid_1__id_-1", "vm_uuid_1"} {
			if err := snapshots.Indexes().DropOne(ctx, index); err != nil && !indexNotFound(err) {
				return err
			}
		}

		cursor, err := snapshots.Find(ctx, bson.D{{Key: "metadata", Value: bson.D{{Key: "$exists", Value: false}}}})
		if err != nil {
			return err
		}

		var old []oldSnapshot
		if err := cursor.All(ctx, &old); err != nil {
			return err
		}

		for _, s := range old {
			if _, err := snapshots.ReplaceOne(ctx, bson.D{{Key: "_id", Value: s.UUID}}, snapshotManifest(s)); err != nil {
				return err
			}
		}

		_, err = snapshots.Indexes().CreateMany(ctx, []mongo.IndexModel{
			{Keys: bson.D{{Key: "metadata.slug", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true)},
			{Keys: bson.D{{Key: "metadata.owner_uuid", Value: 1}, {Key: "_id", Value: -1}}},
			{Keys: bson.D{{Key: "metadata.node", Value: 1}}},
			{Keys: bson.D{{Key: "metadata.owners.uuid", Value: 1}, {Key: "metadata.owners.kind", Value: 1}}},
		})

		return err
	},
}

// upgradedWhileTaken is why a snapshot that was being taken when the workload
// was upgraded failed.
const upgradedWhileTaken = "the workload was upgraded while it was being taken"

// oldSnapshot is a snapshot as it was kept before a snapshot was a kind.
type oldSnapshot struct {
	UUID        string    `bson:"_id"`
	Name        string    `bson:"name"`
	OwnerUUID   string    `bson:"owner_uuid"`
	VMUUID      string    `bson:"vm_uuid"`
	VMName      string    `bson:"vm_name"`
	Kind        string    `bson:"kind"`
	Image       string    `bson:"image"`
	Disk        uint64    `bson:"disk"`
	Engine      string    `bson:"engine"`
	Size        int64     `bson:"size"`
	State       int       `bson:"state"`
	Reason      string    `bson:"reason"`
	CreatedAt   time.Time `bson:"created_at"`
	CompletedAt time.Time `bson:"completed_at"`
}

// snapshotManifest is an old snapshot as the snapshot kind keeps it.
func snapshotManifest(s oldSnapshot) bson.D {
	state, expected, reason := snapshotStates(s.State, s.Reason)

	// when it last changed: when it was done with, or else when it was asked
	// for.
	changed := s.CompletedAt
	if changed.IsZero() {
		changed = s.CreatedAt
	}

	metadata := bson.D{
		{Key: "name", Value: s.Name},
		{Key: "owner_uuid", Value: s.OwnerUUID},
		{Key: "owners", Value: bson.A{bson.D{{Key: "kind", Value: "vm"}, {Key: "uuid", Value: s.VMUUID}}}},
		{Key: "created_at", Value: s.CreatedAt},
		{Key: "updated_at", Value: changed},
	}

	vm := bson.D{{Key: "uuid", Value: s.VMUUID}}
	if len(s.VMName) > 0 {
		vm = append(vm, bson.E{Key: "name", Value: s.VMName})
	}

	status := bson.D{
		{Key: "state", Value: state},
		{Key: "expected", Value: expected},
	}

	optional := func(key string, value any, present bool) {
		if present {
			status = append(status, bson.E{Key: key, Value: value})
		}
	}

	optional("reason", reason, len(reason) > 0)
	optional("since", moment(changed), !changed.IsZero())
	optional("flavor", s.Kind, len(s.Kind) > 0)
	optional("image", s.Image, len(s.Image) > 0)
	optional("disk", int64(s.Disk), s.Disk > 0)
	optional("engine", s.Engine, len(s.Engine) > 0)
	optional("size", s.Size, s.Size > 0)
	optional("completed_at", moment(s.CompletedAt), !s.CompletedAt.IsZero() && state == "ready")

	return bson.D{
		{Key: "_id", Value: s.UUID},
		{Key: "kind", Value: "snapshot"},
		{Key: "metadata", Value: metadata},
		{Key: "spec", Value: bson.D{{Key: "vm", Value: vm}}},
		{Key: "status", Value: status},
		{Key: "version", Value: int64(1)},
		{Key: "control", Value: bson.D{}},
	}
}

// snapshotStates are what an old snapshot's state is in the snapshot kind's
// words, what it is expected to be, and why it is what it is. One that was
// being taken, or deleted while it was, will never hear how that went, and
// failed: one deleted is still to be.
func snapshotStates(state int, reason string) (string, string, string) {
	switch state {
	case 1: // creating
		return "failed", "ready", upgradedWhileTaken
	case 2: // ready
		return "ready", "ready", ""
	case 4: // deleting
		return "failed", "deleted", upgradedWhileTaken
	}

	// failed, and anything else.
	if len(reason) == 0 {
		reason = "the snapshot could not be taken"
	}

	return "failed", "ready", reason
}
