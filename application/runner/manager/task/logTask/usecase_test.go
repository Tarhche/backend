package logTask

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/logs"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/tasks"
)

// stillThere is a task repository that recognises every task asked of it.
func stillThere() *tasks.MockTasksRepository {
	r := &tasks.MockTasksRepository{}
	r.On("GetOne", mock.Anything, mock.Anything).Return(task.Task{}, nil)

	return r
}

// gone is a task repository for which nothing exists any more.
func gone() *tasks.MockTasksRepository {
	r := &tasks.MockTasksRepository{}
	r.On("GetOne", mock.Anything, mock.Anything).Return(task.Task{}, domain.ErrNotExists)

	return r
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func batch(taskUUID string, lines ...events.LogLine) []byte {
	payload, err := json.Marshal(events.TaskLogged{
		UUID:          taskUUID,
		ContainerUUID: "container-id",
		NodeName:      "runner-worker-01",
		Lines:         lines,
	})
	if err != nil {
		panic(err)
	}

	return payload
}

func TestTaskLogged_Handle(t *testing.T) {
	t.Parallel()

	first := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	second := first.Add(time.Second)

	t.Run("stores what a container wrote", func(t *testing.T) {
		t.Parallel()

		repository := logs.NewInMemoryRepository()

		require.NoError(t, NewTaskLogged(stillThere(), repository, 0, discardLogger()).Handle(context.Background(), batch(
			"task-uuid",
			events.LogLine{Stream: uint8(container.StreamStdout), Content: "listening on :80", At: first},
			events.LogLine{Stream: uint8(container.StreamStderr), Content: "a warning", At: second},
		)))

		stored, err := repository.Get(context.Background(), "task-uuid", time.Time{}, 0)
		require.NoError(t, err)
		require.Len(t, stored, 2)

		assert.Equal(t, "listening on :80", stored[0].Content)
		assert.Equal(t, container.StreamStdout, stored[0].Stream)
		assert.Equal(t, "container-id", stored[0].ContainerID)
		assert.Equal(t, container.StreamStderr, stored[1].Stream)
	})

	t.Run("a line shipped twice is stored once", func(t *testing.T) {
		t.Parallel()

		repository := logs.NewInMemoryRepository()
		handler := NewTaskLogged(stillThere(), repository, 0, discardLogger())

		line := events.LogLine{Stream: uint8(container.StreamStdout), Content: "listening on :80", At: first}

		// a worker that has to pick a stream up again resumes from a timestamp
		// it has already shipped, so the lines around that point arrive twice.
		require.NoError(t, handler.Handle(context.Background(), batch("task-uuid", line)))
		require.NoError(t, handler.Handle(context.Background(), batch("task-uuid", line)))

		assert.Equal(t, 1, repository.Count("task-uuid"))
	})

	t.Run("stops storing once a container has written its fill", func(t *testing.T) {
		t.Parallel()

		repository := logs.NewInMemoryRepository()

		// a cap of ten bytes is reached by the first line, so the second is
		// dropped rather than stored.
		handler := NewTaskLogged(stillThere(), repository, 10, discardLogger())

		require.NoError(t, handler.Handle(context.Background(), batch("task-uuid",
			events.LogLine{Stream: uint8(container.StreamStdout), Content: "0123456789abc", At: first},
		)))
		require.NoError(t, handler.Handle(context.Background(), batch("task-uuid",
			events.LogLine{Stream: uint8(container.StreamStdout), Content: "more", At: second},
		)))

		assert.Equal(t, 1, repository.Count("task-uuid"))
	})

	t.Run("one container's fill does not silence another", func(t *testing.T) {
		t.Parallel()

		repository := logs.NewInMemoryRepository()
		handler := NewTaskLogged(stillThere(), repository, 10, discardLogger())

		require.NoError(t, handler.Handle(context.Background(), batch("chatty",
			events.LogLine{Stream: uint8(container.StreamStdout), Content: "0123456789abc", At: first},
		)))
		require.NoError(t, handler.Handle(context.Background(), batch("quiet",
			events.LogLine{Stream: uint8(container.StreamStdout), Content: "hello", At: first},
		)))

		assert.Equal(t, 1, repository.Count("quiet"))
	})

	t.Run("an empty batch is nothing to store", func(t *testing.T) {
		t.Parallel()

		repository := logs.NewInMemoryRepository()

		require.NoError(t, NewTaskLogged(stillThere(), repository, 0, discardLogger()).Handle(context.Background(), batch("task-uuid")))

		assert.Equal(t, 0, repository.Count("task-uuid"))
	})

	t.Run("a batch naming no task is dropped", func(t *testing.T) {
		t.Parallel()

		repository := logs.NewInMemoryRepository()

		require.NoError(t, NewTaskLogged(stillThere(), repository, 0, discardLogger()).Handle(context.Background(), batch(
			"",
			events.LogLine{Stream: uint8(container.StreamStdout), Content: "orphaned", At: first},
		)))

		assert.Equal(t, 0, repository.Count(""))
	})

	t.Run("a batch that arrives after its task went is not kept", func(t *testing.T) {
		t.Parallel()

		repository := logs.NewInMemoryRepository()

		// a worker has lines in hand when a container is deleted, and rows
		// nothing owns would never be cleared by anything.
		require.NoError(t, NewTaskLogged(gone(), repository, 0, discardLogger()).Handle(context.Background(), batch(
			"task-uuid",
			events.LogLine{Stream: uint8(container.StreamStdout), Content: "one last line", At: first},
		)))

		assert.Equal(t, 0, repository.Count("task-uuid"))
	})

	t.Run("a malformed batch is dropped rather than redelivered", func(t *testing.T) {
		t.Parallel()

		repository := logs.NewInMemoryRepository()

		assert.NoError(t, NewTaskLogged(stillThere(), repository, 0, discardLogger()).Handle(context.Background(), []byte("{")))
	})
}
