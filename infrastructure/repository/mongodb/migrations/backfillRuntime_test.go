package migrations

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestBackfillRuntime(t *testing.T) {
	t.Parallel()

	t.Run("it comes after everything released before it", func(t *testing.T) {
		t.Parallel()

		all := All()

		names := make([]string, len(all))
		for i, migration := range all {
			names[i] = migration.Name
		}

		assert.Equal(t, backfillRuntime.Name, names[len(names)-1])
		assert.Less(t, slices.Index(names, renameRunnerToWorkload.Name), slices.Index(names, backfillRuntime.Name))

		slices.Sort(names)
		assert.Len(t, slices.Compact(names), len(all), "a name is a migration")
	})

	t.Run("it touches only what names no class", func(t *testing.T) {
		t.Parallel()

		// a missing field and a null both match null; an empty one is
		// what a class written as nothing looks like.
		assert.Equal(t, bson.D{{Key: "runtime", Value: bson.D{{Key: "$in", Value: bson.A{nil, ""}}}}}, withoutRuntime())
	})
}
