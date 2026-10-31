package kind

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// outlivingBoxControlPlane is a box's control-plane strategy whose boxes are
// made again with their shelf: one kept as gone with its shelf takes the
// shelf's name as its image, and keeps what shelf it was handed.
type outlivingBoxControlPlane struct {
	boxControlPlane

	// parents are the parents it was handed, in order.
	parents []Raw
}

var _ Outliver[boxSpec, boxStatus] = &outlivingBoxControlPlane{}

func (b *outlivingBoxControlPlane) Outlive(r Resource[boxSpec, boxStatus], parent Raw) (Resource[boxSpec, boxStatus], error) {
	b.parents = append(b.parents, parent)

	if b.failure != nil {
		return Resource[boxSpec, boxStatus]{}, b.failure
	}

	if len(parent.Metadata.Name) > 0 {
		r.Spec.Image = parent.Metadata.Name
	}

	return r, nil
}

func TestControlPlaneBinding_Outlive(t *testing.T) {
	t.Parallel()

	shelf := Raw{Kind: "shelf", Metadata: Metadata{UUID: "shelf-uuid", Name: "redis:7"}}

	t.Run("what its strategy keeps of its parent is kept, a generation later when its spec changed", func(t *testing.T) {
		t.Parallel()

		strategy := &outlivingBoxControlPlane{}

		kept, err := BindControlPlane[boxSpec, boxStatus](box(), strategy).Outlive(rawBox(t), shelf)
		require.NoError(t, err)

		typed, err := Decode[boxSpec, boxStatus](kept)
		require.NoError(t, err)

		assert.Equal(t, "redis:7", typed.Spec.Image)
		assert.Equal(t, int64(2), typed.Metadata.Generation, "its spec changed")
		assert.Equal(t, "box", typed.Kind)
		assert.Equal(t, []Raw{shelf}, strategy.parents)
	})

	t.Run("and its generation is as it was when nothing changed", func(t *testing.T) {
		t.Parallel()

		kept, err := BindControlPlane[boxSpec, boxStatus](box(), &outlivingBoxControlPlane{}).Outlive(rawBox(t), Raw{Kind: "shelf"})
		require.NoError(t, err)

		typed, err := Decode[boxSpec, boxStatus](kept)
		require.NoError(t, err)
		assert.Equal(t, int64(1), typed.Metadata.Generation)
	})

	t.Run("a strategy that is no outliver leaves it as it is", func(t *testing.T) {
		t.Parallel()

		r := rawBox(t)

		kept, err := BindControlPlane[boxSpec, boxStatus](box(), &boxControlPlane{}).Outlive(r, shelf)
		require.NoError(t, err)
		assert.Equal(t, r, kept)
	})

	t.Run("what its strategy fails with is what it fails with", func(t *testing.T) {
		t.Parallel()

		broken := errors.New("broken")

		_, err := BindControlPlane[boxSpec, boxStatus](box(), &outlivingBoxControlPlane{boxControlPlane: boxControlPlane{failure: broken}}).Outlive(rawBox(t), shelf)
		assert.ErrorIs(t, err, broken)
	})
}
