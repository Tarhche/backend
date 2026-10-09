package migrations

import (
	"context"
	"fmt"
	"slices"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// moveResourcesToWorkloads moves the resources of every kind, kept until now
// in a collection of each kind's own, named by its plural, into one
// collection, workloads, as they are: each keeps its uuid, its kind and its
// version, and what the control plane was in the middle of asking it.
//
// A uuid and a slug are each held to one resource in workloads, whatever
// their kinds, where each kind's collection held them to one of its own: a
// stack's slug was unique among stacks alone. So before anything is moved,
// what workloads and every collection keep is read, and a resource kept among
// another kind's, two kinds' resources under one uuid or two resources under
// one slug fail the migration, naming them, with nothing moved and nothing
// dropped.
//
// The indexes of workloads are made first, as the control plane makes them,
// so that nothing that goes in can share a slug with what is there. What is
// in workloads already, of its kind, was moved by a run cut short and is left
// as it is, and a collection is dropped once what it kept is in workloads, so
// this runs again as well as it ran the first time.
//
// The names of collections and fields are written out rather than taken from
// the repositories and the kinds: this is what they were when it was written,
// and it has to keep meaning that however they change later.
var moveResourcesToWorkloads = Migration{
	Name: "2026-10-09-move-resources-to-workloads",
	Up: func(ctx context.Context, database *mongo.Database) error {
		workloads := database.Collection("workloads")

		if _, err := workloads.Indexes().CreateMany(ctx, []mongo.IndexModel{
			{Keys: bson.D{{Key: "metadata.slug", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.D{
				{Key: "metadata.slug", Value: bson.D{{Key: "$exists", Value: true}}},
			})},
			{Keys: bson.D{{Key: "kind", Value: 1}, {Key: "_id", Value: -1}}},
			{Keys: bson.D{{Key: "kind", Value: 1}, {Key: "metadata.owner_uuid", Value: 1}, {Key: "_id", Value: -1}}},
			{Keys: bson.D{{Key: "kind", Value: 1}, {Key: "metadata.node", Value: 1}}},
			{Keys: bson.D{{Key: "metadata.owners.uuid", Value: 1}, {Key: "metadata.owners.kind", Value: 1}}},
		}); err != nil {
			return err
		}

		there, err := resourcesIn(ctx, workloads)
		if err != nil {
			return err
		}

		var moving []keptResource
		for _, kept := range kindCollections {
			resources, err := resourcesIn(ctx, database.Collection(kept.collection))
			if err != nil {
				return err
			}

			moving = append(moving, resources...)
		}

		if err := clashIn(there, moving); err != nil {
			return err
		}

		// what is there already stays as it is: it is what was moved of the
		// same resource before.
		merge := mongo.Pipeline{{{Key: "$merge", Value: bson.D{
			{Key: "into", Value: "workloads"},
			{Key: "on", Value: "_id"},
			{Key: "whenMatched", Value: "keepExisting"},
			{Key: "whenNotMatched", Value: "insert"},
		}}}}

		// in a fixed order, so a run that fails says the same thing every time.
		for _, kept := range kindCollections {
			collection := database.Collection(kept.collection)

			cursor, err := collection.Aggregate(ctx, merge)
			if err != nil {
				return err
			}

			if err := cursor.Close(ctx); err != nil {
				return err
			}

			if err := collection.Drop(ctx); err != nil {
				return err
			}
		}

		return nil
	},
}

// kindCollections are the collections the resources of each kind were kept
// in before workloads, and the kind each kept, in the order the kinds are
// registered.
var kindCollections = []struct {
	collection string
	kind       string
}{
	{collection: "vms", kind: "vm"},
	{collection: "snapshots", kind: "snapshot"},
	{collection: "stacks", kind: "stack"},
	{collection: "tasks", kind: "task"},
	{collection: "containers", kind: "container"},
	{collection: "images", kind: "image"},
	{collection: "networks", kind: "network"},
	{collection: "volumes", kind: "volume"},
}

// keptResource is a resource as the move reads it: where it is kept, its
// kind, and what workloads holds to one resource, its uuid and its slug.
type keptResource struct {
	Collection string
	Kind       string
	UUID       string
	Slug       string
}

func (r keptResource) String() string {
	return fmt.Sprintf("the %s %q in %s", r.Kind, r.UUID, r.Collection)
}

// resourcesIn is what a collection keeps, as the move reads it, by uuid.
func resourcesIn(ctx context.Context, collection *mongo.Collection) ([]keptResource, error) {
	cursor, err := collection.Find(ctx, bson.D{}, options.Find().
		SetProjection(bson.D{{Key: "kind", Value: 1}, {Key: "metadata.slug", Value: 1}}).
		SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}

	var read []struct {
		UUID     string `bson:"_id"`
		Kind     string `bson:"kind"`
		Metadata struct {
			Slug string `bson:"slug"`
		} `bson:"metadata"`
	}
	if err := cursor.All(ctx, &read); err != nil {
		return nil, err
	}

	resources := make([]keptResource, len(read))
	for i, r := range read {
		resources[i] = keptResource{Collection: collection.Name(), Kind: r.Kind, UUID: r.UUID, Slug: r.Metadata.Slug}
	}

	return resources, nil
}

// clashIn is what keeps the resources moving from being kept in workloads,
// beside one another and beside what is there, or nil when nothing does: one
// kept among another kind's, two kinds' resources under one uuid, or two
// resources under one slug. One that is there already, of its kind and under
// its uuid, is the same resource, moved before: what is there of it is what
// stays, and what is moving of it clashes with nothing.
func clashIn(there []keptResource, moving []keptResource) error {
	byUUID := make(map[string]keptResource, len(there)+len(moving))
	bySlug := make(map[string]keptResource, len(there)+len(moving))

	for _, r := range slices.Concat(there, moving) {
		if of, moved := kindKeptIn(r.Collection); moved && r.Kind != of {
			return fmt.Errorf("%q in %s is a %q rather than a %s: nothing was moved", r.UUID, r.Collection, r.Kind, of)
		}

		if other, taken := byUUID[r.UUID]; taken {
			if other.Kind != r.Kind {
				return fmt.Errorf("%s and %s share a uuid, which workloads holds to one resource: nothing was moved", other, r)
			}

			continue
		}

		byUUID[r.UUID] = r

		if len(r.Slug) == 0 {
			continue
		}

		if other, taken := bySlug[r.Slug]; taken {
			return fmt.Errorf("%s and %s share the slug %q, which workloads holds to one resource: nothing was moved", other, r, r.Slug)
		}

		bySlug[r.Slug] = r
	}

	return nil
}

// kindKeptIn is the kind a collection the resources are moved out of kept,
// and whether it is one.
func kindKeptIn(collection string) (string, bool) {
	for _, kept := range kindCollections {
		if kept.collection == collection {
			return kept.kind, true
		}
	}

	return "", false
}
