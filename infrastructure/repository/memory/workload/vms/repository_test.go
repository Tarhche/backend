package vms

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestRepository(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a vm is kept under the uuid it is given", func(t *testing.T) {
		t.Parallel()

		repository := NewRepository()

		v := vm.VM{Name: "web", Slug: "web-abcde", OwnerUUID: "owner"}
		uuid, err := repository.Save(ctx, &v)
		require.NoError(t, err)

		stored, err := repository.GetOneBySlug(ctx, "web-abcde")
		require.NoError(t, err)
		assert.Equal(t, uuid, stored.UUID)
		assert.False(t, stored.CreatedAt.IsZero())

		_, err = repository.GetOneByOwner(ctx, "somebody-else", uuid)
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("a slug is one vm's", func(t *testing.T) {
		t.Parallel()

		repository := NewRepository(vm.VM{UUID: "a", Slug: "web-abcde"})

		_, err := repository.Save(ctx, &vm.VM{Slug: "web-abcde"})
		assert.ErrorIs(t, err, domain.ErrAlreadyExists)
	})

	t.Run("a copy read before a newer request is refused", func(t *testing.T) {
		t.Parallel()

		asked := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
		repository := NewRepository(vm.VM{UUID: "a", UpdatedAt: asked})

		stale := vm.VM{UUID: "a", UpdatedAt: asked.Add(-time.Second)}
		_, err := repository.Save(ctx, &stale)
		assert.ErrorIs(t, err, ErrStale)

		// one that knows of it is written, and so is a newer request.
		current := vm.VM{UUID: "a", UpdatedAt: asked, Reason: "reported"}
		_, err = repository.Save(ctx, &current)
		require.NoError(t, err)

		newer := vm.VM{UUID: "a", UpdatedAt: asked.Add(time.Second)}
		_, err = repository.Save(ctx, &newer)
		require.NoError(t, err)
	})

	t.Run("listings are newest first and paged", func(t *testing.T) {
		t.Parallel()

		repository := NewRepository(
			vm.VM{UUID: "01", OwnerUUID: "owner", Kind: vm.KindDocker, NodeName: "node-1"},
			vm.VM{UUID: "02", OwnerUUID: "owner", Kind: vm.KindMachine, NodeName: "node-2"},
			vm.VM{UUID: "03", OwnerUUID: "other", Kind: vm.KindDocker, NodeName: "node-1"},
		)

		page, err := repository.GetAll(ctx, 1, 1)
		require.NoError(t, err)
		require.Len(t, page, 1)
		assert.Equal(t, "02", page[0].UUID)

		owned, err := repository.GetAllByOwner(ctx, "owner", 0, 10)
		require.NoError(t, err)
		assert.Len(t, owned, 2)

		docker, err := repository.GetAllByOwnerAndKind(ctx, "owner", vm.KindDocker)
		require.NoError(t, err)
		require.Len(t, docker, 1)
		assert.Equal(t, "01", docker[0].UUID)

		held, err := repository.GetAllByNode(ctx, "node-1")
		require.NoError(t, err)
		assert.Len(t, held, 2)

		count, err := repository.CountByOwner(ctx, "owner")
		require.NoError(t, err)
		assert.Equal(t, uint(2), count)
	})
}
