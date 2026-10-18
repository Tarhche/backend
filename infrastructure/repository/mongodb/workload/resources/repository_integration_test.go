//go:build integration

package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/domain/workload/resource/resourcetest"
)

// databases tells apart the databases one run makes.
var databases atomic.Int64

// database is a fresh database on the server MONGO_TEST_URI names, or a
// skipped test when none is named.
func database(t *testing.T) *mongo.Database {
	t.Helper()

	uri := os.Getenv("MONGO_TEST_URI")
	if len(uri) == 0 {
		t.Skip("MONGO_TEST_URI names no database to test against")
	}

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	require.NoError(t, err)

	db := client.Database(fmt.Sprintf("resources_test_%s_%d", time.Now().Format("150405000000"), databases.Add(1)))
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	return db
}

func TestRepository(t *testing.T) {
	t.Parallel()

	resourcetest.Repository(t, func(t *testing.T, kinds ...string) resource.Repository {
		repository := NewRepository(database(t))

		for _, name := range kinds {
			require.NoError(t, repository.EnsureKind(context.Background(), kind.Descriptor{Name: name, Plural: name + "s"}))
		}

		return repository
	})
}

func TestRepository_EnsureKind(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := database(t)
	repository := NewRepository(db)

	t.Run("a kind it was not told of is not kept", func(t *testing.T) {
		_, err := repository.GetOne(ctx, "fan", "fan-uuid")

		assert.ErrorIs(t, err, kind.ErrUnknownKind)
	})

	t.Run("every kind's resources are kept in one collection, workloads, indexed the ways they are read", func(t *testing.T) {
		fan := kind.Descriptor{Name: "fan", Plural: "fans"}

		require.NoError(t, repository.EnsureKind(ctx, fan))
		require.NoError(t, repository.EnsureKind(ctx, fan), "it is told of a kind again at every start")
		require.NoError(t, repository.EnsureKind(ctx, kind.Descriptor{Name: "light", Plural: "lights"}))

		_, err := repository.Create(ctx, resource.Record{Raw: kind.Raw{Kind: "fan", Metadata: kind.Metadata{UUID: "fan-uuid"}}})
		require.NoError(t, err)

		_, err = repository.Create(ctx, resource.Record{Raw: kind.Raw{Kind: "light", Metadata: kind.Metadata{UUID: "light-uuid"}}})
		require.NoError(t, err)

		for uuid, kindName := range map[string]string{"fan-uuid": "fan", "light-uuid": "light"} {
			stored, err := db.Collection("workloads").CountDocuments(ctx, bson.D{{Key: "_id", Value: uuid}, {Key: "kind", Value: kindName}})
			require.NoError(t, err)
			assert.Equal(t, int64(1), stored, "a %s is kept as one", kindName)
		}

		collections, err := db.ListCollectionNames(ctx, bson.D{})
		require.NoError(t, err)
		assert.Equal(t, []string{"workloads"}, collections, "no kind has a collection of its own")

		cursor, err := db.Collection("workloads").Indexes().List(ctx)
		require.NoError(t, err)

		var indexes []struct {
			Key     bson.D `bson:"key"`
			Unique  bool   `bson:"unique"`
			Sparse  bool   `bson:"sparse"`
			Partial bson.D `bson:"partialFilterExpression"`
		}
		require.NoError(t, cursor.All(ctx, &indexes))

		keys := make(map[string]bool)
		for _, index := range indexes {
			var names string
			for _, key := range index.Key {
				names += key.Key + " "
			}

			keys[names] = true

			if names == "metadata.slug " {
				assert.True(t, index.Unique, "a slug is unique, whatever the kinds")
				assert.False(t, index.Sparse)
				assert.Equal(t, bson.D{{Key: "metadata.slug", Value: bson.D{{Key: "$exists", Value: true}}}}, index.Partial, "among the resources that have one")
			}
		}

		for _, want := range []string{"_id ", "metadata.slug ", "kind _id ", "kind metadata.owner_uuid _id ", "kind metadata.node ", "metadata.owners.uuid metadata.owners.kind ", "metadata.refs.uuid metadata.refs.kind "} {
			assert.True(t, keys[want], "an index on %q", want)
		}

		assert.Len(t, indexes, 7, "and on nothing else")
	})

	t.Run("and one without a name cannot be", func(t *testing.T) {
		assert.Error(t, repository.EnsureKind(ctx, kind.Descriptor{Plural: "fans"}))
	})
}

