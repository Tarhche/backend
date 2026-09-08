package schedule

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/runner/stack"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	stacksMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/stacks"
)

func TestScheduler_On(t *testing.T) {
	t.Parallel()

	scheduled := func(t *testing.T, producer *messagingMock.MockProduceConsumer) events.TaskScheduled {
		t.Helper()

		var event events.TaskScheduled
		require.NoError(t, json.Unmarshal(producer.Calls[0].Arguments.Get(2).([]byte), &event))

		return event
	}

	t.Run("a service of a stack goes where its stack is", func(t *testing.T) {
		t.Parallel()

		var (
			stacks   stacksMock.MockStacksRepository
			producer messagingMock.MockProduceConsumer
		)

		stacks.On("GetOne", mock.Anything, "stack-uuid").
			Return(stack.Stack{UUID: "stack-uuid", Slug: "shop-abcde", NodeName: "runner-worker-01"}, nil).Once()
		defer stacks.AssertExpectations(t)

		producer.On("Produce", mock.Anything, events.TaskScheduledName, mock.Anything).Return(nil).Once()
		defer producer.AssertExpectations(t)

		service := task.Task{UUID: "task-uuid", StackUUID: "stack-uuid", ServiceName: "api"}

		// nominated somewhere else, which a stack's service does not get to be.
		require.NoError(t, New(&stacks, &producer).On(context.Background(), &service, "runner-worker-03", 0))

		event := scheduled(t, &producer)
		assert.Equal(t, "runner-worker-01", event.NominatedNode)
		assert.Equal(t, "shop-abcde", event.StackSlug)
	})

	t.Run("a container of its own goes where it is asked to", func(t *testing.T) {
		t.Parallel()

		var (
			stacks   stacksMock.MockStacksRepository
			producer messagingMock.MockProduceConsumer
		)

		producer.On("Produce", mock.Anything, events.TaskScheduledName, mock.Anything).Return(nil).Once()
		defer producer.AssertExpectations(t)

		standalone := task.Task{UUID: "task-uuid"}

		require.NoError(t, New(&stacks, &producer).On(context.Background(), &standalone, "runner-worker-03", 2))

		event := scheduled(t, &producer)
		assert.Equal(t, "runner-worker-03", event.NominatedNode)
		assert.Equal(t, 2, event.Attempt)

		stacks.AssertNotCalled(t, "GetOne", mock.Anything, mock.Anything)
	})
}
