package schedule

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/task/events"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
)

func TestScheduler_On(t *testing.T) {
	t.Parallel()

	t.Run("a task goes where it is asked to, as the attempt it is", func(t *testing.T) {
		t.Parallel()

		var producer messagingMock.MockProduceConsumer

		producer.On("Produce", mock.Anything, events.TaskScheduledName, mock.Anything).Return(nil).Once()
		defer producer.AssertExpectations(t)

		standalone := task.Task{UUID: "task-uuid"}

		require.NoError(t, New(&producer).On(context.Background(), &standalone, "workload-orchestrator-03", 2))

		var event events.TaskScheduled
		require.NoError(t, json.Unmarshal(producer.Calls[0].Arguments.Get(2).([]byte), &event))

		assert.Equal(t, "task-uuid", event.UUID)
		assert.Equal(t, "workload-orchestrator-03", event.NominatedNode)
		assert.Equal(t, 2, event.Attempt)
	})
}
