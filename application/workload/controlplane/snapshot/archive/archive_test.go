package archive

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
)

// bucket stands in for the snapshots bucket: what Delete is asked, and what it
// answers.
type bucket struct {
	deleted []string
	answer  error
}

func (b *bucket) Store(context.Context, string, io.Reader, int64) error { return nil }

func (b *bucket) Read(context.Context, string) (io.ReadSeekCloser, error) { return nil, nil }

func (b *bucket) Delete(_ context.Context, objectName string) error {
	b.deleted = append(b.deleted, objectName)

	return b.answer
}

func TestRemover_Remove(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := slog.New(slog.DiscardHandler)

	t.Run("the archive under the snapshot's key is taken away", func(t *testing.T) {
		t.Parallel()

		store := &bucket{}
		require.NoError(t, NewRemover(store, logger).Remove(ctx, "snapshot-uuid"))
		assert.Equal(t, []string{"snapshots/snapshot-uuid.msb"}, store.deleted)
	})

	t.Run("one that is not there is the outcome asked for", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, NewRemover(&bucket{answer: domain.ErrNotExists}, logger).Remove(ctx, "snapshot-uuid"))
	})

	t.Run("a bucket that cannot be reached is an error", func(t *testing.T) {
		t.Parallel()

		assert.Error(t, NewRemover(&bucket{answer: errors.New("connection refused")}, logger).Remove(ctx, "snapshot-uuid"))
	})

	t.Run("with no bucket there is nothing to take away", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, NewRemover(nil, logger).Remove(ctx, "snapshot-uuid"))
	})
}
