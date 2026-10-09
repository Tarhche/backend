// Package resourcetest holds a resource.Repository to what every one of them
// promises, so that the one kept in memory, which the control plane's tests
// run on, and the one kept in MongoDB, which it serves on, cannot drift apart.
package resourcetest

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// Maker is a new repository that keeps the kinds named, and nothing else in
// it yet.
type Maker func(t *testing.T, kinds ...string) resource.Repository

// The kinds the suite keeps: two of them, to hold them apart.
const (
	fans   = "fan"
	lights = "light"
)

// Repository fails t for everything the repository make makes does otherwise
// than every repository of resources has to.
func Repository(t *testing.T, make Maker) {
	t.Helper()

	t.Run("a resource is kept as it was given, and given a uuid and a first version", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t, fans, lights)

		given := aFan("", "kitchen-abcde")

		created, err := repository.Create(ctx, given)
		require.NoError(t, err)

		assert.NotEmpty(t, created.Metadata.UUID)
		assert.Equal(t, int64(1), created.Version)

		parsed, err := uuid.FromString(created.Metadata.UUID)
		require.NoError(t, err, "a resource is given a uuid")
		assert.Equal(t, byte(7), parsed.Version(), "a uuid that orders by when it was made, newest last")

		stored, err := repository.GetOne(ctx, fans, created.Metadata.UUID)
		require.NoError(t, err)

		want := given
		want.Metadata.UUID = created.Metadata.UUID
		want.Version = 1

		Equal(t, want, stored)
		Equal(t, want, created)
	})

	t.Run("one with a uuid keeps it", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t, fans)

		created, err := repository.Create(ctx, aFan("fan-uuid", ""))
		require.NoError(t, err)

		assert.Equal(t, "fan-uuid", created.Metadata.UUID)

		_, err = repository.GetOne(ctx, fans, "fan-uuid")
		assert.NoError(t, err)
	})

	t.Run("a uuid or a slug another resource of the kind has already is taken", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t, fans, lights)

		_, err := repository.Create(ctx, aFan("fan-uuid", "kitchen-abcde"))
		require.NoError(t, err)

		_, err = repository.Create(ctx, aFan("fan-uuid", "hall-abcde"))
		assert.ErrorIs(t, err, domain.ErrAlreadyExists, "the uuid is taken")

		_, err = repository.Create(ctx, aFan("another-uuid", "kitchen-abcde"))
		assert.ErrorIs(t, err, domain.ErrAlreadyExists, "the slug is taken")

		light := aFan("fan-uuid", "kitchen-abcde")
		light.Kind = lights
		_, err = repository.Create(ctx, light)
		assert.NoError(t, err, "a light is not a fan: its uuid and its slug are its own kind's")

		for i := range 3 {
			_, err := repository.Create(ctx, aFan(fmt.Sprintf("no-slug-%d", i), ""))
			assert.NoError(t, err, "resources reached under no name do not share one")
		}
	})

	t.Run("a resource is written back over the version it was read at, and only over that", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t, fans)

		created, err := repository.Create(ctx, aFan("fan-uuid", "kitchen-abcde"))
		require.NoError(t, err)

		heartbeat := created.Clone()
		result := created.Clone()

		heartbeat.Status = json.RawMessage(`{"state":"running","speed":2}`)
		written, err := repository.Update(ctx, heartbeat)
		require.NoError(t, err)
		assert.Equal(t, int64(2), written.Version)

		result.Status = json.RawMessage(`{"state":"failed","reason":"it fell over"}`)
		_, err = repository.Update(ctx, result)
		assert.ErrorIs(t, err, resource.ErrConflict, "a copy read before the heartbeat was written cannot be written over it")

		stored, err := repository.GetOne(ctx, fans, "fan-uuid")
		require.NoError(t, err)
		assert.JSONEq(t, `{"state":"running","speed":2}`, string(stored.Status))
		assert.Equal(t, int64(2), stored.Version)

		stored.Status = json.RawMessage(`{"state":"failed","reason":"it fell over"}`)
		again, err := repository.Update(ctx, stored)
		require.NoError(t, err, "read again, it is written")
		assert.Equal(t, int64(3), again.Version)
	})

	t.Run("one that is gone cannot be written back", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t, fans)

		created, err := repository.Create(ctx, aFan("fan-uuid", ""))
		require.NoError(t, err)

		require.NoError(t, repository.Delete(ctx, fans, "fan-uuid"))

		_, err = repository.Update(ctx, created)
		assert.ErrorIs(t, err, domain.ErrNotExists)

		_, err = repository.GetOne(ctx, fans, "fan-uuid")
		assert.ErrorIs(t, err, domain.ErrNotExists)

		assert.NoError(t, repository.Delete(ctx, fans, "fan-uuid"), "what is gone is gone already")
	})

	t.Run("nor be given a slug another has", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t, fans)

		_, err := repository.Create(ctx, aFan("kitchen-uuid", "kitchen-abcde"))
		require.NoError(t, err)

		hall, err := repository.Create(ctx, aFan("hall-uuid", "hall-abcde"))
		require.NoError(t, err)

		hall.Metadata.Slug = "kitchen-abcde"
		_, err = repository.Update(ctx, hall)
		assert.ErrorIs(t, err, domain.ErrAlreadyExists)
	})

	t.Run("a resource is found by its uuid, its owner's and its slug, within its kind", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t, fans, lights)

		_, err := repository.Create(ctx, aFan("fan-uuid", "kitchen-abcde"))
		require.NoError(t, err)

		found, err := repository.GetOneByOwner(ctx, fans, "owner-uuid", "fan-uuid")
		require.NoError(t, err)
		assert.Equal(t, "fan-uuid", found.Metadata.UUID)

		_, err = repository.GetOneByOwner(ctx, fans, "somebody-else", "fan-uuid")
		assert.ErrorIs(t, err, domain.ErrNotExists, "somebody else's is not there for them")

		found, err = repository.GetOneBySlug(ctx, fans, "kitchen-abcde")
		require.NoError(t, err)
		assert.Equal(t, "fan-uuid", found.Metadata.UUID)

		_, err = repository.GetOneBySlug(ctx, fans, "hall-abcde")
		assert.ErrorIs(t, err, domain.ErrNotExists)

		_, err = repository.GetOne(ctx, lights, "fan-uuid")
		assert.ErrorIs(t, err, domain.ErrNotExists, "a fan is not a light")

		_, err = repository.GetOneBySlug(ctx, lights, "kitchen-abcde")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("a listing is a page of what its filter lets through, newest first, and how many it lets through", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t, fans, lights)

		for _, each := range []struct {
			uuid, owner, node, house, room string
		}{
			{uuid: "fan-1", owner: "alice", node: "node-1", house: "house-1", room: "kitchen"},
			{uuid: "fan-2", owner: "bob", node: "node-1", house: "house-2", room: "hall"},
			{uuid: "fan-3", owner: "alice", node: "node-2", house: "house-1", room: "kitchen"},
			{uuid: "fan-4", owner: "alice", node: "", house: "", room: ""},
			{uuid: "fan-5", owner: "bob", node: "node-2", house: "house-1", room: "hall"},
		} {
			r := aFan(each.uuid, "")
			r.Metadata.OwnerUUID = each.owner
			r.Metadata.Node = each.node
			r.Metadata.Owners = nil
			r.Metadata.Labels = nil

			if len(each.house) > 0 {
				r.Metadata.Owners = []kind.Reference{{Kind: "house", UUID: each.house}}
			}

			if len(each.room) > 0 {
				r.Metadata.Labels = map[string]string{"room": each.room, "com.example.made-by": "a test"}
			}

			_, err := repository.Create(ctx, r)
			require.NoError(t, err)
		}

		light := aFan("light-1", "")
		light.Kind = lights
		light.Metadata.OwnerUUID = "alice"
		_, err := repository.Create(ctx, light)
		require.NoError(t, err)

		for name, tt := range map[string]struct {
			filter resource.Filter
			offset uint
			limit  uint
			want   []string
			total  uint
		}{
			"everybody's":                     {limit: 10, want: []string{"fan-5", "fan-4", "fan-3", "fan-2", "fan-1"}, total: 5},
			"a page of them":                  {offset: 1, limit: 2, want: []string{"fan-4", "fan-3"}, total: 5},
			"a page past the last":            {offset: 7, limit: 2, want: []string{}, total: 5},
			"all of them from somewhere":      {offset: 2, want: []string{"fan-3", "fan-2", "fan-1"}, total: 5},
			"one person's own":                {filter: resource.Filter{OwnerUUID: "alice"}, limit: 10, want: []string{"fan-4", "fan-3", "fan-1"}, total: 3},
			"what one node holds":             {filter: resource.Filter{Node: "node-1"}, limit: 10, want: []string{"fan-2", "fan-1"}, total: 2},
			"what belongs to one resource":    {filter: resource.Filter{Parent: kind.Reference{Kind: "house", UUID: "house-1"}}, limit: 10, want: []string{"fan-5", "fan-3", "fan-1"}, total: 3},
			"of any kind with its uuid":       {filter: resource.Filter{Parent: kind.Reference{UUID: "house-1"}}, limit: 10, want: []string{"fan-5", "fan-3", "fan-1"}, total: 3},
			"but not of another kind":         {filter: resource.Filter{Parent: kind.Reference{Kind: "garden", UUID: "house-1"}}, limit: 10, want: []string{}, total: 0},
			"narrowed every way at once":      {filter: resource.Filter{OwnerUUID: "alice", Node: "node-2", Parent: kind.Reference{Kind: "house", UUID: "house-1"}}, limit: 10, want: []string{"fan-3"}, total: 1},
			"and narrowed to nothing at all":  {filter: resource.Filter{OwnerUUID: "carol"}, limit: 10, want: []string{}, total: 0},
			"a page of one person's, and how": {filter: resource.Filter{OwnerUUID: "alice"}, offset: 2, limit: 1, want: []string{"fan-1"}, total: 3},
			"labelled so":                     {filter: resource.Filter{Labels: map[string]string{"room": "kitchen"}}, limit: 10, want: []string{"fan-3", "fan-1"}, total: 2},
			"labelled under a key with dots":  {filter: resource.Filter{Labels: map[string]string{"com.example.made-by": "a test"}}, limit: 10, want: []string{"fan-5", "fan-3", "fan-2", "fan-1"}, total: 4},
			"labelled every way it says":      {filter: resource.Filter{Labels: map[string]string{"room": "hall", "com.example.made-by": "a test"}}, limit: 10, want: []string{"fan-5", "fan-2"}, total: 2},
			"and labelled, and someone's":     {filter: resource.Filter{OwnerUUID: "alice", Labels: map[string]string{"room": "kitchen"}}, offset: 1, limit: 1, want: []string{"fan-1"}, total: 2},
			"labelled as none is":             {filter: resource.Filter{Labels: map[string]string{"room": "attic"}}, limit: 10, want: []string{}, total: 0},
		} {
			t.Run(name, func(t *testing.T) {
				page, total, err := repository.GetAll(ctx, fans, tt.filter, tt.offset, tt.limit)
				require.NoError(t, err)

				assert.Equal(t, tt.want, uuids(page))
				assert.Equal(t, tt.total, total)
			})
		}

		page, total, err := repository.GetAll(ctx, lights, resource.Filter{}, 0, 10)
		require.NoError(t, err)
		assert.Equal(t, []string{"light-1"}, uuids(page), "a light is listed among lights alone")
		assert.Equal(t, uint(1), total)
	})

	t.Run("what is read is a copy: changing it changes nothing kept", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t, fans)

		created, err := repository.Create(ctx, aFan("fan-uuid", ""))
		require.NoError(t, err)

		created.Metadata.Labels["room"] = "hall"
		created.Pending.IDs[0] = "changed"

		stored, err := repository.GetOne(ctx, fans, "fan-uuid")
		require.NoError(t, err)

		stored.Metadata.Labels["room"] = "attic"

		again, err := repository.GetOne(ctx, fans, "fan-uuid")
		require.NoError(t, err)

		assert.Equal(t, "kitchen", again.Metadata.Labels["room"])
		assert.Equal(t, []string{"command-1"}, again.Pending.IDs)
	})

	t.Run("what the control plane keeps beside the manifest can be cleared", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t, fans)

		created, err := repository.Create(ctx, aFan("fan-uuid", "kitchen-abcde"))
		require.NoError(t, err)

		created.Pending = nil
		created.Answer = nil
		created.Attempts = 0
		created.TriedAt = time.Time{}
		created.Reset = false
		created.Metadata.Slug = ""
		created.Metadata.Node = ""
		created.Metadata.Labels = nil
		created.Metadata.Owners = nil
		created.Metadata.ExpiresAt = time.Time{}
		created.Metadata.Lifetime = 0

		written, err := repository.Update(ctx, created)
		require.NoError(t, err)

		stored, err := repository.GetOne(ctx, fans, "fan-uuid")
		require.NoError(t, err)

		Equal(t, written, stored)
		assert.Nil(t, stored.Pending)
		assert.Nil(t, stored.Answer)
		assert.Empty(t, stored.Metadata.Slug)
		assert.True(t, stored.TriedAt.IsZero())
		assert.False(t, stored.Reset)
		assert.True(t, stored.Metadata.ExpiresAt.IsZero())

		_, err = repository.Create(ctx, aFan("another-uuid", "kitchen-abcde"))
		assert.NoError(t, err, "a slug given up is free")
	})
}

