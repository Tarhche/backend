package kind

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// travel sends a value the way a message does, and reads it back.
func travel[T any](t *testing.T, value T) T {
	t.Helper()

	payload, err := json.Marshal(value)
	require.NoError(t, err)

	var arrived T
	require.NoError(t, json.Unmarshal(payload, &arrived))

	return arrived
}

func TestResource(t *testing.T) {
	t.Parallel()

	t.Run("a resource travels raw and arrives as its kind's own", func(t *testing.T) {
		t.Parallel()

		sent := aBox()
		sent.Metadata.Lifetime = time.Hour
		sent.Metadata.ExpiresAt = time.Date(2026, 10, 6, 13, 0, 0, 0, time.UTC)
		sent.Metadata.CreatedAt = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
		sent.Status.Since = time.Date(2026, 10, 6, 12, 1, 0, 0, time.UTC)

		raw, err := Encode(sent)
		require.NoError(t, err)

		arrived, err := Decode[boxSpec, boxStatus](travel(t, raw))
		require.NoError(t, err)

		assert.Equal(t, sent, arrived)
	})

	t.Run("it travels in the manifest's shape, its status one flat object", func(t *testing.T) {
		t.Parallel()

		raw, err := Encode(aBox())
		require.NoError(t, err)

		payload, err := json.Marshal(raw)
		require.NoError(t, err)

		assert.JSONEq(t, `{
			"kind": "box",
			"metadata": {
				"uuid": "box-uuid", "name": "shop", "slug": "shop-abcde", "owner_uuid": "owner-uuid",
				"labels": {"team": "web"}, "owners": [{"kind": "vm", "uuid": "vm-uuid"}], "node": "node-1"
			},
			"spec": {"image": "nginx:alpine", "size": 2},
			"status": {"state": "running", "expected": "running", "uptime": 42}
		}`, string(payload))
	})

	t.Run("a spec or a status that is not there is the zero value", func(t *testing.T) {
		t.Parallel()

		typed, err := Decode[boxSpec, boxStatus](Raw{Kind: "box", Metadata: Metadata{UUID: "box-uuid"}})
		require.NoError(t, err)

		assert.Equal(t, Resource[boxSpec, boxStatus]{Kind: "box", Metadata: Metadata{UUID: "box-uuid"}}, typed)
	})

	t.Run("a spec or a status that is not the kind's cannot be read", func(t *testing.T) {
		t.Parallel()

		for name, raw := range map[string]Raw{
			"spec":   {Kind: "box", Spec: json.RawMessage(`{"image": 7}`)},
			"status": {Kind: "box", Status: json.RawMessage(`[]`)},
		} {
			_, err := Decode[boxSpec, boxStatus](raw)

			assert.ErrorContains(t, err, "the box's "+name+" cannot be read", name)
		}
	})

	t.Run("a spec or a status that cannot be written does not travel", func(t *testing.T) {
		t.Parallel()

		_, err := Encode(Resource[chan int, boxStatus]{Kind: "box"})
		assert.ErrorContains(t, err, "the box's spec cannot be written")

		_, err = Encode(Resource[boxSpec, chan int]{Kind: "box"})
		assert.ErrorContains(t, err, "the box's status cannot be written")
	})

	t.Run("what travels shares nothing with what it was made from", func(t *testing.T) {
		t.Parallel()

		sent := aBox()

		raw, err := Encode(sent)
		require.NoError(t, err)

		raw.Metadata.Labels["team"] = "changed"
		raw.Metadata.Owners[0].UUID = "changed"

		typed, err := Decode[boxSpec, boxStatus](raw)
		require.NoError(t, err)

		typed.Metadata.Labels["team"] = "changed again"

		assert.Equal(t, "web", sent.Metadata.Labels["team"])
		assert.Equal(t, "vm-uuid", sent.Metadata.Owners[0].UUID)
		assert.Equal(t, "changed", raw.Metadata.Labels["team"])
	})
}

func TestStatus_Common(t *testing.T) {
	t.Parallel()

	status := boxStatus{Status: Status{State: boxRunning}, Uptime: 42}

	var stated Stated = &status
	stated.Common().State = boxStopped

	assert.Equal(t, boxStopped, status.State, "the part every kind shares is the kind's own status's")
	assert.Equal(t, 42, status.Uptime)
}

func TestMetadata_Expired(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

	for name, tt := range map[string]struct {
		metadata Metadata
		want     bool
	}{
		"one kept until it is deleted never expires": {
			metadata: Metadata{},
		},
		"nor does one with a lifetime and no end to it": {
			metadata: Metadata{Lifetime: time.Hour},
		},
		"nor one with an end and no lifetime": {
			metadata: Metadata{ExpiresAt: now.Add(-time.Hour)},
		},
		"one whose lifetime is not over has not": {
			metadata: Metadata{Lifetime: time.Hour, ExpiresAt: now.Add(time.Minute)},
		},
		"one whose lifetime ends now has": {
			metadata: Metadata{Lifetime: time.Hour, ExpiresAt: now},
			want:     true,
		},
		"and one whose lifetime ended has": {
			metadata: Metadata{Lifetime: time.Hour, ExpiresAt: now.Add(-time.Minute)},
			want:     true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.metadata.Expired(now))
		})
	}
}

func TestMetadata_Owner(t *testing.T) {
	t.Parallel()

	metadata := Metadata{Owners: []Reference{{Kind: "vm", UUID: "vm-uuid"}, {Kind: "stack", UUID: "stack-uuid"}}}

	for name, tt := range map[string]struct {
		kind  string
		want  Reference
		found bool
	}{
		"the vm it lives in":             {kind: "vm", want: Reference{Kind: "vm", UUID: "vm-uuid"}, found: true},
		"the stack that made it":         {kind: "stack", want: Reference{Kind: "stack", UUID: "stack-uuid"}, found: true},
		"nothing of a kind it is not in": {kind: "task"},
		"nothing of no kind at all":      {kind: ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			owner, found := metadata.Owner(tt.kind)

			assert.Equal(t, tt.found, found)
			assert.Equal(t, tt.want, owner)
		})
	}
}
