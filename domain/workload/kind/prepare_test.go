package kind

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
)

// preparingBoxControlPlane is a box's control-plane strategy that readies its
// starts: a box on no node is placed on node-2 first, one of no size is
// started at one, one with stickers on it has "started" stuck on too, where
// they are, and one whose image is banned is refused.
type preparingBoxControlPlane struct {
	boxControlPlane

	// prepared are the actions it was asked to ready, with their payloads.
	prepared []string
	payloads []any
}

var _ Preparer[boxSpec, boxStatus] = &preparingBoxControlPlane{}

func (b *preparingBoxControlPlane) Prepare(_ context.Context, r Resource[boxSpec, boxStatus], action string, payload any) (Resource[boxSpec, boxStatus], domain.ValidationErrors, error) {
	b.prepared = append(b.prepared, action)
	b.payloads = append(b.payloads, payload)

	if b.failure != nil {
		return Resource[boxSpec, boxStatus]{}, nil, b.failure
	}

	if r.Spec.Image == "banned" {
		return Resource[boxSpec, boxStatus]{}, domain.ValidationErrors{"image": "banned"}, nil
	}

	if action == "start" && len(r.Metadata.Node) == 0 {
		r.Metadata.Node = "node-2"
	}

	if action == "start" && r.Spec.Size == 0 {
		r.Spec.Size = 1
	}

	if action == "start" && r.Spec.Stickers != nil {
		r.Spec.Stickers["started"] = "yes"
	}

	return r, nil, nil
}

// extendedBoxControlPlane is a box's control-plane strategy whose listings
// show boxes kept elsewhere beside its own.
type extendedBoxControlPlane struct {
	boxControlPlane

	extras Extras
}

var _ Extender = &extendedBoxControlPlane{}

func (b *extendedBoxControlPlane) Extras() Extras {
	return b.extras
}

// shelf is a box kept elsewhere, which can be read and nothing else.
type shelf struct{}

var _ Extras = shelf{}

func (shelf) All(context.Context) ([]Raw, error) {
	return []Raw{{Kind: "box", Metadata: Metadata{UUID: "shelved"}}}, nil
}

func (shelf) One(_ context.Context, uuid string) (Raw, error) {
	if uuid != "shelved" {
		return Raw{}, domain.ErrNotExists
	}

	return Raw{Kind: "box", Metadata: Metadata{UUID: uuid}}, nil
}

func (shelf) Act(context.Context, Raw, string, []byte) (Raw, bool, domain.ValidationErrors, error) {
	return Raw{}, false, domain.ValidationErrors{"box": "shelved"}, nil
}

func (shelf) Query(context.Context, Raw, string, []byte) ([]byte, domain.ValidationErrors, error) {
	return []byte(`"on the shelf"`), nil, nil
}