// TestRepository_unstorable holds what the database cannot keep as it is to
// being resource.ErrUnstorable, at its create and at its update, neither of
// which keeps anything of it.
func TestRepository_unstorable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository := NewRepository(database(t))
	require.NoError(t, repository.EnsureKind(ctx, kind.Descriptor{Name: "fan", Plural: "fans"}))

	for name, change := range map[string]func(r *resource.Record){
		"a label of its spec whose key has a NUL in it": func(r *resource.Record) {
			r.Spec = json.RawMessage(`{"labels":{"a\u0000b":"c"}}`)
		},
		"one of its metadata": func(r *resource.Record) {
			r.Metadata.Labels = map[string]string{"a\x00b": "c"}
		},
		"a status beyond what a document holds": func(r *resource.Record) {
			r.Status = json.RawMessage(`{"output":"` + strings.Repeat("x", 17<<20) + `"}`)
		},
		"a spec nested deeper than the database nests one": func(r *resource.Record) {
			r.Spec = json.RawMessage(strings.Repeat(`{"d":`, 200) + `1` + strings.Repeat(`}`, 200))
		},
	} {
		t.Run(name, func(t *testing.T) {
			uuid := strings.ReplaceAll(name, " ", "-")
			fan := resource.Record{Raw: kind.Raw{Kind: "fan", Metadata: kind.Metadata{UUID: uuid}, Spec: json.RawMessage(`{"blades":3}`)}}

			unkept := fan
			change(&unkept)

			_, err := repository.Create(ctx, unkept)
			assert.ErrorIs(t, err, resource.ErrUnstorable, "at its create")

			_, err = repository.GetOne(ctx, "fan", uuid)
			require.ErrorIs(t, err, domain.ErrNotExists, "nothing of it is kept")

			created, err := repository.Create(ctx, fan)
			require.NoError(t, err)

			change(&created)

			_, err = repository.Update(ctx, created)
			assert.ErrorIs(t, err, resource.ErrUnstorable, "at its update")

			stored, err := repository.GetOne(ctx, "fan", uuid)
			require.NoError(t, err)
			assert.Equal(t, int64(1), stored.Version, "what was kept is kept as it was")
			assert.JSONEq(t, `{"blades":3}`, string(stored.Spec))
		})
	}
}

// TestRepository_keptBefore holds a document kept while a kind's JSON was
// read as relaxed extended JSON to being read back as it was then, an object
// of one of extended JSON's own keys as what it was read as, and as that once
// it is written back as it is read.
func TestRepository_keptBefore(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := database(t)
	repository := NewRepository(db)
	require.NoError(t, repository.EnsureKind(ctx, kind.Descriptor{Name: "fan", Plural: "fans"}))

	for name, tt := range map[string]struct {
		written string
		read    string
	}{
		"a spec of no such key": {
			written: `{"blades":3,"colour":"white","nested":{"list":[1,2.5,"three",null,true],"empty":{},"none":[]},"big":4611686018427387904,"bigger":18446744073709551615}`,
			read:    `{"blades":3,"colour":"white","nested":{"list":[1,2.5,"three",null,true],"empty":{},"none":[]},"big":4611686018427387904,"bigger":1.8446744073709552E+19}`,
		},
		"a label read as a date":       {written: `{"labels":{"$date":"2026-10-06T12:00:00Z"}}`, read: `{"labels":{"$date":"2026-10-06T12:00:00Z"}}`},
		"a label read as an object id": {written: `{"labels":{"$oid":"5f0c8a7e9d1b2c3d4e5f6a7b"}}`, read: `{"labels":{"$oid":"5f0c8a7e9d1b2c3d4e5f6a7b"}}`},
		"and one read as a long, which is read back as the number it was kept as": {written: `{"labels":{"$numberLong":"5"}}`, read: `{"labels":5}`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			uuid := strings.ReplaceAll(name, " ", "-")

			stored, err := toDocument(resource.Record{Raw: kind.Raw{Kind: "fan", Metadata: kind.Metadata{UUID: uuid}}, Version: 1})
			require.NoError(t, err)

			stored.Spec, err = extendedValueOf(json.RawMessage(tt.written))
			require.NoError(t, err)

			_, err = db.Collection(collectionName).InsertOne(ctx, stored)
			require.NoError(t, err)

			read, err := repository.GetOne(ctx, "fan", uuid)
			require.NoError(t, err)
			assert.Equal(t, tt.read, string(read.Spec), "read back as it was then")

			_, err = repository.Update(ctx, read)
			require.NoError(t, err)

			again, err := repository.GetOne(ctx, "fan", uuid)
			require.NoError(t, err)
			assert.Equal(t, tt.read, string(again.Spec), "and so once it is written back as it was read")
		})
	}
}
