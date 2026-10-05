package stackResult

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/stack/events"
	stacksMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/stacks"
)

func message(t *testing.T, value any) []byte {
	t.Helper()

	payload, err := json.Marshal(value)
	require.NoError(t, err)

	return payload
}

func TestStackCompleted_Handle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for name, tt := range map[string]struct {
		state  stack.State
		action stack.Action
		want   stack.State
		gone   bool
	}{
		"a deployed stack is running":           {state: stack.Deploying, action: stack.ActionUp, want: stack.Running},
		"a started one is running":              {state: stack.Starting, action: stack.ActionStart, want: stack.Running},
		"a restarted one is running":            {state: stack.Restarting, action: stack.ActionRestart, want: stack.Running},
		"a stopped one is stopped":              {state: stack.Stopping, action: stack.ActionStop, want: stack.Stopped},
		"one taken down goes":                   {state: stack.Removing, action: stack.ActionDown, gone: true},
		"a result it is not waiting on is late": {state: stack.Starting, action: stack.ActionStop, want: stack.Starting},
		"one heard already is not heard again":  {state: stack.Running, action: stack.ActionUp, want: stack.Running},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			repository := stacksMemory.NewRepository(stack.Stack{UUID: "s1", State: tt.state, Reason: "before"})

			require.NoError(t, NewStackCompleted(repository, slog.New(slog.DiscardHandler)).Handle(ctx, message(t, events.StackCompleted{StackUUID: "s1", Action: tt.action, Output: "Container web Started"})))

			stored, kept := repository.Stored("s1")
			if tt.gone {
				assert.False(t, kept)

				return
			}

			assert.Equal(t, tt.want.String(), stored.State.String())
		})
	}
}

func TestStackFailed_Handle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a failed command fails the stack, with what compose printed", func(t *testing.T) {
		t.Parallel()

		repository := stacksMemory.NewRepository(stack.Stack{UUID: "s1", State: stack.Deploying})
		output := strings.Repeat("pulling...\n", 3000) + "Error response from daemon: manifest unknown"

		require.NoError(t, NewStackFailed(repository, slog.New(slog.DiscardHandler)).Handle(ctx, message(t, events.StackFailed{StackUUID: "s1", Action: stack.ActionUp, Reason: "exit status 1", Output: output})))

		stored, _ := repository.Stored("s1")
		assert.Equal(t, stack.Failed, stored.State)
		assert.Equal(t, "exit status 1", stored.Reason)
		assert.Len(t, stored.Output, stack.MaxOutput, "the tail of it")
		assert.True(t, strings.HasSuffix(stored.Output, "manifest unknown"))
	})

	t.Run("taking one down can fail too", func(t *testing.T) {
		t.Parallel()

		repository := stacksMemory.NewRepository(stack.Stack{UUID: "s1", State: stack.Removing})

		require.NoError(t, NewStackFailed(repository, slog.New(slog.DiscardHandler)).Handle(ctx, message(t, events.StackFailed{StackUUID: "s1", Action: stack.ActionDown})))

		stored, kept := repository.Stored("s1")
		require.True(t, kept)
		assert.Equal(t, stack.Failed, stored.State)
		assert.Equal(t, defaultReason, stored.Reason)
	})

	t.Run("what will never be handled is not handed back", func(t *testing.T) {
		t.Parallel()

		repository := stacksMemory.NewRepository()

		assert.NoError(t, NewStackFailed(repository, slog.New(slog.DiscardHandler)).Handle(ctx, []byte("{")))
		assert.NoError(t, NewStackCompleted(repository, slog.New(slog.DiscardHandler)).Handle(ctx, message(t, events.StackCompleted{StackUUID: "missing", Action: stack.ActionUp})))
	})
}