func TestControlPlaneBinding_Prepare(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a strategy that readies nothing leaves the resource as it was", func(t *testing.T) {
		t.Parallel()

		r := rawBox(t)

		prepared, invalid, err := BindControlPlane[boxSpec, boxStatus](box(), &boxControlPlane{}).Prepare(ctx, r, "start", nil)
		require.NoError(t, err)
		assert.Empty(t, invalid)
		assert.Equal(t, r, prepared)
	})

	t.Run("one that does is handed the resource and the payload as its own, and is what the command carries", func(t *testing.T) {
		t.Parallel()

		strategy := &preparingBoxControlPlane{}

		prepared, invalid, err := BindControlPlane[boxSpec, boxStatus](box(), strategy).Prepare(ctx, rawBox(t, func(r *Resource[boxSpec, boxStatus]) {
			r.Metadata.Node = ""
		}), "start", nil)
		require.NoError(t, err)
		assert.Empty(t, invalid)

		typed, err := Decode[boxSpec, boxStatus](prepared)
		require.NoError(t, err)

		assert.Equal(t, "node-2", typed.Metadata.Node, "placed before it is started")
		assert.Equal(t, "box", typed.Kind)
		assert.Equal(t, []string{"start"}, strategy.prepared)
		assert.Equal(t, []any{nil}, strategy.payloads, "an action asked with nothing is handed nothing")
	})

	for name, tt := range map[string]struct {
		box        func(r *Resource[boxSpec, boxStatus])
		generation int64
	}{
		"one whose spec it readies is a generation later": {
			box:        func(r *Resource[boxSpec, boxStatus]) { r.Spec.Size = 0 },
			generation: 2,
		},
		"however it changed it, where the resource it was handed has it too": {
			box:        func(r *Resource[boxSpec, boxStatus]) { r.Spec.Stickers = map[string]string{"fragile": "yes"} },
			generation: 2,
		},
		"one kept before generations was at its first": {
			box:        func(r *Resource[boxSpec, boxStatus]) { r.Spec.Size, r.Metadata.Generation = 0, 0 },
			generation: 2,
		},
		"and one whose spec it leaves as it was is the generation it was": {
			box:        func(r *Resource[boxSpec, boxStatus]) { r.Metadata.Node, r.Metadata.Generation = "", 3 },
			generation: 3,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			prepared, invalid, err := BindControlPlane[boxSpec, boxStatus](box(), &preparingBoxControlPlane{}).Prepare(ctx, rawBox(t, tt.box), "start", nil)
			require.NoError(t, err)
			require.Empty(t, invalid)

			typed, err := Decode[boxSpec, boxStatus](prepared)
			require.NoError(t, err)

			assert.Equal(t, tt.generation, typed.Metadata.Generation)
		})
	}

	t.Run("what it refuses is said field by field, and nothing is ready", func(t *testing.T) {
		t.Parallel()

		prepared, invalid, err := BindControlPlane[boxSpec, boxStatus](box(), &preparingBoxControlPlane{}).Prepare(ctx, rawBox(t, func(r *Resource[boxSpec, boxStatus]) {
			r.Spec.Image = "banned"
		}), "stop", nil)
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"image": "banned"}, invalid)
		assert.Equal(t, Raw{}, prepared)
	})

	t.Run("and what kept it from looking is an error", func(t *testing.T) {
		t.Parallel()

		failure := errors.New("the snapshots cannot be read")

		_, _, err := BindControlPlane[boxSpec, boxStatus](box(), &preparingBoxControlPlane{boxControlPlane: boxControlPlane{failure: failure}}).Prepare(ctx, rawBox(t), "start", nil)

		assert.ErrorIs(t, err, failure)
	})

	for name, tt := range map[string]struct {
		action  string
		payload string
		r       Raw
		err     error
	}{
		"only a node's command is readied": {
			action: "resize",
			err:    ErrUnknownAction,
		},
		"not a query": {
			action: "logs",
			err:    ErrUnknownAction,
		},
		"nor an action the kind does not have": {
			action: "explode",
			err:    ErrUnknownAction,
		},
		"a payload that cannot be read is an error": {
			action:  "start",
			payload: `{"size": 3}`,
			err:     ErrInvalidPayload,
		},
		"and so is another kind's resource": {
			action: "start",
			r:      Raw{Kind: "vm"},
			err:    ErrUnknownKind,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			r := tt.r
			if len(r.Kind) == 0 {
				r = rawBox(t)
			}

			_, _, err := BindControlPlane[boxSpec, boxStatus](box(), &preparingBoxControlPlane{}).Prepare(ctx, r, tt.action, []byte(tt.payload))

			assert.ErrorIs(t, err, tt.err)
		})
	}
}

func TestControlPlaneBinding_Extras(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a strategy that is no extender shows nothing beside its records", func(t *testing.T) {
		t.Parallel()

		extras, extends := BindControlPlane[boxSpec, boxStatus](box(), &boxControlPlane{}).Extras()

		assert.False(t, extends)
		assert.Nil(t, extras)
	})

	t.Run("nor does an extender with none", func(t *testing.T) {
		t.Parallel()

		_, extends := BindControlPlane[boxSpec, boxStatus](box(), &extendedBoxControlPlane{}).Extras()

		assert.False(t, extends)
	})

	t.Run("one with extras has them shown", func(t *testing.T) {
		t.Parallel()

		extras, extends := BindControlPlane[boxSpec, boxStatus](box(), &extendedBoxControlPlane{extras: shelf{}}).Extras()
		require.True(t, extends)

		all, err := extras.All(ctx)
		require.NoError(t, err)
		require.Len(t, all, 1)
		assert.Equal(t, "shelved", all[0].Metadata.UUID)

		answer, refused, err := extras.Query(ctx, all[0], "logs", nil)
		require.NoError(t, err)
		assert.Empty(t, refused)
		assert.JSONEq(t, `"on the shelf"`, string(json.RawMessage(answer)))
	})
}
