package gateway

import (
	"context"
	"io"
	"log/slog"
	"testing"

	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestReplyBus(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("releases its subscription when it shuts down", func(t *testing.T) {
		t.Parallel()

		var subscribed context.Context

		var publishSubscriberMock messagingMock.MockPublishSubscriber
		publishSubscriberMock.On("Subscribe", mock.Anything, "replies", mock.Anything).
			Run(func(args mock.Arguments) { subscribed = args.Get(0).(context.Context) }).
			Return(nil).
			Once()
		defer publishSubscriberMock.AssertExpectations(t)

		bus := newReplyBus(&publishSubscriberMock, "replies", logger)

		require.NoError(t, bus.start(context.Background()))
		require.NotNil(t, subscribed)
		require.NoError(t, subscribed.Err(), "the subscription should start out live")

		require.NoError(t, bus.shutdown())

		assert.Error(t, subscribed.Err(), "shutdown left the reply subscription running")
	})

	t.Run("shuts down when it never subscribed", func(t *testing.T) {
		t.Parallel()

		bus := newReplyBus(&messagingMock.MockPublishSubscriber{}, "replies", logger)

		assert.NoError(t, bus.shutdown())
		assert.NoError(t, bus.shutdown())
	})
}
