package watchUserStacks

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/runner/watch"
	"github.com/khanzadimahdi/testproject/domain/user"
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

// asking is the context a request arrives in: whoever the middleware
// established before it got here.
func asking() context.Context {
	return auth.ToContext(context.Background(), &user.User{UUID: ownerUUID})
}

func TestUseCase_Handle(t *testing.T) {
	t.Parallel()

	t.Run("a request opens a watch on the asker's own stacks", func(t *testing.T) {
		t.Parallel()

		watchers := watch.NewWatchers()

		useCase := NewUseCase(watchers, gateway.NewStreams())
		require.NoError(t, useCase.Handle(asking(), request(t)))

		assert.Equal(t, []string{requestID}, watchers.Stacks(ownerUUID))
		assert.Empty(t, watchers.Stacks("somebody-else"), "somebody else's stack is not their news")
	})

	t.Run("a client that walks away is no longer watching", func(t *testing.T) {
		t.Parallel()

		watchers := watch.NewWatchers()
		streams := gateway.NewStreams()

		useCase := NewUseCase(watchers, streams)
		require.NoError(t, useCase.Handle(asking(), request(t)))

		cancellation, err := json.Marshal(&gateway.StreamCancelled{RequestID: requestID})
		require.NoError(t, err)
		require.NoError(t, streams.Handle(context.Background(), cancellation))

		assert.Empty(t, watchers.Stacks(ownerUUID), "nothing is told to a watch nobody is behind")
	})

	t.Run("a malformed request is dropped rather than redelivered", func(t *testing.T) {
		t.Parallel()

		watchers := watch.NewWatchers()

		useCase := NewUseCase(watchers, gateway.NewStreams())

		assert.NoError(t, useCase.Handle(asking(), []byte("{")))
		assert.Empty(t, watchers.Stacks(ownerUUID))
	})
}
