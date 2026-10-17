package executions

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
)

// TestToDocument holds what a save sets of an execution to every field of it,
// by its stored name, those left empty too, and never the _id.
func TestToDocument(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 10, 12, 0, 0, 123_456_789, time.UTC)
	root := graph.Ref{Kind: "stack", UUID: "s-uuid"}

	for name, tt := range map[string]struct {
		given graph.Execution
		want  bson.D
	}{
		"one done, with its record and its times": {
			given: graph.Execution{
				Root:       root,
				Generation: 2,
				Node:       "container/shop_web",
				Kind:       "container",
				Action:     graph.ActionCreate,
				Level:      2,
				State:      graph.StateDone,
				Reason:     "it was there already",
				Ref:        graph.Ref{Kind: "container", UUID: "c-uuid"},
				StartedAt:  at,
				FinishedAt: at,
				UpdatedAt:  at,
			},
			want: bson.D{
				{Key: "root", Value: bson.D{{Key: "kind", Value: "stack"}, {Key: "uuid", Value: "s-uuid"}}},
				{Key: "generation", Value: int64(2)},
				{Key: "node", Value: "container/shop_web"},
				{Key: "kind", Value: "container"},
				{Key: "action", Value: "create"},
				{Key: "level", Value: int32(2)},
				{Key: "state", Value: "done"},
				{Key: "reason", Value: "it was there already"},
				{Key: "ref", Value: bson.D{{Key: "kind", Value: "container"}, {Key: "uuid", Value: "c-uuid"}}},
				{Key: "started_at", Value: bson.NewDateTimeFromTime(at)},
				{Key: "finished_at", Value: bson.NewDateTimeFromTime(at)},
				{Key: "updated_at", Value: bson.NewDateTimeFromTime(at)},
			},
		},
		"and one pending, with what it has not got as null and nothing": {
			given: graph.Execution{
				Root:       root,
				Generation: 2,
				Node:       "container/shop_web",
				Kind:       "container",
				Action:     graph.ActionCreate,
				Level:      2,
				State:      graph.StatePending,
				UpdatedAt:  at,
			},
			want: bson.D{
				{Key: "root", Value: bson.D{{Key: "kind", Value: "stack"}, {Key: "uuid", Value: "s-uuid"}}},
				{Key: "generation", Value: int64(2)},
				{Key: "node", Value: "container/shop_web"},
				{Key: "kind", Value: "container"},
				{Key: "action", Value: "create"},
				{Key: "level", Value: int32(2)},
				{Key: "state", Value: "pending"},
				{Key: "reason", Value: ""},
				{Key: "ref", Value: nil},
				{Key: "started_at", Value: nil},
				{Key: "finished_at", Value: nil},
				{Key: "updated_at", Value: bson.NewDateTimeFromTime(at)},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			raw, err := bson.Marshal(toDocument(tt.given))
			require.NoError(t, err)

			var set bson.D
			require.NoError(t, bson.Unmarshal(raw, &set))

			assert.Equal(t, tt.want, set)

			var stored document
			require.NoError(t, bson.Unmarshal(raw, &stored))

			read := toExecution(stored)
			want := tt.given
			want.StartedAt = optionalAt(want.StartedAt)
			want.FinishedAt = optionalAt(want.FinishedAt)
			want.UpdatedAt = optionalAt(want.UpdatedAt)

			assert.Equal(t, want, read, "and it is read back as it was saved, to the millisecond")
		})
	}
}

func TestIDOf(t *testing.T) {
	t.Parallel()

	id, err := idOf(graph.Execution{Root: graph.Ref{Kind: "stack", UUID: "s-uuid"}, Node: "network/shop_default"})
	require.NoError(t, err)
	assert.Equal(t, "s-uuid|network/shop_default", id)

	_, err = idOf(graph.Execution{Root: graph.Ref{Kind: "stack"}, Node: "network/shop_default"})
	assert.Error(t, err, "an execution of no root is under nothing")

	_, err = idOf(graph.Execution{Root: graph.Ref{Kind: "stack", UUID: "s-uuid"}})
	assert.Error(t, err, "nor is one of no node")
}

// optionalAt is a time as it is read back: to the millisecond, and nothing
// for none.
func optionalAt(t time.Time) time.Time {
	return moment(optional(t))
}
