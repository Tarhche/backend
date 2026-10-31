package slugs

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
)

func TestGenerate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a slug is the name and a suffix", func(t *testing.T) {
		t.Parallel()

		generated, err := Generate(ctx, "My Web Server")
		require.NoError(t, err)

		assert.True(t, strings.HasPrefix(generated, "my-web-server-"), generated)
	})

	t.Run("one that is taken anywhere is not given", func(t *testing.T) {
		t.Parallel()

		var asked []string

		// the first two are held, by one thing and then another.
		heldBy := func(n int) Taken {
			return func(_ context.Context, slug string) (bool, error) {
				asked = append(asked, slug)

				return len(asked) <= n, nil
			}
		}

		generated, err := Generate(ctx, "web", heldBy(1), heldBy(2))
		require.NoError(t, err)

		assert.Contains(t, asked, generated)
		assert.GreaterOrEqual(t, len(asked), 3)
	})

	t.Run("there is an end to trying", func(t *testing.T) {
		t.Parallel()

		_, err := Generate(ctx, "web", func(context.Context, string) (bool, error) { return true, nil })

		assert.ErrorIs(t, err, ErrExhausted)
	})

	t.Run("a lookup that fails is not taken for a free slug", func(t *testing.T) {
		t.Parallel()

		failure := errors.New("the database is away")

		_, err := Generate(ctx, "web", func(context.Context, string) (bool, error) { return false, failure })

		assert.ErrorIs(t, err, failure)
	})
}

func TestBy(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	held, err := By(func(context.Context, string) (int, error) { return 1, nil })(ctx, "web-abcde")
	require.NoError(t, err)
	assert.True(t, held)

	held, err = By(func(context.Context, string) (int, error) { return 0, domain.ErrNotExists })(ctx, "web-abcde")
	require.NoError(t, err)
	assert.False(t, held)

	_, err = By(func(context.Context, string) (int, error) { return 0, errors.New("away") })(ctx, "web-abcde")
	assert.Error(t, err)
}

func TestKeep(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	holding := func(held ...string) Taken {
		return func(_ context.Context, slug string) (bool, error) {
			for _, h := range held {
				if h == slug {
					return true, nil
				}
			}

			return false, nil
		}
	}

	t.Run("the slug asked for is kept when nothing holds it", func(t *testing.T) {
		t.Parallel()

		kept, err := Keep(ctx, "web-abcde", "web", holding("other-fghij"))
		require.NoError(t, err)
		assert.Equal(t, "web-abcde", kept)
	})

	t.Run("and another is given when something does", func(t *testing.T) {
		t.Parallel()

		kept, err := Keep(ctx, "web-abcde", "web", holding("web-abcde"))
		require.NoError(t, err)
		assert.NotEqual(t, "web-abcde", kept)
		assert.True(t, strings.HasPrefix(kept, "web-"), kept)
	})

	t.Run("and when none is asked for", func(t *testing.T) {
		t.Parallel()

		kept, err := Keep(ctx, "", "web")
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(kept, "web-"), kept)
	})

	t.Run("a lookup that fails is not taken for a free slug", func(t *testing.T) {
		t.Parallel()

		failure := errors.New("the database is away")

		_, err := Keep(ctx, "web-abcde", "web", func(context.Context, string) (bool, error) { return false, failure })
		assert.ErrorIs(t, err, failure)
	})
}
