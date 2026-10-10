package kind

import (
	"encoding/json"
	"slices"
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
				"labels": {"team": "web"}, "owners": [{"kind": "shelf", "uuid": "shelf-uuid"}],
				"refs": [{"kind": "vm", "uuid": "vm-uuid"}], "node": "node-1", "generation": 1
			},
			"spec": {"image": "nginx:alpine", "size": 2},
			"status": {"state": "running", "expected": "running", "uptime": 42}
		}`, string(payload))
	})

	t.Run("when it was asked to go travels once it was", func(t *testing.T) {
		t.Parallel()

		sent := aBox()
		sent.Metadata.DeletionRequestedAt = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

		raw, err := Encode(sent)
		require.NoError(t, err)

		payload, err := json.Marshal(raw.Metadata)
		require.NoError(t, err)
		assert.Contains(t, string(payload), `"deletion_requested_at":"2026-10-06T12:00:00Z"`)

		arrived, err := Decode[boxSpec, boxStatus](travel(t, raw))
		require.NoError(t, err)
		assert.Equal(t, sent, arrived)
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
		raw.Metadata.Refs[0].UUID = "changed"

		typed, err := Decode[boxSpec, boxStatus](raw)
		require.NoError(t, err)

		typed.Metadata.Labels["team"] = "changed again"
		typed.Metadata.Refs[0].UUID = "changed again"

		assert.Equal(t, "web", sent.Metadata.Labels["team"])
		assert.Equal(t, "shelf-uuid", sent.Metadata.Owners[0].UUID)
		assert.Equal(t, "vm-uuid", sent.Metadata.Refs[0].UUID)
		assert.Equal(t, "changed", raw.Metadata.Labels["team"])
		assert.Equal(t, "changed", raw.Metadata.Refs[0].UUID)
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

	metadata := Metadata{
		Refs:   []Reference{{Kind: "vm", UUID: "vm-uuid"}, {Kind: "volume", UUID: "volume-uuid"}},
		Owners: []Reference{{Kind: "stack", UUID: "stack-uuid"}, {Kind: "vm", UUID: "another-vm-uuid"}},
	}

	// kept before refs: the vm it lives in is among its owners.
	kept := Metadata{Owners: []Reference{{Kind: "vm", UUID: "vm-uuid"}, {Kind: "stack", UUID: "stack-uuid"}}}

	for name, tt := range map[string]struct {
		metadata Metadata
		kind     string
		want     Reference
		found    bool
	}{
		"the vm it lives in, among its refs before its owners": {metadata: metadata, kind: "vm", want: Reference{Kind: "vm", UUID: "vm-uuid"}, found: true},
		"what it uses":                                  {metadata: metadata, kind: "volume", want: Reference{Kind: "volume", UUID: "volume-uuid"}, found: true},
		"the stack that owns it":                        {metadata: metadata, kind: "stack", want: Reference{Kind: "stack", UUID: "stack-uuid"}, found: true},
		"the vm one kept before refs lives in":          {metadata: kept, kind: "vm", want: Reference{Kind: "vm", UUID: "vm-uuid"}, found: true},
		"nothing of a kind it is not in":                {metadata: metadata, kind: "task"},
		"nothing of no kind at all":                     {metadata: metadata, kind: ""},
		"nor anything of a resource that names nothing": {kind: "vm"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			owner, found := tt.metadata.Owner(tt.kind)

			assert.Equal(t, tt.found, found)
			assert.Equal(t, tt.want, owner)
		})
	}
}

func TestMetadata_BelongsTo(t *testing.T) {
	t.Parallel()

	metadata := Metadata{
		Refs:   []Reference{{Kind: "vm", UUID: "vm-uuid"}, {Kind: "garden", UUID: "shared-uuid"}},
		Owners: []Reference{{Kind: "stack", UUID: "stack-uuid"}},
	}

	for name, tt := range map[string]struct {
		metadata Metadata
		to       Reference
		want     bool
	}{
		"the vm it lives in, among its refs":            {metadata: metadata, to: Reference{Kind: "vm", UUID: "vm-uuid"}, want: true},
		"the stack among its owners":                    {metadata: metadata, to: Reference{Kind: "stack", UUID: "stack-uuid"}, want: true},
		"anything of a uuid it names, of no kind given": {metadata: metadata, to: Reference{UUID: "shared-uuid"}, want: true},
		"the vm one kept before refs lives in":          {metadata: Metadata{Owners: []Reference{{Kind: "vm", UUID: "vm-uuid"}}}, to: Reference{Kind: "vm", UUID: "vm-uuid"}, want: true},
		"but not one of another kind":                   {metadata: metadata, to: Reference{Kind: "house", UUID: "shared-uuid"}},
		"nor one it does not name":                      {metadata: metadata, to: Reference{Kind: "vm", UUID: "another-uuid"}},
		"nor anything, naming nothing":                  {to: Reference{UUID: "vm-uuid"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.metadata.BelongsTo(tt.to))
		})
	}
}

func TestMetadata_Belongs(t *testing.T) {
	t.Parallel()

	metadata := Metadata{
		Refs:   []Reference{{Kind: "vm", UUID: "vm-uuid"}, {Kind: "volume", UUID: "volume-uuid"}},
		Owners: []Reference{{Kind: "stack", UUID: "stack-uuid"}, {Kind: "vm", UUID: "vm-uuid"}},
	}

	assert.Equal(t, []Reference{{Kind: "vm", UUID: "vm-uuid"}, {Kind: "volume", UUID: "volume-uuid"}, {Kind: "stack", UUID: "stack-uuid"}}, metadata.Belongs(), "its refs, and then its owners, each once")
	assert.Empty(t, Metadata{}.Belongs(), "and nothing, for one that belongs to nothing")
}

func TestLivingIn(t *testing.T) {
	t.Parallel()

	vm := Reference{Kind: "vm", UUID: "vm-uuid"}

	for name, tt := range map[string]struct {
		refs []Reference
		want []Reference
	}{
		"the resource it lives in, when it uses nothing else": {want: []Reference{vm}},
		"first, and what else it uses as it was":              {refs: []Reference{{Kind: "volume", UUID: "volume-uuid"}, {Kind: "network", UUID: "network-uuid"}}, want: []Reference{vm, {Kind: "volume", UUID: "volume-uuid"}, {Kind: "network", UUID: "network-uuid"}}},
		"in place of another of its kind":                     {refs: []Reference{{Kind: "vm", UUID: "another-uuid"}, {Kind: "volume", UUID: "volume-uuid"}}, want: []Reference{vm, {Kind: "volume", UUID: "volume-uuid"}}},
		"and once, however often it was named":                {refs: []Reference{vm, {Kind: "volume", UUID: "volume-uuid"}, vm, {Kind: "volume", UUID: "volume-uuid"}}, want: []Reference{vm, {Kind: "volume", UUID: "volume-uuid"}}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			refs := slices.Clone(tt.refs)

			assert.Equal(t, tt.want, LivingIn(tt.refs, vm))
			assert.Equal(t, refs, tt.refs, "and what it was handed is as it was")
		})
	}
}
