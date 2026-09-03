package fanout

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func services(uuids ...string) []task.Task {
	items := make([]task.Task, len(uuids))
	for i, uuid := range uuids {
		items[i] = task.Task{UUID: uuid}
	}

	return items
}

func TestOver(t *testing.T) {
	t.Parallel()

	t.Run("reaches every service of the stack", func(t *testing.T) {
		t.Parallel()

		var reached []string

		err := Over(context.Background(), services("a", "b", "c"), discardLogger(),
			func(_ context.Context, uuid string) error {
				reached = append(reached, uuid)

				return nil
			})

		require.NoError(t, err)
		assert.Equal(t, []string{"a", "b", "c"}, reached)
	})

	t.Run("a service that is already gone does not stop the rest", func(t *testing.T) {
		t.Parallel()

		var reached []string

		// what was asked for is that the stack end up in a state, and a service
		// that is not there is not a reason to leave its neighbours alone.
		err := Over(context.Background(), services("a", "b", "c"), discardLogger(),
			func(_ context.Context, uuid string) error {
				reached = append(reached, uuid)

				if uuid == "b" {
					return domain.ErrNotExists
				}

				return nil
			})

		require.NoError(t, err)
		assert.Equal(t, []string{"a", "b", "c"}, reached)
	})

	t.Run("anything else stops the fan-out, because the work was not done", func(t *testing.T) {
		t.Parallel()

		var reached []string

		unreachable := errors.New("the broker is unreachable")

		err := Over(context.Background(), services("a", "b", "c"), discardLogger(),
			func(_ context.Context, uuid string) error {
				reached = append(reached, uuid)

				if uuid == "b" {
					return unreachable
				}

				return nil
			})

		assert.ErrorIs(t, err, unreachable)
		assert.Equal(t, []string{"a", "b"}, reached, "the rest is not attempted once the runner is failing")
	})

	t.Run("a stack with no services is nothing to do", func(t *testing.T) {
		t.Parallel()

		called := false

		err := Over(context.Background(), nil, discardLogger(), func(context.Context, string) error {
			called = true

			return nil
		})

		require.NoError(t, err)
		assert.False(t, called)
	})

	t.Run("an error wrapping a missing service is still a missing service", func(t *testing.T) {
		t.Parallel()

		err := Over(context.Background(), services("a"), discardLogger(),
			func(context.Context, string) error {
				return errors.Join(errors.New("looking it up"), domain.ErrNotExists)
			})

		assert.NoError(t, err)
	})
}
