package watchTasks

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/runner/watch"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/gateway"
)

const (
	requestID = "server-side-request-id"
	ownerUUID = "owner-uuid"
)

func request(t *testing.T) []byte {
	t.Helper()

	payload, err := json.Marshal(Request{ID: requestID})
	require.NoError(t, err)

	return payload
}

func TestUseCase_Handle(t *testing.T) {
	t.Parallel()

	t.Run("a request opens a watch on every task", func(t *testing.T) {
		t.Parallel()

		watchers := watch.NewWatchers()

		useCase := NewUseCase(watchers, gateway.NewStreams())
		require.NoError(t, useCase.Handle(context.Background(), request(t)))

		assert.Equal(t, []string{requestID}, watchers.Tasks("somebody"))
		assert.Equal(t, []string{requestID}, watchers.Tasks(ownerUUID), "whosever the task is")
	})

	t.Run("a client that walks away is no longer watching", func(t *testing.T) {
		t.Parallel()

		watchers := watch.NewWatchers()
		streams := gateway.NewStreams()

		useCase := NewUseCase(watchers, streams)
		require.NoError(t, useCase.Handle(context.Background(), request(t)))

		cancellation, err := json.Marshal(&gateway.StreamCancelled{RequestID: requestID})
		require.NoError(t, err)
		require.NoError(t, streams.Handle(context.Background(), cancellation))

		assert.Empty(t, watchers.Tasks("somebody"), "nothing is told to a watch nobody is behind")
	})

	t.Run("a malformed request is dropped rather than redelivered", func(t *testing.T) {
		t.Parallel()

		watchers := watch.NewWatchers()

		useCase := NewUseCase(watchers, gateway.NewStreams())

		assert.NoError(t, useCase.Handle(context.Background(), []byte("{")))
		assert.Empty(t, watchers.Tasks("somebody"))
	})
}
