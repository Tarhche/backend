package tasks

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

func TestRepository(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a task is kept under the uuid it is given, newest first", func(t *testing.T) {
		t.Parallel()

		repository := NewRepository()

		first := task.Task{Name: "first", Slug: "first-abcde", OwnerUUID: task.GuestOwnerUUID}
		_, err := repository.Save(ctx, &first)
		require.NoError(t, err)

		second := task.Task{Name: "second", Slug: "second-abcde", OwnerUUID: task.GuestOwnerUUID}
		_, err = repository.Save(ctx, &second)
		require.NoError(t, err)

		listed, err := repository.GetAllByOwner(ctx, task.GuestOwnerUUID, 0, 10)
		require.NoError(t, err)
		require.Len(t, listed, 2)
		assert.Equal(t, []string{second.UUID, first.UUID}, []string{listed[0].UUID, listed[1].UUID})
		assert.False(t, listed[0].CreatedAt.IsZero())

		page, err := repository.GetAllByOwner(ctx, task.GuestOwnerUUID, 1, 10)
		require.NoError(t, err)
		require.Len(t, page, 1)
		assert.Equal(t, first.UUID, page[0].UUID)
	})

	t.Run("a task is nobody else's", func(t *testing.T) {
		t.Parallel()

		repository := NewRepository(task.Task{UUID: "a", OwnerUUID: task.GuestOwnerUUID})

		_, err := repository.GetOneByOwner(ctx, "somebody", "a")
		assert.ErrorIs(t, err, domain.ErrNotExists)

		count, err := repository.CountByOwner(ctx, "somebody")
		require.NoError(t, err)
		assert.Zero(t, count)

		found, err := repository.GetOneByOwner(ctx, task.GuestOwnerUUID, "a")
		require.NoError(t, err)
		assert.Equal(t, "a", found.UUID)
	})

	t.Run("a slug is one task's", func(t *testing.T) {
		t.Parallel()

		repository := NewRepository(task.Task{UUID: "a", Slug: "run-abcde"})

		_, err := repository.Save(ctx, &task.Task{Slug: "run-abcde"})
		assert.ErrorIs(t, err, domain.ErrAlreadyExists)
	})
}