// moment is a time as a repository keeps one: to the millisecond, in UTC.
func moment(hour int) time.Time {
	return time.Date(2026, 10, 6, hour, 30, 15, 123_000_000, time.UTC)
}

// aFan is a fan with everything a resource can have.
func aFan(uuid string, slug string) resource.Record {
	return resource.Record{
		Raw: kind.Raw{
			Kind: fans,
			Metadata: kind.Metadata{
				UUID:      uuid,
				Name:      "kitchen",
				Slug:      slug,
				OwnerUUID: "owner-uuid",
				Labels:    map[string]string{"room": "kitchen", "com.example.made-by": "a test"},
				Owners:    []kind.Reference{{Kind: "house", UUID: "house-uuid"}},
				Node:      "node-1",
				Lifetime:  time.Hour,
				ExpiresAt: moment(13),
				CreatedAt: moment(12),
				UpdatedAt: moment(12),
			},
			Spec:   json.RawMessage(`{"blades":3,"colour":"white","nested":{"list":[1,2.5,"three",null,true]},"big":4611686018427387904}`),
			Status: json.RawMessage(`{"state":"starting","expected":"running","since":"2026-10-06T12:30:15.123456789Z","speed":0}`),
		},
		Pending: &resource.Pending{
			Action:  "start",
			Payload: json.RawMessage(`{"speed":2}`),
			IDs:     []string{"command-1"},
			SentAt:  moment(12),
		},
		Attempts: 1,
		TriedAt:  moment(12),
		Reset:    true,
		Answer: &kind.ResourceActedOn{
			ID:     "command-0",
			Kind:   fans,
			UUID:   uuid,
			Action: "create",
			Node:   "node-1",
			OK:     true,
			Output: "made",
			At:     moment(12),
		},
	}
}

func uuids(records []resource.Record) []string {
	listed := make([]string, len(records))
	for i := range records {
		listed[i] = records[i].Metadata.UUID
	}

	return listed
}

// Equal fails t unless got is want, as a repository keeps it: its spec and
// status the same JSON, however it is spaced or ordered.
func Equal(t *testing.T, want resource.Record, got resource.Record) {
	t.Helper()

	assert.JSONEq(t, jsonOr(want.Spec), jsonOr(got.Spec), "spec")
	assert.JSONEq(t, jsonOr(want.Status), jsonOr(got.Status), "status")

	if want.Pending != nil && got.Pending != nil {
		assert.JSONEq(t, jsonOr(want.Pending.Payload), jsonOr(got.Pending.Payload), "the pending command's payload")

		want.Pending = &resource.Pending{Action: want.Pending.Action, IDs: want.Pending.IDs, SentAt: want.Pending.SentAt}
		got.Pending = &resource.Pending{Action: got.Pending.Action, IDs: got.Pending.IDs, SentAt: got.Pending.SentAt}
	}

	want.Spec, got.Spec = nil, nil
	want.Status, got.Status = nil, nil

	assert.Equal(t, want, got)
}

func jsonOr(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "null"
	}

	return string(raw)
}
